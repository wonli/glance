package workspace

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestClosingLastTabReleasesSnapshot(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	c, err := OpenCollection(store)
	if err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	addAndCloseLargeSnapshot(t, c)
	runtime.GC()
	runtime.ReadMemStats(&after)
	if retained := int64(after.HeapAlloc) - int64(before.HeapAlloc); retained > 2*1024*1024 {
		t.Fatalf("closed snapshot retained %d bytes", retained)
	}
	runtime.KeepAlive(c)
}

func addAndCloseLargeSnapshot(t *testing.T, c *Collection) {
	t.Helper()
	if err := c.AddDocument(Session{Tab: Tab{ID: "large", DocumentID: "large"}, Format: "json", Payload: make([]byte, 16*1024*1024)}, true); err != nil {
		t.Fatal(err)
	}
	if err := c.CloseTab("large"); err != nil {
		t.Fatal(err)
	}
}
