package desktopui

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"glance/internal/workspace"
)

func newPDFWorkspaceRestoreTestApp(t *testing.T, payload []byte, sourcePath string, dirty bool) (*App, *workspace.Store, workspace.Session) {
	t.Helper()
	store, err := workspace.Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	collection, err := workspace.OpenCollection(store)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	runtime := newDesktopRuntime()
	runtime.workspaceStore = store
	runtime.collection = collection
	app := newAppWithRuntime(nil, runtime)
	tab := workspace.Tab{ID: "tab-pdf", DocumentID: "doc-pdf", Kind: "file", Title: "restore.pdf", SourcePath: sourcePath}
	if payload == nil {
		if err := collection.AddTab(tab, "pdf"); err != nil {
			_ = store.Close()
			t.Fatal(err)
		}
	} else if err := collection.AddDocument(workspace.Session{Tab: tab, Format: "pdf", Payload: payload, Dirty: dirty}, true); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	collection.ReleasePayload(tab.ID)
	app.tabMeta[tab.ID] = tab
	record, ok := collection.GetMetadata(tab.ID)
	if !ok {
		_ = store.Close()
		t.Fatal("PDF workspace record was not created")
	}
	t.Cleanup(func() {
		close(app.shutdown)
		app.stopAndWaitManagedBackground()
		runtime.removeSession(tab.DocumentID)
		if err := store.Close(); err != nil {
			t.Errorf("close PDF workspace test store: %v", err)
		}
	})
	return app, store, *record
}

func receivePDFWorkspaceRestoreResult(t *testing.T, app *App) pdfWorkspaceRestoreResult {
	t.Helper()
	select {
	case result := <-app.workspacePDFRestoreResults:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for PDF workspace restore result")
		return pdfWorkspaceRestoreResult{}
	}
}

func TestPDFWorkspaceRestoreStreamsStoredSnapshot(t *testing.T) {
	want := pdfTestBytes(t)
	app, _, record := newPDFWorkspaceRestoreTestApp(t, want, "", true)
	if record.Payload != nil {
		t.Fatalf("metadata record retained %d PDF payload bytes", len(record.Payload))
	}
	record.ViewState, _ = json.Marshal(sessionViewSnapshot{PDFPageNumber: 2, PDFZoomPercent: 150, PDFFitMode: "custom"})
	if err := app.collection.SaveViewState(record.Tab.ID, record.ViewState); err != nil {
		t.Fatal(err)
	}
	app.startWorkspacePDFRestore(record)
	result := receivePDFWorkspaceRestoreResult(t, app)
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.doc == nil {
		t.Fatal("restore result has no PDF document")
	}
	got, err := result.doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("streamed PDF restore changed the snapshot bytes")
	}
	app.applyPDFWorkspaceRestoreResult(result)
	if app.pdfDoc != result.doc || app.activeTabID != record.Tab.ID {
		t.Fatal("current restore result was not installed and activated")
	}
	if app.documentSessionState.documentID != record.Tab.DocumentID {
		t.Fatalf("restore attached document ID %q, want %q", app.documentSessionState.documentID, record.Tab.DocumentID)
	}
	if !app.documentSessionState.pdfSnapshotPresent || app.documentSessionState.pdfPersistedRevision != result.doc.Info().Revision {
		t.Fatalf("SQLite restore persistence seed = present:%v revision:%d, want present:true revision:%d", app.documentSessionState.pdfSnapshotPresent, app.documentSessionState.pdfPersistedRevision, result.doc.Info().Revision)
	}
	if app.pdfView == nil || app.pdfView.page != 1 || app.pdfView.zoom != 1.5 || app.pdfView.fit != "" {
		if app.pdfView == nil {
			t.Fatal("PDF view state was not restored")
		}
		t.Fatalf("PDF view state = page:%d zoom:%.2f fit:%q", app.pdfView.page, app.pdfView.zoom, app.pdfView.fit)
	}
	installed, ok := app.collection.GetMetadata(record.Tab.ID)
	if !ok || installed.Payload != nil {
		t.Fatal("PDF restore hydrated the collection payload cache")
	}
}

func TestPDFWorkspaceRestoreOpensSourceWithoutSnapshot(t *testing.T) {
	want := pdfTestBytes(t)
	source := filepath.Join(t.TempDir(), "source.pdf")
	if err := os.WriteFile(source, want, 0o600); err != nil {
		t.Fatal(err)
	}
	app, _, record := newPDFWorkspaceRestoreTestApp(t, nil, source, false)
	if record.SnapshotPresent || record.Payload != nil {
		t.Fatalf("source-only metadata = present:%v payload:%d", record.SnapshotPresent, len(record.Payload))
	}
	app.startWorkspacePDFRestore(record)
	result := receivePDFWorkspaceRestoreResult(t, app)
	if result.err != nil {
		t.Fatal(result.err)
	}
	got, err := result.doc.Snapshot()
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("source restore bytes differ: size=%d err=%v", len(got), err)
	}
	app.applyPDFWorkspaceRestoreResult(result)
	if app.pdfDoc != result.doc {
		t.Fatal("source-restored PDF was not installed")
	}
	if app.documentSessionState.pdfSnapshotPresent || app.documentSessionState.pdfPersistedRevision != 0 {
		t.Fatalf("source-only restore persistence seed = present:%v revision:%d, want no stored snapshot", app.documentSessionState.pdfSnapshotPresent, app.documentSessionState.pdfPersistedRevision)
	}
}

func TestPDFWorkspaceRestoreStaleResultClosesDocument(t *testing.T) {
	app, _, record := newPDFWorkspaceRestoreTestApp(t, pdfTestBytes(t), "", false)
	app.startWorkspacePDFRestore(record)
	result := receivePDFWorkspaceRestoreResult(t, app)
	if result.err != nil || result.doc == nil {
		t.Fatalf("restore result = doc:%v err:%v", result.doc != nil, result.err)
	}
	app.activateRecent()
	app.applyPDFWorkspaceRestoreResult(result)
	if _, err := result.doc.Snapshot(); err == nil {
		t.Fatal("stale restore document remained open")
	}
	if app.pdfDoc != nil || app.activeTabID != workspace.RecentTabID {
		t.Fatal("stale restore replaced the active recent view")
	}
}

func TestUnopenedPDFCloseUsesMetadataWithoutLoadingPayload(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.pdf")
	if err := os.WriteFile(source, pdfTestBytes(t), 0o600); err != nil {
		t.Fatal(err)
	}
	app, _, record := newPDFWorkspaceRestoreTestApp(t, []byte("invalid stored PDF snapshot"), source, false)
	if record.Payload != nil {
		t.Fatalf("metadata record retained %d PDF payload bytes", len(record.Payload))
	}
	app.closeWorkspaceTab(record.Tab.ID)
	if _, ok := app.collection.GetMetadata(record.Tab.ID); ok {
		t.Fatal("clean unopened PDF tab was not closed")
	}
	if app.statusRef.ID != "" || app.modal.Visible {
		t.Fatalf("clean PDF close attempted a document restore: status=%q modal=%v", app.statusRef.ID, app.modal.Visible)
	}
}
