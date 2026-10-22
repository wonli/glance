package desktopui

import (
	"context"
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/proofreading"
	"image"
	"testing"
	"time"
)

func TestProofreadingNavigatorWheelFineTargetAndOuterIsolation(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		a, s := proofreadingModelFixture(t)
		view := &pdfViewState{info: document.PDFInfo{Pages: 3}, scale: float64(scale)}
		view.canvas.Axis = layout.Vertical
		var router input.Router
		var ops op.Ops
		frame := func() {
			ops.Reset()
			gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(int(600*scale), int(700*scale)))}
			a.layoutProofreadingXNavigatorCanvas(gtx, view, 3, "", false)
			router.Frame(&ops)
		}
		scroll := func(y float32) {
			router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(575*scale, 200*scale), Scroll: f32.Pt(0, y*scale)})
			frame()
			frame()
		}
		frame()
		scroll(4)
		if s.controller.Model.Text(1) != "abcd" {
			t.Fatal("tiny touch moved a character")
		}
		scroll(12)
		if s.controller.Model.Text(1) != "abcde" || s.controller.Model.Text(2) != "fgh" {
			t.Fatal("fine wheel did not move exactly one character", s.xPages)
		}
		scroll(-16)
		if s.controller.Model.Text(1) != "abcd" {
			t.Fatal("one reverse step did not return one character")
		}
		scroll(48)
		if s.controller.Model.Text(1) != "abcdefg" {
			t.Fatal("accumulated steps lost")
		}
		a.undoProofreadingPagination()
		if s.controller.Model.Text(1) != "abcdef" {
			t.Fatal("one undo should return only one character")
		}
		a.undoProofreadingPagination()
		a.undoProofreadingPagination()
		if view.canvas.Position.First != 0 || view.canvas.Position.Offset != 0 {
			t.Fatal("navigator moved A/X outer canvas")
		}
		view.canvas.Position.First = 1
		view.canvas.Position.Offset = 0
		frame()
		scroll(16)
		if s.controller.Model.Text(1) != "abcd" || s.controller.Model.Text(2) != "efghi" {
			t.Fatal("A page selection did not choose page 2", s.xPages)
		}
	}
}

func TestProofreadingNavigatorIgnoresReviewedTarget(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	_ = s.controller.Model.Review(1, true)
	a.refreshProofreadingModelView()
	var ops op.Ops
	var router input.Router
	view := &pdfViewState{info: document.PDFInfo{Pages: 3}, scale: 1}
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(600, 700))}
		a.layoutProofreadingXNavigatorCanvas(gtx, view, 3, "", false)
		router.Frame(&ops)
	}
	frame()
	rev := s.controller.Model.Snapshot().Revision
	router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(575, 200), Scroll: f32.Pt(0, 160), Modifiers: key.ModShift})
	frame()
	frame()
	if s.controller.Model.Snapshot().Revision != rev {
		t.Fatal("reviewed target changed")
	}
}

