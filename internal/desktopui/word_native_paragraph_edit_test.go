package desktopui

import (
	"image"

	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestWordEnterSplitsAtCaretAndFocusesFollowingParagraph(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	run := word.WordBlocks()[0].Paragraph.Inlines[0].Text
	if err := word.ReplaceTextRun(run.ID, "HelloWorld"); err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState("enter.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	app := &App{documentSessionState: state, theme: material.NewTheme(), wordCommandResults: make(chan wordCommandResult, 1)}
	router := new(input.Router)
	var ops op.Ops
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: metric, Constraints: layout.Constraints{Max: image.Pt(1100, 700)}}
		app.layoutWord(gtx)
		router.Frame(&ops)
	}
	frame()
	var bounds image.Rectangle
	var findRun func([]input.SemanticNode)
	findRun = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if node.Desc.Description == "正文文字 "+run.ID {
				bounds = node.Desc.Bounds
			}
			findRun(node.Children)
		}
	}
	findRun(router.AppendSemantics(nil))
	if bounds.Empty() {
		t.Fatal("editable text has no accessible hit bounds")
	}
	position := f32.Pt(float32(bounds.Min.X+bounds.Dx()/2), float32(bounds.Min.Y+bounds.Dy()/2))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: position})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: position})
	frame()
	editor := state.wordRunEditors[run.ID]
	if editor == nil || !router.Source().Focused(editor) {
		t.Fatal("text run did not receive focus before Enter")
	}
	editor.SetCaret(5, 5)
	frame()
	router.Queue(key.Event{Name: key.NameReturn, State: key.Press})
	router.Queue(key.EditEvent{Text: "X"})
	frame()
	if editor.Text() != "HelloWorld" {
		t.Fatalf("input after Enter modified the old paragraph while its split was pending: %q", editor.Text())
	}

	waitWordInteraction(t, app, func() bool { return len(state.wordBlocks) >= 2 && state.selectedWordRunID != run.ID })
	blocks := word.WordBlocks()
	if len(blocks) != 2 || blocks[0].Paragraph == nil || blocks[1].Paragraph == nil || blocks[0].Paragraph.Inlines[0].Text.Text != "Hello" || blocks[1].Paragraph.Inlines[0].Text.Text != "World" {
		t.Fatalf("Enter at character five produced body blocks %#v; want paragraphs Hello / World", blocks)
	}
	newRunID := blocks[1].Paragraph.Inlines[0].Text.ID
	frame()
	newEditor := state.wordRunEditors[newRunID]
	if state.selectedWordParagraphID != blocks[1].Paragraph.ID || state.selectedWordRunID != newRunID || newEditor == nil || !router.Source().Focused(newEditor) {
		t.Fatalf("following paragraph did not receive editing focus: paragraph=%q run=%q editor=%p focused=%v", state.selectedWordParagraphID, state.selectedWordRunID, newEditor, newEditor != nil && router.Source().Focused(newEditor))
	}
	if _, col := newEditor.CaretPos(); col != 0 {
		t.Fatalf("caret in following paragraph = %d; want 0", col)
	}
	router.Queue(key.EditEvent{Text: "Y"})
	frame()
	if newEditor.Text() != "YWorld" || state.wordRunDrafts[newRunID] != "YWorld" {
		t.Fatalf("typing after the split did not continue in the new paragraph: editor=%q draft=%q", newEditor.Text(), state.wordRunDrafts[newRunID])
	}
}

