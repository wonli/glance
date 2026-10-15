package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"glance/internal/fontcatalog"
	"golang.org/x/image/font/gofont/goregular"
)

type controlledFontScanner struct {
	mu      sync.Mutex
	calls   int
	release chan struct{}
	err     error
}

func (s *controlledFontScanner) Scan(ctx context.Context, previous fontcatalog.Snapshot, force bool) (fontcatalog.ScanResult, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	if s.release != nil {
		select {
		case <-s.release:
		case <-ctx.Done():
			return fontcatalog.ScanResult{}, ctx.Err()
		}
	}
	return fontcatalog.ScanResult{Faces: []fontcatalog.Face{{ID: "fixture", Family: "Example", Path: "/fixture", Digest: "example", Weight: 400}}}, s.err
}
func TestFontRefreshCoalesces(t *testing.T) {
	scanner := &controlledFontScanner{release: make(chan struct{})}
	s := NewFontService(FontOptions{Scanner: scanner})
	defer s.Close()
	done := make(chan error, 2)
	go func() { done <- s.Refresh(context.Background(), true) }()
	for {
		scanner.mu.Lock()
		n := scanner.calls
		scanner.mu.Unlock()
		if n > 0 {
			break
		}
	}
	go func() { done <- s.Refresh(context.Background(), true) }()
	close(scanner.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Version != 1 {
		t.Fatal("refresh changed identical catalog version")
	}
}
func TestFontRefreshFailureRetainsSnapshot(t *testing.T) {
	scanner := &controlledFontScanner{}
	s := NewFontService(FontOptions{Scanner: scanner})
	defer s.Close()
	if e := s.Refresh(context.Background(), true); e != nil {
		t.Fatal(e)
	}
	scanner.err = errors.New("unavailable")
	if e := s.Refresh(context.Background(), true); e == nil {
		t.Fatal("expected error")
	}
	if len(s.Snapshot().Families()) != 1 || s.Snapshot().Version != 1 {
		t.Fatal("failure destroyed catalog")
	}
}
func TestFontUnchangedRefreshKeepsVersion(t *testing.T) {
	s := NewFontService(FontOptions{Scanner: &controlledFontScanner{}})
	defer s.Close()
	s.Refresh(context.Background(), true)
	s.Refresh(context.Background(), true)
	if s.Snapshot().Version != 1 {
		t.Fatal("unnecessary layout invalidation")
	}
}
func TestFontRefreshCannotOverwriteRegistration(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "private.ttf")
	os.WriteFile(p, goregular.TTF, 0600)
	s := NewFontService(FontOptions{Scanner: &controlledFontScanner{}})
	defer s.Close()
	s.Refresh(context.Background(), true)
	if e := s.Register(context.Background(), fontcatalog.PrivateResource{Path: p}); e != nil {
		t.Fatal(e)
	}
	s.Refresh(context.Background(), true)
	if len(s.Snapshot().Search("Go")) != 1 {
		t.Fatal("registration lost to scan")
	}
}
func TestFontServiceCloseDrainsWorkers(t *testing.T) {
	s := NewFontService(FontOptions{Scanner: &controlledFontScanner{release: make(chan struct{})}})
	done := make(chan error)
	go func() { done <- s.Refresh(context.Background(), true) }()
	s.Close()
	if err := <-done; err == nil {
		t.Fatal("closed service accepted refresh")
	}
}

func TestFontPublicationIsSynchronous(t *testing.T) {
	var published uint64
	s := NewFontService(FontOptions{Scanner: &controlledFontScanner{}, Publish: func(snapshot fontcatalog.Snapshot) { published = snapshot.Version }})
	defer s.Close()
	if err := s.Refresh(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if published != 1 {
		t.Fatal("render snapshot only published by delayed window subscription")
	}
}

func TestFontRefreshDoesNotResurrectDeletedPrivateFont(t *testing.T) {
	p := filepath.Join(t.TempDir(), "private.ttf")
	os.WriteFile(p, goregular.TTF, 0600)
	s := NewFontService(FontOptions{Scanner: &controlledFontScanner{}})
	defer s.Close()
	s.Register(context.Background(), fontcatalog.PrivateResource{Path: p})
	os.Remove(p)
	if err := s.Refresh(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if len(s.Snapshot().Search("Go")) != 0 {
		t.Fatal("deleted private resource resurrected")
	}
}
