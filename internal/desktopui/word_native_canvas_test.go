package desktopui

import (
	"image"
	"path/filepath"

	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"

	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/format/docx"
)

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
		for {
			app.processAsyncResults()
			if !state.formatRunning.Load() && state.wordDeferredToolbarAction == "" && len(app.wordCommandResults) == 0 {
				break
			}
			select {
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
		"粘贴":        {Min: image.Pt(8, 86), Max: image.Pt(52, 134)},
		"Word 水平标尺": {Min: image.Pt(0, 144), Max: image.Pt(1440, 177)},
		"Word 文档画布": {Min: image.Pt(270, 193), Max: image.Pt(1170, 793)},
		"Word 垂直标尺": {Min: image.Pt(231, 197), Max: image.Pt(256, 833)},
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
