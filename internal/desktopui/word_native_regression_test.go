package desktopui

import (
	"fmt"
	"image"
	"path/filepath"
	"reflect"
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

func TestWordRibbonKeepsHeightAndPictureActionsStayOnOneRow(t *testing.T) {
	state, closeDocument := newLongWordFlowState(t, 2)
	defer closeDocument()
	state.wordToolbarTab = wordTabPicture
	state.selectedWordImageID = "selected-image"
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	gtx := layout.Context{
		Source:      router.Source(),
		Ops:         &ops,
		Now:         time.Now(),
		Constraints: layout.Constraints{Max: image.Pt(1100, 700)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	app.layoutWordFlow(gtx)
	router.Frame(&ops)
	if wordRibbonCategoryHeightDP != 34 || wordRibbonCommandHeightDP != 56 || wordRibbonHeightDP != 90 {
		t.Fatalf("Word ribbon heights = categories %d dp, commands %d dp, total %d dp; want Pen Clpgm 34/56/90 dp", wordRibbonCategoryHeightDP, wordRibbonCommandHeightDP, wordRibbonHeightDP)
	}
	want := map[string]image.Rectangle{
		"替换图片": {}, "删除图片": {}, "调整大小": {}, "向左旋转 90°": {}, "向右旋转 90°": {},
	}
	var visit func([]input.SemanticNode)
	visit = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if _, ok := want[node.Desc.Description]; ok {
				want[node.Desc.Description] = node.Desc.Bounds
			}
			visit(node.Children)
		}
	}
	visit(router.AppendSemantics(nil))
	rowTop, rowBottom := 0, 0
	for label, bounds := range want {
		if bounds.Empty() {
			t.Errorf("picture action %q has no visible bounds", label)
			continue
		}
		if rowTop == 0 || bounds.Min.Y < rowTop {
			rowTop = bounds.Min.Y
		}
		if bounds.Max.Y > rowBottom {
			rowBottom = bounds.Max.Y
		}
	}
	if rowBottom-rowTop > 36 {
		t.Errorf("picture ribbon actions occupy %d vertical pixels; want one 36 dp row", rowBottom-rowTop)
	}
	firstTop := -1
	for label, bounds := range want {
		if bounds.Empty() {
			continue
		}
		if firstTop < 0 {
			firstTop = bounds.Min.Y
			continue
		}
		if delta := bounds.Min.Y - firstTop; delta < -2 || delta > 2 {
			t.Errorf("picture action %q starts at y=%d; want all actions on one horizontal row near y=%d", label, bounds.Min.Y, firstTop)
		}
	}
}

func TestWordHomeRibbonMatchesPenClpgmGroupGeometry(t *testing.T) {
	state, closeDocument := newLongWordFlowState(t, 2)
	defer closeDocument()
	state.wordToolbarTab = wordTabHome
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	gtx := layout.Context{
		Source:      router.Source(),
		Ops:         &ops,
		Now:         time.Now(),
		Constraints: layout.Constraints{Max: image.Pt(1440, 900)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	app.layoutWordFlow(gtx)
	router.Frame(&ops)
	want := map[string]image.Rectangle{
		"文件":  {Min: image.Pt(22, 2), Max: image.Pt(53, 32)},
		"开始":  {Min: image.Pt(81, 2), Max: image.Pt(112, 32)},
		"粘贴":  {Min: image.Pt(34, 44), Max: image.Pt(70, 80)},
		"剪切":  {Min: image.Pt(76, 44), Max: image.Pt(112, 80)},
		"字体":  {Min: image.Pt(221, 44), Max: image.Pt(257, 80)},
		"加粗":  {Min: image.Pt(347, 44), Max: image.Pt(383, 80)},
		"左对齐": {Min: image.Pt(660, 44), Max: image.Pt(696, 80)},
		"查找":  {Min: image.Pt(931, 44), Max: image.Pt(967, 80)},
		"编辑器": {Min: image.Pt(1076, 44), Max: image.Pt(1112, 80)},
	}
	found := make(map[string]image.Rectangle, len(want))
	hasHistoryAction := false
	var visit func([]input.SemanticNode)
	visit = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			switch node.Desc.Description {
			case "撤销", "重做":
				hasHistoryAction = true
			}
			if _, ok := want[node.Desc.Description]; ok {
				found[node.Desc.Description] = node.Desc.Bounds
			}
			visit(node.Children)
		}
	}
	visit(router.AppendSemantics(nil))
	for label, expected := range want {
		bounds := found[label]
		if bounds.Empty() {
			t.Errorf("Clpgm ribbon control %q has no semantic bounds", label)
			continue
		}
		if bounds.Min.X != expected.Min.X || (expected.Max.X > expected.Min.X && bounds.Max.X != expected.Max.X) || bounds.Min.Y != expected.Min.Y || (expected.Max.Y > expected.Min.Y && bounds.Max.Y != expected.Max.Y) {
			t.Errorf("Clpgm ribbon control %q bounds = %v; want %v", label, bounds, expected)
		}
	}
	if hasHistoryAction {
		t.Error("Home ribbon includes undo/redo controls that are absent from Pen node Clpgm")
	}
}

func TestWordCanvasWidthCanBeDraggedFromHorizontalRuler(t *testing.T) {
	state, closeDocument := newLongWordFlowState(t, 8)
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
	start := f32.Pt(float32(initial.Max.X), float32(wordRibbonHeightDP+wordRulerHeightDP/2))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 8, Buttons: pointer.ButtonPrimary, Position: start})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 8, Buttons: pointer.ButtonPrimary, Position: start.Add(f32.Pt(40, 0))})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 8, Position: start.Add(f32.Pt(40, 0))})
	frame()
	if state.wordCanvasWidthDP <= startWidth {
		t.Fatalf("Word canvas width after dragging ruler edge = %d dp; want more than %d dp", state.wordCanvasWidthDP, startWidth)
	}
}

