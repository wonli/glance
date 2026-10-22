package desktopui

import (
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/format/pdfdoc"
	"glance/internal/proofreading"
	"image"
	"strings"
	"testing"
	"time"
)

func TestProofreadingScrollOverlayAllowsTextEditing(t *testing.T) {
	a := New(nil, "")
	a.proofreading = &proofreadingWorkspace{}
	var editor widget.Editor
	editor.SetText("校对文字")
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(400, 200))}
		material.Editor(a.theme, &editor, "").Layout(gtx)
		a.registerProofreadingXCanvasScrollPointer(gtx)
		router.Frame(&ops)
	}
	frame()
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(20, 10), Buttons: pointer.ButtonPrimary})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(20, 10)})
	frame()
	frame()
	if !router.Source().Focused(&editor) {
		t.Fatal("X scroll overlay blocked editor focus")
	}
	router.Queue(key.EditEvent{Text: "新增"})
	frame()
	if editor.Text() == "校对文字" {
		t.Fatal("typing after clicking X was lost")
	}
}

func TestProofreadingEnterThenImmediateActionUsesCommittedText(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	if err := s.controller.Model.Lock(1, true); err != nil {
		t.Fatal(err)
	}
	a.refreshProofreadingModelView()
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(400, 300))}
		a.processProofreadingHistoryKeys(gtx)
		a.processProofreadingEditorEvents(gtx)
		a.processProofreadingMenuEvents(gtx)
		a.syncProofreadingEditors()
		material.Editor(a.theme, &s.xPageEditors[0], "").Layout(gtx)
		a.registerProofreadingXCanvasScrollPointer(gtx)
		router.Frame(&ops)
	}
	frame()
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(15, 10), Buttons: pointer.ButtonPrimary})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(15, 10)})
	frame()
	router.Queue(key.EditEvent{Text: "\n\n\n"})
	frame()
	if got := s.controller.Model.Text(1); strings.Count(got, "\n") != 3 {
		t.Fatal("Enter edits were not committed before actions", got)
	}
	before := s.controller.Model.Text(1)
	if err := s.controller.Model.Adjust(1, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.controller.Model.Adjust(1, -100); err != nil {
		t.Fatal(err)
	}
	if s.controller.Model.Text(1) != before {
		t.Fatal("scroll lost Enter edits")
	}
	frame()
	if s.xPageEditors[0].Text() != before {
		t.Fatal("editor and model diverged")
	}
	router.Queue(key.Event{Name: "Z", State: key.Press, Modifiers: key.ModShortcut})
	frame()
	if s.controller.Model.Text(1) == before {
		t.Fatal("shared undo did not reverse last ownership adjustment")
	}
}

func TestProofreadingWheelInsideXPageScrollsItsContentAndOutsideMovesAX(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	if err := s.controller.Model.ReplacePage(3, "ijklmnopqrst"); err != nil {
		t.Fatal(err)
	}
	if err := s.controller.Model.Lock(1, true); err != nil {
		t.Fatal(err)
	}
	if err := s.controller.Model.Adjust(1, 8); err != nil {
		t.Fatal(err)
	}
	a.refreshProofreadingModelView()
	s.controller.Layout = application.ProofreadingLayoutSnapshot{Revision: s.controller.Model.Snapshot().Revision, StyleRevision: s.xTextSizeStep, Pages: map[proofreading.PageID]application.ProofreadingPageLayout{1: {Text: "abcdefghijklmnopqrst", Height: 608, Lines: []application.ProofreadingLine{{Start: 0, End: 4, Top: 0, Bottom: 544}, {Start: 4, End: 8, Top: 544, Bottom: 560}, {Start: 8, End: 12, Top: 560, Bottom: 576}, {Start: 12, End: 16, Top: 576, Bottom: 592}, {Start: 16, End: 20, Top: 592, Bottom: 608}}}}}
	var router input.Router
	var ops op.Ops
	view := &pdfViewState{info: document.PDFInfo{Pages: 3}, scale: 1}
	view.canvas.Axis = layout.Vertical
	s.xCanvas.Axis = layout.Vertical
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(500, 700))}
		a.processProofreadingEditorEvents(gtx)
		a.layoutProofreadingXPageList(gtx, view, 3, "", false)
		router.Frame(&ops)
	}
	frame()
	outer := s.xCanvas.Position.Offset
	modelText := s.controller.Model.Text(1)
	modelRevision := s.controller.Model.Snapshot().Revision
	router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(100, 100), Scroll: f32.Pt(0, 16)})
	frame()
	frame()
	if s.xCanvas.Position.Offset != outer {
		t.Fatal("inside X page moved outer A/X list")
	}
	if s.controller.Model.Text(1) != modelText || s.controller.Model.Snapshot().Revision != modelRevision {
		t.Fatal("view scrolling changed text or page ownership")
	}
	if s.xPageScrollOffsets[0] <= 0 {
		t.Fatal("inside X page did not scroll its own content")
	}
	text := s.controller.Model.Text(1)
	router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(5, 100), Scroll: f32.Pt(0, 100)})
	frame()
	frame()
	if s.xCanvas.Position.Offset == outer || view.canvas.Position.Offset != s.xCanvas.Position.Offset {
		t.Fatal("X exterior scroll should move A/X together")
	}
	if s.controller.Model.Text(1) != text {
		t.Fatal("X exterior scroll changed ownership; only the navigator should do that", s.controller.Model.Text(1))
	}
	if s.xPageScrollOffsets[0] != 16 {
		t.Fatal("loading characters from the X margin also moved the page's independent viewing offset", s.xPageScrollOffsets[0])
	}
}

