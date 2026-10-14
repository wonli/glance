package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

type testDocumentAdapter struct {
	mu         sync.Mutex
	format     Format
	path       string
	dirty      bool
	readOnly   bool
	saveErr    error
	snapshot   []byte
	closeCalls int
}

func (d *testDocumentAdapter) Format() Format { return d.format }
func (d *testDocumentAdapter) Path() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.path
}
func (d *testDocumentAdapter) Dirty() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dirty
}
func (d *testDocumentAdapter) ReadOnly() bool { return d.readOnly }
func (d *testDocumentAdapter) Save(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.saveErr != nil {
		return d.saveErr
	}
	d.dirty = false
	return nil
}
func (d *testDocumentAdapter) SaveWithSnapshot(ctx context.Context) ([]byte, error) {
	if err := d.Save(ctx); err != nil {
		return nil, err
	}
	return append([]byte(nil), d.snapshot...), nil
}
func (d *testDocumentAdapter) SaveAs(_ context.Context, path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.saveErr != nil {
		return d.saveErr
	}
	d.path = path
	d.dirty = false
	return nil
}
func (d *testDocumentAdapter) SaveAsWithSnapshot(ctx context.Context, path string) ([]byte, error) {
	if err := d.SaveAs(ctx, path); err != nil {
		return nil, err
	}
	return append([]byte(nil), d.snapshot...), nil
}
func (d *testDocumentAdapter) Close(context.Context) error {
	d.mu.Lock()
	d.closeCalls++
	d.mu.Unlock()
	return nil
}

type testFormatFactory struct {
	mu      sync.Mutex
	format  Format
	opened  int
	created int
	openDoc DocumentAdapter
	newDoc  DocumentAdapter
}

func (f *testFormatFactory) Format() Format { return f.format }
func (f *testFormatFactory) Open(_ context.Context, path string) (DocumentAdapter, error) {
	f.mu.Lock()
	f.opened++
	f.mu.Unlock()
	if f.openDoc == nil {
		return nil, errors.New("unexpected open")
	}
	return f.openDoc, nil
}
func (f *testFormatFactory) Create(context.Context) (DocumentAdapter, error) {
	f.mu.Lock()
	f.created++
	f.mu.Unlock()
	if f.newDoc == nil {
		return nil, errors.New("unexpected create")
	}
	return f.newDoc, nil
}

func TestRuntimeOpenDocumentReusesCanonicalSession(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "report.json")
	adapter := &testDocumentAdapter{format: FormatJSON}
	factory := &testFormatFactory{format: FormatJSON, openDoc: adapter}
	runtime, err := NewRuntime(ctx, RuntimeOptions{Factories: []FormatFactory{factory}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	first, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	second, err := runtime.OpenDocument(ctx, filepath.Join(filepath.Dir(path), ".", filepath.Base(path)))
	if err != nil {
		t.Fatalf("OpenDocument with equivalent path: %v", err)
	}
	if first.DocumentID != second.DocumentID {
		t.Fatalf("equivalent paths produced document IDs %q and %q", first.DocumentID, second.DocumentID)
	}
	if factory.opened != 1 {
		t.Fatalf("factory opened %d adapters, want one", factory.opened)
	}
	if first.Format != FormatJSON || first.Dirty || first.ReadOnly {
		t.Fatalf("unexpected document metadata: %+v", first)
	}
	documents, err := runtime.ListDocuments(ctx)
	if err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}
	if len(documents) != 1 {
		t.Fatalf("ListDocuments returned %d entries, want one", len(documents))
	}
}