func TestWordBackspaceAtStartAndDeleteAtEndJoinParagraphs(t *testing.T) {
	for _, test := range []struct {
		name    string
		keyName key.Name
		target  int
		caret   int
	}{
		{name: "backspace at start", keyName: key.NameDeleteBackward, target: 1, caret: 0},
		{name: "delete at end", keyName: key.NameDeleteForward, target: 0, caret: 4},
	} {
		t.Run(test.name, func(t *testing.T) {
			word, state, app, router, frame := newWordJoinInteraction(t, "left", "right")
			defer word.Close()
			run := state.wordBlocks[test.target].Paragraph.Inlines[0].Text
			editor := focusWordRunForInteraction(t, state, router, frame, run.ID)
			editor.SetCaret(test.caret, test.caret)
			frame()
			router.Queue(key.Event{Name: test.keyName, State: key.Press})
			frame()
			waitWordInteraction(t, app, func() bool { return len(state.wordBlocks) == 1 && !state.formatRunning.Load() })
			paragraph := state.wordBlocks[0].Paragraph
			if paragraph == nil || len(paragraph.Inlines) != 2 || paragraph.Inlines[0].Text.Text != "left" || paragraph.Inlines[1].Text.Text != "right" {
				t.Fatalf("joined paragraphs = %#v; want left/right in one paragraph", state.wordBlocks)
			}
			if state.selectedWordParagraphID != paragraph.ID || state.selectedWordRunID != paragraph.Inlines[0].Text.ID {
				t.Fatalf("join selection = %q/%q; want seam in left run %q", state.selectedWordParagraphID, state.selectedWordRunID, paragraph.Inlines[0].Text.ID)
			}
			frame()
			joinedEditor := state.wordRunEditors[state.selectedWordRunID]
			if joinedEditor == nil || !router.Source().Focused(joinedEditor) {
				t.Fatal("editor at the paragraph seam did not regain focus")
			}
			if caret, selection := joinedEditor.Selection(); caret != 4 || selection != 4 {
				t.Fatalf("caret after paragraph join = %d:%d; want 4:4", caret, selection)
			}
		})
	}
}

func TestWordQueuedRightArrowBeforeBackspaceDeletesInParagraph(t *testing.T) {
	word, state, _, router, frame := newWordJoinInteraction(t, "left", "right")
	defer word.Close()
	run := state.wordBlocks[1].Paragraph.Inlines[0].Text
	editor := focusWordRunForInteraction(t, state, router, frame, run.ID)
	editor.SetCaret(0, 0)
	frame()
	router.Queue(key.Event{Name: key.NameRightArrow, State: key.Press})
	router.Queue(key.Event{Name: key.NameDeleteBackward, State: key.Press})
	frame()
	if len(state.wordBlocks) != 2 || editor.Text() != "ight" || state.wordRunDrafts[run.ID] != "ight" {
		t.Fatalf("queued Right then Backspace produced %q, %d paragraphs, draft %q; want ordinary deletion to leave two paragraphs and \"ight\"", editor.Text(), len(state.wordBlocks), state.wordRunDrafts[run.ID])
	}
}

func TestWordParagraphJoinCompletionUpdatesInactiveSession(t *testing.T) {
	word, state, _, _, _ := newWordJoinInteraction(t, "left", "right")
	defer word.Close()
	run := state.wordBlocks[1].Paragraph.Inlines[0].Text
	joined, err := any(word).(document.WordParagraphJoiner).JoinParagraphAtBoundary(run.ID, document.WordJoinPrevious)
	if err != nil {
		t.Fatal(err)
	}
	state.wordParagraphJoinIntent = &wordParagraphJoinIntent{runID: run.ID, direction: document.WordJoinPrevious}
	state.wordFlushRequested.Store(true)
	state.wordFlushAllDrafts.Store(true)
	state.formatRunning.Store(true)
	blocks := cloneWordBlockSnapshot(word.WordBlocks())
	otherState := newDocumentSessionState("other.docx")
	app := &App{
		documentSessionState: otherState,
		documentGeneration:   2,
		wordCommandResults:   make(chan wordCommandResult, 1),
	}
	app.wordCommandResults <- wordCommandResult{
		state:           state,
		generation:      1,
		operation:       "合并段落",
		blocks:          blocks,
		paragraphJoin:   true,
		joinParagraphID: joined.ParagraphID,
		joinRunID:       joined.RunID,
		joinCaret:       joined.Caret,
	}
	app.processAsyncResults()
	if state.formatRunning.Load() || state.wordParagraphJoinIntent != nil || state.wordFlushRequested.Load() || state.wordFlushAllDrafts.Load() {
		t.Fatalf("inactive session join remained pending: running=%v intent=%#v flush=%v/%v", state.formatRunning.Load(), state.wordParagraphJoinIntent, state.wordFlushRequested.Load(), state.wordFlushAllDrafts.Load())
	}
	if len(state.wordBlocks) != 1 || state.wordBlocks[0].Paragraph == nil || state.wordBlocks[0].Paragraph.ID != joined.ParagraphID {
		t.Fatalf("inactive session projection was not refreshed: %#v", state.wordBlocks)
	}
}

