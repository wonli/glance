package desktopui

import (
	"context"
	"strings"
	"time"
	"unicode"

	"glance/internal/agentsettings"
	"glance/internal/i18n"
	"glance/internal/ocr"
	"glance/internal/proofreading"
)

const proofreadingOCROverflowMinimumRunes = 8

type proofreadingOCROverflowTail struct {
	Start        int
	RuneCount    int
	MatchedRunes int
}

type proofreadingOCROverflowRequest struct {
	mainGeneration     uint64
	overflowGeneration uint64
	preComparison      bool
	sourceHash         string
	modelRevision      uint64
	comparison         proofreadingOCRStepComparison
	currentText        string
	nextText           string
}

type proofreadingOCRBoundarySkip struct {
	page       int
	sourceHash string
	draftHash  string
}

func proofreadingOCROverflowTailForComparison(comparison proofreadingOCRStepComparison) (proofreadingOCROverflowTail, bool) {
	if !comparison.HasEvidence || !comparison.Complete || comparison.AlignmentLimited || comparison.Result.Status != "matched" ||
		comparison.BStart < 0 || comparison.BEnd < comparison.BStart {
		return proofreadingOCROverflowTail{}, false
	}
	bRunes := []rune(comparison.BText)
	if comparison.BEnd-comparison.BStart != len(bRunes) || len(bRunes) == 0 {
		return proofreadingOCROverflowTail{}, false
	}
	bOnly := make(map[int]bool)
	for _, frame := range comparison.Frames {
		if frame.Kind == proofreadingOCRDiffBOnly {
			bOnly[frame.BPosition] = true
		}
	}
	start := -1
	matched := 0
	for index := len(bRunes) - 1; index >= 0; index-- {
		if isProofreadingOCRWhitespace(bRunes[index]) {
			continue
		}
		if !bOnly[comparison.BStart+index] {
			break
		}
		start = index
		matched++
	}
	if start <= 0 || matched < proofreadingOCROverflowMinimumRunes {
		return proofreadingOCROverflowTail{}, false
	}
	return proofreadingOCROverflowTail{Start: start, RuneCount: len(bRunes) - start, MatchedRunes: matched}, true
}

func proofreadingOCRConfirmedOverflowTail(comparison proofreadingOCRStepComparison, next ocr.Layout) (proofreadingOCROverflowTail, bool) {
	tail, ok := proofreadingOCROverflowTailForComparison(comparison)
	if !ok {
		return proofreadingOCROverflowTail{}, false
	}
	bRunes := []rune(comparison.BText)
	suffix := newProofreadingOCRDiffSource(string(bRunes[tail.Start:]))
	nextText := proofreadingOCRPageText(next).text
	nextSource := newProofreadingOCRDiffSource(nextText)
	if len(suffix.characters) != tail.MatchedRunes || len(nextSource.characters) < len(suffix.characters) {
		return proofreadingOCROverflowTail{}, false
	}
	for i, value := range suffix.characters {
		if nextSource.characters[i] != value {
			return proofreadingOCROverflowTail{}, false
		}
	}
	return tail, true
}

// A complete, exact current-page prefix identifies its end without fetching the
// next PDF page. Map back to raw runes so whitespace and every tail character
// retain their original order when ownership moves to the next box.
func proofreadingOCRExactPageTail(current ocr.Layout, bText string) (proofreadingOCROverflowTail, bool) {
	if current.Width <= 0 || current.Height <= 0 {
		return proofreadingOCROverflowTail{}, false
	}
	source := newProofreadingOCRDiffSource(proofreadingOCRPageText(current).text)
	target := newProofreadingOCRDiffSource(bText)
	if len(source.characters) < proofreadingOCROverflowMinimumRunes || len(target.characters) < len(source.characters) {
		return proofreadingOCROverflowTail{}, false
	}
	for i, r := range source.characters {
		if target.characters[i] != r {
			return proofreadingOCROverflowTail{}, false
		}
	}
	end := len([]rune(bText))
	start := end
	if len(target.characters) > len(source.characters) {
		start = target.starts[len(source.characters)]
	}
	return proofreadingOCROverflowTail{Start: start, RuneCount: end - start, MatchedRunes: len(source.characters)}, true
}

