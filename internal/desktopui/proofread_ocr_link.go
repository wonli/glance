package desktopui

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"

	"glance/internal/proofreading"
	"glance/internal/textalign"
)

type proofreadingOCRLinkState struct {
	Active            bool
	APage             int
	BlockID           int
	PathChoiceIndex   int
	BRange            textalign.Range
	SourceAHash       string
	SourceBHash       string
	ResultFingerprint string
	OCRGeneration     uint64
	ModelRevision     uint64
	XFirstPage        int
	XLastPage         int
	XScrollPending    bool
	ASourceRow        int
	ASourceOffset     int
	PulseAt           time.Time
}

type proofreadingOCRBlockRange struct {
	PathChoiceIndex int
	Range           textalign.Range
}

func proofreadingOCRBlockChoice(page proofreadingOCRPage, blockID int) []proofreadingOCRBlockRange {
	if blockID < 0 || (page.result.Status != "matched" && page.result.Status != "ambiguous") {
		return nil
	}
	choices := make([]proofreadingOCRBlockRange, 0, len(page.result.PathChoices))
	seen := make(map[proofreadingOCRBlockRange]struct{})
	for pathIndex, candidate := range page.result.PathChoices {
		for _, evidence := range candidate.Evidence {
			if evidence.BlockID != blockID || evidence.Range.Start < 0 || evidence.Range.End <= evidence.Range.Start {
				continue
			}
			choice := proofreadingOCRBlockRange{PathChoiceIndex: pathIndex, Range: evidence.Range}
			if _, ok := seen[choice]; ok {
				continue
			}
			seen[choice] = struct{}{}
			choices = append(choices, choice)
		}
	}
	return choices
}

func proofreadingOCRChoiceIndex(current, delta, choiceCount int) int {
	if choiceCount <= 0 {
		return -1
	}
	current = ((current % choiceCount) + choiceCount) % choiceCount
	return ((current+delta)%choiceCount + choiceCount) % choiceCount
}

func proofreadingOCRBlockResultFingerprint(page proofreadingOCRPage, blockID int) string {
	choices := proofreadingOCRBlockChoice(page, blockID)
	if len(choices) == 0 {
		return ""
	}
	var value strings.Builder
	fmt.Fprintf(&value, "%d|%s|%d|", page.result.Page, page.result.Status, blockID)
	for _, choice := range choices {
		fmt.Fprintf(&value, "%d:%d:%d;", choice.PathChoiceIndex, choice.Range.Start, choice.Range.End)
	}
	hash := sha256.Sum256([]byte(value.String()))
	return fmt.Sprintf("%x", hash[:])
}

func proofreadingOCRLinkCurrent(link proofreadingOCRLinkState, scan *proofreadingOCRScan, state *proofreadingWorkspace, modelRevision uint64) bool {
	if !link.Active || scan == nil || state == nil || scan.stage != proofreadingOCRComplete ||
		link.APage < 0 || link.APage >= len(scan.pages) || link.OCRGeneration != scan.generation ||
		link.ModelRevision != modelRevision || link.SourceAHash == "" || link.SourceAHash != state.aSourceHash ||
		link.SourceBHash == "" || link.SourceBHash != state.sourceBHash ||
		link.SourceAHash != scan.sourceAHash || link.SourceBHash != scan.sourceBHash ||
		!scan.markdownSnapshotSet || scan.markdownSnapshot != state.bDraftText {
		return false
	}
	page := scan.pages[link.APage]
	if link.ResultFingerprint == "" || link.ResultFingerprint != proofreadingOCRBlockResultFingerprint(page, link.BlockID) {
		return false
	}
	foundChoice := false
	for _, choice := range proofreadingOCRBlockChoice(page, link.BlockID) {
		if choice.PathChoiceIndex == link.PathChoiceIndex && choice.Range == link.BRange {
			foundChoice = true
			break
		}
	}
	if !foundChoice {
		return false
	}
	segments := proofreadingOCRRangeSegments(state.xPages, link.BRange)
	if len(segments) == 0 || segments[0].PageIndex != link.XFirstPage || segments[len(segments)-1].PageIndex != link.XLastPage {
		return false
	}
	return true
}

