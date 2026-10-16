package desktopui

import (
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"

	"glance/internal/document"
)

func TestWordTypingReplacesCrossRunSelection(t *testing.T) {
	app, state, router, frame := newWordSelectionPageTest(t, `<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>甲乙</w:t></w:r><w:r><w:rPr><w:i/></w:rPr><w:t>丙丁</w:t></w:r></w:p>`)
	p := state.wordBlocks[0].Paragraph
	first, last := p.Inlines[0].Text, p.Inlines[1].Text
	point := state.wordPageVisibleInteractions[0].Transform.PageToScreen(f32.Pt(40, 30))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: point, Time: time.Second})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point, Time: time.Second + time.Millisecond})
	frame()
	editor := state.wordRunEditors[last.ID]
	if editor == nil || !router.Source().Focused(editor) {
		t.Fatal("final selected run did not receive the editor focus")
	}
	state.wordPageTextSelection = &wordPageTextSelection{
		scope:    "document",
		anchor:   wordPageCaretPosition{runID: first.ID, runeOffset: 1},
		focus:    wordPageCaretPosition{runID: last.ID, runeOffset: 1},
		runOrder: map[string]int{first.ID: 0, last.ID: 1},
	}
	router.Queue(key.EditEvent{Range: key.Range{Start: 2, End: 2}, Text: "新🌱"})
	frame()
	waitWordReplaceCompletion(t, app, state)
	blocks := state.word.WordBlocks()
	gotStart, gotEnd := blocks[0].Paragraph.Inlines[0].Text.Text, blocks[0].Paragraph.Inlines[1].Text.Text
	if gotStart != "甲新🌱" || gotEnd != "丁" {
		t.Fatalf("typing over cross-run selection left start/end text %q/%q; want %q/%q", gotStart, gotEnd, "甲新🌱", "丁")
	}
	if !blocks[0].Paragraph.Inlines[0].Text.Style.Bold || !blocks[0].Paragraph.Inlines[1].Text.Style.Italic {
		t.Fatal("selection replacement lost original run formatting")
	}
	if text, ok := wordPageSelectedText(state); ok || text != "" {
		t.Fatalf("replacement kept selected text active: %q, %v", text, ok)
	}
}

func TestWordFindSelectsAndReplacesCrossRunMatchInOneUndo(t *testing.T) {
	app, state, _, _ := newWordSelectionPageTest(t, `<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>Be</w:t></w:r><w:r><w:rPr><w:i/></w:rPr><w:t>ta</w:t></w:r></w:p>`)
	app.showWordReplaceDialog()
	app.modal.Input.SetText("beta")
	app.modal.SecondInput.SetText("GAMMA")
	app.modal.OnAction("find-next")
	if selected, ok := wordPageSelectedText(state); !ok || selected != "Beta" {
		t.Fatalf("cross-run find selection = %q, %v; want Beta", selected, ok)
	}
	app.modal.OnAction("replace")
	waitWordReplaceCompletion(t, app, state)
	blocks := state.word.WordBlocks()
	first, second := blocks[0].Paragraph.Inlines[0].Text, blocks[0].Paragraph.Inlines[1].Text
	if first.Text != "GAMMA" || second.Text != "" {
		t.Fatalf("cross-run replacement left run text %q/%q; want GAMMA/empty", first.Text, second.Text)
	}
	if !first.Style.Bold || !second.Style.Italic {
		t.Fatal("cross-run replacement lost existing run formatting")
	}
	if applied, err := any(state.word).(document.WordHistory).Undo(); err != nil || !applied {
		t.Fatalf("undo cross-run replacement = %v, %v", applied, err)
	}
	blocks = state.word.WordBlocks()
	if first, second := blocks[0].Paragraph.Inlines[0].Text.Text, blocks[0].Paragraph.Inlines[1].Text.Text; first != "Be" || second != "ta" {
		t.Fatalf("undo restored text %q/%q; want Be/ta", first, second)
	}
}

