package desktopui

import (
	"image"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/document"
)

func TestProofreadingPageFilterLayoutMapsRowsToOriginalPDFPages(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	s.pageOwnership = []proofreadingPageOwnership{{}, {Reviewed: true}, {}, {Reviewed: true}}
	s.pageFilter = proofreadingPageFilterReviewed
	pages := a.proofreadingPageIndices(4)
	if got, ok := proofreadingPDFPageForRow(s, pages, 1); !ok || got != 3 {
		t.Fatalf("filtered row 1 resolved to page %d, %v; want original PDF page 3", got, ok)
	}
	if got, ok := proofreadingPDFPageForRow(s, pages, 0); !ok || got != 1 {
		t.Fatalf("filtered row 0 resolved to page %d, %v; want original PDF page 1", got, ok)
	}
}

func TestProofreadingPageFilterLayoutKeepsXAndPDFRowsSynchronized(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	s.pageOwnership = []proofreadingPageOwnership{{}, {Reviewed: true}, {}}
	s.pageFilter = proofreadingPageFilterReviewed
	s.filteredPageIndices = []int{1}
	s.xCanvas.Axis = layout.Vertical
	view := &pdfViewState{info: document.PDFInfo{Pages: 3}, page: 0}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(800, 640))}
	a.layoutProofreadingXPageList(gtx, view, 3, "", false)
	if view.page != 1 {
		t.Fatalf("X row 0 selected logical PDF page %d, want page 1", view.page)
	}
	if len(s.visibleEditorPages) != 1 || s.visibleEditorPages[0] != 1 {
		t.Fatalf("X visible editor pages=%v, want original page index [1]", s.visibleEditorPages)
	}
	if s.xCanvas.Position.First != view.canvas.Position.First || s.xCanvas.Position.Offset != view.canvas.Position.Offset {
		t.Fatalf("filtered positions diverged: A=%+v X=%+v", view.canvas.Position, s.xCanvas.Position)
	}
}