func proofreadingOCRLinkSourcePinned(link proofreadingOCRLinkState, view *pdfViewState, pages []int) bool {
	if !link.Active || view == nil || view.page != link.APage {
		return false
	}
	row := proofreadingFilterRowForPage(pages, link.APage)
	return row >= 0 && row == link.ASourceRow && view.canvas.Position.First == row && view.canvas.Position.Offset == link.ASourceOffset
}

func (page *proofreadingOCRPage) blockButton(blockID int) *widget.Clickable {
	if page == nil {
		return nil
	}
	if page.blockButtons == nil {
		page.blockButtons = make(map[int]*widget.Clickable)
	}
	button := page.blockButtons[blockID]
	if button == nil {
		button = &widget.Clickable{}
		page.blockButtons[blockID] = button
	}
	return button
}

func (a *App) proofreadingOCRLinkRevision() uint64 {
	if a == nil || a.proofreading == nil || a.proofreading.controller == nil || a.proofreading.controller.Model == nil {
		return 0
	}
	return a.proofreading.controller.Model.Revision()
}

func (a *App) proofreadingOCRLinkValid() bool {
	if a == nil || a.proofreading == nil || a.proofreading.ocrScan == nil {
		return false
	}
	state := a.proofreading
	return proofreadingOCRLinkCurrent(state.ocrScan.link, state.ocrScan, state, a.proofreadingOCRLinkRevision())
}

func (a *App) validateProofreadingOCRLink() bool {
	if a == nil || a.proofreading == nil || a.proofreading.ocrScan == nil || !a.proofreading.ocrScan.link.Active {
		return false
	}
	if a.proofreadingOCRLinkValid() {
		return true
	}
	a.clearProofreadingOCRLink(true)
	return false
}

func (a *App) activateProofreadingOCRBlock(pageIndex, blockID int) bool {
	if a == nil || a.proofreading == nil || a.proofreading.ocrScan == nil || a.pdfView == nil {
		return false
	}
	state, scan, view := a.proofreading, a.proofreading.ocrScan, a.pdfView
	if scan.stage != proofreadingOCRComplete || pageIndex < 0 || pageIndex >= len(scan.pages) ||
		pageIndex >= len(state.xPages) || scan.pages[pageIndex].result.Page != pageIndex+1 {
		return false
	}
	choices := proofreadingOCRBlockChoice(scan.pages[pageIndex], blockID)
	if len(choices) == 0 {
		return false
	}
	choice := choices[0]
	segments := proofreadingOCRRangeSegments(state.xPages, choice.Range)
	if len(segments) == 0 {
		return false
	}
	pageCount := max(1, view.info.Pages)
	pages := a.proofreadingPageIndices(pageCount)
	hiddenTarget := false
	for _, segment := range segments {
		if proofreadingFilterRowForPage(pages, segment.PageIndex) < 0 {
			hiddenTarget = true
			break
		}
	}
	if hiddenTarget {
		state.pageFilter = proofreadingPageFilterAll
		pages = a.proofreadingPageIndices(pageCount)
	}
	row := proofreadingFilterRowForPage(pages, pageIndex)
	if row < 0 {
		return false
	}
	view.pageMap = pages
	view.canvas.Position.First = row
	view.canvas.Position.Offset = 0
	view.canvas.Position.OffsetLast = 0
	view.canvas.Position.BeforeEnd = false
	view.syncPage(pageIndex)
	view.followPDFThumbnailPage(pageIndex)
	state.page = pageIndex
	state.navigator.initialized = false
	link := proofreadingOCRLinkState{
		Active: true, APage: pageIndex, BlockID: blockID, PathChoiceIndex: choice.PathChoiceIndex,
		BRange: choice.Range, SourceAHash: state.aSourceHash, SourceBHash: state.sourceBHash,
		ResultFingerprint: proofreadingOCRBlockResultFingerprint(scan.pages[pageIndex], blockID),
		OCRGeneration:     scan.generation, ModelRevision: a.proofreadingOCRLinkRevision(),
		XFirstPage: segments[0].PageIndex, XLastPage: segments[len(segments)-1].PageIndex,
		ASourceRow: row, ASourceOffset: view.canvas.Position.Offset, PulseAt: time.Now(),
	}
	scan.link = link
	scan.highlight = proofreadingOCRHighlightState{
		Active: true, APage: pageIndex, BlockID: blockID, BRange: choice.Range,
		SourceAHash: link.SourceAHash, SourceBHash: link.SourceBHash,
		OCRGeneration: link.OCRGeneration, ModelRevision: link.ModelRevision,
		PulseAt: link.PulseAt,
	}
	scan.link.XScrollPending = !a.positionProofreadingOCRLinkTarget(link)
	if view != nil {
		view.trimCacheWindow()
	}
	if a.window != nil {
		a.window.Invalidate()
	}
	return true
}