func TestProofreadingWheelOutsideXPageWithoutLockMovesAX(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	var router input.Router
	var ops op.Ops
	view := &pdfViewState{info: document.PDFInfo{Pages: 3}, scale: 1}
	view.canvas.Axis = layout.Vertical
	s.xCanvas.Axis = layout.Vertical
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(500, 700))}
		a.layoutProofreadingXPageList(gtx, view, 3, "", false)
		router.Frame(&ops)
	}
	frame()
	router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(5, 100), Scroll: f32.Pt(0, 30)})
	frame()
	frame()
	if s.xCanvas.Position.Offset == 0 || view.canvas.Position.Offset != s.xCanvas.Position.Offset {
		t.Fatal("without a locked page, X exterior scrolling should remain linked to A")
	}
}

func TestProofreadingAScrollPositionIsMirroredToX(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	doc, err := pdfdoc.OpenSnapshot(pdfTestBytes(t))
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	a.pdfDoc = doc
	view := &pdfViewState{info: doc.Info(), scale: 1}
	view.canvas.Axis = layout.Vertical
	view.canvas.Position.First = 1
	view.canvas.Position.Offset = 32
	a.pdfView = view
	s.xCanvas.Axis = layout.Vertical
	var ops op.Ops
	var router input.Router
	gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(700, 700))}
	a.layoutProofreadingProjectionPages(gtx, "", false)
	if s.xCanvas.Position.First != view.canvas.Position.First || s.xCanvas.Position.Offset != view.canvas.Position.Offset {
		t.Fatalf("A scroll position was not mirrored to X: A=%+v X=%+v", view.canvas.Position, s.xCanvas.Position)
	}
}

func TestProofreadingPageScrollbarRangeUsesOnlyOwnedContent(t *testing.T) {
	shaped := application.ProofreadingPageLayout{Text: "abcdefghNEXT", Height: 60, Lines: []application.ProofreadingLine{
		{Start: 0, End: 4, Top: 0, Bottom: 20},
		{Start: 4, End: 8, Top: 20, Bottom: 40},
		{Start: 8, End: 12, Top: 40, Bottom: 60},
	}}
	contentHeight := proofreadingPageContentHeight(shaped, "abcdefgh")
	if contentHeight != 40 {
		t.Fatalf("following page content leaked into local scroll extent: got %v, want 40", contentHeight)
	}
	start, end, scrollable := proofreadingPageScrollbarRange(10, 25, contentHeight)
	if !scrollable || start <= 0 || end >= 1 || end <= start {
		t.Fatalf("invalid local scrollbar range: start=%v end=%v scrollable=%v", start, end, scrollable)
	}
	start, end, scrollable = proofreadingPageScrollbarRange(0, 50, contentHeight)
	if scrollable || start != 0 || end != 1 {
		t.Fatalf("non-overflowing page should not show scrollbar: %v %v %v", start, end, scrollable)
	}
}

