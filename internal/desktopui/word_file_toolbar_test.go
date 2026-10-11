package desktopui

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"glance/internal/application"
	"glance/internal/format/docx"
	"glance/internal/workspace"
)

type wordAutosaveViewIntentTestHandler struct {
	app      *App
	requests chan application.ViewIntent
}

func (h wordAutosaveViewIntentTestHandler) HandleViewIntent(_ context.Context, intent application.ViewIntent) (json.RawMessage, error) {
	h.requests <- intent
	return h.app.applyDesktopViewIntent(intent)
}

func newWordAutosaveTestApp(t *testing.T, path string, word *docx.Document) (*App, *documentSessionState) {
	t.Helper()
	runtime := newDesktopRuntime()
	runtime.workspaceStore = openWordHistoryTestStore(t)
	shared := newSharedDocumentSession(path)
	shared.documentID = "autosave-test-session"
	shared.word = word
	shared.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	runtime.registerSession(shared)
	state := newDocumentSessionView(shared, 1)
	state.documentID = shared.documentID
	state.word = word
	state.setWordBlocks(shared.wordBlocks)
	app := newAppWithRuntimeAndWindowID(nil, "", runtime, 1)
	app.documentSessionState = state
	app.sessions = map[string]*documentSessionState{shared.documentID: state}
	app.activeTabID = shared.documentID
	app.tabMeta = map[string]workspace.Tab{shared.documentID: {
		ID: shared.documentID, DocumentID: shared.documentID, Kind: "document",
		Title: filepath.Base(path), SourcePath: path,
	}}
	runtime.attachWindow(1, app)
	runtime.applicationRuntime.RegisterViewIntentHandler(1, wordAutosaveViewIntentTestHandler{app: app, requests: make(chan application.ViewIntent, 1)})
	t.Cleanup(func() { _ = runtime.applicationRuntime.Close(context.Background()) })
	return app, state
}

func TestWordAutosaveRibbonUsesMCPCapabilityHandler(t *testing.T) {
	fixture := newWindowViewFixture(t)
	app, state := fixture.apps[0], fixture.states[0]
	state.wordAutoSave = false
	requests := make(chan application.ViewIntent, 1)
	app.ensureDesktopRuntime().applicationRuntime.RegisterViewIntentHandler(app.windowID, wordAutosaveViewIntentTestHandler{app: app, requests: requests})

	app.executeWordToolbarAction("autosave")
	var intent application.ViewIntent
	select {
	case intent = <-requests:
	case <-time.After(2 * time.Second):
		t.Fatal("AutoSave Ribbon action did not use the registered application capability")
	}
	var arguments map[string]any
	if intent.Action != "word.autosave.set" || json.Unmarshal(intent.Arguments, &arguments) != nil || arguments["enabled"] != true {
		t.Fatalf("AutoSave capability intent = %+v", intent)
	}
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("AutoSave capability result: %v", result.err)
		}
		app.processWordCommandAsyncResult()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for AutoSave capability result")
	}
	if !state.wordAutoSave {
		t.Fatal("AutoSave capability did not update the active document view")
	}
}

type wordFileCommandTestInjector interface {
	setWordFileCommandRunner(func(fileCommandKind, string) fileCommandResult)
}

func findWordFileTestAction(t *testing.T, id string) ribbonAction {
	t.Helper()
	for _, action := range wordToolbarActions(wordTabFile) {
		if action.id == id {
			return action
		}
	}
	t.Fatalf("Word File action %q not found", id)
	return ribbonAction{}
}

func TestWordFileNewCreatesBlankDOCXAndRetainsCurrentTab(t *testing.T) {
	current, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer current.Close()
	current.SetDirty(false)
	state := newDocumentSessionState("current.docx")
	state.word = current
	state.wordBlocks = cloneWordBlockSnapshot(current.WordBlocks())
	app := &App{
		documentSessionState: state,
		sessions:             map[string]*documentSessionState{},
		tabMeta:              map[string]workspace.Tab{"current": {ID: "current", DocumentID: "current", Kind: "document", Title: "current.docx", SourcePath: "current.docx"}},
		activeTabID:          "current",
	}

	app.executeWordToolbarAction("new")

	if app.word == nil || app.word == current || len(app.word.WordBlocks()) == 0 {
		t.Fatal("File > New did not install a fresh editable DOCX")
	}
	if app.activeTabID == "current" || app.tabMeta[app.activeTabID].Kind != "draft" || app.tabMeta[app.activeTabID].Title != "新建文档" {
		t.Fatalf("new Word tab = id %q meta %#v; want a new DOCX draft tab", app.activeTabID, app.tabMeta[app.activeTabID])
	}
	if app.sessions["current"] != state || state.word != current {
		t.Fatal("creating a Word draft discarded the previously open document session")
	}
}

