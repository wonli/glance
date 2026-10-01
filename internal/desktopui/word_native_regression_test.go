package desktopui

import (
	"fmt"
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
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/format/docx"
)

func TestWordFlowCanReverseScrollImmediatelyFromBottom(t *testing.T) {
	state, closeDocument := newLongWordFlowState(t, 160)
	defer closeDocument()
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: metric, Constraints: layout.Constraints{Max: image.Pt(1100, 700)}}
		app.layoutWordFlow(gtx)
		router.Frame(&ops)
	}
	frame()

	router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(540, 320), Scroll: f32.Pt(0, 100000)})
	frame()
	if state.wordListState.Position.BeforeEnd {
		t.Fatalf("large downward scroll did not reach the end: %#v", state.wordListState.Position)
	}
	bottom := state.wordListState.Position

	router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(540, 320), Scroll: f32.Pt(0, -80)})
	frame()
	upward := state.wordListState.Position
	if upward.First == bottom.First && upward.Offset == bottom.Offset {
		t.Fatalf("one reverse wheel event did not move up from bottom: bottom=%#v after=%#v", bottom, upward)
	}
}

func TestWordFlowScrollsDownImmediatelyFromTop(t *testing.T) {
	state, closeDocument := newLongWordFlowState(t, 160)
	defer closeDocument()
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: metric, Constraints: layout.Constraints{Max: image.Pt(1100, 700)}}
		app.layoutWordFlow(gtx)
		router.Frame(&ops)
	}
	frame()
	state.wordListState.ScrollTo(0)
	frame()
	top := state.wordListState.Position

	router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(540, 320), Scroll: f32.Pt(0, 80)})
	frame()
	down := state.wordListState.Position
	if down.First == top.First && down.Offset == top.Offset {
		t.Fatalf("one downward wheel event did not move down from top: top=%#v after=%#v", top, down)
	}
}

func TestWordScrollbarTracksAtWorkspaceRightEdge(t *testing.T) {
	state, closeDocument := newLongWordFlowState(t, 160)
	defer closeDocument()
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(1200, 700)}}
	app.layoutWordFlow(gtx)
	router.Frame(&ops)
	var bounds, pageBounds image.Rectangle
	var visit func([]input.SemanticNode)
	visit = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if node.Desc.Description == "Word 文档滚动条" {
				bounds = node.Desc.Bounds
			}
			if node.Desc.Description == "Word 文档画布" {
				pageBounds = node.Desc.Bounds
			}
			visit(node.Children)
		}
	}
	visit(router.AppendSemantics(nil))
	if bounds.Empty() {
		t.Fatal("Word document scrollbar has no accessible bounds")
	}
	if bounds.Max.X < 1180 || bounds.Max.X > 1200 {
		t.Fatalf("Word scrollbar bounds = %v; want the right edge of the 1200px workspace", bounds)
	}
	if pageBounds.Empty() {
		t.Fatal("Word page canvas has no accessible bounds")
	}
	if center := (pageBounds.Min.X + pageBounds.Max.X) / 2; center < 590 || center > 610 {
		t.Fatalf("Word page canvas bounds = %v; want it centered in the 1200px workspace", pageBounds)
	}
}

func TestWordCanvasSideDragResizesCenteredCanvas(t *testing.T) {
	state, closeDocument := newLongWordFlowState(t, 12)
	defer closeDocument()
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: metric, Constraints: layout.Constraints{Max: image.Pt(1100, 700)}}
		app.layoutWordFlow(gtx)
		router.Frame(&ops)
	}
	canvasBounds := func() image.Rectangle {
		t.Helper()
		var bounds image.Rectangle
		var visit func([]input.SemanticNode)
		visit = func(nodes []input.SemanticNode) {
			for _, node := range nodes {
				if node.Desc.Description == "Word 文档画布" {
					bounds = node.Desc.Bounds
				}
				visit(node.Children)
			}
		}
		visit(router.AppendSemantics(nil))
		if bounds.Empty() {
			t.Fatal("Word page canvas has no accessible bounds")
		}
		return bounds
	}
	frame()
	initial := canvasBounds()
	startWidth := state.wordCanvasWidthDP
	start := f32.Pt(float32(initial.Max.X), float32(initial.Min.Y+100))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 5, Buttons: pointer.ButtonPrimary, Position: start})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 5, Buttons: pointer.ButtonPrimary, Position: start.Add(f32.Pt(40, 0))})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 5, Position: start.Add(f32.Pt(40, 0))})
	frame()
	if state.wordCanvasWidthDP <= startWidth {
		t.Fatalf("Word canvas width after dragging right edge = %d dp; want more than %d dp", state.wordCanvasWidthDP, startWidth)
	}
	resized := canvasBounds()
	if center := (resized.Min.X + resized.Max.X) / 2; center < 540 || center > 560 {
		t.Fatalf("resized Word canvas bounds = %v; want centered in the 1100px workspace", resized)
	}
}