func TestWordParagraphJoinFlushesFocusedDraftBeforeJoining(t *testing.T) {
	word, state, app, router, frame := newWordJoinInteraction(t, "left", "right")
	defer word.Close()
	run := state.wordBlocks[1].Paragraph.Inlines[0].Text
	editor := focusWordRunForInteraction(t, state, router, frame, run.ID)
	editor.SetText("draft right")
	editor.SetCaret(0, 0)
	frame()
	if state.wordRunDrafts[run.ID] != "draft right" {
		t.Fatalf("focused editor draft = %q; want captured pending text", state.wordRunDrafts[run.ID])
	}
	router.Queue(key.Event{Name: key.NameDeleteBackward, State: key.Press})
	frame()
	waitWordInteraction(t, app, func() bool { return len(state.wordBlocks) == 1 && !state.formatRunning.Load() })
	blocks := word.WordBlocks()
	if len(blocks) != 1 || len(blocks[0].Paragraph.Inlines) != 2 || blocks[0].Paragraph.Inlines[1].Text.Text != "draft right" {
		t.Fatalf("join did not flush focused text draft first: %#v", blocks)
	}
}

func TestWordParagraphJoinLeavesOrdinaryDeleteGesturesAlone(t *testing.T) {
	tests := []struct {
		name      string
		caret     int
		selection int
		modifiers key.Modifiers
		keyName   key.Name
		want      string
	}{
		{name: "interior backspace", caret: 1, selection: 1, keyName: key.NameDeleteBackward, want: "ight"},
		{name: "selected text", caret: 0, selection: 2, keyName: key.NameDeleteBackward, want: "ght"},
		{name: "modified backspace", caret: 0, selection: 0, modifiers: key.ModShift, keyName: key.NameDeleteBackward, want: "right"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			word, state, app, router, frame := newWordJoinInteraction(t, "left", "right")
			defer word.Close()
			run := state.wordBlocks[1].Paragraph.Inlines[0].Text
			editor := focusWordRunForInteraction(t, state, router, frame, run.ID)
			editor.SetCaret(test.caret, test.selection)
			frame()
			router.Queue(key.Event{Name: test.keyName, Modifiers: test.modifiers, State: key.Press})
			frame()
			if test.modifiers == 0 && test.selection == test.caret {
				waitWordInteraction(t, app, func() bool { return state.wordRunDrafts[run.ID] == test.want })
			} else {
				frame()
			}
			if len(state.wordBlocks) != 2 || editor.Text() != test.want {
				t.Fatalf("ordinary delete produced %q and %d paragraphs; want %q and 2", editor.Text(), len(state.wordBlocks), test.want)
			}
		})
	}
}

func TestWordParagraphJoinDoesNothingWithoutAdjacentParagraph(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	run := word.WordBlocks()[0].Paragraph.Inlines[0].Text
	if err := word.ReplaceTextRun(run.ID, "only"); err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState("single.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	_, router, frame := newWordJoinFrame(t, state)
	editor := focusWordRunForInteraction(t, state, router, frame, run.ID)
	editor.SetCaret(0, 0)
	frame()
	router.Queue(key.Event{Name: key.NameDeleteBackward, State: key.Press})
	frame()
	if len(state.wordBlocks) != 1 || editor.Text() != "only" {
		t.Fatalf("boundary key with no previous paragraph changed document: %#v / %q", state.wordBlocks, editor.Text())
	}
	editor.SetCaret(editor.Len(), editor.Len())
	frame()
	router.Queue(key.Event{Name: key.NameDeleteForward, State: key.Press})
	frame()
	if len(state.wordBlocks) != 1 || editor.Text() != "only" {
		t.Fatalf("boundary key with no next paragraph changed document: %#v / %q", state.wordBlocks, editor.Text())
	}
}