func TestWordFileRibbonOpenAndSaveAsUseWorkspacePickers(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	word.SetDirty(true)
	state := newDocumentSessionState("current.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	app := &App{
		documentSessionState: state,
		activeTabID:          "current",
		fileCommandResults:   make(chan fileCommandResult, 1),
		wordCommandResults:   make(chan wordCommandResult, 1),
		shutdown:             make(chan struct{}),
	}
	pickerCalls := make(chan struct {
		kind   fileCommandKind
		format string
	}, 2)
	injector, ok := any(app).(wordFileCommandTestInjector)
	if !ok {
		t.Fatal("Word ribbon file actions cannot be tested through the workspace picker boundary")
	}
	injector.setWordFileCommandRunner(func(kind fileCommandKind, format string) fileCommandResult {
		pickerCalls <- struct {
			kind   fileCommandKind
			format string
		}{kind: kind, format: format}
		return fileCommandResult{kind: kind}
	})
	apply := func(action string, wantKind fileCommandKind, wantFormat string, wantStatus string) {
		t.Helper()
		if !app.wordToolbarActionEnabled(findWordFileTestAction(t, action)) {
			t.Fatalf("Word File action %q should be enabled", action)
		}
		app.executeWordToolbarAction(action)
		select {
		case call := <-pickerCalls:
			if call.kind != wantKind || call.format != wantFormat {
				t.Fatalf("%s picker request = %#v; want kind=%v format=%q", action, call, wantKind, wantFormat)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("%s did not reach the workspace file picker", action)
		}
		select {
		case result := <-app.fileCommandResults:
			app.fileCommandResults <- result
		case <-time.After(3 * time.Second):
			t.Fatalf("%s picker did not finish", action)
		}
		app.processAsyncResults()
		if app.status != wantStatus {
			t.Fatalf("%s cancellation status = %q; want %q", action, app.status, wantStatus)
		}
	}
	apply("open", commandOpenFile, "", "已取消打开文件")
	if app.word != word || app.path != "current.docx" {
		t.Fatal("canceling Open changed the active Word document")
	}
	apply("save-as", commandSaveAs, "docx", "已取消另存为")
	if app.word != word || app.path != "current.docx" || !word.Dirty() {
		t.Fatal("canceling Save As changed the source Word document or its dirty state")
	}
}

func TestWordFileRibbonRecentReturnsToWorkspaceRecentView(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState("current.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	app := &App{
		documentSessionState: state,
		sessions:             map[string]*documentSessionState{},
		tabMeta:              map[string]workspace.Tab{"current": {ID: "current", DocumentID: "current", Kind: "document", Title: "current.docx", SourcePath: "current.docx"}},
		activeTabID:          "current",
	}
	if !app.wordToolbarActionEnabled(findWordFileTestAction(t, "recent")) {
		t.Fatal("Word File > Recent should be enabled")
	}

	app.executeWordToolbarAction("recent")

	if app.activeTabID != workspace.RecentTabID || app.word != nil || app.status != "最近打开的本地文档" {
		t.Fatalf("File > Recent state = tab %q, word %v, status %q", app.activeTabID, app.word, app.status)
	}
	if app.sessions["current"] != state || state.word != word {
		t.Fatal("File > Recent discarded the previously open Word document session")
	}
}

func TestWordFileAutosavePersistsToggleAndSavesCommittedEdits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "autosave.docx")
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	if err := word.SaveAsDocument(path); err != nil {
		t.Fatal(err)
	}
	word.SetDirty(false)
	paragraph := word.WordBlocks()[0].Paragraph
	runID := paragraph.Inlines[0].Text.ID
	app, state := newWordAutosaveTestApp(t, path, word)
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = runID
	app.saveResults = make(chan saveResult, 1)
	autosave := findWordFileTestAction(t, "autosave")
	if !app.wordToolbarActionEnabled(autosave) {
		t.Fatal("Word File > AutoSave should be available for an active DOCX")
	}
	app.executeWordToolbarAction("autosave")
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("enable AutoSave: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the AutoSave capability")
	}
	if !state.wordAutoSave || !wordToolbarActionActive(state, autosave) {
		t.Fatal("AutoSave toggle did not become active")
	}

	app.executeWordToolbarAction("bold")
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatal(result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the committed Word edit")
	}
	if !app.saving {
		t.Fatal("a successful edit did not start background AutoSave")
	}
	select {
	case result := <-app.saveResults:
		if result.err != nil {
			t.Fatal(result.err)
		}
		app.saveResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the AutoSave result")
	}
	if word.Dirty() || app.saving || app.status != "已保存 "+path {
		t.Fatalf("AutoSave completion: dirty=%v saving=%v status=%q", word.Dirty(), app.saving, app.status)
	}
	reopened, err := docx.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !reopened.WordBlocks()[0].Paragraph.Inlines[0].Text.Style.Bold {
		t.Fatal("the AutoSaved DOCX did not retain the committed edit after reopening")
	}
	payload, err := word.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := app.restoreSessionState(workspace.Session{
		Tab:       workspace.Tab{ID: "current", Kind: "document", Title: filepath.Base(path), SourcePath: path},
		Format:    "docx",
		Payload:   payload,
		ViewState: app.viewStateBytes(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.word.Close()
	if !restored.wordAutoSave {
		t.Fatal("workspace recovery did not restore the AutoSave toggle")
	}
}

func TestWordFileAutosaveOnDraftWaitsForSaveAs(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	app, state := newWordAutosaveTestApp(t, "", word)
	app.executeWordToolbarAction("autosave")
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("enable AutoSave on draft: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for draft AutoSave capability")
	}
	if !state.wordAutoSave || app.status != "自动保存将在文档另存为后启用" {
		t.Fatalf("draft AutoSave state: enabled=%v status=%q", state.wordAutoSave, app.status)
	}
	if app.saving {
		t.Fatal("an unsaved draft attempted to write to a nonexistent source path")
	}
}