func (a *App) moveProofreadingOCRLinkChoice(delta int) bool {
	if a == nil || a.proofreading == nil || a.proofreading.ocrScan == nil || !a.validateProofreadingOCRLink() {
		return false
	}
	state, scan := a.proofreading, a.proofreading.ocrScan
	link := scan.link
	choices := proofreadingOCRBlockChoice(scan.pages[link.APage], link.BlockID)
	current := -1
	for i, choice := range choices {
		if choice.PathChoiceIndex == link.PathChoiceIndex && choice.Range == link.BRange {
			current = i
			break
		}
	}
	next := proofreadingOCRChoiceIndex(current, delta, len(choices))
	if next < 0 || next == current {
		return false
	}
	choice := choices[next]
	segments := proofreadingOCRRangeSegments(state.xPages, choice.Range)
	if len(segments) == 0 {
		a.clearProofreadingOCRLink(true)
		return false
	}
	link.PathChoiceIndex = choice.PathChoiceIndex
	link.BRange = choice.Range
	link.PulseAt = time.Now()
	link.XFirstPage = segments[0].PageIndex
	link.XLastPage = segments[len(segments)-1].PageIndex
	scan.link = link
	scan.highlight.BRange = choice.Range
	scan.highlight.PulseAt = link.PulseAt
	scan.link.XScrollPending = !a.positionProofreadingOCRLinkTarget(link)
	if a.window != nil {
		a.window.Invalidate()
	}
	return true
}

func (a *App) positionProofreadingOCRLinkTarget(link proofreadingOCRLinkState) bool {
	if a == nil || a.proofreading == nil || a.pdfView == nil || !link.Active {
		return false
	}
	state, view := a.proofreading, a.pdfView
	segments := proofreadingOCRRangeSegments(state.xPages, link.BRange)
	if len(segments) == 0 {
		return false
	}
	first := segments[0]
	pageCount := max(1, view.info.Pages)
	pages := a.proofreadingPageIndices(pageCount)
	hiddenTarget := false
	for _, segment := range segments {
		if proofreadingFilterRowForPage(pages, segment.PageIndex) < 0 {
			hiddenTarget = true
			break
		}
	}
	if hiddenTarget {
		state.pageFilter = proofreadingPageFilterAll
		pages = a.proofreadingPageIndices(pageCount)
		sourceRow := proofreadingFilterRowForPage(pages, link.APage)
		if sourceRow < 0 {
			return false
		}
		view.canvas.Position.First = sourceRow
		view.canvas.Position.Offset = link.ASourceOffset
		view.canvas.Position.OffsetLast = 0
		view.canvas.Position.BeforeEnd = false
		link.ASourceRow = sourceRow
		if state.ocrScan != nil && state.ocrScan.link.Active && state.ocrScan.link.APage == link.APage {
			state.ocrScan.link.ASourceRow = sourceRow
		}
	}
	row := proofreadingFilterRowForPage(pages, first.PageIndex)
	if row < 0 {
		return false
	}
	view.pageMap = pages
	state.xCanvas.Axis = layout.Vertical
	state.xCanvas.Position.First = row
	state.xCanvas.Position.Offset = 0
	state.xCanvas.Position.OffsetLast = 0
	state.xCanvas.Position.BeforeEnd = false
	applied := a.positionProofreadingOCRTextRange(first)
	if a.window != nil {
		a.window.Invalidate()
	}
	return applied
}