func TestWordSelectAllThenTypingReplacesTheDocumentBody(t *testing.T) {
	app, state, router, frame := newWordSelectionPageTest(t, `<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>甲乙</w:t></w:r><w:r><w:t>丙丁</w:t></w:r></w:p>`)
	p := state.wordBlocks[0].Paragraph
	last := p.Inlines[1].Text
	point := state.wordPageVisibleInteractions[0].Transform.PageToScreen(f32.Pt(40, 30))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: point, Time: time.Second})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point, Time: time.Second + time.Millisecond})
	frame()
	router.Queue(key.Event{Name: key.Name("A"), State: key.Press, Modifiers: key.ModShortcut})
	frame()
	if !wordPageTextSelectionActive(state.wordPageTextSelection) {
		t.Fatal("Cmd/Ctrl+A did not select the document before replacement input")
	}
	lastEditor := state.wordRunEditors[last.ID]
	if lastEditor == nil || !router.Source().Focused(lastEditor) {
		t.Fatal("document-wide selection did not leave the final run ready for typing")
	}
	router.Queue(key.EditEvent{Range: key.Range{Start: 2, End: 2}, Text: "新文"})
	frame()
	waitWordReplaceCompletion(t, app, state)
	blocks := state.word.WordBlocks()
	if blocks[0].Paragraph.Inlines[0].Text.Text != "新文" || blocks[0].Paragraph.Inlines[1].Text.Text != "" {
		t.Fatalf("document replacement left runs %q/%q; want replacement only in the first formatted run", blocks[0].Paragraph.Inlines[0].Text.Text, blocks[0].Paragraph.Inlines[1].Text.Text)
	}
	if !blocks[0].Paragraph.Inlines[0].Text.Style.Bold {
		t.Fatal("document replacement did not inherit first-run formatting")
	}
}

func TestWordSelectionReplacementFailureKeepsTypedTextAsDraft(t *testing.T) {
	app, state, router, frame := newWordSelectionPageTest(t, `<w:p><w:r><w:t>ABC</w:t></w:r><w:r><w:tab/></w:r><w:r><w:t>DEF</w:t></w:r></w:p>`)
	p := state.wordBlocks[0].Paragraph
	first, last := p.Inlines[0].Text, p.Inlines[2].Text
	point := state.wordPageVisibleInteractions[0].Transform.PageToScreen(f32.Pt(60, 30))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: point, Time: time.Second})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point, Time: time.Second + time.Millisecond})
	frame()
	editor := state.wordRunEditors[last.ID]
	editor.SetCaret(3, 3)
	state.wordPageTextSelection = &wordPageTextSelection{
		scope:    "range",
		anchor:   wordPageCaretPosition{runID: first.ID, runeOffset: 1},
		focus:    wordPageCaretPosition{runID: last.ID, runeOffset: 2},
		runOrder: map[string]int{first.ID: 0, last.ID: 1},
	}
	router.Queue(key.EditEvent{Range: key.Range{Start: 3, End: 3}, Text: "新"})
	frame()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		app.processAsyncResults()
		if state.wordSelectionEditIntent == nil && !state.formatRunning.Load() && !state.wordFlushRequested.Load() {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if state.wordSelectionEditIntent != nil || state.formatRunning.Load() || state.wordFlushRequested.Load() {
		t.Fatal("rejected replacement left the Word editor locked")
	}
	if got := state.word.WordBlocks()[0].Paragraph.Inlines[0].Text.Text + state.word.WordBlocks()[0].Paragraph.Inlines[2].Text.Text; got != "ABCDEF" {
		t.Fatalf("unsafe replacement mutated document text: %q", got)
	}
	if draft, ok := state.wordDraft(last.ID); !ok || draft != "DEF新" {
		t.Fatalf("rejected replacement lost the typed draft: %q, %v", draft, ok)
	}
}