func TestRuntimeOpenAcceptsAdapterDeclaredSourceFormat(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.xls")
	adapter := &declaredSourceFormatAdapter{testDocumentAdapter: &testDocumentAdapter{format: FormatXLSX}, source: FormatXLS}
	factory := &testFormatFactory{format: FormatXLS, openDoc: adapter}
	runtime, err := NewRuntime(ctx, RuntimeOptions{Factories: []FormatFactory{factory}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	info, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument declared XLS-to-XLSX adapter: %v", err)
	}
	if info.Format != FormatXLSX || info.SourcePath != canonicalPath(path) {
		t.Fatalf("opened info = %+v; want XLSX editor format and original XLS source path", info)
	}
}

func TestRuntimeOpenRejectsUndeclaredFormatMismatch(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.xls")
	adapter := &testDocumentAdapter{format: FormatXLSX}
	factory := &testFormatFactory{format: FormatXLS, openDoc: adapter}
	runtime, err := NewRuntime(ctx, RuntimeOptions{Factories: []FormatFactory{factory}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	if _, err := runtime.OpenDocument(ctx, path); err == nil {
		t.Fatal("OpenDocument accepted a format mismatch without an exact source-format declaration")
	}
	if adapter.closeCalls != 1 {
		t.Fatalf("rejected mismatched adapter close calls = %d; want 1", adapter.closeCalls)
	}
}

func TestRuntimeSaveFailurePreservesDirtySessionAndDirtyCloseIsRejected(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "report.json")
	saveErr := errors.New("disk full")
	adapter := &testDocumentAdapter{format: FormatJSON, dirty: true, saveErr: saveErr}
	factory := &testFormatFactory{format: FormatJSON, openDoc: adapter}
	runtime, err := NewRuntime(ctx, RuntimeOptions{Factories: []FormatFactory{factory}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); !errors.Is(err, ErrDirtyDocument) {
		t.Fatalf("CloseDocument error = %v, want ErrDirtyDocument", err)
	}
	if adapter.closeCalls != 0 {
		t.Fatalf("dirty adapter closed %d times", adapter.closeCalls)
	}
	if _, _, err := runtime.SaveDocumentWithSnapshot(ctx, doc.DocumentID); !errors.Is(err, saveErr) {
		t.Fatalf("SaveDocument error = %v, want %v", err, saveErr)
	}
	current, err := runtime.GetDocument(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if !current.Dirty {
		t.Fatal("failed save cleared dirty state")
	}
}

func TestRuntimeCancelledOpenDoesNotCallFactory(t *testing.T) {
	factory := &testFormatFactory{format: FormatJSON, openDoc: &testDocumentAdapter{format: FormatJSON}}
	runtime, err := NewRuntime(context.Background(), RuntimeOptions{Factories: []FormatFactory{factory}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runtime.OpenDocument(ctx, filepath.Join(t.TempDir(), "cancelled.json")); !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenDocument error = %v, want context.Canceled", err)
	}
	if factory.opened != 0 {
		t.Fatalf("factory was called %d times for cancelled request", factory.opened)
	}
}

func TestRuntimeCreateSaveAsAndRejectOverwrite(t *testing.T) {
	ctx := context.Background()
	created := &testDocumentAdapter{format: FormatJSON, dirty: true}
	secondDraft := &testDocumentAdapter{format: FormatJSON, dirty: true}
	factory := &testFormatFactory{format: FormatJSON, newDoc: created}
	runtime, err := NewRuntime(ctx, RuntimeOptions{Factories: []FormatFactory{factory}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	first, err := runtime.CreateDocument(ctx, FormatJSON)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	if first.DocumentID == "" || first.SourcePath != "" || !first.Dirty {
		t.Fatalf("unexpected created document metadata: %+v", first)
	}
	firstTarget := filepath.Join(t.TempDir(), "new.json")
	saved, err := runtime.SaveDocumentAs(ctx, first.DocumentID, firstTarget)
	if err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if saved.SourcePath != canonicalPath(firstTarget) || saved.Dirty {
		t.Fatalf("unexpected saved document metadata: %+v", saved)
	}
	opened, err := runtime.OpenDocument(ctx, firstTarget)
	if err != nil {
		t.Fatalf("OpenDocument after SaveAs: %v", err)
	}
	if opened.DocumentID != first.DocumentID {
		t.Fatalf("open after SaveAs returned ID %q, want %q", opened.DocumentID, first.DocumentID)
	}

	factory.newDoc = secondDraft
	second, err := runtime.CreateDocument(ctx, FormatJSON)
	if err != nil {
		t.Fatalf("CreateDocument second draft: %v", err)
	}
	existingTarget := filepath.Join(t.TempDir(), "existing.json")
	if err := os.WriteFile(existingTarget, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("create existing target: %v", err)
	}
	if _, err := runtime.SaveDocumentAs(ctx, second.DocumentID, existingTarget); !errors.Is(err, ErrTargetExists) {
		t.Fatalf("SaveDocumentAs overwrite error = %v, want ErrTargetExists", err)
	}
	current, err := runtime.GetDocument(ctx, second.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument after rejected overwrite: %v", err)
	}
	if current.SourcePath != "" || !current.Dirty {
		t.Fatalf("rejected overwrite changed session metadata: %+v", current)
	}
}

func TestRuntimeSaveOperationsReturnFormatSnapshot(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "source.json")
	adapter := &testDocumentAdapter{format: FormatJSON, dirty: true, snapshot: []byte("saved bytes")}
	factory := &testFormatFactory{format: FormatJSON, openDoc: adapter}
	runtime, err := NewRuntime(ctx, RuntimeOptions{Factories: []FormatFactory{factory}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	info, snapshot, err := runtime.SaveDocumentWithSnapshot(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("SaveDocumentWithSnapshot: %v", err)
	}
	if string(snapshot) != "saved bytes" || info.Dirty {
		t.Fatalf("save returned snapshot %q and metadata %+v", snapshot, info)
	}
	newPath := filepath.Join(t.TempDir(), "copy.json")
	info, snapshot, err = runtime.SaveDocumentAsWithSnapshot(ctx, doc.DocumentID, newPath)
	if err != nil {
		t.Fatalf("SaveDocumentAsWithSnapshot: %v", err)
	}
	if string(snapshot) != "saved bytes" || info.SourcePath != canonicalPath(newPath) || info.Dirty {
		t.Fatalf("save as returned snapshot %q and metadata %+v", snapshot, info)
	}
}

func TestRuntimeMarkDocumentChangedAdvancesRevision(t *testing.T) {
	ctx := context.Background()
	adapter := &testDocumentAdapter{format: FormatJSON, dirty: true}
	factory := &testFormatFactory{format: FormatJSON, openDoc: adapter}
	runtime, err := NewRuntime(ctx, RuntimeOptions{Factories: []FormatFactory{factory}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.OpenDocument(ctx, filepath.Join(t.TempDir(), "revision.json"))
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	updated, err := runtime.MarkDocumentChanged(ctx, doc.DocumentID)
	if err != nil {
		t.Fatalf("MarkDocumentChanged: %v", err)
	}
	if updated.Revision != doc.Revision+1 {
		t.Fatalf("revision = %d, want %d", updated.Revision, doc.Revision+1)
	}
}

func TestDefaultFactoryCreatesAndReopensJSON(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, FormatJSON)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	target := filepath.Join(t.TempDir(), "report.json")
	saved, err := runtime.SaveDocumentAs(ctx, doc.DocumentID, target)
	if err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	if saved.Dirty || saved.SourcePath != canonicalPath(target) {
		t.Fatalf("unexpected save metadata: %+v", saved)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("saved JSON does not exist: %v", err)
	}
	if err := runtime.CloseDocument(ctx, doc.DocumentID); err != nil {
		t.Fatalf("CloseDocument: %v", err)
	}
	reopened, err := runtime.OpenDocument(ctx, target)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	if reopened.Format != FormatJSON || reopened.Dirty {
		t.Fatalf("unexpected reopened metadata: %+v", reopened)
	}
}

func TestRuntimeOwnsWorkspaceStoreLifecycle(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "workspace.db")
	runtime, err := NewRuntime(ctx, RuntimeOptions{WorkspaceDBPath: path})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	first, err := runtime.OpenWorkspace(path)
	if err != nil {
		t.Fatalf("OpenWorkspace: %v", err)
	}
	second, err := runtime.OpenWorkspace(path)
	if err != nil {
		t.Fatalf("OpenWorkspace second call: %v", err)
	}
	if first != second {
		t.Fatal("Runtime returned multiple workspace stores for one database path")
	}
	if err := runtime.Close(ctx); err != nil {
		t.Fatalf("Runtime.Close: %v", err)
	}
	if _, err := first.Load(); err == nil {
		t.Fatal("workspace store remained open after Runtime.Close")
	}
	if _, err := runtime.OpenWorkspace(path); !errors.Is(err, ErrRuntimeClosed) {
		t.Fatalf("OpenWorkspace after close = %v, want ErrRuntimeClosed", err)
	}
}

func TestRuntimeAttachesExistingLiveSessionAndKeepsRevisionMonotonic(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "live.json")
	adapter := &testDocumentAdapter{format: FormatJSON, path: path, dirty: true}
	factory := &testFormatFactory{format: FormatJSON, openDoc: &testDocumentAdapter{format: FormatJSON}}
	runtime, err := NewRuntime(ctx, RuntimeOptions{Factories: []FormatFactory{factory}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	attached, err := runtime.RegisterLiveSession(ctx, "shared-live-doc", 7, adapter)
	if err != nil {
		t.Fatalf("RegisterLiveSession: %v", err)
	}
	opened, err := runtime.OpenDocument(ctx, path)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	if opened.DocumentID != attached.DocumentID {
		t.Fatalf("OpenDocument returned %q, want attached ID %q", opened.DocumentID, attached.DocumentID)
	}
	if factory.opened != 0 {
		t.Fatalf("OpenDocument opened %d new adapters instead of reusing the live session", factory.opened)
	}
	if err := runtime.SyncDocumentRevision(ctx, attached.DocumentID, 5); err != nil {
		t.Fatalf("SyncDocumentRevision backwards: %v", err)
	}
	unchanged, err := runtime.GetDocument(ctx, attached.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if unchanged.Revision != 7 {
		t.Fatalf("revision moved backwards to %d", unchanged.Revision)
	}
	if err := runtime.SyncDocumentRevision(ctx, attached.DocumentID, 9); err != nil {
		t.Fatalf("SyncDocumentRevision forwards: %v", err)
	}
	updated, err := runtime.GetDocument(ctx, attached.DocumentID)
	if err != nil {
		t.Fatalf("GetDocument after revision sync: %v", err)
	}
	if updated.Revision != 9 || !updated.Dirty {
		t.Fatalf("unexpected synced live session: %+v", updated)
	}
}
