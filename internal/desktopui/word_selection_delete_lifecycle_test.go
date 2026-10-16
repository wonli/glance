package desktopui

import (
	"errors"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
)

func pendingWordSelectionDelete(t *testing.T) (*App, *documentSessionState, string) {
	t.Helper()
	app, state, _, _ := newWordSelectionPageTest(t, `<w:p><w:r><w:t>ABC</w:t></w:r><w:r><w:t>DEF</w:t></w:r></w:p>`)
	p := state.wordBlocks[0].Paragraph
	first, last := p.Inlines[0].Text, p.Inlines[1].Text
	wordRunEditor(state, *first)
	wordRunEditor(state, *last)
	state.wordSelectionDeleteIntent = &wordSelectionDeleteIntent{start: wordPageCaretPosition{runID: first.ID, runeOffset: 1}, end: wordPageCaretPosition{runID: last.ID, runeOffset: 2}}
	state.wordFlushRequested.Store(true)
	state.wordFlushAllDrafts.Store(true)
	state.wordForceDraftFlush.Store(true)
	return app, state, first.ID
}

func TestWordSelectionDeleteInactiveDraftFailureCancelsIntent(t *testing.T) {
	app, state, id := pendingWordSelectionDelete(t)
	captureWordTextDraft(state, id, "ABC新")
	app.documentSessionState = newDocumentSessionState("other.docx")
	app.documentGeneration++
	app.wordCommandResults <- wordCommandResult{state: state, textDraft: true, runID: id, runText: "ABC新", err: errors.New("synthetic draft failure")}
	app.processWordCommandAsyncResult()
	if state.wordSelectionDeleteIntent != nil || state.wordFlushRequested.Load() || state.wordFlushAllDrafts.Load() || state.wordForceDraftFlush.Load() {
		t.Fatal("failed inactive flush kept deletion pending/read-only")
	}
	if value, ok := state.wordDraft(id); !ok || value != "ABC新" {
		t.Fatal("failed inactive flush discarded draft")
	}
	if app.documentSessionState.wordSelectionDeleteIntent != nil {
		t.Fatal("inactive failure touched current session")
	}
}

func TestWordSelectionDeleteInactiveDraftSuccessContinuesOwningSession(t *testing.T) {
	app, state, id := pendingWordSelectionDelete(t)
	captureWordTextDraft(state, id, "ABC新")
	if err := state.word.ReplaceTextRun(id, "ABC新"); err != nil {
		t.Fatal(err)
	}
	state.setWordBlocks(state.word.WordBlocks())
	state.wordSaveAfterDrafts = true
	app.documentSessionState = newDocumentSessionState("other.docx")
	app.documentGeneration++
	app.wordCommandResults <- wordCommandResult{state: state, textDraft: true, runID: id, runText: "ABC新", blocks: state.word.WordBlocks()}
	app.processWordCommandAsyncResult()
	select {
	case result := <-app.wordCommandResults:
		if !result.selectionDelete || result.state != state || result.err != nil {
			t.Fatalf("inactive flush result: %#v", result)
		}
		app.wordCommandResults <- result
		app.processWordCommandAsyncResult()
	case <-time.After(time.Second):
		t.Fatal("successful inactive draft flush did not continue deletion")
	}
	if state.wordSelectionDeleteIntent != nil || state.formatRunning.Load() || wordHasPendingTextDrafts(state) {
		t.Fatal("inactive deletion did not reconcile drafts and release operation")
	}
	p := state.word.WordBlocks()[0].Paragraph
	if p.Inlines[0].Text.Text != "A" || p.Inlines[1].Text.Text != "F" {
		t.Fatal("inactive deletion edited wrong text")
	}
	if !state.wordSaveAfterDrafts {
		t.Fatal("inactive deletion consumed App-dependent deferred save")
	}
}