func TestWordParagraphEditorUsesCanvasWidthBeforeCommit(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	run := word.WordBlocks()[0].Paragraph.Inlines[0].Text
	if err := word.ReplaceTextRun(run.ID, "短"); err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState("draft-width.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: metric, Constraints: layout.Constraints{Max: image.Pt(1200, 700)}}
		app.layoutWordFlow(gtx)
		router.Frame(&ops)
	}
	readRunBounds := func() image.Rectangle {
		t.Helper()
		var bounds image.Rectangle
		var visit func([]input.SemanticNode)
		visit = func(nodes []input.SemanticNode) {
			for _, node := range nodes {
				if node.Desc.Description == "正文文字 "+run.ID {
					bounds = node.Desc.Bounds
				}
				visit(node.Children)
			}
		}
		visit(router.AppendSemantics(nil))
		if bounds.Empty() {
			t.Fatalf("Word text run %q has no accessible bounds", run.ID)
		}
		return bounds
	}
	frame()
	initial := readRunBounds()
	if initial.Dx() < 500 {
		t.Fatalf("Word paragraph editor width = %d; want it to follow the available page text width", initial.Dx())
	}
	wordRunEditor(state, *run).SetText(strings.Repeat("追加", 12))
	frame()
	afterTyping := readRunBounds()
	if afterTyping.Dx() != initial.Dx() {
		t.Fatalf("paragraph editor width changed from %d to %d while typing; it should stay tied to the page canvas", initial.Dx(), afterTyping.Dx())
	}
}

func TestWordInlineGeometrySharesLinesAndWraps(t *testing.T) {
	positions, height := wordInlineGeometry(70, []image.Point{{X: 30, Y: 20}, {X: 25, Y: 18}, {X: 30, Y: 24}}, document.WordAlignLeft)
	want := []image.Point{{X: 0, Y: 0}, {X: 30, Y: 0}, {X: 0, Y: 20}}
	if len(positions) != len(want) {
		t.Fatalf("inline positions = %#v; want %#v", positions, want)
	}
	for i := range want {
		if positions[i] != want[i] {
			t.Errorf("inline position %d = %v; want %v", i, positions[i], want[i])
		}
	}
	if height != 44 {
		t.Fatalf("wrapped paragraph height = %d; want 44", height)
	}
}

func TestWordInlineGeometryHonorsCenterAlignment(t *testing.T) {
	positions, height := wordInlineGeometry(100, []image.Point{{X: 20, Y: 18}, {X: 30, Y: 18}}, document.WordAlignCenter)
	if len(positions) != 2 || positions[0] != (image.Point{X: 25, Y: 0}) || positions[1] != (image.Point{X: 45, Y: 0}) || height != 18 {
		t.Fatalf("center-aligned inline flow = positions %v height %d", positions, height)
	}
}

