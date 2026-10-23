package diagramrender

import (
	"testing"
	"time"
)

func TestIdleCleanupDoesNotWaitOnWindowThread(t *testing.T) {
	defaults.Lock()
	done := make(chan struct{})
	go func() { _ = IdleCleanup(); close(done) }()
	select {
	case <-done:
		defaults.Unlock()
	case <-time.After(time.Second):
		defaults.Unlock()
		t.Fatal("capturing idle cleanup blocked on renderer teardown")
	}
}

func TestIdleCleanupDoesNotCloseNewlyReusedRenderer(t *testing.T) {
	r := New(Bundle{})
	r.engine = &captureEngine{}
	defaults.Lock()
	previous := defaults.r
	defaults.r = r
	defaults.Unlock()
	defer func() { defaults.Lock(); defaults.r = previous; defaults.Unlock() }()
	cleanup := IdleCleanup()
	got, err := Default()
	if err != nil || got != r {
		t.Fatalf("reuse renderer: %v", err)
	}
	cleanup()
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed {
		t.Fatal("stale idle cleanup closed a reopened document's renderer")
	}
}