func TestProofreadingIndependentPageScrollbarCanBeDragged(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	content := strings.Repeat("甲", 200)
	if err := s.controller.Model.ReplacePage(1, content); err != nil {
		t.Fatal(err)
	}
	if err := s.controller.Model.Lock(1, true); err != nil {
		t.Fatal(err)
	}
	a.refreshProofreadingModelView()
	lines := make([]application.ProofreadingLine, 0, 40)
	for start := 0; start < len([]rune(content)); start += 5 {
		lines = append(lines, application.ProofreadingLine{Start: start, End: min(start+5, len([]rune(content))), Top: float64(start / 5 * 15), Bottom: float64((start/5 + 1) * 15)})
	}
	s.controller.Layout = application.ProofreadingLayoutSnapshot{Revision: s.controller.Model.Snapshot().Revision, StyleRevision: s.xTextSizeStep, Pages: map[proofreading.PageID]application.ProofreadingPageLayout{1: {Text: content, Height: 600, Lines: lines}}}
	var router input.Router
	var ops op.Ops
	view := &pdfViewState{info: document.PDFInfo{Pages: 3}, scale: 1}
	view.canvas.Axis = layout.Vertical
	s.xCanvas.Axis = layout.Vertical
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(500, 700))}
		a.processProofreadingEditorEvents(gtx)
		a.layoutProofreadingXPageList(gtx, view, 3, "", false)
		router.Frame(&ops)
	}
	frame()
	modelText := s.controller.Model.Text(1)
	modelRevision := s.controller.Model.Snapshot().Revision
	var scrollbarBounds image.Rectangle
	var visit func([]input.SemanticNode)
	visit = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if node.Desc.Description == a.textID("proofread.x_content_scrollbar.description") {
				scrollbarBounds = node.Desc.Bounds
			}
			visit(node.Children)
		}
	}
	visit(router.AppendSemantics(nil))
	if scrollbarBounds.Empty() {
		t.Fatal("overflowing X page has no accessible independent scrollbar")
	}
	start := f32.Pt(float32((scrollbarBounds.Min.X+scrollbarBounds.Max.X)/2), float32(scrollbarBounds.Min.Y+scrollbarBounds.Dy()/2))
	end := f32.Pt(start.X, float32(scrollbarBounds.Max.Y-8))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 18, Position: start, Buttons: pointer.ButtonPrimary})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 18, Position: end, Buttons: pointer.ButtonPrimary})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 18, Position: end})
	frame()
	if s.xPageScrollOffsets[0] <= 0 {
		t.Fatal("dragging the page scrollbar did not move its viewport")
	}
	if s.xCanvas.Position.Offset != 0 || view.canvas.Position.Offset != 0 {
		t.Fatal("dragging the page scrollbar moved the outer A/X pages")
	}
	if s.controller.Model.Text(1) != modelText || s.controller.Model.Snapshot().Revision != modelRevision {
		t.Fatal("dragging the page scrollbar changed content or ownership")
	}
}

func TestProofreadingLongPageUsesBoundedEditorWithoutLosingTail(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	long := "BEGIN\n" + strings.Repeat("正文内容需要留在模型中，只有可见内容进入编辑器。\n", 1200) + "END"
	if err := s.controller.Model.ReplacePage(1, long); err != nil {
		t.Fatal(err)
	}
	_ = s.controller.Model.Review(1, true)
	a.refreshProofreadingModelView()
	a.syncProofreadingEditors()
	if len([]rune(s.xPageEditors[0].Text())) > 4096 {
		t.Fatal("long page was sent wholesale to native editor/GPU")
	}
	// An edit to the visible window must leave the unseen tail intact.
	visible := s.xPageEditors[0].Text()
	s.xPageEditors[0].SetText("新增\n" + visible)
	var ops op.Ops
	var router input.Router
	gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(500, 700))}
	a.processProofreadingEditorEvents(gtx)
	if got := s.controller.Model.Text(1); got != "新增\n"+long {
		t.Fatal("visible-window edit replaced or duplicated unseen content")
	}
}
