package desktopui

import (
	"encoding/json"
	"testing"
	"time"

	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestWordViewSetSelectsExplicitUnicodeRunRangeWithoutEditingDocument(t *testing.T) {
	app, state := newWordAutosaveTestApp(t, "/tmp/selection.docx", mustNewBlankWord(t))
	bodyEditor, ok := any(state.word).(document.WordBodyTextEditor)
	if !ok {
		t.Fatal("DOCX adapter does not support body text paragraphs")
	}
	_, runID, err := bodyEditor.AppendBodyTextParagraph("员工工资表")
	if err != nil {
		t.Fatalf("AppendBodyTextParagraph: %v", err)
	}
	state.setWordBlocks(state.word.WordBlocks())
	revision := state.wordRevisionAtomic.Load()
	dirty := state.word.Dirty()
	arguments := json.RawMessage(`{"selection":{"scope":"range","start_run_id":"` + runID + `","start_offset":2,"end_run_id":"` + runID + `","end_offset":5}}`)
	result, err := app.applyWordViewSettings(state, arguments)
	if err != nil {
		t.Fatalf("applyWordViewSettings(selection): %v", err)
	}
	selected, ok := wordPageSelectedText(state)
	if !ok || selected != "工资表" {
		t.Fatalf("selected text = %q, %v; want %q", selected, ok, "工资表")
	}
	var output struct {
		Selection wordSelectionSnapshotData `json:"selection"`
		Changed   bool                      `json:"changed"`
	}
	if err := json.Unmarshal(result, &output); err != nil {
		t.Fatalf("decode view result: %v", err)
	}
	if output.Selection.Scope != "range" || output.Selection.StartRunID != runID || output.Selection.StartOffset != 2 || output.Selection.EndOffset != 5 || output.Selection.SelectedRunes != 3 || !output.Changed {
		t.Fatalf("view selection result = %+v, changed=%v", output.Selection, output.Changed)
	}
	if state.wordRevisionAtomic.Load() != revision || state.word.Dirty() != dirty {
		t.Fatalf("selection changed document state: revision %d (was %d), dirty=%v (was %v)", state.wordRevisionAtomic.Load(), revision, state.word.Dirty(), dirty)
	}
}

func TestWordViewSetRejectsRangeOutsideCurrentRunWithoutChangingSelection(t *testing.T) {
	app, state := newWordAutosaveTestApp(t, "/tmp/selection-invalid.docx", mustNewBlankWord(t))
	bodyEditor := document.WordBodyTextEditor(state.word)
	_, runID, err := bodyEditor.AppendBodyTextParagraph("工资")
	if err != nil {
		t.Fatalf("AppendBodyTextParagraph: %v", err)
	}
	state.setWordBlocks(state.word.WordBlocks())
	if _, err := app.applyWordViewSettings(state, json.RawMessage(`{"selection":{"scope":"document"}}`)); err != nil {
		t.Fatalf("select document: %v", err)
	}
	previous := wordSelectionSnapshot(state)
	arguments := json.RawMessage(`{"selection":{"scope":"range","start_run_id":"` + runID + `","start_offset":0,"end_run_id":"` + runID + `","end_offset":99}}`)
	if _, err := app.applyWordViewSettings(state, arguments); err == nil {
		t.Fatal("expected out-of-range selection to be rejected")
	}
	if current := wordSelectionSnapshot(state); current != previous {
		t.Fatalf("invalid range changed selection: before=%+v after=%+v", previous, current)
	}
}

func TestWordSelectRibbonUsesRegisteredViewCapability(t *testing.T) {
	fixture := newWindowViewFixture(t)
	app, state := fixture.apps[0], fixture.states[0]
	requests := make(chan application.ViewIntent, 1)
	app.ensureDesktopRuntime().applicationRuntime.RegisterViewIntentHandler(app.windowID, wordAutosaveViewIntentTestHandler{app: app, requests: requests})
	if firstEditableRunForWindowTest(t, state).ID == "" {
		t.Fatal("blank Word document has no editable run")
	}
	app.executeWordToolbarAction("select")
	select {
	case intent := <-requests:
		var arguments struct {
			Selection struct {
				Scope string `json:"scope"`
			} `json:"selection"`
		}
		if intent.Action != "word.view.set" || json.Unmarshal(intent.Arguments, &arguments) != nil || arguments.Selection.Scope != "document" {
			t.Fatalf("Select Ribbon action did not route through word.view.set: %+v args=%s", intent, intent.Arguments)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Select Ribbon action did not invoke the registered capability")
	}
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("Select capability result: %v", result.err)
		}
		app.processWordCommandAsyncResult()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Select capability result")
	}
	if state.wordPageTextSelection == nil || state.wordPageTextSelection.scope != "document" || state.selectedWordRunID == "" {
		t.Fatalf("Select Ribbon action did not update view selection: %+v", state.wordPageTextSelection)
	}
}

func mustNewBlankWord(t *testing.T) *docx.Document {
	t.Helper()
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = word.Close() })
	return word
}
