package desktopui

import (
	"image"
	"path/filepath"

	"strings"
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

func TestWordBlankEndClickInsertsAndFocusesEditableParagraph(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState("blank.docx")
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
	var clickAt image.Point
	var findTarget func([]input.SemanticNode)
	findTarget = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if node.Desc.Description == "单击此处继续输入" {
				clickAt = image.Pt((node.Desc.Bounds.Min.X+node.Desc.Bounds.Max.X)/2, (node.Desc.Bounds.Min.Y+node.Desc.Bounds.Max.Y)/2)
			}
			findTarget(node.Children)
		}
	}
	findTarget(router.AppendSemantics(nil))
	if clickAt == (image.Point{}) {
		t.Fatal("blank-body target has no accessible hit bounds")
	}
	position := f32.Pt(float32(clickAt.X), float32(clickAt.Y))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: position})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: position})
	frame()
	if history := state.wordAppendTarget.History(); len(history) == 0 {
		semanticID, semanticHit := router.SemanticAt(position)
		t.Fatalf("blank-body target did not receive click at %v (semantic=%d/%v)", clickAt, semanticID, semanticHit)
	}

	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("insert body paragraph: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		semanticID, semanticHit := router.SemanticAt(position)
		t.Fatalf("blank body click did not queue paragraph insertion: point=%v semantic=%d/%v insert=%v format=%v flush=%v history=%#v position=%#v", clickAt, semanticID, semanticHit, state.wordInsertionRunning, state.formatRunning.Load(), state.wordInsertAfterDraftFlush, state.wordAppendTarget.History(), state.wordListState.Position)
	}
	if len(state.wordBlocks) != 2 {
		t.Fatalf("body blocks after blank click = %d; want 2", len(state.wordBlocks))
	}
	paragraph := state.wordBlocks[1].Paragraph
	if paragraph == nil || len(paragraph.Inlines) != 1 || paragraph.Inlines[0].Text == nil {
		t.Fatalf("inserted paragraph projection = %#v", state.wordBlocks[1])
	}
	runID := paragraph.Inlines[0].Text.ID
	frame()
	editor := state.wordRunEditors[runID]
	if editor == nil || !router.Source().Focused(editor) {
		t.Fatalf("inserted paragraph editor was not focused: editor=%p focused=%v", editor, editor != nil && router.Source().Focused(editor))
	}
	editor.SetText("typed at blank body")
	frame()
	if state.wordRunDrafts[runID] != "typed at blank body" {
		t.Fatalf("in-place run draft = %q", state.wordRunDrafts[runID])
	}
	state.wordForceDraftFlush.Store(true)
	state.wordFlushRequested.Store(true)
	state.wordFlushAllDrafts.Store(true)
	frame()
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("commit in-place paragraph text: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("text draft did not commit after focus loss")
	}
	blocks := word.WordBlocks()
	if got := blocks[1].Paragraph.Inlines[0].Text.Text; got != "typed at blank body" {
		t.Fatalf("inserted paragraph text = %q", got)
	}
	path := filepath.Join(t.TempDir(), "blank-edited.docx")
	if err := word.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	reopened, err := docx.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got := reopened.WordBlocks()[1].Paragraph.Inlines[0].Text.Text; got != "typed at blank body" {
		t.Fatalf("saved blank paragraph text = %q", got)
	}
}

