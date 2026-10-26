package desktopui

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/document"
)

func TestProofreadingPaginationScrollbarDragSyncsPagesWithoutEditing(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	view := &pdfViewState{info: document.PDFInfo{Pages: 3}}
	a.pdfView = view
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(900, 36))}
		a.layoutProofreadingPaginationHeading(gtx, view)
		router.Frame(&ops)
	}
	frame()
	var bounds image.Rectangle
	var visit func([]input.SemanticNode)
	visit = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if node.Desc.Description == a.textID("proofread.pagination.scrollbar.description") {
				bounds = node.Desc.Bounds
			}
			visit(node.Children)
		}
	}
	visit(router.AppendSemantics(nil))
	if bounds.Empty() {
		t.Fatal("page heading has no quick navigation scrollbar")
	}
	revision := s.controller.Model.Revision()
	start := f32.Pt(float32(bounds.Min.X+8), float32((bounds.Min.Y+bounds.Max.Y)/2))
	end := f32.Pt(float32(bounds.Max.X-2), start.Y)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 18, Position: start, Buttons: pointer.ButtonPrimary})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 18, Position: end, Buttons: pointer.ButtonPrimary})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 18, Position: end})
	frame()
	if view.page != 2 || view.canvas.Position.First != 2 || s.xCanvas.Position.First != 2 {
		t.Fatalf("drag did not synchronize last A/X page: page=%d A=%+v X=%+v", view.page, view.canvas.Position, s.xCanvas.Position)
	}
	if s.controller.Model.Revision() != revision {
		t.Fatal("quick page navigation edited document ownership")
	}
}

func TestProofreadingPaginationScrollbarFilteredTrackClick(t *testing.T) {
	for _, tc := range []struct {
		name     string
		reviewed []bool
		want     int
	}{
		{"filtered", []bool{true, false, true}, 2},
		{"single", []bool{true, false, false}, 0},
		{"empty", []bool{false, false, false}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, s := proofreadingModelFixture(t)
			s.pageFilter = proofreadingPageFilterReviewed
			for i, v := range tc.reviewed {
				s.pageOwnership[i].Reviewed = v
			}
			view := &pdfViewState{info: document.PDFInfo{Pages: 3}}
			a.pdfView = view
			var router input.Router
			var ops op.Ops
			frame := func() {
				ops.Reset()
				gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(320, 18))}
				a.layoutProofreadingPaginationScrollbar(gtx, view)
				router.Frame(&ops)
			}
			frame()
			point := f32.Pt(176, 9)
			router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 2, Position: point, Buttons: pointer.ButtonPrimary})
			frame()
			router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 2, Position: point})
			frame()
			if view.page != tc.want {
				t.Fatalf("track click selected page %d, want %d", view.page, tc.want)
			}
		})
	}
}