func proofreadingOCRBoundaryTailFromPages(current, next ocr.Layout, bText string) (proofreadingOCROverflowTail, bool) {
	if exact, ok := proofreadingOCRExactPageTail(current, bText); ok && exact.RuneCount > 0 {
		return exact, true
	}
	// For an inexact current page, anchor the split at both PDF pages.
	if current.Width <= 0 || current.Height <= 0 || next.Width <= 0 || next.Height <= 0 {
		return proofreadingOCROverflowTail{}, false
	}
	bRunes := []rune(bText)
	bSource := newProofreadingOCRDiffSource(bText)
	currentSource := newProofreadingOCRDiffSource(proofreadingOCRPageText(current).text)
	nextSource := newProofreadingOCRDiffSource(proofreadingOCRPageText(next).text)
	if len(bSource.characters) < proofreadingOCROverflowMinimumRunes*2 ||
		len(currentSource.characters) < proofreadingOCROverflowMinimumRunes ||
		len(nextSource.characters) < proofreadingOCROverflowMinimumRunes {
		return proofreadingOCROverflowTail{}, false
	}
	currentStarts := proofreadingOCRPrefixMatches(bSource.characters, currentSource.characters)
	if len(currentStarts) == 0 || currentStarts[0] < proofreadingOCROverflowMinimumRunes {
		return proofreadingOCROverflowTail{}, false
	}

	nextMatches := proofreadingOCRPrefixMatches(bSource.characters, nextSource.characters)
	reversedCurrent := proofreadingOCRReverseRunes(currentSource.characters)
	reversedB := proofreadingOCRReverseRunes(bSource.characters)
	currentMatches := proofreadingOCRPrefixMatches(reversedB, reversedCurrent)
	bestStart, bestScore, bestCurrent, bestMatched := -1, -1, -1, 0
	ambiguous := false
	for start := proofreadingOCROverflowMinimumRunes; start+proofreadingOCROverflowMinimumRunes <= len(bSource.characters); start++ {
		nextOverlap := nextMatches[start]
		if nextOverlap < proofreadingOCROverflowMinimumRunes {
			continue
		}
		currentOverlap := currentMatches[len(bSource.characters)-start]
		if currentOverlap < proofreadingOCROverflowMinimumRunes {
			continue
		}
		rawStart := bSource.starts[start]
		if rawStart <= 0 {
			continue
		}
		score := currentOverlap + nextOverlap
		if score > bestScore || (score == bestScore && currentOverlap > bestCurrent) {
			bestStart, bestScore, bestCurrent = rawStart, score, currentOverlap
			bestMatched = min(currentOverlap, nextOverlap)
			ambiguous = false
		} else if score == bestScore && currentOverlap == bestCurrent && rawStart != bestStart {
			ambiguous = true
		}
	}
	if bestStart <= 0 || ambiguous {
		return proofreadingOCROverflowTail{}, false
	}
	return proofreadingOCROverflowTail{
		Start: bestStart, RuneCount: len(bRunes) - bestStart, MatchedRunes: bestMatched,
	}, true
}

func proofreadingOCRPrefixMatches(text, pattern []rune) []int {
	combined := make([]rune, 0, len(pattern)+1+len(text))
	combined = append(combined, pattern...)
	combined = append(combined, -1)
	combined = append(combined, text...)
	z := make([]int, len(combined))
	left, right := 0, 0
	for index := 1; index < len(combined); index++ {
		if index < right {
			z[index] = min(right-index, z[index-left])
		}
		for index+z[index] < len(combined) && combined[z[index]] == combined[index+z[index]] {
			z[index]++
		}
		if index+z[index] > right {
			left, right = index, index+z[index]
		}
	}
	matches := make([]int, len(text))
	for index := range text {
		matches[index] = min(len(pattern), z[len(pattern)+1+index])
	}
	return matches
}

func proofreadingOCRReverseRunes(source []rune) []rune {
	reversed := make([]rune, len(source))
	for index, value := range source {
		reversed[len(source)-1-index] = value
	}
	return reversed
}

func proofreadingOCRPagePrefixMatchesText(layout ocr.Layout, text string) bool {
	pageSource := newProofreadingOCRDiffSource(proofreadingOCRPageText(layout).text)
	textSource := newProofreadingOCRDiffSource(text)
	if len(pageSource.characters) < proofreadingOCROverflowMinimumRunes || len(textSource.characters) == 0 {
		return false
	}
	matches := proofreadingOCRPrefixMatches(textSource.characters, pageSource.characters)
	return len(matches) > 0 && matches[0] >= proofreadingOCROverflowMinimumRunes
}

func isProofreadingOCRWhitespace(value rune) bool {
	return unicode.IsSpace(value)
}

