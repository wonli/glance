package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"glance/internal/fontcatalog"
)

type FontScanner interface {
	Scan(context.Context, fontcatalog.Snapshot, bool) (fontcatalog.ScanResult, error)
}
type FontOptions struct {
	Publish   func(fontcatalog.Snapshot)
	Scanner   FontScanner
	CachePath string
}
type FontEvent struct {
	Snapshot   fontcatalog.Snapshot
	Refreshing bool
	Err        error
	CacheErr   error
}
type FontService struct {
	mu          sync.Mutex
	opMu        sync.Mutex
	options     FontOptions
	snapshot    fontcatalog.Snapshot
	registered  map[string][]fontcatalog.Face
	subscribers map[chan FontEvent]struct{}
	refreshing  chan struct{}
	lastErr     error
	cacheErr    error
	closed      bool
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

func NewFontService(options FontOptions) *FontService {
	ctx, cancel := context.WithCancel(context.Background())
	return &FontService{options: options, registered: map[string][]fontcatalog.Face{}, subscribers: map[chan FontEvent]struct{}{}, ctx: ctx, cancel: cancel}
}
func (s *FontService) Snapshot() fontcatalog.Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot
}
func (s *FontService) eventLocked() FontEvent {
	return FontEvent{Snapshot: s.snapshot, Refreshing: s.refreshing != nil, Err: s.lastErr, CacheErr: s.cacheErr}
}
func (s *FontService) publishLocked() {
	event := s.eventLocked()
	if s.options.Publish != nil {
		s.options.Publish(s.snapshot)
	}
	for ch := range s.subscribers {
		select {
		case ch <- event:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- event:
			default:
			}
		}
	}
}
func (s *FontService) Subscribe() (<-chan FontEvent, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan FontEvent, 1)
	if s.closed {
		close(ch)
		return ch, func() {}
	}
	s.subscribers[ch] = struct{}{}
	ch <- s.eventLocked()
	var once sync.Once
	return ch, func() { once.Do(func() { s.mu.Lock(); delete(s.subscribers, ch); close(ch); s.mu.Unlock() }) }
}
func (s *FontService) Close() error {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	s.mu.Unlock()
	s.wg.Wait()
	return nil
}

// Initialize publishes a persisted catalog before checking filesystem changes.
func (s *FontService) Initialize(ctx context.Context) error {
	if s.options.CachePath != "" {
		if cached, e := fontcatalog.LoadCache(s.options.CachePath); e == nil {
			s.mu.Lock()
			if s.snapshot.Version == 0 {
				s.snapshot = cached
				s.publishLocked()
			}
			s.mu.Unlock()
		}
	}
	return s.Refresh(ctx, false)
}
func (s *FontService) Refresh(ctx context.Context, force bool) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errors.New("font service closed")
	}
	if running := s.refreshing; running != nil {
		s.mu.Unlock()
		select {
		case <-running:
			s.mu.Lock()
			err := s.lastErr
			s.mu.Unlock()
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-s.ctx.Done():
			return s.ctx.Err()
		}
	}
	s.refreshing = make(chan struct{})
	running := s.refreshing
	s.wg.Add(1)
	s.lastErr = nil
	s.publishLocked()
	s.mu.Unlock()
	defer s.wg.Done()
	job, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	defer cancel()
	s.opMu.Lock()
	defer s.opMu.Unlock()
	previous := s.Snapshot()
	var result fontcatalog.ScanResult
	var err error
	if s.options.Scanner == nil {
		err = errors.New("font scanner unavailable")
	} else {
		result, err = s.options.Scanner.Scan(job, previous, force)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if job.Err() != nil {
		err = job.Err()
	}
	if err == nil {
		for path, faces := range s.registered {
			found := false
			for _, f := range result.Faces {
				if f.Path == path {
					found = true
					break
				}
			}
			if !found {
				s.mu.Unlock()
				valid, e := fontcatalog.Inspect(job, path, fontcatalog.Private)
				s.mu.Lock()
				if e == nil && len(valid) > 0 && len(faces) > 0 && valid[0].Digest == faces[0].Digest {
					result.Faces = append(result.Faces, valid...)
				} else {
					delete(s.registered, path)
				}
			}
		}
		candidate := fontcatalog.NewSnapshot(previous.Version, result.Faces)
		for _, f := range previous.Faces() {
			if f.Preferred {
				if preferred, e := candidate.Prefer(f.ID); e == nil {
					candidate = preferred
				}
			}
		}
		if previous.Version == 0 || !reflect.DeepEqual(candidate.Faces(), previous.Faces()) {
			candidate = fontcatalog.NewSnapshot(previous.Version+1, candidate.Faces())
		}
		candidate.CheckedAt = time.Now().UTC()
		s.snapshot = candidate
		s.mu.Unlock()
		cacheErr := s.persist(candidate)
		s.mu.Lock()
		s.cacheErr = cacheErr
	}
	s.lastErr = err
	s.refreshing = nil
	close(running)
	s.publishLocked()
	return err
}
func (s *FontService) Register(ctx context.Context, resource fontcatalog.PrivateResource) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errors.New("font service closed")
	}
	s.wg.Add(1)
	s.mu.Unlock()
	defer s.wg.Done()
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	faces, err := fontcatalog.Inspect(ctx, resource.Path, fontcatalog.Private)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("font service closed")
	}
	s.registered[resource.Path] = faces
	all := s.snapshot.Faces()
	filtered := all[:0]
	for _, f := range all {
		if f.Path != resource.Path {
			filtered = append(filtered, f)
		}
	}
	filtered = append(filtered, faces...)
	next := fontcatalog.NewSnapshot(s.snapshot.Version, filtered)
	if !reflect.DeepEqual(next.Faces(), s.snapshot.Faces()) {
		checked := s.snapshot.CheckedAt
		s.snapshot = fontcatalog.NewSnapshot(s.snapshot.Version+1, filtered)
		s.snapshot.CheckedAt = checked
	}
	snapshot := s.snapshot
	s.mu.Unlock()
	cacheErr := s.persist(snapshot)
	s.mu.Lock()
	s.cacheErr = cacheErr
	s.publishLocked()
	return nil
}
func (s *FontService) persist(snapshot fontcatalog.Snapshot) error {
	if s.options.CachePath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.options.CachePath), 0700); err != nil {
		return err
	}
	return fontcatalog.SaveCache(s.options.CachePath, snapshot)
}

func (s *FontService) Prefer(id fontcatalog.ResourceID) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errors.New("font service closed")
	}
	s.wg.Add(1)
	s.mu.Unlock()
	defer s.wg.Done()
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errors.New("font service closed")
	}
	next, err := s.snapshot.Prefer(id)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	if !reflect.DeepEqual(next.Faces(), s.snapshot.Faces()) {
		checked := next.CheckedAt
		next = fontcatalog.NewSnapshot(s.snapshot.Version+1, next.Faces())
		next.CheckedAt = checked
		s.snapshot = next
	}
	s.mu.Unlock()
	cacheErr := s.persist(next)
	s.mu.Lock()
	s.cacheErr = cacheErr
	s.publishLocked()
	s.mu.Unlock()
	return nil
}
