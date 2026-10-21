package desktopui

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"glance/internal/document"
	"glance/internal/format/pdfdoc"
	"glance/internal/workspace"
)

type pdfPersistenceFixture struct {
	app        *App
	runtime    *desktopRuntime
	store      *workspace.Store
	collection *workspace.Collection
	dbPath     string
	tab        workspace.Tab
	state      *documentSessionState
	doc        *pdfdoc.Document
	original   []byte
}

func newPDFPersistenceFixture(t *testing.T) *pdfPersistenceFixture {
	t.Helper()
	original := pdfTestBytes(t)
	dbPath := filepath.Join(t.TempDir(), "workspace.db")
	store, err := workspace.Open(dbPath)
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
	tab := workspace.Tab{ID: "tab-pdf", DocumentID: "doc-pdf", Kind: "file", Title: "report.pdf"}
	if err := collection.AddDocument(workspace.Session{Tab: tab, Format: "pdf", Payload: original}, true); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	collection.ReleasePayload(tab.ID)
	doc, err := pdfdoc.OpenSnapshot(original)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	shared := newSharedDocumentSession("")
	shared.documentID = tab.DocumentID
	shared.pdfDoc = doc
	shared.pdfSnapshotPresent = true
	shared.pdfPersistedRevision = doc.Info().Revision
	state := newDocumentSessionView(shared, app.windowID)
	state.documentID = tab.DocumentID
	t.Cleanup(func() {
		close(app.shutdown)
		app.stopAndWaitManagedBackground()
		_ = doc.Close()
		runtime.removeSession(tab.DocumentID)
		if err := store.Close(); err != nil {
			t.Errorf("close PDF persistence test store: %v", err)
		}
	})
	return &pdfPersistenceFixture{app: app, runtime: runtime, store: store, collection: collection, dbPath: dbPath, tab: tab, state: state, doc: doc, original: original}
}

func (f *pdfPersistenceFixture) write(viewState []byte) error {
	return f.runtime.writeWorkspaceSnapshot(workspaceWriteJob{
		tabID:      f.tab.ID,
		documentID: f.tab.DocumentID,
		format:     "pdf",
		state:      f.state,
		viewState:  viewState,
	})
}

func TestPersistPDFViewStateDoesNotRewritePayload(t *testing.T) {
	f := newPDFPersistenceFixture(t)
	if err := f.doc.Close(); err != nil {
		t.Fatal(err)
	}
	viewState := []byte(`{"page":7}`)
	if err := f.write(viewState); err != nil {
		t.Fatalf("persist view state without snapshotting the closed PDF: %v", err)
	}
	var got bytes.Buffer
	if _, err := f.collection.StreamPayload(f.tab.ID, &got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), f.original) {
		t.Fatal("view-state persistence changed the stored PDF payload")
	}
	meta, ok := f.collection.GetMetadata(f.tab.ID)
	if !ok || !bytes.Equal(meta.ViewState, viewState) || meta.Payload != nil {
		t.Fatalf("metadata after view-state persistence = %+v, present=%v", meta, ok)
	}
}

func TestPersistPDFRevisionUpdatesSnapshotWithoutCaching(t *testing.T) {
	f := newPDFPersistenceFixture(t)
	page, err := f.doc.Page(0)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.doc.Apply(document.PDFCommand{Kind: "replace", Page: 0, Revision: page.Revision, Run: 0, Text: "EDITED PDF REVISION"}); err != nil {
		t.Fatal(err)
	}
	firstRevision := f.doc.Info().Revision
	if err := f.write([]byte(`{"page":1}`)); err != nil {
		t.Fatal(err)
	}
	assertPersistedPDFMatchesDocument(t, f)
	if f.state.pdfPersistedRevision != firstRevision || !f.state.pdfSnapshotPresent {
		t.Fatalf("persisted PDF state = revision:%d present:%v, want revision:%d present:true", f.state.pdfPersistedRevision, f.state.pdfSnapshotPresent, firstRevision)
	}

	if !f.doc.Undo() {
		t.Fatal("undo did not restore the original PDF revision")
	}
	secondRevision := f.doc.Info().Revision
	if secondRevision == firstRevision {
		t.Fatal("undo did not publish a new PDF revision")
	}
	if err := f.write([]byte(`{"page":2}`)); err != nil {
		t.Fatal(err)
	}
	assertPersistedPDFMatchesDocument(t, f)
	if f.state.pdfPersistedRevision != secondRevision {
		t.Fatalf("persisted revision after undo = %d, want %d", f.state.pdfPersistedRevision, secondRevision)
	}
}

