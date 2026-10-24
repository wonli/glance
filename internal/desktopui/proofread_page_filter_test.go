package desktopui

import (
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/document"
	"image"
)

func TestProofreadingPageFilterIncludesExpectedPageIndexes(t *testing.T) {
	ownership := []proofreadingPageOwnership{
		{Reviewed: false},
		{Reviewed: true},
		{Reviewed: false},
		{Reviewed: true},
	}
	tests := []struct {
		name   string
		filter proofreadingPageFilter
		want   []int
	}{
		{name: "all", filter: proofreadingPageFilterAll, want: []int{0, 1, 2, 3}},
		{name: "reviewed", filter: proofreadingPageFilterReviewed, want: []int{1, 3}},
		{name: "unreviewed", filter: proofreadingPageFilterUnreviewed, want: []int{0, 2}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := proofreadingFilteredPages(ownership, 4, test.filter)
			if len(got) != len(test.want) {
				t.Fatalf("filtered pages=%v, want %v", got, test.want)
			}
			for i := range test.want {
				if got[i] != test.want[i] {
					t.Fatalf("filtered pages=%v, want %v", got, test.want)
				}
			}
		})
	}
}

func TestProofreadingPageFilterTreatsMissingOwnershipAsUnreviewed(t *testing.T) {
	got := proofreadingFilteredPages([]proofreadingPageOwnership{{Reviewed: true}}, 3, proofreadingPageFilterUnreviewed)
	want := []int{1, 2}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("filtered pages=%v, want %v", got, want)
	}
}

func TestProofreadingPageFilterCanBeToggledOff(t *testing.T) {
	if got := proofreadingFilterModeAfterClick(proofreadingPageFilterAll, proofreadingPageFilterReviewed); got != proofreadingPageFilterReviewed {
		t.Fatalf("first reviewed click selected %v, want reviewed", got)
	}
	if got := proofreadingFilterModeAfterClick(proofreadingPageFilterReviewed, proofreadingPageFilterReviewed); got != proofreadingPageFilterAll {
		t.Fatalf("second reviewed click selected %v, want all", got)
	}
	if got := proofreadingFilterModeAfterClick(proofreadingPageFilterReviewed, proofreadingPageFilterUnreviewed); got != proofreadingPageFilterUnreviewed {
		t.Fatalf("switching status selected %v, want unreviewed", got)
	}
}

func TestProofreadingPageFilterRowMapping(t *testing.T) {
	pages := []int{2, 5, 9}
	if got, ok := proofreadingFilterPageAtRow(pages, 1); !ok || got != 5 {
		t.Fatalf("row 1 page=%d, want 5", got)
	}
	if _, ok := proofreadingFilterPageAtRow(pages, 3); ok {
		t.Fatal("out-of-range row unexpectedly resolved")
	}
	if got := proofreadingFilterRowForPage(pages, 9); got != 2 {
		t.Fatalf("page 9 row=%d, want 2", got)
	}
	if got := proofreadingFilterRowForPage(pages, 4); got != -1 {
		t.Fatalf("excluded page row=%d, want -1", got)
	}
}

func TestProofreadingPageFilterTargetKeepsOrAdvancesCurrentPage(t *testing.T) {
	pages := []int{1, 4, 7}
	for _, test := range []struct {
		current int
		want    int
	}{
		{current: 4, want: 4},
		{current: 5, want: 7},
		{current: 8, want: 1},
	} {
		if got := proofreadingFilterTargetPage(pages, test.current); got != test.want {
			t.Fatalf("current page %d selected %d, want %d", test.current, got, test.want)
		}
	}
	if got := proofreadingFilterTargetPage(nil, 5); got != -1 {
		t.Fatalf("empty filter selected page %d, want -1", got)
	}
}

func TestProofreadingPageFilterWithoutReviewedPagesIsEmpty(t *testing.T) {
	ownership := []proofreadingPageOwnership{{}, {}, {}}
	if got := proofreadingFilteredPages(ownership, 3, proofreadingPageFilterReviewed); len(got) != 0 {
		t.Fatalf("reviewed pages=%v, want none", got)
	}
}

func TestProofreadingPageFilterInteractionSwitchesAndToggles(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	s.pageOwnership = []proofreadingPageOwnership{{Reviewed: true}, {}, {Reviewed: true}, {}, {Reviewed: true}}
	view := &pdfViewState{info: document.PDFInfo{Pages: 5}, page: 1}
	view.canvas.Position = layout.Position{First: 1}
	a.pdfView = view

	a.setProofreadingPageFilter(proofreadingPageFilterReviewed)
	if s.pageFilter != proofreadingPageFilterReviewed || a.pdfView.page != 2 || a.pdfView.canvas.Position.First != 1 {
		t.Fatalf("reviewed filter did not advance to next matching page: filter=%v page=%d row=%d", s.pageFilter, a.pdfView.page, a.pdfView.canvas.Position.First)
	}
	a.setProofreadingPageFilter(proofreadingPageFilterReviewed)
	if s.pageFilter != proofreadingPageFilterAll || a.pdfView.page != 2 || a.pdfView.canvas.Position.First != 2 {
		t.Fatalf("toggling reviewed off failed: filter=%v page=%d row=%d", s.pageFilter, a.pdfView.page, a.pdfView.canvas.Position.First)
	}
	a.setProofreadingPageFilter(proofreadingPageFilterUnreviewed)
	if s.pageFilter != proofreadingPageFilterUnreviewed || a.pdfView.page != 3 || a.pdfView.canvas.Position.First != 1 {
		t.Fatalf("unreviewed filter did not advance to next matching page: filter=%v page=%d row=%d", s.pageFilter, a.pdfView.page, a.pdfView.canvas.Position.First)
	}
}