func (a *App) prepareProofreadingOCRStepComparison(comparison proofreadingOCRStepComparison) bool {
	if a == nil || a.proofreading == nil || a.proofreading.ocrStep == nil {
		return false
	}
	if !proofreadingOCRStepComparisonValid(a.proofreading, comparison) {
		return false
	}
	if strings.TrimSpace(comparison.BText) == "" {
		step := a.proofreading.ocrStep
		if page, ok := step.pages[comparison.APage]; ok && step.pageSourceHash[comparison.APage] == a.proofreading.aSourceHash {
			return a.fillEmptyProofreadingBPageFromOCR(comparison.APage, page.layout)
		}
		step.autoCompareAfterOCR = true
		step.phase = proofreadingOCRStepRecognized
		a.requestProofreadingOCRStep()
		if !step.active() {
			step.autoCompareAfterOCR = false
			return false
		}
		return true
	}
	return a.startProofreadingOCROverflowCheck(comparison)
}

func (a *App) presentProofreadingOCRStepComparison(comparison proofreadingOCRStepComparison) {
	if a == nil || a.proofreading == nil || a.proofreading.ocrStep == nil ||
		!proofreadingOCRStepComparisonValid(a.proofreading, comparison) {
		return
	}
	step := a.proofreading.ocrStep
	pair := proofreadingOCRStepPair{APage: comparison.APage, BPage: comparison.BPage}
	step.comparisons[pair] = comparison
	_ = saveProofreadingOCRComparisonCache(comparison)
	startProofreadingOCRDiffPlayback(step, comparison, time.Now())
	step.phase = proofreadingOCRStepCompared
	step.err = ""
	step.status = "proofread.ai.step.compared"
}

func (a *App) fillEmptyProofreadingBPageFromOCR(pageIndex int, layout ocr.Layout) bool {
	if a == nil || a.proofreading == nil || a.proofreading.controller == nil || pageIndex < 0 || pageIndex >= len(a.proofreading.xPages) {
		return false
	}
	state := a.proofreading
	view := a.ensurePDFView()
	if view == nil || view.page != pageIndex || a.proofreadingCurrentBPage() != pageIndex ||
		strings.TrimSpace(state.xPages[pageIndex].Content) != "" {
		return false
	}
	pageID := proofreading.PageID(pageIndex + 1)
	meta, ok := state.controller.Model.Page(pageID)
	if !ok || meta.Reviewed || meta.Locked || meta.Manual || proofreadingOCRPageHasTypedOrProtectedText(state.controller.Model, pageID) {
		return false
	}
	recognized := strings.TrimSpace(proofreadingOCRPagePreview(layout))
	if recognized == "" {
		return false
	}
	if err := state.controller.Model.ReplacePage(pageID, recognized); err != nil {
		state.draftPersistError = err.Error()
		return false
	}
	a.invalidateProofreadingOCRStepComparisons()
	state.ocrStep.cancelTask()
	a.proofreadingModelChanged(true)
	a.compareProofreadingOCRStep()
	return true
}

func (a *App) startProofreadingOCROverflowCheck(comparison proofreadingOCRStepComparison) bool {
	if a == nil || a.proofreading == nil || a.proofreading.controller == nil || a.pdfDoc == nil ||
		comparison.APage < 0 || comparison.APage != comparison.BPage ||
		comparison.BPage+1 >= len(a.proofreading.xPages) {
		return false
	}
	state := a.proofreading
	step := state.ocrStep
	if step == nil || comparison.SourceHash != state.aSourceHash || comparison.BText != state.xPages[comparison.BPage].Content ||
		comparison.BStart != state.xPages[comparison.BPage].ContentStart || comparison.BEnd != state.xPages[comparison.BPage].ContentEnd {
		return false
	}
	view := a.ensurePDFView()
	if view == nil || view.page != comparison.APage || a.proofreadingCurrentBPage() != comparison.BPage {
		return false
	}
	currentID := proofreading.PageID(comparison.BPage + 1)
	nextID := proofreading.PageID(comparison.BPage + 2)
	currentMeta, currentOK := state.controller.Model.Page(currentID)
	nextMeta, nextOK := state.controller.Model.Page(nextID)
	if !currentOK || !nextOK || currentMeta.Reviewed || currentMeta.Locked ||
		nextMeta.Reviewed || nextMeta.Locked ||
		proofreadingOCRPageHasTypedOrProtectedText(state.controller.Model, nextID) {
		return false
	}
	currentOCR, currentExists := step.pages[comparison.APage]
	nextOCR, nextExists := step.pages[comparison.BPage+1]
	if !currentExists || !nextExists || step.pageSourceHash[comparison.APage] != state.aSourceHash ||
		step.pageSourceHash[comparison.BPage+1] != state.aSourceHash {
		return false
	}
	candidate, ok := proofreadingOCRBoundaryTailFromPages(currentOCR.layout, nextOCR.layout, comparison.BText)
	if !ok {
		candidate, ok = proofreadingOCROverflowTailForComparison(comparison)
		if !ok {
			return false
		}
		confirmed, confirmedOK := proofreadingOCRConfirmedOverflowTail(comparison, nextOCR.layout)
		if !confirmedOK || confirmed != candidate {
			return false
		}
	}
	if state.controller.Model.BoundaryReturnableRunes(currentID) < candidate.RuneCount {
		return false
	}
	step.phase = proofreadingOCRStepCheckingBoundary
	return a.applyProofreadingOCROverflow(step.generation, 0, comparison, candidate, nextOCR.layout, state.controller.Model.Revision())
}