func TestWordClickTextFocusesRunAtCaretAndAcceptsTyping(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	run := paragraph.Inlines[0].Text
	const original = "Click this sentence to edit it in place."
	if err := word.ReplaceTextRun(run.ID, original); err != nil {
		t.Fatal(err)
	}
	bodyEditor, ok := any(word).(document.WordBodyEditor)
	if !ok {
		t.Fatal("DOCX adapter does not support body paragraph insertion")
	}
	if _, err := bodyEditor.InsertBodyParagraph(); err != nil {
		t.Fatal(err)
	}
	blocks := word.WordBlocks()
	secondRun := blocks[len(blocks)-1].Paragraph.Inlines[0].Text
	if err := word.ReplaceTextRun(secondRun.ID, "Move focus here"); err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState("in-place.docx")
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
	var textBounds image.Rectangle
	var findText func([]input.SemanticNode)
	textBounds = image.Rectangle{}
	findText = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if node.Desc.Description == "正文文字 "+run.ID {
				textBounds = node.Desc.Bounds
			}
			findText(node.Children)
		}
	}
	findText(router.AppendSemantics(nil))
	if textBounds.Empty() {
		t.Fatal("editable Word text has no accessible hit bounds")
	}
	clickAt := image.Pt(textBounds.Min.X+min(10, max(1, textBounds.Dx()-1)), textBounds.Min.Y+textBounds.Dy()/2)
	position := f32.Pt(float32(clickAt.X), float32(clickAt.Y))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: position})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: position})
	frame()
	editor := state.wordRunEditors[run.ID]
	if editor == nil || !router.Source().Focused(editor) {
		t.Fatalf("clicking Word text did not focus its in-place editor: editor=%p focused=%v bounds=%v", editor, editor != nil && router.Source().Focused(editor), textBounds)
	}
	_, col := editor.CaretPos()
	if col <= 0 || col >= len(original) {
		t.Fatalf("click did not place the caret inside the text: line/column=%d/%d bounds=%v", func() int { line, _ := editor.CaretPos(); return line }(), col, textBounds)
	}
	router.Queue(key.EditEvent{Text: "X"})
	frame()
	if got := editor.Text(); got == original || !strings.Contains(got, "X") {
		t.Fatalf("typing at the clicked caret did not edit text in place: %q", got)
	}
	if state.wordRunDrafts[run.ID] != editor.Text() {
		t.Fatalf("in-place edit was not captured as a saveable draft: editor=%q draft=%q", editor.Text(), state.wordRunDrafts[run.ID])
	}
	textBounds = image.Rectangle{}
	findText = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if node.Desc.Description == "正文文字 "+secondRun.ID {
				textBounds = node.Desc.Bounds
			}
			findText(node.Children)
		}
	}
	findText(router.AppendSemantics(nil))
	if textBounds.Empty() {
		t.Fatal("second editable Word run has no accessible hit bounds")
	}
	secondPosition := f32.Pt(float32(textBounds.Min.X+textBounds.Dx()/2), float32(textBounds.Min.Y+textBounds.Dy()/2))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: secondPosition})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: secondPosition})
	frame()
	if !router.Source().Focused(state.wordRunEditors[secondRun.ID]) {
		t.Fatal("clicking a second Word run did not move editor focus")
	}
	deadline := time.After(3 * time.Second)
	for wordHasPendingTextDrafts(state) || state.formatRunning.Load() {
		select {
		case result := <-app.wordCommandResults:
			app.wordCommandResults <- result
			app.processAsyncResults()
		case <-deadline:
			t.Fatal("leaving the edited run did not commit its text draft")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	updated, exists := selectedWordRun(word.WordBlocks(), run.ID)
	if !exists || updated.Text != editor.Text() {
		t.Fatalf("blurred run text = %#v, exists=%v; want editor text %q", updated, exists, editor.Text())
	}
}

func TestWordSaveFlushesFocusedInPlaceDraftBeforeWriting(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "save-draft.docx")
	if err := word.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	_ = word.Close()
	word, err = docx.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState(path)
	state.word = word
	run := word.WordBlocks()[0].Paragraph.Inlines[0].Text
	editor := wordRunEditor(state, *run)
	const edited = "saved from the focused editor"
	editor.SetText(edited)
	captureWordTextDraft(state, run.ID, edited)
	state.wordFocusedRunID = run.ID
	app := &App{
		documentSessionState: state,
		wordCommandResults:   make(chan wordCommandResult, 1),
		saveResults:          make(chan saveResult, 1),
	}
	app.word, app.path = word, path
	app.startSave()
	deadline := time.After(3 * time.Second)
	for {
		caseResult := false
		select {
		case result := <-app.wordCommandResults:
			app.wordCommandResults <- result
			app.processAsyncResults()
			caseResult = true
		case result := <-app.saveResults:
			if result.err != nil {
				t.Fatalf("save after in-place text flush: %v", result.err)
			}
			goto saved
		case <-deadline:
			t.Fatal("save did not finish after committing the focused text draft")
		default:
		}
		if !caseResult {
			time.Sleep(time.Millisecond)
		}
	}

saved:
	reopened, err := docx.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got := reopened.WordBlocks()[0].Paragraph.Inlines[0].Text.Text; got != edited {
		t.Fatalf("saved Word text = %q; want %q", got, edited)
	}
}
