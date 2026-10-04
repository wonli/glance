package desktopui

import (
	"path/filepath"
	"testing"

	"glance/internal/format/docx"
	"glance/internal/workspace"
)

func TestRuntimeReusesLogicalSessionAcrossWindows(t *testing.T) {
	runtime := newDesktopRuntime()
	shared := newSharedDocumentSession("/tmp/shared.docx")
	shared.documentID = "doc-1"
	shared.word, _ = docx.NewBlank()
	if shared.word == nil {
		t.Fatal("failed to create the shared Word adapter")
	}
	defer shared.word.Close()
	if got := runtime.registerSession(shared); got != shared {
		t.Fatal("first registration did not return the supplied session")
	}

	firstView := newDocumentSessionView(shared, 1)
	secondView := newDocumentSessionView(shared, 2)
	first := &App{
		desktopRuntime:       runtime,
		documentSessionState: firstView,
		sessions:             map[string]*documentSessionState{"doc-1": firstView},
	}
	second := &App{
		desktopRuntime:       runtime,
		documentSessionState: secondView,
		sessions:             map[string]*documentSessionState{"doc-1": secondView},
	}
	runtime.attachWindow(1, first)
	runtime.attachWindow(2, second)

	if got, ok := runtime.session("doc-1"); !ok || got != shared {
		t.Fatalf("runtime session = %p, found=%v; want shared session %p", got, ok, shared)
	}
	if got := runtime.registerSession(newSharedDocumentSession("/tmp/shared.docx")); got != shared {
		t.Fatal("registering the same logical document created a second shared session")
	}
	if len(runtime.sessions) != 1 {
		t.Fatalf("registered document sessions = %d; want one", len(runtime.sessions))
	}
	views := runtime.viewsForSession("doc-1")
	if len(views) != 2 || views[0].documentSessionState == views[1].documentSessionState {
		t.Fatalf("runtime views = %#v; want two independent views", views)
	}
	for _, app := range views {
		if app.documentSessionState.sharedDocumentSession != shared {
			t.Fatal("window view does not use the registered shared session")
		}
	}
}

func TestRuntimeReusesLogicalSessionForOpenPath(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "existing.docx")
	if err := word.SaveAs(path); err != nil {
		_ = word.Close()
		t.Fatal(err)
	}
	_ = word.Close()

	runtime := newDesktopRuntime()
	first, err := runtime.openDocumentSession(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.word.Close()
	second, err := runtime.openDocumentSession(filepath.Join(filepath.Dir(path), ".", filepath.Base(path)))
	if err != nil {
		t.Fatal(err)
	}
	if second != first || second.word != first.word {
		t.Fatal("opening the same canonical path created another session or adapter")
	}
	if len(runtime.sessions) != 1 || len(runtime.sessionsByCanonicalPath) != 1 {
		t.Fatalf("runtime registry sizes = sessions %d / paths %d; want 1 / 1", len(runtime.sessions), len(runtime.sessionsByCanonicalPath))
	}
}

func TestRuntimeOwnsOneWorkspaceWriter(t *testing.T) {
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
	runtime.startWorkspaceWriter()
	writerDone := runtime.workspaceWriterDone
	runtime.startWorkspaceWriter()
	if runtime.workspaceWriterDone != writerDone {
		t.Fatal("starting the writer twice created another workspace writer")
	}
	first := &App{desktopRuntime: runtime}
	second := &App{desktopRuntime: runtime}
	if first.workspaceStore != second.workspaceStore || first.collection != second.collection || first.workspaceWriteJobs != second.workspaceWriteJobs || first.workspaceWriteResults != second.workspaceWriteResults || first.workspaceWriterDone != second.workspaceWriterDone {
		t.Fatal("windows do not share the runtime workspace store, collection, and writer")
	}
	if first.workspaceWriteJobs == nil || first.workspaceWriterDone == nil {
		t.Fatal("runtime did not start its workspace writer")
	}
	close(runtime.workspaceWriteJobs)
	<-runtime.workspaceWriterDone
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	headless := New(nil, "")
	if headless.desktopRuntime == nil || headless.workspaceStore != nil || headless.collection != nil {
		t.Fatal("New(nil, \"\") must have an isolated runtime without opening a workspace database")
	}
}