func (a *App) startProofreadingOCRBoundaryPrefetch(pageIndex int, page proofreadingXPage) bool {
	if a == nil || a.proofreading == nil || a.proofreading.controller == nil || a.pdfDoc == nil || pageIndex < 0 ||
		pageIndex+1 >= len(a.proofreading.xPages) || strings.TrimSpace(page.Content) == "" {
		return false
	}
	state := a.proofreading
	step := state.ocrStep
	view := a.ensurePDFView()
	if step == nil || view == nil || view.page != pageIndex || a.proofreadingCurrentBPage() != pageIndex {
		return false
	}
	currentID := proofreading.PageID(pageIndex + 1)
	nextID := proofreading.PageID(pageIndex + 2)
	currentMeta, currentOK := state.controller.Model.Page(currentID)
	nextMeta, nextOK := state.controller.Model.Page(nextID)
	if !currentOK || !nextOK || currentMeta.Reviewed || currentMeta.Locked ||
		nextMeta.Reviewed || nextMeta.Locked ||
		proofreadingOCRPageHasTypedOrProtectedText(state.controller.Model, nextID) ||
		state.controller.Model.BoundaryReturnableRunes(currentID) == 0 {
		return false
	}
	if nextPage, exists := step.pages[pageIndex+1]; exists && step.pageSourceHash[pageIndex+1] == state.aSourceHash &&
		nextPage.layout.Width > 0 && nextPage.layout.Height > 0 {
		return false
	}
	endpoint, endpointErr := proofreadingOCREndpointFromEnvironment()
	comparison := proofreadingOCRStepComparison{
		APage: pageIndex, BPage: pageIndex, SourceHash: state.aSourceHash,
		BText: page.Content, BStart: page.ContentStart, BEnd: page.ContentEnd,
	}
	currentOCR, currentOCRFound := step.pages[pageIndex]
	if !currentOCRFound || step.pageSourceHash[pageIndex] != state.aSourceHash {
		return false
	}
	if exact, ok := proofreadingOCRExactPageTail(currentOCR.layout, page.Content); ok && exact.RuneCount == 0 {
		return false
	}
	step.page, step.bPage = pageIndex, pageIndex
	step.phase = proofreadingOCRStepCheckingBoundary
	step.overflowGeneration++
	request := &proofreadingOCROverflowRequest{
		mainGeneration: step.generation, overflowGeneration: step.overflowGeneration,
		preComparison: true, sourceHash: state.aSourceHash, modelRevision: state.controller.Model.Revision(),
		comparison: comparison, currentText: page.Content, nextText: state.xPages[pageIndex+1].Content,
	}
	step.overflowRequest = request
	if step.overflowCancel != nil {
		step.overflowCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	step.overflowCancel = cancel
	window, shutdown := a.window, a.shutdown
	pageCount, sourceHash, nextPage, document := view.info.Pages, state.aSourceHash, pageIndex+1, a.pdfDoc
	store := a.proofreadingStore()
	credentials := state.ocrCredentialStore
	if credentials == nil {
		credentials = agentsettings.SystemCredentialStore{}
	}
	publish := func(result proofreadingOCRStepResult) {
		result.generation = request.mainGeneration
		result.overflowGeneration = request.overflowGeneration
		result.overflowPrefetch = true
		result.sourceHash = request.sourceHash
		if result.ocrPage != nil {
			layout, layoutErr := result.ocrPage.Layout()
			if layoutErr != nil {
				result.err = layoutErr
			} else {
				result.ocrLayout = &layout
				if request.preComparison {
					if candidate, ok := proofreadingOCRBoundaryTailFromPages(currentOCR.layout, layout, request.currentText); ok {
						result.overflowCandidate = &candidate
					}
				}
			}
		}
		select {
		case step.results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	}
	if !a.launchManagedBackground(func() {
		cached, cacheHit, cacheErr := loadProofreadingOCRCachePage(sourceHash, ocr.DefaultModel, ocr.Options{}, nextPage+1, pageCount)
		if ctx.Err() != nil {
			return
		}
		if cacheHit {
			publish(proofreadingOCRStepResult{page: nextPage, sourceHash: sourceHash, ocrPage: &cached, cacheHit: true, cacheErr: cacheErr})
			return
		}
		if endpointErr != nil {
			publish(proofreadingOCRStepResult{page: nextPage, err: endpointErr, cacheErr: cacheErr})
			return
		}
		_, token, found, authErr := loadProofreadingOCRAuthentication(ctx, credentials, store, endpoint)
		if ctx.Err() != nil {
			return
		}
		if !found {
			publish(proofreadingOCRStepResult{page: nextPage, needToken: true, endpoint: endpoint, cacheErr: cacheErr, err: authErr})
			return
		}
		err := runProofreadingOCRPageStep(ctx, document, nextPage, pageCount, sourceHash, token, endpoint, false, credentials, store, func(proofreadingOCRStepPhase, int, int) {}, publish)
		if err != nil && ctx.Err() == nil {
			publish(proofreadingOCRStepResult{page: nextPage, err: err})
		}
	}) {
		cancel()
		step.overflowCancel = nil
		step.overflowRequest = nil
		step.phase = proofreadingOCRStepRecognized
		return false
	}
	return true
}

func (a *App) setProofreadingOCRBoundarySkip(request *proofreadingOCROverflowRequest) {
	if a == nil || a.proofreading == nil || a.proofreading.ocrStep == nil || request == nil {
		return
	}
	a.proofreading.ocrStep.boundarySkip = &proofreadingOCRBoundarySkip{
		page: request.comparison.APage, sourceHash: request.sourceHash,
		draftHash: proofreadingOCRDraftSnapshotHash(request.currentText),
	}
}

func (a *App) consumeProofreadingOCRBoundarySkip(page int, sourceHash, draftHash string) bool {
	if a == nil || a.proofreading == nil || a.proofreading.ocrStep == nil {
		return false
	}
	step := a.proofreading.ocrStep
	skip := step.boundarySkip
	if skip == nil {
		return false
	}
	step.boundarySkip = nil
	return skip.page == page && skip.sourceHash == sourceHash && skip.draftHash == draftHash
}

func (a *App) openProofreadingOCROverflowTokenDialog(endpoint string, request *proofreadingOCROverflowRequest) bool {
	if a == nil || a.proofreading == nil || a.modal.Visible || !a.proofreadingOCROverflowRequestCurrent(request) {
		return false
	}
	a.modal.ShowInputMessage(
		"proofread.ocr.token",
		i18n.Message{ID: "proofread.ai.token.title"},
		i18n.Message{ID: "proofread.ai.step.token_body"},
		"",
		i18n.Message{ID: "proofread.ai.token.hint"},
		func(action string) {
			token := strings.TrimSpace(a.modal.Input.Text())
			a.modal.Input.SetText("")
			a.modal.Input.Mask = 0
			if action != "start" && action != "confirm" {
				a.modal.Hide()
				if request.preComparison && a.proofreadingOCROverflowRequestCurrent(request) {
					a.setProofreadingOCRBoundarySkip(request)
				}
				a.cancelProofreadingOCROverflowRequest(request)
				if request.preComparison {
					a.compareProofreadingOCRStep()
				} else {
					a.presentProofreadingOCRStepComparison(request.comparison)
				}
				return
			}
			if token == "" {
				a.modal.SetMessageRef(i18n.Message{ID: "proofread.ai.token.required"})
				a.modal.Input.Mask = '•'
				a.modal.FocusInputRequested = true
				return
			}
			a.modal.Hide()
			a.startProofreadingOCROverflowPageStep(request, token, endpoint, true)
		},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
		modalDialogActionSpec{ID: "start", LabelRef: i18n.Message{ID: "proofread.ai.step.recognize"}},
	)
	a.modal.Input.Mask = '•'
	a.modal.CardWidthDp = 470
	a.modal.FocusInputRequested = true
	return true
}

func (a *App) startProofreadingOCROverflowPageStep(
	request *proofreadingOCROverflowRequest,
	token, endpoint string,
	persistCredential bool,
) {
	if !a.proofreadingOCROverflowRequestCurrent(request) {
		a.cancelProofreadingOCROverflowRequest(request)
		if request != nil && request.preComparison {
			a.compareProofreadingOCRStep()
		} else if request != nil {
			a.presentProofreadingOCRStepComparison(request.comparison)
		}
		return
	}
	state := a.proofreading
	step := state.ocrStep
	view := a.ensurePDFView()
	pageIndex, pageCount, sourceHash, document := request.comparison.APage+1, view.info.Pages, request.sourceHash, a.pdfDoc
	store := a.proofreadingStore()
	credentials := state.ocrCredentialStore
	if credentials == nil {
		credentials = agentsettings.SystemCredentialStore{}
	}
	currentOCR, currentOCRFound := step.pages[request.comparison.APage]
	if !currentOCRFound || step.pageSourceHash[request.comparison.APage] != request.sourceHash {
		a.cancelProofreadingOCROverflowRequest(request)
		a.compareProofreadingOCRStep()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	step.overflowCancel = cancel
	window, shutdown := a.window, a.shutdown
	publish := func(result proofreadingOCRStepResult) {
		result.page = pageIndex
		result.generation = request.mainGeneration
		result.overflowGeneration = request.overflowGeneration
		result.overflowPrefetch = true
		result.sourceHash = sourceHash
		if result.ocrPage != nil {
			layout, layoutErr := result.ocrPage.Layout()
			if layoutErr != nil {
				result.err = layoutErr
			} else {
				result.ocrLayout = &layout
				if request.preComparison {
					if candidate, ok := proofreadingOCRBoundaryTailFromPages(currentOCR.layout, layout, request.currentText); ok {
						result.overflowCandidate = &candidate
					}
				}
			}
		}
		select {
		case step.results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	}
	if !a.launchManagedBackground(func() {
		err := runProofreadingOCRPageStep(ctx, document, pageIndex, pageCount, sourceHash, token, endpoint, persistCredential, credentials, store, func(proofreadingOCRStepPhase, int, int) {}, publish)
		if err != nil && ctx.Err() == nil {
			publish(proofreadingOCRStepResult{err: err})
		}
	}) {
		cancel()
		if request.preComparison {
			a.setProofreadingOCRBoundarySkip(request)
			a.cancelProofreadingOCROverflowRequest(request)
			a.compareProofreadingOCRStep()
		} else {
			if step.overflowRequest == request {
				step.overflowCancel = nil
				step.overflowRequest = nil
			}
			if step.phase == proofreadingOCRStepCheckingBoundary {
				step.phase = proofreadingOCRStepIdle
			}
			a.presentProofreadingOCRStepComparison(request.comparison)
		}
	}
}

func (a *App) applyProofreadingOCRBoundaryPrefetch(
	request *proofreadingOCROverflowRequest,
	candidate proofreadingOCROverflowTail,
	nextLayout ocr.Layout,
) bool {
	if !a.proofreadingOCROverflowRequestCurrent(request) {
		return false
	}
	return a.applyProofreadingOCRBoundaryAt(
		request.comparison.APage, request.sourceHash, request.currentText, request.nextText,
		request.modelRevision, candidate, nextLayout,
	)
}

func (a *App) adjustProofreadingOCRBoundaryFromCachedPages(pageIndex int, page proofreadingXPage) bool {
	if a == nil || a.proofreading == nil || a.proofreading.controller == nil || a.pdfDoc == nil || pageIndex < 0 ||
		pageIndex+1 >= len(a.proofreading.xPages) {
		return false
	}
	state := a.proofreading
	step := state.ocrStep
	if step == nil {
		return false
	}
	currentOCR, currentExists := step.pages[pageIndex]
	nextOCR := step.pages[pageIndex+1]
	if step.pageSourceHash[pageIndex+1] != state.aSourceHash {
		nextOCR = proofreadingOCRPage{}
	}
	if !currentExists || step.pageSourceHash[pageIndex] != state.aSourceHash || strings.TrimSpace(page.Content) == "" {
		return false
	}
	candidate, ok := proofreadingOCRBoundaryTailFromPages(currentOCR.layout, nextOCR.layout, page.Content)
	if !ok {
		return false
	}
	return a.applyProofreadingOCRBoundaryAt(
		pageIndex, state.aSourceHash, page.Content, state.xPages[pageIndex+1].Content,
		state.controller.Model.Revision(), candidate, nextOCR.layout,
	)
}

func (a *App) applyProofreadingOCRBoundaryAt(
	pageIndex int,
	sourceHash, currentText, nextText string,
	modelRevision uint64,
	candidate proofreadingOCROverflowTail,
	nextLayout ocr.Layout,
) bool {
	if a == nil || a.proofreading == nil || a.proofreading.controller == nil || a.pdfDoc == nil || candidate.RuneCount <= 0 ||
		candidate.MatchedRunes < proofreadingOCROverflowMinimumRunes {
		return false
	}
	state := a.proofreading
	step := state.ocrStep
	if step == nil || pageIndex < 0 || pageIndex+1 >= len(state.xPages) || sourceHash != state.aSourceHash ||
		modelRevision != state.controller.Model.Revision() {
		return false
	}
	view := a.ensurePDFView()
	if view == nil || view.page != pageIndex || a.proofreadingCurrentBPage() != pageIndex {
		return false
	}
	currentPage, nextPage := state.xPages[pageIndex], state.xPages[pageIndex+1]
	if currentPage.Content != currentText || nextPage.Content != nextText {
		return false
	}
	currentOCR, currentExists := step.pages[pageIndex]
	if !currentExists || step.pageSourceHash[pageIndex] != sourceHash {
		return false
	}
	confirmed, ok := proofreadingOCRBoundaryTailFromPages(currentOCR.layout, nextLayout, currentText)
	if !ok || confirmed != candidate {
		return false
	}
	currentID, nextID := proofreading.PageID(pageIndex+1), proofreading.PageID(pageIndex+2)
	currentMeta, currentOK := state.controller.Model.Page(currentID)
	nextMeta, nextOK := state.controller.Model.Page(nextID)
	if !currentOK || !nextOK || currentMeta.Reviewed || currentMeta.Locked ||
		nextMeta.Reviewed || nextMeta.Locked ||
		proofreadingOCRPageHasTypedOrProtectedText(state.controller.Model, nextID) ||
		state.controller.Model.BoundaryReturnableRunes(currentID) < candidate.RuneCount {
		return false
	}
	if proofreadingOCRPagePrefixMatchesText(nextLayout, nextPage.Content) {
		return false
	}
	if err := state.controller.Model.AdjustBoundary(currentID, -candidate.RuneCount); err != nil {
		return false
	}
	destinationEmpty := strings.TrimSpace(nextPage.Content) == ""
	filledDestination := false
	if destinationEmpty && strings.TrimSpace(state.controller.Model.Text(nextID)) == "" {
		recognized := strings.TrimSpace(proofreadingOCRPagePreview(nextLayout))
		if recognized != "" {
			if err := state.controller.Model.ReplacePage(nextID, recognized); err == nil {
				filledDestination = true
			}
		}
	}
	a.invalidateProofreadingOCRStepComparisons()
	step.cancelTask()
	if filledDestination {
		a.proofreadingModelChanged(true)
	} else {
		a.proofreadingBoundaryChanged(pageIndex)
	}
	a.compareProofreadingOCRStep()
	return true
}

func (a *App) proofreadingOCROverflowRequestCurrent(request *proofreadingOCROverflowRequest) bool {
	if a == nil || a.proofreading == nil || a.proofreading.controller == nil || request == nil || a.pdfDoc == nil {
		return false
	}
	state := a.proofreading
	step := state.ocrStep
	if step == nil || step.overflowRequest != request || step.generation != request.mainGeneration ||
		step.overflowGeneration != request.overflowGeneration || state.aSourceHash != request.sourceHash ||
		state.controller.Model.Revision() != request.modelRevision || request.comparison.BPage < 0 ||
		request.comparison.BPage+1 >= len(state.xPages) {
		return false
	}
	if state.xPages[request.comparison.BPage].Content != request.currentText ||
		state.xPages[request.comparison.BPage+1].Content != request.nextText {
		return false
	}
	view := a.ensurePDFView()
	return view != nil && view.page == request.comparison.APage && a.proofreadingCurrentBPage() == request.comparison.BPage
}

func (a *App) cancelProofreadingOCROverflowRequest(request *proofreadingOCROverflowRequest) {
	if a == nil || a.proofreading == nil || a.proofreading.ocrStep == nil {
		return
	}
	step := a.proofreading.ocrStep
	if step.overflowRequest != request {
		return
	}
	if step.overflowCancel != nil {
		step.overflowCancel()
		step.overflowCancel = nil
	}
	step.overflowGeneration++
	step.overflowRequest = nil
	if step.phase == proofreadingOCRStepCheckingBoundary {
		step.phase = proofreadingOCRStepIdle
	}
}

func (a *App) applyProofreadingOCROverflow(
	mainGeneration, overflowGeneration uint64,
	comparison proofreadingOCRStepComparison,
	candidate proofreadingOCROverflowTail,
	nextLayout ocr.Layout,
	modelRevision uint64,
) bool {
	if a == nil || a.proofreading == nil || a.proofreading.controller == nil || a.proofreading.ocrStep == nil {
		return false
	}
	state := a.proofreading
	step := state.ocrStep
	if mainGeneration != step.generation || (overflowGeneration != 0 && overflowGeneration != step.overflowGeneration) ||
		modelRevision != state.controller.Model.Revision() || comparison.SourceHash != state.aSourceHash ||
		comparison.BPage < 0 || comparison.BPage+1 >= len(state.xPages) || !proofreadingOCRStepComparisonValid(state, comparison) {
		return false
	}
	view := a.ensurePDFView()
	if view == nil || view.page != comparison.APage || a.proofreadingCurrentBPage() != comparison.BPage {
		return false
	}
	currentPage := state.xPages[comparison.BPage]
	nextPage := state.xPages[comparison.BPage+1]
	if currentPage.Content != comparison.BText || currentPage.ContentStart != comparison.BStart || currentPage.ContentEnd != comparison.BEnd {
		return false
	}
	currentID := proofreading.PageID(comparison.BPage + 1)
	nextID := proofreading.PageID(comparison.BPage + 2)
	currentMeta, currentOK := state.controller.Model.Page(currentID)
	nextMeta, nextOK := state.controller.Model.Page(nextID)
	if !currentOK || !nextOK || currentMeta.Reviewed || currentMeta.Locked ||
		nextMeta.Reviewed || nextMeta.Locked ||
		proofreadingOCRPageHasTypedOrProtectedText(state.controller.Model, nextID) ||
		state.controller.Model.BoundaryReturnableRunes(currentID) < candidate.RuneCount {
		return false
	}
	currentOCR, currentOCRFound := step.pages[comparison.APage]
	directCandidate, directOK := proofreadingOCRBoundaryTailFromPages(currentOCR.layout, nextLayout, comparison.BText)
	if !currentOCRFound || step.pageSourceHash[comparison.APage] != comparison.SourceHash || !directOK || directCandidate != candidate {
		confirmed, confirmedOK := proofreadingOCRConfirmedOverflowTail(comparison, nextLayout)
		if !confirmedOK || confirmed != candidate {
			return false
		}
	}
	if proofreadingOCRPagePrefixMatchesText(nextLayout, nextPage.Content) {
		return false
	}
	if overflowGeneration != 0 && (step.overflowRequest == nil || step.overflowRequest.mainGeneration != mainGeneration ||
		step.overflowRequest.modelRevision != modelRevision || step.overflowRequest.currentText != currentPage.Content ||
		step.overflowRequest.nextText != nextPage.Content) {
		return false
	}
	destinationEmpty := strings.TrimSpace(nextPage.Content) == ""
	if err := state.controller.Model.AdjustBoundary(currentID, -candidate.RuneCount); err != nil {
		return false
	}
	filledDestination := false
	if destinationEmpty && strings.TrimSpace(state.controller.Model.Text(nextID)) == "" {
		recognized := strings.TrimSpace(proofreadingOCRPagePreview(nextLayout))
		if recognized != "" {
			if err := state.controller.Model.ReplacePage(nextID, recognized); err == nil {
				filledDestination = true
			}
		}
	}
	a.invalidateProofreadingOCRStepComparisons()
	step.cancelTask()
	if filledDestination {
		a.proofreadingModelChanged(true)
	} else {
		a.proofreadingBoundaryChanged(comparison.BPage)
	}
	a.compareProofreadingOCRStep()
	return true
}

func (a *App) invalidateProofreadingOCRStepComparisons() {
	if a == nil || a.proofreading == nil || a.proofreading.ocrStep == nil {
		return
	}
	step := a.proofreading.ocrStep
	step.comparisons = make(map[proofreadingOCRStepPair]proofreadingOCRStepComparison)
	step.diffCursor = 0
	step.diffRunning = false
	step.diffPaused = false
}

func proofreadingOCRPageHasTypedOrProtectedText(model *proofreading.Model, page proofreading.PageID) bool {
	if model == nil {
		return true
	}
	for _, run := range model.PageSnapshot(page).Runs {
		if run.Typed || run.Protected {
			return true
		}
	}
	return false
}