func TestWordRulerIndentMarkersPersistParagraphWidthChanges(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState("ruler-indents.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	paragraph := *state.wordBlocks[0].Paragraph
	run := *paragraph.Inlines[0].Text
	state.selectedWordRunID = run.ID
	state.selectedWordParagraphID = paragraph.ID
	app := &App{documentSessionState: state, theme: material.NewTheme(), wordCommandResults: make(chan wordCommandResult, 1)}
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
	editor := focusWordRunForInteraction(t, state, router, frame, run.ID)
	draftText := run.Text + " pending ruler draft"
	editor.SetText(draftText)
	state.wordRunDrafts[run.ID] = draftText
	state.wordRunCommitted[run.ID] = run.Text
	waitForIndentCommit := func() {
		t.Helper()
		deadline := time.After(3 * time.Second)
		for state.formatRunning.Load() {
			select {
			case result := <-app.wordCommandResults:
				app.wordCommandResults <- result
				app.processAsyncResults()
			case <-deadline:
				t.Fatal("ruler indentation edit did not complete")
			default:
				time.Sleep(time.Millisecond)
			}
		}
	}
	dragMarker := func(description string, deltaX float32, pointerID pointer.ID) {
		t.Helper()
		frame()
		var bounds image.Rectangle
		var visit func([]input.SemanticNode)
		visit = func(nodes []input.SemanticNode) {
			for _, node := range nodes {
				if node.Desc.Description == description {
					bounds = node.Desc.Bounds
				}
				visit(node.Children)
			}
		}
		visit(router.AppendSemantics(nil))
		if bounds.Empty() {
			t.Fatalf("Word ruler is missing the interactive handle %q", description)
		}
		start := f32.Pt(float32(bounds.Min.X+bounds.Dx()/2), float32(bounds.Min.Y+bounds.Dy()/2))
		end := start.Add(f32.Pt(deltaX, 0))
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: pointerID, Buttons: pointer.ButtonPrimary, Position: start})
		frame()
		if state.wordRulerIndentDragSide == 0 {
			t.Fatalf("ruler handle %q did not capture the pointer press", description)
		}
		// Model the Word text flush that can start when the ruler takes focus while
		// retaining the active pointer capture for this drag.
		state.formatRunning.Store(true)
		state.wordFlushRequested.Store(true)
		state.wordFlushAllDrafts.Store(true)
		router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: pointerID, Buttons: pointer.ButtonPrimary, Position: end})
		frame()
		if state.wordRulerIndentDragPreviewEMU == state.wordRulerIndentDragStartEMU {
			t.Fatalf("ruler handle %q stopped tracking movement while a Word text flush was running", description)
		}
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: pointerID, Position: end})
		frame()
		if state.wordDeferredToolbarAction != "ruler-indents" {
			t.Fatalf("ruler handle %q did not defer its commit behind the pending text draft: %q", description, state.wordDeferredToolbarAction)
		}
		state.formatRunning.Store(false)
		app.startNextWordTextCommit()
		waitForIndentCommit()
		frame()
	}
	dragMarker("拖动调整段落左缩进", 48, 20)
	leftParagraph, ok := selectedWordParagraph(state.wordBlocks, paragraph.ID)
	if !ok || leftParagraph.LeftIndentEMU < 48*wordEMUPerDP-100 {
		t.Fatalf("left ruler drag produced paragraph indents %d/%d EMU; want at least %d left EMU (status %q, dirty %v)", leftParagraph.LeftIndentEMU, leftParagraph.RightIndentEMU, 48*wordEMUPerDP-100, app.status, state.word.Dirty())
	}
	draftText += " after second ruler drag"
	editor.SetText(draftText)
	state.wordRunDrafts[run.ID] = draftText
	dragMarker("拖动调整段落右缩进", -24, 21)
	updated, ok := selectedWordParagraph(state.wordBlocks, paragraph.ID)
	if !ok || updated.RightIndentEMU < 24*wordEMUPerDP-100 {
		t.Fatalf("right ruler drag produced paragraph indents %d/%d EMU; want at least %d right EMU", updated.LeftIndentEMU, updated.RightIndentEMU, 24*wordEMUPerDP-100)
	}
	output := filepath.Join(t.TempDir(), "ruler-indents.docx")
	if err := state.word.SaveAs(output); err != nil {
		t.Fatal(err)
	}
	reopened, err := docx.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedParagraph, ok := selectedWordParagraph(reopened.WordBlocks(), paragraph.ID)
	if !ok || reopenedParagraph.LeftIndentEMU != updated.LeftIndentEMU || reopenedParagraph.RightIndentEMU != updated.RightIndentEMU {
		t.Fatalf("reopened ruler indents = %d/%d EMU, want %d/%d", reopenedParagraph.LeftIndentEMU, reopenedParagraph.RightIndentEMU, updated.LeftIndentEMU, updated.RightIndentEMU)
	}
	if got := reopenedParagraph.Inlines[0].Text.Text; got != draftText {
		t.Fatalf("ruler edit did not flush the pending Word text draft before save: got %q, want %q", got, draftText)
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
	wide, wideLeft := wordPageGeometry(1200, wordPaperMaxWidthDP)
	if wide != 900 || wideLeft != 150 {
		t.Fatalf("wide page geometry = width %d left %d; want 900/150", wide, wideLeft)
	}
	narrow, narrowLeft := wordPageGeometry(600, wordPaperMaxWidthDP)
	if narrow != 600 || narrowLeft != 0 {
		t.Fatalf("narrow page geometry = width %d left %d; want 600/0", narrow, narrowLeft)
	}
}