func TestProofreadingNavigatorCoarseWaitsAndTargetChangeDiscardsPending(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	var ops op.Ops
	var router input.Router
	view := &pdfViewState{info: document.PDFInfo{Pages: 3}, scale: 1}
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(600, 700))}
		a.layoutProofreadingXNavigatorCanvas(gtx, view, 3, "", false)
		router.Frame(&ops)
	}
	frame()
	router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(575, 200), Scroll: f32.Pt(0, 16), Modifiers: key.ModShift})
	frame()
	if s.controller.Model.Text(1) != "abcd" || len(s.navigator.pending) != 1 {
		t.Fatal("coarse step was lost or used missing geometry")
	}
	s.controller.Layout = application.ProofreadingLayoutSnapshot{Revision: s.controller.Model.Snapshot().Revision, Pages: map[proofreading.PageID]application.ProofreadingPageLayout{1: {Text: "abcdefgh", Lines: []application.ProofreadingLine{{Start: 0, End: 6}, {Start: 6, End: 8}}}}}
	frame()
	if s.controller.Model.Text(1) != "abcdef" || len(s.navigator.pending) != 0 {
		t.Fatal("coarse step did not resume at shaped boundary")
	}
	router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(575, 200), Scroll: f32.Pt(0, 16), Modifiers: key.ModShift})
	frame()
	if len(s.navigator.pending) != 1 {
		t.Fatal("coarse step should wait for new revision")
	}
	a.undoProofreadingPagination()
	if len(s.navigator.pending) != 0 || s.controller.Model.Text(1) != "abcd" {
		t.Fatal("undo left queued coarse input that could replay")
	}
	router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(575, 200), Scroll: f32.Pt(0, 16), Modifiers: key.ModShift})
	frame()
	if len(s.navigator.pending) != 1 {
		t.Fatal("fixture must have pending coarse input before switching")
	}
	view.canvas.Position.First = 1
	frame()
	if len(s.navigator.pending) != 0 || s.controller.Model.Text(2) != "efgh" {
		t.Fatal("old target's pending input leaked into new target")
	}
}

func TestProofreadingNavigatorDuringIMELeavesDraftAndOwnershipAlone(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	var ops op.Ops
	var router input.Router
	view := &pdfViewState{info: document.PDFInfo{Pages: 3}, scale: 1}
	focus := true
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(600, 700))}
		a.processProofreadingEditorEvents(gtx)
		if focus {
			gtx.Execute(key.FocusCmd{Tag: &s.xPageEditors[0]})
			focus = false
		}
		a.layoutProofreadingXNavigatorCanvas(gtx, view, 3, "", false)
		router.Frame(&ops)
	}
	frame()
	frame()
	router.Queue(key.CompositionEvent{Start: 0, End: 2}, key.EditEvent{Range: key.Range{Start: 0, End: 0}, Text: "ni"})
	frame()
	if !a.proofreadingEditorComposing() {
		t.Fatal("IME fixture not composing")
	}
	router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(575, 200), Scroll: f32.Pt(0, 160)})
	frame()
	if s.controller.Model.Text(1) != "abcd" || s.xPageEditors[0].Text() != "niabcd" {
		t.Fatal("navigator committed/reset composition or moved text")
	}
	router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 2}, Text: "你好"}, key.CompositionEvent{Start: -1, End: -1})
	frame()
	frame()
	if s.controller.Model.Text(1) != "你好abcd" || s.controller.Model.Text(2) != "efgh" || len(s.navigator.pending) != 0 {
		t.Fatal("IME commit lost or ignored wheel replayed", s.xPages)
	}
}

func TestProofreadingNavigatorRevealsBoundaryAfterBackgroundLayout(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	value := ""
	for range 100 {
		value += "中文校对行\n"
	}
	_ = s.controller.Model.ReplacePage(1, value)
	_, err := s.controller.AdjustPageUnits(1, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	a.refreshProofreadingModelView()
	result, err := reflowProofreadingForEditor(context.Background(), s.controller.Model.Snapshot(), []application.ProofreadingPageGeometry{{Page: 1, Width: 340, Height: 544}, {Page: 2, Width: 340, Height: 544}, {Page: 3, Width: 340, Height: 544}}, application.ProofreadingTextStyle{Size: 12, Family: string(a.theme.Face)}, false)
	if err != nil {
		t.Fatal(err)
	}
	s.controller.AcceptLayout(result)
	s.navigator = proofreadingNavigator{initialized: true, page: 0, reveal: true}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(600, 700))}
	view := &pdfViewState{info: document.PDFInfo{Pages: 3}, scale: 1}
	a.layoutProofreadingXNavigatorCanvas(gtx, view, 3, "", false)
	if s.xPageScrollOffsets[0] <= 0 || s.navigator.reveal {
		t.Fatal("new boundary stayed offscreen")
	}
	if s.xEditorWindows[0].End != len([]rune(s.controller.Model.Text(1))) {
		t.Fatal("visible editor window did not include new tail")
	}
}