func TestWordPageColumnCentersAndFitsNarrowWindows(t *testing.T) {
	wide, wideLeft := wordPageGeometry(1200, 816)
	if wide != 816 || wideLeft != 192 {
		t.Fatalf("wide page geometry = width %d left %d; want 816/192", wide, wideLeft)
	}
	narrow, narrowLeft := wordPageGeometry(600, 816)
	if narrow != 600 || narrowLeft != 0 {
		t.Fatalf("narrow page geometry = width %d left %d; want 600/0", narrow, narrowLeft)
	}
}

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
		t.Fatalf("blank body click did not queue paragraph insertion: point=%v semantic=%d/%v insert=%v format=%v flush=%v history=%#v position=%#v", clickAt, semanticID, semanticHit, state.wordInsertionRunning, state.formatRunning, state.wordInsertAfterDraftFlush, state.wordAppendTarget.History(), state.wordListState.Position)
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
	state.wordForceDraftFlush, state.wordFlushRequested, state.wordFlushAllDrafts = true, true, true
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
	for wordHasPendingTextDrafts(state) || state.formatRunning {
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

	deadline := time.After(3 * time.Second)
	for len(state.wordBlocks) < 2 {
		select {
		case result := <-app.wordCommandResults:
			app.wordCommandResults <- result
			app.processAsyncResults()
		case <-deadline:
			t.Fatalf("Enter did not split the paragraph before timeout (blocks=%d, running=%v)", len(state.wordBlocks), state.formatRunning)
		default:
			time.Sleep(time.Millisecond)
		}
	}
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
			waitWordInteraction(t, app, func() bool { return len(state.wordBlocks) == 1 && !state.formatRunning })
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
	state.wordFlushRequested = true
	state.wordFlushAllDrafts = true
	state.formatRunning = true
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
	if state.formatRunning || state.wordParagraphJoinIntent != nil || state.wordFlushRequested || state.wordFlushAllDrafts {
		t.Fatalf("inactive session join remained pending: running=%v intent=%#v flush=%v/%v", state.formatRunning, state.wordParagraphJoinIntent, state.wordFlushRequested, state.wordFlushAllDrafts)
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
	waitWordInteraction(t, app, func() bool { return len(state.wordBlocks) == 1 && !state.formatRunning })
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

func newWordJoinInteraction(t *testing.T, first, second string) (*docx.Document, *documentSessionState, *App, *input.Router, func()) {
	t.Helper()
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	firstRun := word.WordBlocks()[0].Paragraph.Inlines[0].Text
	if err := word.ReplaceTextRun(firstRun.ID, first); err != nil {
		t.Fatal(err)
	}
	paragraphID, err := any(word).(document.WordBodyEditor).InsertBodyParagraph()
	if err != nil {
		t.Fatal(err)
	}
	var secondRun *document.WordTextRun
	for _, block := range word.WordBlocks() {
		if block.Paragraph != nil && block.Paragraph.ID == paragraphID {
			secondRun = block.Paragraph.Inlines[0].Text
		}
	}
	if secondRun == nil {
		t.Fatal("new Word paragraph has no text run")
	}
	if err := word.ReplaceTextRun(secondRun.ID, second); err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState("join.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	app, router, frame := newWordJoinFrame(t, state)
	return word, state, app, router, frame
}

func newWordJoinFrame(t *testing.T, state *documentSessionState) (*App, *input.Router, func()) {
	t.Helper()
	app := &App{documentSessionState: state, theme: material.NewTheme(), wordCommandResults: make(chan wordCommandResult, 4)}
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
	return app, router, frame
}

func focusWordRunForInteraction(t *testing.T, state *documentSessionState, router *input.Router, frame func(), runID string) *widget.Editor {
	t.Helper()
	frame()
	var bounds image.Rectangle
	var findRun func([]input.SemanticNode)
	findRun = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if node.Desc.Description == "正文文字 "+runID {
				bounds = node.Desc.Bounds
			}
			findRun(node.Children)
		}
	}
	findRun(router.AppendSemantics(nil))
	if bounds.Empty() {
		t.Fatalf("Word text run %q has no accessible hit bounds", runID)
	}
	position := f32.Pt(float32(bounds.Min.X+bounds.Dx()/2), float32(bounds.Min.Y+bounds.Dy()/2))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: position})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: position})
	frame()
	editor := state.wordRunEditors[runID]
	if editor == nil || !router.Source().Focused(editor) {
		t.Fatalf("Word text run %q did not receive focus", runID)
	}
	return editor
}

func waitWordInteraction(t *testing.T, app *App, done func() bool) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for !done() {
		app.processAsyncResults()
		select {
		case <-deadline:
			t.Fatal("Word paragraph join did not complete before timeout")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func newLongWordFlowState(t *testing.T, paragraphs int) (*documentSessionState, func()) {
	t.Helper()
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	blocks := make([]document.WordBlock, paragraphs)
	for i := range blocks {
		id := fmt.Sprintf("p%d", i)
		paragraph := document.WordParagraph{ID: id, Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: id + ".r0", Text: strings.Repeat("scroll regression content ", 28), Editable: true}}}}
		blocks[i] = document.WordBlock{ID: fmt.Sprintf("b%d", i), Kind: document.WordParagraphBlock, Paragraph: &paragraph}
	}
	state := newDocumentSessionState("scroll.docx")
	state.word = word
	state.wordBlocks = blocks
	return state, func() { _ = word.Close() }
}
