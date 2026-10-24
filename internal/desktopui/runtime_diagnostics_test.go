package desktopui

import (
	"path/filepath"
	"testing"

	"glance/internal/workspace"
)

func TestWorkspaceDiagnosticsCollection(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "workspace.db")
	store, err := workspace.Open(dbPath)
	if err != nil {
		t.Fatalf("workspace.Open: %v", err)
	}
	defer store.Close()

	col, err := workspace.OpenCollection(store)
	if err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}

	tab := workspace.Tab{
		ID:         "test-tab-1",
		Kind:       "document",
		Title:      "Test.docx",
		DocumentID: "doc-1",
		SourcePath: "/tmp/Test.docx",
		Position:   0,
	}
	if err := col.AddDocument(workspace.Session{
		Tab:     tab,
		Format:  "docx",
		Payload: []byte("synthetic-docx-payload"),
	}, true); err != nil {
		t.Fatalf("AddDocument: %v", err)
	}

	runtime := newDesktopRuntime()
	runtime.workspaceStore = store
	runtime.collection = col

	diagObj := runtime.diagnoseWorkspace()
	snap, ok := diagObj.(WorkspaceDiagnosticsSnapshot)
	if !ok {
		t.Fatalf("diagnoseWorkspace returned %T, want WorkspaceDiagnosticsSnapshot", diagObj)
	}

	if snap.Store == nil {
		t.Fatal("expected Store diagnostics to be non-nil")
	}
	if snap.Store.DocumentsCount != 1 {
		t.Errorf("expected 1 document in store, got %d", snap.Store.DocumentsCount)
	}
	if snap.Store.DocumentsPayloadBytes != int64(len("synthetic-docx-payload")) {
		t.Errorf("expected %d payload bytes, got %d", len("synthetic-docx-payload"), snap.Store.DocumentsPayloadBytes)
	}

	if snap.Collection.TabCount != 1 {
		t.Errorf("expected 1 tab in collection, got %d", snap.Collection.TabCount)
	}
	if len(snap.Collection.Tabs) != 1 || snap.Collection.Tabs[0].ID != "test-tab-1" {
		t.Fatalf("unexpected tabs in collection: %+v", snap.Collection.Tabs)
	}

	// Verify ShrinkMemory does not error
	if err := runtime.shrinkWorkspace(); err != nil {
		t.Errorf("shrinkWorkspace failed: %v", err)
	}
}
