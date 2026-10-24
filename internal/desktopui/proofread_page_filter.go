package desktopui

import (
	"sort"

	"gioui.org/io/key"
	"gioui.org/layout"
)

type proofreadingPageFilter uint8

const (
	proofreadingPageFilterAll proofreadingPageFilter = iota
	proofreadingPageFilterReviewed
	proofreadingPageFilterUnreviewed
)

func proofreadingFilteredPages(ownership []proofreadingPageOwnership, pageCount int, filter proofreadingPageFilter) []int {
	pageCount = max(0, pageCount)
	pages := make([]int, 0, pageCount)
	for page := 0; page < pageCount; page++ {
		reviewed := page < len(ownership) && ownership[page].Reviewed
		if filter == proofreadingPageFilterReviewed && !reviewed {
			continue
		}
		if filter == proofreadingPageFilterUnreviewed && reviewed {
			continue
		}
		pages = append(pages, page)
	}
	return pages
}

func proofreadingFilterModeAfterClick(current, requested proofreadingPageFilter) proofreadingPageFilter {
	if requested != proofreadingPageFilterReviewed && requested != proofreadingPageFilterUnreviewed {
		return proofreadingPageFilterAll
	}
	if current == requested {
		return proofreadingPageFilterAll
	}
	return requested
}

func proofreadingFilterRowForPage(pages []int, page int) int {
	row := sort.SearchInts(pages, page)
	if row >= len(pages) || pages[row] != page {
		return -1
	}
	return row
}

func proofreadingFilterPageAtRow(pages []int, row int) (int, bool) {
	if row < 0 || row >= len(pages) {
		return 0, false
	}
	return pages[row], true
}

func proofreadingFilterTargetPage(pages []int, current int) int {
	if len(pages) == 0 {
		return -1
	}
	row := sort.SearchInts(pages, current)
	if row < len(pages) {
		return pages[row]
	}
	return pages[0]
}

func proofreadingFilterAdjacentPage(pages []int, current, direction int) (int, bool) {
	if direction != -1 && direction != 1 {
		return 0, false
	}
	row := proofreadingFilterRowForPage(pages, current)
	if row < 0 {
		return 0, false
	}
	row += direction
	return proofreadingFilterPageAtRow(pages, row)
}

func (a *App) proofreadingPageIndices(pageCount int) []int {
	if a == nil || a.proofreading == nil {
		return nil
	}
	state := a.proofreading
	state.filteredPageIndices = proofreadingFilteredPages(state.pageOwnership, pageCount, state.pageFilter)
	return state.filteredPageIndices
}

func proofreadingPDFPageForRow(state *proofreadingWorkspace, pages []int, row int) (int, bool) {
	if state == nil {
		if row < 0 || row >= len(pages) {
			return 0, false
		}
		return pages[row], true
	}
	return proofreadingFilterPageAtRow(pages, row)
}

func (v *pdfViewState) pageForCanvasRow(row int) (int, bool) {
	if v == nil || row < 0 {
		return 0, false
	}
	if v.pageMap != nil {
		return proofreadingFilterPageAtRow(v.pageMap, row)
	}
	if row >= max(1, v.info.Pages) {
		return 0, false
	}
	return row, true
}

func (a *App) syncPDFPageFromCanvas(view *pdfViewState) bool {
	if view == nil {
		return false
	}
	if a == nil || a.proofreading == nil {
		view.pageMap = nil
		return view.syncPageFromCanvas()
	}
	pages := a.proofreadingPageIndices(max(1, view.info.Pages))
	view.pageMap = pages
	return view.syncPageFromCanvas()
}

func (a *App) setProofreadingPageFilter(requested proofreadingPageFilter) {
	if a == nil || a.proofreading == nil {
		return
	}
	state := a.proofreading
	state.pageFilter = proofreadingFilterModeAfterClick(state.pageFilter, requested)
	a.reconcileProofreadingPageFilter()
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) clearProofreadingFocusOutsideFilter(gtx layout.Context) {
	if a == nil || a.proofreading == nil || a.pdfView == nil || a.proofreading.pageFilter == proofreadingPageFilterAll {
		return
	}
	state := a.proofreading
	pages := a.proofreadingPageIndices(max(1, a.pdfView.info.Pages))
	for page := range state.xPageEditors {
		if gtx.Source.Focused(&state.xPageEditors[page]) && proofreadingFilterRowForPage(pages, page) < 0 {
			gtx.Execute(key.FocusCmd{})
			return
		}
	}
}

func (a *App) reconcileProofreadingPageFilter() {
	if a == nil || a.proofreading == nil {
		return
	}
	state := a.proofreading
	pageCount := len(state.pageOwnership)
	view := a.pdfView
	if view != nil && view.info.Pages > 0 {
		pageCount = view.info.Pages
	}
	pages := a.proofreadingPageIndices(pageCount)
	if view == nil {
		return
	}
	view.pageMap = pages
	if len(pages) == 0 {
		view.canvas.Position.First = 0
		view.canvas.Position.Offset = 0
		view.canvas.Position.OffsetLast = 0
		view.canvas.Position.BeforeEnd = false
		state.xCanvas.Position = view.canvas.Position
		state.navigator.initialized = false
		return
	}
	target := proofreadingFilterTargetPage(pages, view.page)
	row := proofreadingFilterRowForPage(pages, target)
	if target != view.page {
		view.canvas.Position.Offset = 0
		view.canvas.Position.OffsetLast = 0
		view.canvas.Position.BeforeEnd = false
	}
	view.canvas.Position.First = row
	state.xCanvas.Position = view.canvas.Position
	if view.syncPage(target) {
		view.trimCacheWindow()
		state.navigator.initialized = false
	}
}

func (a *App) setProofreadingCurrentPage(page int) bool {
	if a == nil || a.proofreading == nil || a.pdfView == nil {
		return false
	}
	state, view := a.proofreading, a.pdfView
	pages := a.proofreadingPageIndices(max(1, view.info.Pages))
	row := proofreadingFilterRowForPage(pages, page)
	if row < 0 {
		return false
	}
	changed := view.page != page
	view.pageMap = pages
	view.canvas.Position.First = row
	view.canvas.Position.Offset = 0
	view.canvas.Position.OffsetLast = 0
	view.canvas.Position.BeforeEnd = false
	state.xCanvas.Position = view.canvas.Position
	if changed {
		view.syncPage(page)
		view.followPDFThumbnailPage(page)
		state.navigator.initialized = false
	}
	view.trimCacheWindow()
	if a.window != nil {
		a.window.Invalidate()
	}
	return changed
}

func (a *App) navigateProofreadingPage(direction int) bool {
	if a == nil || a.proofreading == nil || a.pdfView == nil || a.proofreading.pageFilter == proofreadingPageFilterAll {
		return false
	}
	pages := a.proofreadingPageIndices(max(1, a.pdfView.info.Pages))
	page, ok := proofreadingFilterAdjacentPage(pages, a.pdfView.page, direction)
	if !ok {
		return false
	}
	return a.setProofreadingCurrentPage(page)
}

func (a *App) selectProofreadingThumbnail(page int) {
	if a == nil || a.proofreading == nil || a.pdfView == nil {
		return
	}
	state := a.proofreading
	pages := a.proofreadingPageIndices(max(1, a.pdfView.info.Pages))
	if proofreadingFilterRowForPage(pages, page) < 0 {
		state.pageFilter = proofreadingPageFilterAll
	}
	a.proofreadingPageIndices(max(1, a.pdfView.info.Pages))
	a.setProofreadingCurrentPage(page)
}
