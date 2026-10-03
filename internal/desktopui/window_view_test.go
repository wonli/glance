package desktopui

import (
	"path/filepath"
	"testing"

	"gioui.org/layout"
	"gioui.org/widget"
	"glance/internal/document"
	"glance/internal/format/docx"
	"glance/internal/workspace"
)

type windowViewFixture struct {
	runtime *desktopRuntime
	store   *workspace.Store
	apps    []*App
	states  []*documentSessionState
	shared  []*sharedDocumentSession
	ids     []string
}

func newWindowViewFixture(t *testing.T) *windowViewFixture {
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
	fixture := &windowViewFixture{runtime: runtime, store: store}
	for index, id := range []string{"doc-one", "doc-two"} {
		word, err := docx.NewBlank()
		if err != nil {
			t.Fatal(err)
		}
		word.SetDirty(false)
		shared := newSharedDocumentSession("/tmp/" + id + ".docx")
		shared.documentID = id
		shared.word = word
		shared.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
		runtime.registerSession(shared)
		meta := workspace.Tab{ID: id, Kind: "document", Title: id, DocumentID: id, SourcePath: shared.path, Position: index}
		if err := collection.AddTab(meta, "docx"); err != nil {
			t.Fatal(err)
		}
		fixture.shared = append(fixture.shared, shared)
		fixture.ids = append(fixture.ids, id)
	}
	for windowIndex := 0; windowIndex < 2; windowIndex++ {
		app := New(nil, "")
		app.desktopRuntime = runtime
		app.sessions = make(map[string]*documentSessionState)
		app.tabMeta = make(map[string]workspace.Tab)
		for _, record := range collection.Tabs() {
			app.tabMeta[record.Tab.ID] = record.Tab
		}
		state := newDocumentSessionView(fixture.shared[0], uint64(windowIndex+1))
		state.documentID = fixture.ids[0]
		app.documentSessionState = state
		app.sessions[fixture.ids[0]] = state
		app.activeTabID = fixture.ids[0]
		app.setActiveSessionID(fixture.ids[0])
		runtime.attachWindow(uint64(windowIndex+1), app)
		fixture.apps = append(fixture.apps, app)
		fixture.states = append(fixture.states, state)
	}
	t.Cleanup(func() {
		for _, shared := range fixture.shared {
			if shared.word != nil {
				_ = shared.word.Close()
			}
		}
		_ = store.Close()
	})
	return fixture
}

func firstEditableRunForWindowTest(t *testing.T, state *documentSessionState) document.WordTextRun {
	t.Helper()
	for _, paragraph := range allWordParagraphs(wordBlocksForState(state)) {
		for _, inline := range paragraph.Inlines {
			if inline.Text != nil && inline.Text.Editable {
				return *inline.Text
			}
		}
	}
	t.Fatal("blank Word document has no editable text run")
	return document.WordTextRun{}
}

func TestWindowViewSnapshotCopiesPresentationIntoFreshWidgets(t *testing.T) {
	fixture := newWindowViewFixture(t)
	app, state, shared := fixture.apps[0], fixture.states[0], fixture.shared[0]
	run := firstEditableRunForWindowTest(t, state)
	state.wordViewMode = wordViewPrintLayout
	state.wordPageFitMode = true
	state.wordMultiplePagesMode = true
	state.wordLayoutZoom = 1.25
	state.wordRulerHidden = true
	state.wordGridlinesVisible = true
	state.wordNavigationVisible = true
	state.wordNavigationSearchQuery = "chapter"
	state.wordToolbarTab = wordTabView
	state.selectedWordRunID = run.ID
	state.selectedWordParagraphID = allWordParagraphs(wordBlocksForState(state))[0].ID
	state.wordListState.Position.First = 3
	state.wordPageListState.Position.First = 2
	state.wordToolbarActionButtons["copy"] = new(widget.Clickable)
	state.wordNavigationButtons["heading"] = new(widget.Clickable)
	editor := wordRunEditor(state, run)
	editor.SetText("current draft")
	editor.SetCaret(4, 7)
	captureWordTextDraft(state, run.ID, editor.Text())

	snapshot := captureWindowView(app)
	view := newDocumentViewFromSnapshot(shared, 2, snapshot)
	if view.sharedDocumentSession != shared || view.wordViewMode != state.wordViewMode || view.wordPageFitMode != state.wordPageFitMode || view.wordMultiplePagesMode != state.wordMultiplePagesMode || view.wordLayoutZoom != state.wordLayoutZoom || !view.wordRulerHidden || !view.wordGridlinesVisible || !view.wordNavigationVisible || view.wordNavigationSearchQuery != "chapter" || view.wordToolbarTab != wordTabView {
		t.Fatalf("new view did not copy presentation state: %#v", view)
	}
	if view.selectedWordRunID != run.ID || view.selectedWordParagraphID != state.selectedWordParagraphID || view.wordListState.Position.First != 0 || view.wordPageListState.Position.First != 0 {
		t.Fatalf("new view did not copy selection and visible position: run=%q paragraph=%q list=%d page=%d", view.selectedWordRunID, view.selectedWordParagraphID, view.wordListState.Position.First, view.wordPageListState.Position.First)
	}
	if view.wordToolbarActionButtons["copy"] != nil || view.wordNavigationButtons["heading"] != nil || &view.wordListState == &state.wordListState || &view.wordPageListState == &state.wordPageListState {
		t.Fatal("new window reused Gio widgets from the source view")
	}
	viewEditor := view.wordRunEditors[run.ID]
	if viewEditor == nil || viewEditor == editor || viewEditor.Text() != "current draft" {
		t.Fatalf("new view editor = %p, text %q; want a fresh editor with the latest shared draft", viewEditor, func() string {
			if viewEditor == nil {
				return ""
			}
			return viewEditor.Text()
		}())
	}
	if start, end := viewEditor.Selection(); start != 4 || end != 7 {
		t.Fatalf("new view did not restore the caret selection: %d..%d", start, end)
	}
	viewEditor.SetCaret(1, 1)
	if start, end := editor.Selection(); start != 4 || end != 7 {
		t.Fatalf("new view changed the source caret: %d..%d", start, end)
	}
}