func assertPersistedPDFMatchesDocument(t *testing.T, f *pdfPersistenceFixture) {
	t.Helper()
	want, err := f.doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	if _, err := f.collection.StreamPayload(f.tab.ID, &got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatal("workspace payload does not match the persisted PDF revision")
	}
	meta, ok := f.collection.GetMetadata(f.tab.ID)
	if !ok || meta.Payload != nil || meta.PayloadSize != int64(len(want)) {
		t.Fatalf("PDF collection cache after persistence = present:%v payload:%d size:%d, want no payload cache and size %d", ok, len(meta.Payload), meta.PayloadSize, len(want))
	}
}

func TestPDFWorkspaceWriteFailureDoesNotAdvancePersistedRevision(t *testing.T) {
	f := newPDFPersistenceFixture(t)
	page, err := f.doc.Page(0)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.doc.Apply(document.PDFCommand{Kind: "replace", Page: 0, Revision: page.Revision, Run: 0, Text: "EDITED PDF REVISION"}); err != nil {
		t.Fatal(err)
	}
	initialPersistedRevision := f.state.pdfPersistedRevision
	db, err := sql.Open("sqlite", f.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER fail_pdf_payload_update BEFORE UPDATE OF payload ON documents BEGIN SELECT RAISE(FAIL, 'injected PDF snapshot write failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := f.write([]byte(`{"page":3}`)); err == nil {
		t.Fatal("PDF snapshot write unexpectedly succeeded despite the injected database failure")
	}
	if f.state.pdfPersistedRevision != initialPersistedRevision {
		t.Fatalf("failed write advanced persisted revision to %d, want %d", f.state.pdfPersistedRevision, initialPersistedRevision)
	}
	if !f.doc.Dirty() {
		t.Fatal("failed workspace write cleared the PDF dirty state")
	}
}

func TestPDFWorkspaceMappedSnapshotLifecycleAcrossWindows(t *testing.T) {
	original := pdfTestBytes(t)
	sourcePath := filepath.Join(t.TempDir(), "source.pdf")
	if err := os.WriteFile(sourcePath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "workspace.db")
	store, err := workspace.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	collection, err := workspace.OpenCollection(store)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	runtime := newDesktopRuntime()
	runtime.workspaceStore, runtime.collection = store, collection
	app := newAppWithRuntime(nil, runtime)
	tab := workspace.Tab{ID: "tab-pdf-life", DocumentID: "doc-pdf-life", Kind: "file", Title: "source.pdf", SourcePath: sourcePath}
	if err := collection.AddTab(tab, "pdf"); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	app.tabMeta[tab.ID] = tab
	var second *App
	t.Cleanup(func() {
		if second != nil {
			close(second.shutdown)
			second.stopAndWaitManagedBackground()
		}
		close(app.shutdown)
		app.stopAndWaitManagedBackground()
		runtime.removeSession(tab.DocumentID)
		if err := store.Close(); err != nil {
			t.Errorf("close PDF lifecycle test store: %v", err)
		}
	})

	metadata, ok := collection.GetMetadata(tab.ID)
	if !ok || metadata.SnapshotPresent {
		t.Fatalf("new source tab snapshot state = present:%v, exists:%v; want no snapshot", metadata != nil && metadata.SnapshotPresent, ok)
	}
	if !app.startWorkspacePDFRestore(*metadata) {
		t.Fatal("source PDF restore did not start")
	}
	opened := receivePDFWorkspaceRestoreResult(t, app)
	if opened.err != nil {
		t.Fatal(opened.err)
	}
	app.applyPDFWorkspaceRestoreResult(opened)
	if app.pdfDoc == nil || app.documentSessionState.pdfSnapshotPresent {
		t.Fatal("source-opened PDF did not retain the no-snapshot state")
	}
	runtime.attachWindow(1, app)
	if err := runtime.persistPDFWorkspaceSnapshot(collection, app.documentSessionState, tab.ID, []byte(`{"page":1}`)); err != nil {
		t.Fatalf("persist initial recovery snapshot: %v", err)
	}
	assertWorkspacePDFSnapshot(t, collection, tab.ID, original)
	if got, err := os.ReadFile(sourcePath); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("initial workspace checkpoint changed source file: size=%d err=%v", len(got), err)
	}

	runtime.removeSession(tab.DocumentID)
	record, ok := collection.GetMetadata(tab.ID)
	if !ok || !record.SnapshotPresent || record.Payload != nil {
		t.Fatalf("restorable PDF metadata = present:%v payload:%d exists:%v", ok && record.SnapshotPresent, len(record.Payload), ok)
	}
	if !app.startWorkspacePDFRestore(*record) {
		t.Fatal("SQLite PDF restore did not start")
	}
	restored := receivePDFWorkspaceRestoreResult(t, app)
	if restored.err != nil {
		t.Fatal(restored.err)
	}
	app.applyPDFWorkspaceRestoreResult(restored)
	doc := app.pdfDoc
	if doc == nil || !app.documentSessionState.pdfSnapshotPresent {
		t.Fatal("SQLite-restored PDF did not seed its persisted snapshot state")
	}
	page, err := doc.Page(0)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Apply(document.PDFCommand{Kind: "replace", Page: 0, Revision: page.Revision, Run: 0, Text: "LIFECYCLE EDIT"}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.persistPDFWorkspaceSnapshot(collection, app.documentSessionState, tab.ID, []byte(`{"page":2}`)); err != nil {
		t.Fatalf("persist edited PDF revision: %v", err)
	}
	edited, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	assertWorkspacePDFSnapshot(t, collection, tab.ID, edited)
	if !doc.Undo() {
		t.Fatal("PDF edit could not be undone")
	}
	if err := runtime.persistPDFWorkspaceSnapshot(collection, app.documentSessionState, tab.ID, []byte(`{"page":1}`)); err != nil {
		t.Fatalf("persist undone PDF revision: %v", err)
	}
	assertWorkspacePDFSnapshot(t, collection, tab.ID, original)

	savePath := filepath.Join(t.TempDir(), "saved-copy.pdf")
	if err := doc.SaveAs(savePath); err != nil {
		t.Fatalf("save restored PDF copy: %v", err)
	}
	reopened, err := pdfdoc.Open(savePath)
	if err != nil {
		t.Fatalf("reopen saved PDF copy: %v", err)
	}
	savedBytes, err := reopened.Snapshot()
	_ = reopened.Close()
	if err != nil || !bytes.Equal(savedBytes, original) {
		t.Fatalf("save/reopen bytes differ from undone snapshot: size=%d err=%v", len(savedBytes), err)
	}
	if got, err := os.ReadFile(sourcePath); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("save/reopen changed original source file: size=%d err=%v", len(got), err)
	}

	second = newAppWithRuntime(nil, runtime)
	second.sessions = make(map[string]*documentSessionState)
	second.tabMeta[tab.ID] = tab
	secondState := newDocumentSessionView(app.documentSessionState.sharedDocumentSession, 2)
	secondState.documentID = tab.DocumentID
	second.documentSessionState = secondState
	second.sessions[tab.ID] = secondState
	second.activeTabID = tab.ID
	second.setActiveSessionID(tab.DocumentID)
	runtime.attachWindow(2, second)
	runtime.detachWindow(1)
	if _, err := doc.Snapshot(); err != nil {
		t.Fatalf("closing the first view released the shared PDF mapping: %v", err)
	}
	runtime.detachWindow(2)
	runtime.removeSession(tab.DocumentID)
	if _, err := doc.Snapshot(); err == nil {
		t.Fatal("released shared PDF session kept its mapped snapshot open")
	}
}

func assertWorkspacePDFSnapshot(t *testing.T, collection *workspace.Collection, tabID string, want []byte) {
	t.Helper()
	var got bytes.Buffer
	if _, err := collection.StreamPayload(tabID, &got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatal("workspace PDF snapshot differs from the expected revision")
	}
	meta, ok := collection.GetMetadata(tabID)
	if !ok || meta.Payload != nil || meta.PayloadSize != int64(len(want)) {
		t.Fatalf("workspace PDF cache = exists:%v payload:%d size:%d; want no cache and size %d", ok, len(meta.Payload), meta.PayloadSize, len(want))
	}
}