func TestWordSelectionDeleteRejectedDispatchReleasesOperation(t *testing.T) {
	app, state, _ := pendingWordSelectionDelete(t)
	app.managedBackgroundClosed = true
	app.startWordSelectionDelete(state)
	if state.formatRunning.Load() || state.wordSelectionDeleteIntent != nil || state.wordFlushRequested.Load() || state.wordForceDraftFlush.Load() {
		t.Fatal("rejected worker dispatch leaked operation or pending intent")
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		t.Fatal("rejected deletion still owns operation")
	}
	state.finishOperation(token)
	if got := state.word.WordBlocks()[0].Paragraph.Inlines[0].Text.Text; got != "ABC" {
		t.Fatalf("rejected deletion mutated text: %q", got)
	}
}

func TestWordSelectionDeleteDrainsEarlierEditEvent(t *testing.T) {
	_, state, router, frame := newWordSelectionPageTest(t, `<w:p><w:r><w:t>ABC</w:t></w:r><w:r><w:t>DEF</w:t></w:r></w:p>`)
	point := state.wordPageVisibleInteractions[0].Transform.PageToScreen(f32.Pt(20, 30))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: point, Time: time.Second})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point, Time: time.Second + time.Millisecond})
	frame()
	p := state.wordBlocks[0].Paragraph
	first, last := p.Inlines[0].Text, p.Inlines[1].Text
	editor := state.wordRunEditors[first.ID]
	editor.SetCaret(1, 1)
	state.wordPageTextSelection = &wordPageTextSelection{anchor: wordPageCaretPosition{runID: first.ID, runeOffset: 1}, focus: wordPageCaretPosition{runID: last.ID, runeOffset: 2}, runOrder: map[string]int{first.ID: 0, last.ID: 1}, scope: "range"}
	router.Queue(key.EditEvent{Range: key.Range{Start: 1, End: 1}, Text: "新字"}, key.SelectionEvent{Start: 3, End: 3}, key.Event{Name: key.NameDeleteBackward, State: key.Press})
	frame()
	if got := editor.Text(); got != "A新BC" {
		t.Fatalf("queued edit + native Delete = %q; want A新BC", got)
	}
	if state.wordSelectionDeleteIntent != nil || state.wordPageTextSelection != nil {
		t.Fatal("queued edit applied stale cross-run deletion")
	}
	if value, _ := state.wordDraft(first.ID); value != "A新BC" {
		t.Fatalf("latest native edit draft = %q", value)
	}
	if got := state.word.WordBlocks()[0].Paragraph.Inlines[1].Text.Text; got != "DEF" {
		t.Fatalf("stale selection changed other run: %q", got)
	}
}

func TestWordSelectionDeletePreservesEarlierSubmitEvent(t *testing.T) {
	app, state, router, frame := newWordSelectionPageTest(t, `<w:p><w:r><w:t>ABC</w:t></w:r><w:r><w:t>DEF</w:t></w:r></w:p>`)
	point := state.wordPageVisibleInteractions[0].Transform.PageToScreen(f32.Pt(20, 30))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: point, Time: time.Second})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point, Time: time.Second + time.Millisecond})
	frame()
	p := state.wordBlocks[0].Paragraph
	first, last := p.Inlines[0].Text, p.Inlines[1].Text
	editor := state.wordRunEditors[first.ID]
	editor.SetCaret(1, 1)
	state.wordPageTextSelection = &wordPageTextSelection{anchor: wordPageCaretPosition{runID: first.ID, runeOffset: 1}, focus: wordPageCaretPosition{runID: last.ID, runeOffset: 2}, runOrder: map[string]int{first.ID: 0, last.ID: 1}, scope: "range"}
	router.Queue(key.Event{Name: key.NameReturn, State: key.Press}, key.Event{Name: key.NameDeleteBackward, State: key.Press})
	frame()
	if state.wordSelectionDeleteIntent != nil || state.wordParagraphSplitIntent == nil {
		t.Fatal("earlier Submit event was discarded or also queued selection deletion")
	}
	waitWordInteraction(t, app, func() bool { return state.wordParagraphSplitIntent == nil && !state.formatRunning.Load() })
	blocks := state.word.WordBlocks()
	if len(blocks) != 2 || blocks[0].Paragraph.Inlines[0].Text.Text != "A" || blocks[1].Paragraph.Inlines[0].Text.Text != "BC" {
		t.Fatal("queued Submit failed to split at original caret")
	}
}