func TestWorkspaceTabsAreSharedButActiveTabIsPerWindow(t *testing.T) {
	fixture := newWindowViewFixture(t)
	first, second := fixture.apps[0], fixture.apps[1]
	first.activateSession(fixture.ids[0], fixture.states[0])
	secondState := newDocumentSessionView(fixture.shared[1], 2)
	secondState.documentID = fixture.ids[1]
	second.sessions[fixture.ids[1]] = secondState
	second.activateSession(fixture.ids[1], secondState)

	if first.activeTabID != fixture.ids[0] || second.activeTabID != fixture.ids[1] {
		t.Fatalf("active tabs = %q / %q; want independent tabs %q / %q", first.activeTabID, second.activeTabID, fixture.ids[0], fixture.ids[1])
	}
	if got := fixture.runtime.collection.ActiveTabID(); got != fixture.ids[0] {
		t.Fatalf("persisted active tab = %q; want primary window tab %q", got, fixture.ids[0])
	}
	secondState.wordListState.Position.First = 9
	second.persistViewState()
	second.persistActiveSession()
	if record, ok := fixture.runtime.collection.Get(fixture.ids[1]); !ok || len(record.ViewState) != 0 {
		t.Fatalf("secondary window persisted view state: found=%v state=%q", ok, func() []byte {
			if record == nil {
				return nil
			}
			return record.ViewState
		}())
	}
	if len(first.workspaceTabIDs()) != 2 || len(second.workspaceTabIDs()) != 2 {
		t.Fatalf("shared tab catalog sizes = %d / %d; want two in each window", len(first.workspaceTabIDs()), len(second.workspaceTabIDs()))
	}
}

func TestClosingDocumentTabDetachesEveryView(t *testing.T) {
	fixture := newWindowViewFixture(t)
	fixture.apps[0].closeWorkspaceTabNow(fixture.ids[0])
	if _, ok := fixture.runtime.session(fixture.ids[0]); ok {
		t.Fatal("closed logical tab remains registered in the runtime")
	}
	if _, ok := fixture.runtime.collection.Get(fixture.ids[0]); ok {
		t.Fatal("closed logical tab remains in the shared workspace catalog")
	}
	for index, app := range fixture.apps {
		app.reconcileWorkspaceTabs()
		if app.activeTabID == fixture.ids[0] || app.activeSessionID() == fixture.ids[0] || app.sessions[fixture.ids[0]] != nil {
			t.Errorf("window %d still points at the closed tab: active=%q session=%q state=%p", index+1, app.activeTabID, app.activeSessionID(), app.sessions[fixture.ids[0]])
		}
	}
}

func TestWindowEditorsRebaseFromSharedDraftWithoutMovingCaret(t *testing.T) {
	fixture := newWindowViewFixture(t)
	first, second := fixture.states[0], fixture.states[1]
	run := firstEditableRunForWindowTest(t, first)
	firstEditor := wordRunEditor(first, run)
	secondEditor := wordRunEditor(second, run)
	firstEditor.SetText("shared draft")
	secondEditor.SetText("before")
	secondEditor.SetCaret(2, 4)
	captureWordTextDraft(first, run.ID, firstEditor.Text())

	syncWindowViewFromSharedSession(layout.Context{}, second)
	if got := secondEditor.Text(); got != "shared draft" {
		t.Fatalf("sibling editor text = %q; want latest shared draft", got)
	}
	if start, end := secondEditor.Selection(); start != 2 || end != 4 {
		t.Fatalf("sibling editor selection moved during rebase: %d..%d", start, end)
	}
}

func TestWordNewWindowAttachesToLiveSessionDuringPendingWork(t *testing.T) {
	fixture := newWindowViewFixture(t)
	app, shared, state := fixture.apps[0], fixture.shared[0], fixture.states[0]
	run := firstEditableRunForWindowTest(t, state)
	captureWordTextDraft(state, run.ID, "pending shared draft")
	if _, ok := shared.beginOperation(sessionOperationMutation); !ok {
		t.Fatal("failed to mark shared session busy")
	}
	defer func() {
		token, ok := shared.currentOperationToken()
		if ok {
			shared.finishOperation(token)
		}
	}()
	beforeTabs, beforeSessions := len(fixture.runtime.collection.Tabs()), len(fixture.runtime.sessions)
	view := newDocumentViewFromSnapshot(shared, 3, captureWindowView(app))
	if view.sharedDocumentSession != shared || !view.formatRunning.Load() {
		t.Fatal("new view did not attach to the busy live session")
	}
	if editor := view.wordRunEditors[run.ID]; editor == nil || editor.Text() != "pending shared draft" {
		t.Fatalf("new view missed the current shared draft: %#v", editor)
	}
	if len(fixture.runtime.collection.Tabs()) != beforeTabs || len(fixture.runtime.sessions) != beforeSessions {
		t.Fatalf("attaching a view changed workspace/session counts: tabs %d/%d sessions %d/%d", len(fixture.runtime.collection.Tabs()), beforeTabs, len(fixture.runtime.sessions), beforeSessions)
	}
}
