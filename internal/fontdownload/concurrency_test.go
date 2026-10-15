package fontdownload

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"
)

type observedDownloadContext struct {
	context.Context
	entered chan struct{}
	once    sync.Once
}

func (c *observedDownloadContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.entered) })
	return c.Context.Done()
}
func testDownloadEntry(url string) Entry {
	sum := sha256.Sum256(goregular.TTF)
	return Entry{ID: "test", Family: "Go", Version: "1", URL: url, SHA256: hex.EncodeToString(sum[:]), MaxBytes: 1 << 20, License: "test"}
}
func TestConcurrentDownloadDeduplicates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started, release := make(chan struct{}), make(chan struct{})
	var count atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if count.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			w.Write(goregular.TTF)
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	d := New(t.TempDir(), server.Client())
	entry := testDownloadEntry(server.URL)
	errors := make(chan error, 2)
	go func() { _, err := d.Fetch(ctx, entry, nil); errors <- err }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	waiter := &observedDownloadContext{Context: ctx, entered: make(chan struct{})}
	go func() { _, err := d.Fetch(waiter, entry, nil); errors <- err }()
	select {
	case <-waiter.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	close(release)
	for i := 0; i < 2; i++ {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}
	if count.Load() != 1 {
		t.Fatalf("downloaded %d times", count.Load())
	}
}
func TestDownloadCancellationCanRetrySameResource(t *testing.T) {
	started := make(chan struct{})
	var count atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if count.Add(1) == 1 {
			close(started)
			<-r.Context().Done()
			return
		}
		w.Write(goregular.TTF)
	}))
	defer server.Close()
	d := New(t.TempDir(), server.Client())
	entry := testDownloadEntry(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := d.Fetch(ctx, entry, nil); done <- err }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("canceled download succeeded")
	}
	if _, err := d.Fetch(context.Background(), entry, nil); err != nil {
		t.Fatal(err)
	}
}
func TestDownloadRejectsRedirectOutsideManifestOrigin(t *testing.T) {
	var contacted atomic.Int32
	outside := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { contacted.Add(1); w.Write(goregular.TTF) }))
	defer outside.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, outside.URL, http.StatusFound) }))
	defer server.Close()
	if _, err := New(t.TempDir(), server.Client()).Fetch(context.Background(), testDownloadEntry(server.URL), nil); err == nil {
		t.Fatal("followed cross-origin redirect")
	}
	if contacted.Load() != 0 {
		t.Fatal("contacted untrusted origin")
	}
}
