package desktopui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"glance/internal/application"
	"glance/internal/format/docx"
)

type wordNewWindowTestViewHandler struct{ app *App }

func (h wordNewWindowTestViewHandler) HandleViewIntent(_ context.Context, intent application.ViewIntent) (json.RawMessage, error) {
	return h.app.applyDesktopViewIntent(intent)
}

func TestWordNewWindowActionOpensSharedDocumentView(t *testing.T) {
	fixture := newWindowViewFixture(t)
	app, state, shared := fixture.apps[0], fixture.states[0], fixture.shared[0]
	run := firstEditableRunForWindowTest(t, state)
	state.wordViewMode = wordViewPrintLayout
	state.wordLayoutZoom = 1.4
	state.wordNavigationVisible = true
	state.wordNavigationSearchQuery = "chapter"
	state.wordToolbarTab = wordTabView
	state.wordListState.Position.First = 0
	state.selectedWordRunID = run.ID
	state.selectedWordParagraphID = allWordParagraphs(wordBlocksForState(state))[0].ID
	editor := wordRunEditor(state, run)
	editor.SetText("pending text from the source view")
	editor.SetCaret(9, 16)
	captureWordTextDraft(state, run.ID, editor.Text())
	if _, ok := shared.beginOperation(sessionOperationMutation); !ok {
		t.Fatal("failed to mark shared session busy")
	}
	defer func() {
		if token, ok := shared.currentOperationToken(); ok {
			shared.finishOperation(token)
		}
	}()

	var launched bool
	var gotDocumentID string
	var gotSnapshot windowViewSnapshot
	launchRequested := make(chan struct{}, 1)
	fixture.runtime.windowLauncher = func(documentID string, snapshot windowViewSnapshot) error {
		launched = true
		gotDocumentID = documentID
		gotSnapshot = snapshot
		launchRequested <- struct{}{}
		return nil
	}
	app.ensureDesktopRuntime().applicationRuntime.RegisterViewIntentHandler(app.windowID, wordNewWindowTestViewHandler{app: app})
	if !app.wordToolbarActionEnabled(findWordViewTestAction(t, "view-new-window")) {
		t.Fatal("New Window should remain available while shared format work is pending")
	}
	beforeTabs, beforeSessions := len(fixture.runtime.collection.Tabs()), len(fixture.runtime.sessions)
	app.executeWordToolbarAction("view-new-window")
	select {
	case <-launchRequested:
	case <-time.After(2 * time.Second):
		t.Fatal("New Window capability did not reach the attached view")
	}
	if !launched || gotDocumentID != fixture.ids[0] {
		t.Fatalf("launch request = %v for document %q; want %q", launched, gotDocumentID, fixture.ids[0])
	}
	if gotSnapshot.wordViewMode != state.wordViewMode || gotSnapshot.wordLayoutZoom != state.wordLayoutZoom || !gotSnapshot.wordNavigationVisible || gotSnapshot.wordNavigationQuery != "chapter" || gotSnapshot.wordToolbarTab != wordTabView || gotSnapshot.selectedWordRunID != run.ID || gotSnapshot.selectedWordSelectionStart != 9 || gotSnapshot.selectedWordSelectionEnd != 16 {
		t.Fatalf("new-window request did not copy the active Word view: %#v", gotSnapshot)
	}
	if app.documentSessionState != state || app.activeTabID != fixture.ids[0] || state.closed.Load() {
		t.Fatal("New Window changed or detached the source view")
	}
	if len(fixture.runtime.collection.Tabs()) != beforeTabs || len(fixture.runtime.sessions) != beforeSessions {
		t.Fatalf("New Window duplicated workspace/session state: tabs %d/%d sessions %d/%d", len(fixture.runtime.collection.Tabs()), beforeTabs, len(fixture.runtime.sessions), beforeSessions)
	}
}

func TestWordNewWindowActionDisabledWithoutWordRuntime(t *testing.T) {
	app := New(nil, "")
	state := newDocumentSessionState("")
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	app.documentSessionState = state
	app.desktopRuntime = nil
	enabled, reason := wordNewWindowAvailability(state, nil)
	if enabled || strings.TrimSpace(reason) == "" || app.wordToolbarActionEnabled(findWordViewTestAction(t, "view-new-window")) {
		t.Fatal("New Window must be disabled when the App has no attached runtime")
	}
	before := app.status
	app.executeWordToolbarAction("view-new-window")
	if strings.TrimSpace(app.status) == "" || app.status == before {
		t.Fatal("disabled New Window did not explain why it is unavailable")
	}
}
