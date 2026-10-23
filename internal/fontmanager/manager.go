package fontmanager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"glance/internal/fontmanager/fontcatalog"
	"glance/internal/fontmanager/fontdownload"
)

type FontScanner interface {
	Scan(context.Context, fontcatalog.Snapshot, bool) (fontcatalog.ScanResult, error)
}
type FontOptions struct {
	Publish     func(fontcatalog.Snapshot)
	Scanner     FontScanner
	CachePath   string
	PrivateRoot string
	Provider    fontdownload.Provider
}
type FontEvent struct {
	Snapshot   fontcatalog.Snapshot
	Refreshing bool
	Err        error
	CacheErr   error
}
type FontService struct {
	mu           sync.Mutex
	opMu         sync.Mutex
	options      FontOptions
	snapshot     fontcatalog.Snapshot
	registered   map[string][]fontcatalog.Face
	subscribers  map[chan FontEvent]struct{}
	refreshing   chan struct{}
	lastErr      error
	cacheErr     error
	closed       bool
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	ready        chan struct{}
	readyOnce    sync.Once
	readyErr     error
	initializing chan struct{}
	initialized  bool
	initErr      error
	downloader   *fontdownload.Downloader
	provider     fontdownload.Provider
}

func NewFontService(options FontOptions) *FontService {
	ctx, cancel := context.WithCancel(context.Background())
	provider := options.Provider
	if provider == nil {
		provider = fontdownload.DefaultManifest()
	}
	var downloader *fontdownload.Downloader
	if options.PrivateRoot != "" {
		downloader = fontdownload.New(options.PrivateRoot, nil)
	}
	return &FontService{options: options, registered: map[string][]fontcatalog.Face{}, subscribers: map[chan FontEvent]struct{}{}, ctx: ctx, cancel: cancel, ready: make(chan struct{}), downloader: downloader, provider: provider}
}

// Manager is the reusable process-level API. FontService remains as a source
// alias for callers being migrated from the application package.
type Manager = FontService
type Options = FontOptions
type Event = FontEvent

func New(options Options) *Manager { return NewFontService(options) }

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

func (s *FontService) beginTask() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("font service closed")
	}
	s.wg.Add(1)
	return nil
}

func (s *FontService) Close() error {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	s.mu.Unlock()
	s.wg.Wait()
	s.readyOnce.Do(func() {
		s.mu.Lock()
		s.readyErr = context.Canceled
		close(s.ready)
		s.mu.Unlock()
	})
	return nil
}

// Ready waits until initial filesystem verification finishes before returning
// a snapshot. A compatible cache remains available if that verification fails.
func (s *FontService) Ready(ctx context.Context) (fontcatalog.Snapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.Initialize(ctx); err != nil {
		if snapshot := s.Snapshot(); snapshot.Version > 0 {
			return snapshot, nil
		}
		return fontcatalog.Snapshot{}, err
	}
	select {
	case <-s.ready:
	case <-ctx.Done():
		return fontcatalog.Snapshot{}, ctx.Err()
	case <-s.ctx.Done():
		return fontcatalog.Snapshot{}, s.ctx.Err()
	}
	s.mu.Lock()
	snapshot, err := s.snapshot, s.readyErr
	s.mu.Unlock()
	if snapshot.Version > 0 {
		return snapshot, nil
	}
	if err == nil {
		err = errors.New("font catalog is not ready")
	}
	return fontcatalog.Snapshot{}, err
}

// Initialize publishes a persisted catalog before checking filesystem changes.
func (s *FontService) Initialize(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	if s.initialized {
		err := s.initErr
		s.mu.Unlock()
		return err
	}
	if running := s.initializing; running != nil {
		s.mu.Unlock()
		select {
		case <-running:
			s.mu.Lock()
			err := s.initErr
			s.mu.Unlock()
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-s.ctx.Done():
			return s.ctx.Err()
		}
	}
	running := make(chan struct{})
	s.initializing = running
	s.mu.Unlock()

	err := s.initialize(ctx)
	s.mu.Lock()
	s.initialized = true
	s.initErr = err
	s.initializing = nil
	close(running)
	s.mu.Unlock()
	return err
}

func (s *FontService) initialize(ctx context.Context) error {
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
	if ctx == nil {
		ctx = context.Background()
	}
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
	s.readyOnce.Do(func() {
		s.readyErr = err
		close(s.ready)
	})
	close(running)
	s.publishLocked()
	return err
}
func (s *FontService) Register(ctx context.Context, resource fontcatalog.PrivateResource) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := s.Ready(ctx); err != nil {
		return err
	}
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
	if _, err := s.Ready(context.Background()); err != nil {
		return err
	}
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

func (s *FontService) ListOnline(ctx context.Context) ([]fontdownload.Entry, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := s.Ready(ctx); err != nil {
		return nil, err
	}
	if err := s.beginTask(); err != nil {
		return nil, err
	}
	defer s.wg.Done()
	if s.provider == nil {
		return nil, nil
	}
	job, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	defer cancel()
	return s.provider.List(job)
}

// FetchAndRegister resolves an entry from the configured trusted provider,
// validates and durably publishes it through the downloader, then registers
// it into the live catalog before returning.
func (s *FontService) FetchAndRegister(ctx context.Context, id string, progress func(fontdownload.Progress)) error {
	if s.downloader == nil {
		return errors.New("font download storage is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := s.Ready(ctx); err != nil {
		return err
	}
	if err := s.beginTask(); err != nil {
		return err
	}
	defer s.wg.Done()
	entries, err := s.ListOnline(ctx)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.ID != id {
			continue
		}
		if ctx == nil {
			ctx = context.Background()
		}
		job, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(s.ctx, cancel)
		defer stop()
		defer cancel()
		resource, err := s.downloader.Fetch(job, entry, progress)
		if err != nil {
			return err
		}
		return s.Register(job, resource)
	}
	return errors.New("font is not present in the trusted provider")
}

func (s *FontService) CleanStaging() error {
	if s.downloader == nil {
		return nil
	}
	return s.downloader.CleanStaging()
}