func TestWordCanvasAndRulersMatchPenClpgmGeometry(t *testing.T) {
	state, closeDocument := newLongWordFlowState(t, 2)
	defer closeDocument()
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	gtx := layout.Context{
		Source:      router.Source(),
		Ops:         &ops,
		Now:         time.Now(),
		Constraints: layout.Constraints{Max: image.Pt(1440, 858)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	rootOffset := op.Offset(image.Pt(0, 42)).Push(gtx.Ops)
	app.layoutWord(gtx)
	rootOffset.Pop()
	router.Frame(&ops)
	want := map[string]image.Rectangle{
		"文件":        {Min: image.Pt(22, 44), Max: image.Pt(53, 74)},
		"开始":        {Min: image.Pt(81, 44), Max: image.Pt(112, 74)},
		"粘贴":        {Min: image.Pt(34, 86), Max: image.Pt(70, 122)},
		"Word 水平标尺": {Min: image.Pt(0, 132), Max: image.Pt(1440, 165)},
		"Word 文档画布": {Min: image.Pt(270, 181), Max: image.Pt(1170, 781)},
		"Word 垂直标尺": {Min: image.Pt(231, 185), Max: image.Pt(256, 833)},
		"Word 状态栏":  {Min: image.Pt(0, 866), Max: image.Pt(1440, 900)},
	}
	found := make(map[string]image.Rectangle, len(want))
	var visit func([]input.SemanticNode)
	visit = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if _, ok := want[node.Desc.Description]; ok {
				found[node.Desc.Description] = node.Desc.Bounds
			}
			visit(node.Children)
		}
	}
	visit(router.AppendSemantics(nil))
	for description, expected := range want {
		if got := found[description]; got != expected {
			t.Errorf("Pen Clpgm %q bounds = %v; want %v", description, got, expected)
		}
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
			t.Fatalf("Enter did not split the paragraph before timeout (blocks=%d, running=%v)", len(state.wordBlocks), state.formatRunning.Load())
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

func TestWordRightArrowMovesToNextStyledRun(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := document.WordParagraph{ID: "p0", Inlines: []document.WordInline{
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-a", Text: "left", Editable: true}},
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-b", Text: "right", Style: document.WordTextStyle{Bold: true}, Editable: true}},
	}}
	state := newDocumentSessionState("navigation.docx")
	state.word = word
	state.wordBlocks = []document.WordBlock{{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &paragraph}}
	_, router, frame := newWordJoinFrame(t, state)
	first := focusWordRunForInteraction(t, state, router, frame, "run-a")
	first.SetCaret(first.Len(), first.Len())
	frame()

	router.Queue(key.Event{Name: key.NameRightArrow, State: key.Press})
	frame()
	next := state.wordRunEditors["run-b"]
	if next == nil || !router.Source().Focused(next) {
		t.Fatal("Right at a run end did not focus the next styled Word run")
	}
	if start, end := next.Selection(); start != 0 || end != 0 {
		t.Fatalf("caret after moving right between runs = %d:%d; want 0:0", start, end)
	}
	if state.selectedWordRunID != "run-b" || state.selectedWordParagraphID != "p0" {
		t.Fatalf("selection after run handoff = %q/%q; want run-b/p0", state.selectedWordRunID, state.selectedWordParagraphID)
	}
}

func TestWordLeftArrowMovesToPreviousParagraphRun(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	first := document.WordParagraph{ID: "p0", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-a", Text: "first", Editable: true}}}}
	second := document.WordParagraph{ID: "p1", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-b", Text: "second", Editable: true}}}}
	state := newDocumentSessionState("navigation.docx")
	state.word = word
	state.wordBlocks = []document.WordBlock{
		{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &first},
		{ID: "p1", Kind: document.WordParagraphBlock, Paragraph: &second},
	}
	_, router, frame := newWordJoinFrame(t, state)
	secondEditor := focusWordRunForInteraction(t, state, router, frame, "run-b")
	secondEditor.SetCaret(0, 0)
	frame()

	router.Queue(key.Event{Name: key.NameLeftArrow, State: key.Press})
	frame()
	previous := state.wordRunEditors["run-a"]
	if previous == nil || !router.Source().Focused(previous) {
		t.Fatal("Left at a paragraph start did not focus the previous paragraph run")
	}
	if start, end := previous.Selection(); start != previous.Len() || end != previous.Len() {
		t.Fatalf("caret after moving left between paragraphs = %d:%d; want %d:%d", start, end, previous.Len(), previous.Len())
	}
}

func TestWordModifiedArrowKeepsNativeSelectionAndDocumentEdges(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := document.WordParagraph{ID: "p0", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-a", Text: "selection", Editable: true}}}}
	state := newDocumentSessionState("navigation.docx")
	state.word = word
	state.wordBlocks = []document.WordBlock{{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &paragraph}}
	_, router, frame := newWordJoinFrame(t, state)
	editor := focusWordRunForInteraction(t, state, router, frame, "run-a")
	editor.SetCaret(0, 0)
	frame()

	router.Queue(key.Event{Name: key.NameRightArrow, State: key.Press, Modifiers: key.ModShift})
	frame()
	if start, end := editor.Selection(); min(start, end) != 0 || max(start, end) != 1 {
		t.Fatalf("Shift+Right selection = %d:%d; want native selection across offsets 0 and 1", start, end)
	}
	editor.SetCaret(0, 0)
	frame()
	router.Queue(key.Event{Name: key.NameLeftArrow, State: key.Press})
	frame()
	if !router.Source().Focused(editor) {
		t.Fatal("Left at the document start moved focus outside the document")
	}
	if start, end := editor.Selection(); start != 0 || end != 0 {
		t.Fatalf("caret at document start after Left = %d:%d; want 0:0", start, end)
	}
}

func TestWordNavigationRunOrderIncludesEditableTableCellsOnly(t *testing.T) {
	paragraph := func(id, text string, editable bool) document.WordParagraph {
		return document.WordParagraph{ID: id, Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: id + ".run", Text: text, Editable: editable}}}}
	}
	first := paragraph("body", "body", true)
	readOnly := paragraph("locked", "locked", false)
	cellA := document.WordTableCell{ID: "a", Paragraphs: []document.WordParagraph{paragraph("a.p0", "A1", true), paragraph("a.p1", "A2", true)}}
	cellB := document.WordTableCell{ID: "b", Paragraphs: []document.WordParagraph{paragraph("b.p0", "B1", true)}}
	blocks := []document.WordBlock{
		{ID: "body", Kind: document.WordParagraphBlock, Paragraph: &first},
		{ID: "locked", Kind: document.WordParagraphBlock, Paragraph: &readOnly},
		{ID: "table", Kind: document.WordTableBlock, Table: &document.WordTable{Rows: []document.WordTableRow{{Cells: []document.WordTableCell{cellA, cellB}}}}},
	}
	var got []string
	for _, item := range wordNavigationRuns(blocks) {
		got = append(got, item.run.ID)
	}
	want := []string{"body.run", "a.p0.run", "a.p1.run", "b.p0.run"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Word navigation order = %#v; want row-major editable runs %#v", got, want)
	}
}

func TestWordUpDownKeepsPreferredVisualColumnAcrossVisualLines(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	text := strings.Repeat("abcdefghij", 5) + "\n" + strings.Repeat("abcdefghij", 3) + "\n" + strings.Repeat("abc", 20)
	paragraph := document.WordParagraph{ID: "p0", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-a", Text: text, Editable: true}}}}
	state := newDocumentSessionState("navigation.docx")
	state.word = word
	state.wordBlocks = []document.WordBlock{{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &paragraph}}
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: metric, Constraints: layout.Constraints{Max: image.Pt(620, 700)}}
		app.layoutWord(gtx)
		router.Frame(&ops)
	}
	editor := focusWordRunForInteraction(t, state, router, frame, "run-a")
	editor.SetCaret(8, 8)
	frame()
	geometry := state.wordNavLayouts["run-a"]
	start := editor.CaretCoords()
	startY := geometry.y + int(start.Y)

	for move := 0; move < 2; move++ {
		router.Queue(key.Event{Name: key.NameDownArrow, State: key.Press})
		frame()
		coords := editor.CaretCoords()
		if gotY := geometry.y + int(coords.Y); gotY <= startY {
			t.Fatalf("Down %d left caret on the same visual line: y=%d, start=%d, caret=%v geometry=%+v", move+1, gotY, startY, coords, geometry)
		}
		if !state.wordPreferredVerticalXSet {
			t.Fatal("Up/Down did not retain a preferred horizontal column")
		}
		visualX := geometry.x + int(coords.X)
		if distance := absWordInt(visualX - state.wordPreferredVerticalX); distance > 10 {
			t.Fatalf("Down %d moved to x=%d; preferred visual x=%d (distance %d)", move+1, visualX, state.wordPreferredVerticalX, distance)
		}
		startY = geometry.y + int(coords.Y)
		geometry = state.wordNavLayouts["run-a"]
	}
}

func TestWordUpDownCrossesParagraphsAtPreferredVisualColumn(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	text := strings.Repeat("0123456789", 40)
	first := document.WordParagraph{ID: "p0", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-a", Text: text, Editable: true}}}}
	second := document.WordParagraph{ID: "p1", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "run-b", Text: text, Editable: true}}}}
	state := newDocumentSessionState("navigation.docx")
	state.word = word
	state.wordBlocks = []document.WordBlock{{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &first}, {ID: "p1", Kind: document.WordParagraphBlock, Paragraph: &second}}
	_, router, frame := newWordJoinFrame(t, state)
	firstEditor := focusWordRunForInteraction(t, state, router, frame, "run-a")
	firstEditor.SetCaret(100, 100)
	frame()
	preferred := state.wordNavLayouts["run-a"].x + int(firstEditor.CaretCoords().X)

	router.Queue(key.Event{Name: key.NameDownArrow, State: key.Press})
	frame()
	frame()
	secondEditor := state.wordRunEditors["run-b"]
	if secondEditor == nil || !router.Source().Focused(secondEditor) {
		t.Fatalf("Down at the last visual line did not focus the next paragraph: selected=%q/%q pending=%q/%q preferred=%v/%d sourceFocusedA=%v bounds=%#v", state.selectedWordRunID, state.selectedWordParagraphID, state.wordPendingFocusRunID, state.wordPendingCaretRunID, state.wordPreferredVerticalXSet, state.wordPreferredVerticalX, router.Source().Focused(firstEditor), state.wordNavLayouts)
	}
	if !state.wordPreferredVerticalXSet {
		t.Fatal("vertical caret column was cleared while moving to the next paragraph")
	}
	secondX := state.wordNavLayouts["run-b"].x + int(secondEditor.CaretCoords().X)
	if distance := absWordInt(secondX - preferred); distance > 10 {
		t.Fatalf("next paragraph caret x=%d; preferred x=%d (distance %d)", secondX, preferred, distance)
	}

	router.Queue(key.Event{Name: key.NameUpArrow, State: key.Press})
	frame()
	frame()
	if !router.Source().Focused(firstEditor) {
		t.Fatal("Up at the first visual line did not return to the previous paragraph")
	}
	firstX := state.wordNavLayouts["run-a"].x + int(firstEditor.CaretCoords().X)
	if distance := absWordInt(firstX - preferred); distance > 10 {
		t.Fatalf("previous paragraph caret x=%d; preferred x=%d (distance %d)", firstX, preferred, distance)
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
