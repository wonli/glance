package desktopui

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/document"
)

func TestProofreadingPageSelectionSurvivesLastPageLayout(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		a, s := proofreadingModelFixture(t)
		view := &pdfViewState{info: document.PDFInfo{Pages: 3}, scale: float64(scale), zoom: 0.75, pageSizeHintW: 400, pageSizeHintH: 600}
		a.pdfView = view
		view.canvas.Axis = layout.Vertical
		view.horizontal.Axis = layout.Horizontal
		var router input.Router
		var ops op.Ops
		frame := func() {
			ops.Reset()
			gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(int(600*scale), int(700*scale)))}
			a.layoutPDFCanvas(gtx)
			a.layoutProofreadingXNavigatorCanvas(gtx, view, 3, "", false)
			router.Frame(&ops)
		}
		for _, page := range []int{2, 1, 2} {
			a.selectProofreadingThumbnail(page)
			for i := 0; i < 4; i++ {
				frame()
				if view.page != page || s.navigator.page != page {
					t.Fatalf("scale=%v frame=%d selected=%d: footer=%d X=%d A=%+v", scale, i, page, view.page, s.navigator.page, view.canvas.Position)
				}
			}
		}
	}
}

func TestProofreadingPageSelectionUsesSameReadingLineAsX(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	view := &pdfViewState{info: document.PDFInfo{Pages: 3}, scale: 1}
	a.pdfView = view
	view.canvas.Axis = layout.Vertical
	view.canvas.Position.Offset = 500
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(600, 700))}
	a.layoutProofreadingXNavigatorCanvas(gtx, view, 3, "", false)
	if view.page != 1 || s.navigator.page != 1 {
		t.Fatalf("reading line is on page 2: footer=%d X=%d", view.page, s.navigator.page)
	}
}