func TestProofreadingPageFilterNavigationSkipsNonmatchingPages(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	s.pageFilter = proofreadingPageFilterReviewed
	s.pageOwnership = []proofreadingPageOwnership{{Reviewed: true}, {}, {Reviewed: true}, {}, {Reviewed: true}}
	a.pdfView = &pdfViewState{info: document.PDFInfo{Pages: 5}, page: 0}

	if !a.navigateProofreadingPage(1) || a.pdfView.page != 2 {
		t.Fatalf("next reviewed page=%d, want page 2", a.pdfView.page)
	}
	if !a.navigateProofreadingPage(1) || a.pdfView.page != 4 {
		t.Fatalf("next reviewed page=%d, want page 4", a.pdfView.page)
	}
	if a.navigateProofreadingPage(1) {
		t.Fatal("next at the end of the filtered page list unexpectedly wrapped")
	}
	if !a.navigateProofreadingPage(-1) || a.pdfView.page != 2 {
		t.Fatalf("previous reviewed page=%d, want page 2", a.pdfView.page)
	}
}

func TestProofreadingPageFilterThumbnailSelectionClearsFilter(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	s.pageFilter = proofreadingPageFilterReviewed
	s.pageOwnership = []proofreadingPageOwnership{{Reviewed: true}, {}, {Reviewed: true}}
	a.pdfView = &pdfViewState{info: document.PDFInfo{Pages: 3}, page: 0}

	a.selectProofreadingThumbnail(1)
	if s.pageFilter != proofreadingPageFilterAll || a.pdfView.page != 1 || a.pdfView.canvas.Position.First != 1 {
		t.Fatalf("thumbnail selection did not clear filter and select exact page: filter=%v page=%d row=%d", s.pageFilter, a.pdfView.page, a.pdfView.canvas.Position.First)
	}
}

func TestProofreadingPageFilterWithNoMatchesKeepsBothListsEmpty(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	s.pageOwnership = []proofreadingPageOwnership{{Reviewed: true}, {Reviewed: true}, {Reviewed: true}}
	a.pdfView = &pdfViewState{info: document.PDFInfo{Pages: 3}, page: 1}

	a.setProofreadingPageFilter(proofreadingPageFilterUnreviewed)
	if len(s.filteredPageIndices) != 0 || a.pdfView.pageMap == nil || len(a.pdfView.pageMap) != 0 {
		t.Fatalf("empty filter mapping=%v page map=%v; want an explicit empty map", s.filteredPageIndices, a.pdfView.pageMap)
	}
	if a.pdfView.canvas.Position.First != 0 || s.xCanvas.Position.First != 0 {
		t.Fatalf("empty filter left invalid rows selected: A=%d X=%d", a.pdfView.canvas.Position.First, s.xCanvas.Position.First)
	}
	if a.navigateProofreadingPage(1) || a.navigateProofreadingPage(-1) {
		t.Fatal("page navigation should be disabled when the filter has no matches")
	}
}

func TestProofreadingPageFilterReevaluatesAfterReviewStatusChanges(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	if err := s.controller.Model.Review(1, true); err != nil {
		t.Fatal(err)
	}
	if err := s.controller.Model.Review(3, true); err != nil {
		t.Fatal(err)
	}
	a.pdfView = &pdfViewState{info: document.PDFInfo{Pages: 3}, page: 0}
	s.pageFilter = proofreadingPageFilterReviewed
	s.xPageMenuPage = 0
	a.refreshProofreadingModelView()

	a.setProofreadingPageReviewed(false)
	if a.pdfView.page != 2 || len(s.filteredPageIndices) != 1 || s.filteredPageIndices[0] != 2 {
		t.Fatalf("reviewed filter did not advance after current page became unreviewed: page=%d filtered=%v", a.pdfView.page, s.filteredPageIndices)
	}
}

func TestProofreadingPageFilterToolbarButtonsApplyAndClearFilters(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	s.pageOwnership = []proofreadingPageOwnership{{Reviewed: true}, {}, {Reviewed: true}}
	a.pdfView = &pdfViewState{info: document.PDFInfo{Pages: 3}, page: 1}
	var router input.Router
	var ops op.Ops
	labelKey, iconName, width := "proofread.toolbar.reviewed_filter", "check", 118
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(width, 32))}
		layoutProofreadingToolbarPill(gtx, a, unit.Dp(width), iconName, labelKey)
		router.Frame(&ops)
	}
	click := func() {
		point := f32.Pt(float32(width)/2, 16)
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Position: point, Buttons: pointer.ButtonPrimary})
		frame()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point})
		frame()
		frame()
	}
	frame()
	click()
	if s.pageFilter != proofreadingPageFilterReviewed {
		t.Fatalf("reviewed toolbar button selected filter %v", s.pageFilter)
	}
	click()
	if s.pageFilter != proofreadingPageFilterAll {
		t.Fatalf("second reviewed toolbar click selected filter %v, want all", s.pageFilter)
	}
	labelKey, iconName, width = "proofread.toolbar.unreviewed_filter", "clock-3", 126
	frame()
	click()
	if s.pageFilter != proofreadingPageFilterUnreviewed {
		t.Fatalf("unreviewed toolbar button selected filter %v", s.pageFilter)
	}
}