func TestWordTypingReplacesSelectionInFlowAndPrintLayout(t *testing.T) {
	word, state, app, router, frame := newWordJoinInteraction(t, "AB", "DEF")
	defer word.Close()
	paragraphs := allWordParagraphs(state.wordBlocks)
	first, last := paragraphs[0].Inlines[0].Text, paragraphs[1].Inlines[0].Text
	lastEditor := focusWordRunForInteraction(t, state, router, frame, last.ID)
	state.wordPageTextSelection = &wordPageTextSelection{
		scope:    "range",
		anchor:   wordPageCaretPosition{runID: first.ID, runeOffset: 1},
		focus:    wordPageCaretPosition{runID: last.ID, runeOffset: 2},
		runOrder: map[string]int{first.ID: 0, last.ID: 1},
	}
	router.Queue(key.EditEvent{Range: key.Range{Start: 3, End: 3}, Text: "X"})
	frame()
	waitWordReplaceCompletion(t, app, state)
	blocks := state.word.WordBlocks()
	if len(blocks) != 1 || blocks[0].Paragraph.Inlines[0].Text.Text != "AX" || blocks[0].Paragraph.Inlines[1].Text.Text != "F" {
		t.Fatalf("flow selection replacement blocks = %#v", blocks)
	}
	if start, end := lastEditor.Selection(); start != end {
		t.Fatalf("old flow editor selection after mutation = %d:%d", start, end)
	}
}

func TestWordTypingWaitsForCompositionToFinishBeforeReplacingSelection(t *testing.T) {
	app, state, router, frame := newWordSelectionPageTest(t, `<w:p><w:r><w:t>甲乙</w:t></w:r><w:r><w:t>丙丁</w:t></w:r></w:p>`)
	p := state.wordBlocks[0].Paragraph
	first, last := p.Inlines[0].Text, p.Inlines[1].Text
	point := state.wordPageVisibleInteractions[0].Transform.PageToScreen(f32.Pt(40, 30))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: point, Time: time.Second})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point, Time: time.Second + time.Millisecond})
	frame()
	editor := state.wordRunEditors[last.ID]
	state.wordPageTextSelection = &wordPageTextSelection{
		scope:    "document",
		anchor:   wordPageCaretPosition{runID: first.ID, runeOffset: 1},
		focus:    wordPageCaretPosition{runID: last.ID, runeOffset: 1},
		runOrder: map[string]int{first.ID: 0, last.ID: 1},
	}
	router.Queue(key.CompositionEvent{Start: 2, End: 3}, key.EditEvent{Range: key.Range{Start: 2, End: 2}, Text: "新"})
	frame()
	if !editorHasActiveComposition(editor) {
		t.Fatal("composition test did not activate the editor's IME range")
	}
	if got := state.word.WordBlocks()[0].Paragraph.Inlines[1].Text.Text; got != "丙丁" {
		t.Fatalf("intermediate composition changed committed text: %q", got)
	}
	if !wordPageTextSelectionActive(state.wordPageTextSelection) || state.wordSelectionEditIntent != nil {
		t.Fatal("intermediate composition replaced or cleared the document selection")
	}
	router.Queue(key.CompositionEvent{Start: -1, End: -1})
	frame()
	waitWordReplaceCompletion(t, app, state)
	blocks := state.word.WordBlocks()
	if blocks[0].Paragraph.Inlines[0].Text.Text != "甲新" || blocks[0].Paragraph.Inlines[1].Text.Text != "丁" {
		t.Fatalf("committed composition did not replace the selection: %#v", blocks[0].Paragraph.Inlines)
	}
}

func waitWordReplaceCompletion(t *testing.T, app *App, state *documentSessionState) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		app.processAsyncResults()
		if state.wordSelectionEditIntent == nil && !state.formatRunning.Load() && !state.wordFlushRequested.Load() && len(app.wordCommandResults) == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("Word selection replacement did not complete: intent=%+v formatRunning=%v flush=%v status=%q", state.wordSelectionEditIntent, state.formatRunning.Load(), state.wordFlushRequested.Load(), app.status)
}