func (a *App) positionProofreadingOCRTextRange(segment proofreadingOCRRangeSegment) bool {
	state := a.proofreading
	if state == nil || state.controller == nil || state.controller.Model == nil || segment.PageIndex < 0 || segment.PageIndex >= len(state.xPageScrollOffsets) || segment.PageIndex >= len(state.xPages) {
		return false
	}
	if state.controller.Layout.Revision != state.controller.Model.Revision() || state.controller.Layout.StyleRevision != state.xTextSizeStep {
		return false
	}
	page := state.xPages[segment.PageIndex]
	shaped, ok := state.controller.Layout.Pages[proofreading.PageID(segment.PageIndex+1)]
	if !ok || !strings.HasPrefix(shaped.Text, page.Content) {
		state.xPageScrollOffsets[segment.PageIndex] = 0
		return false
	}
	lineTop := 0.0
	found := false
	for _, line := range shaped.Lines {
		if segment.Start >= line.Start && segment.Start < line.End {
			lineTop = line.Top
			found = true
			break
		}
	}
	if !found && segment.Start >= len([]rune(page.Content)) && len(shaped.Lines) > 0 {
		lineTop = shaped.Lines[len(shaped.Lines)-1].Top
	}
	_, marginY := proofreadingTextMargins(page.Width, page.Height)
	viewport := max(1.0, page.Height-2*marginY)
	contentHeight := proofreadingPageContentHeight(shaped, page.Content)
	state.xPageScrollOffsets[segment.PageIndex] = clampProofreadingScrollTop(max(0, lineTop-24), viewport, contentHeight)
	return true
}

func (a *App) applyPendingProofreadingOCRLinkScroll(pageIndex int) {
	if a == nil || a.proofreading == nil || a.proofreading.ocrScan == nil {
		return
	}
	scan := a.proofreading.ocrScan
	if !scan.link.Active || !scan.link.XScrollPending || scan.link.XFirstPage != pageIndex {
		return
	}
	segments := proofreadingOCRRangeSegments(a.proofreading.xPages, scan.link.BRange)
	for _, segment := range segments {
		if segment.PageIndex == pageIndex && a.positionProofreadingOCRTextRange(segment) {
			scan.link.XScrollPending = false
			return
		}
	}
}

func (a *App) clearProofreadingOCRLink(syncXToA bool) {
	if a == nil || a.proofreading == nil {
		return
	}
	state := a.proofreading
	if state.ocrScan != nil {
		state.ocrScan.link = proofreadingOCRLinkState{}
		state.ocrScan.highlight = proofreadingOCRHighlightState{}
	}
	if syncXToA && a.pdfView != nil {
		state.xCanvas.Position = a.pdfView.canvas.Position
		state.xCanvas.Axis = layout.Vertical
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) syncProofreadingXPositionToA(view *pdfViewState) bool {
	if a == nil || a.proofreading == nil || view == nil {
		return false
	}
	state := a.proofreading
	if a.validateProofreadingOCRLink() {
		return false
	}
	xPosition, aPosition := state.xCanvas.Position, view.canvas.Position
	if xPosition.First == aPosition.First && xPosition.Offset == aPosition.Offset {
		return false
	}
	view.canvas.Position.First = max(0, min(len(view.pageMap)-1, xPosition.First))
	view.canvas.Position.Offset = xPosition.Offset
	view.canvas.Position.OffsetLast = xPosition.OffsetLast
	view.canvas.Position.BeforeEnd = xPosition.BeforeEnd
	a.syncPDFPageFromCanvas(view)
	return true
}
