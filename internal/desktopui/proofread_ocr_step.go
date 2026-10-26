package desktopui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"sort"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"

	"glance/internal/agentsettings"
	"glance/internal/format/pdfdoc"
	"glance/internal/i18n"
	"glance/internal/ocr"
	"glance/internal/textalign"
)

type proofreadingOCRStepPhase uint8

const (
	proofreadingOCRStepIdle proofreadingOCRStepPhase = iota
	proofreadingOCRStepCheckingCache
	proofreadingOCRStepAuthenticating
	proofreadingOCRStepRendering
	proofreadingOCRStepUploading
	proofreadingOCRStepRunning
	proofreadingOCRStepTokenRequired
	proofreadingOCRStepRecognized
	proofreadingOCRStepComparing
	proofreadingOCRStepCheckingBoundary
	proofreadingOCRStepCompared
	proofreadingOCRStepFailed
)

type proofreadingOCRStepResult struct {
	generation         uint64
	page               int
	sourceHash         string
	ocrPage            *ocr.Page
	ocrLayout          *ocr.Layout
	overflowCandidate  *proofreadingOCROverflowTail
	comparison         *proofreadingOCRStepComparison
	cacheHit           bool
	cacheErr           error
	characterLayout    *ocr.Layout
	characterErr       error
	needToken          bool
	endpoint           string
	err                error
	overflowPrefetch   bool
	overflowGeneration uint64
}

type proofreadingOCRStepHistoryEntry struct {
	page       int
	layout     ocr.Layout
	comparison proofreadingOCRStepComparison
}

type proofreadingOCRStepHistoryResult struct {
	generation uint64
	sourceHash string
	entries    []proofreadingOCRStepHistoryEntry
}

type proofreadingOCRStepProgress struct {
	generation uint64
	phase      proofreadingOCRStepPhase
	extracted  int
	total      int
}

type proofreadingOCRStepPair struct {
	APage int
	BPage int
}

type proofreadingOCRStepComparison struct {
	APage            int
	BPage            int
	SourceHash       string
	DraftHash        string
	BText            string
	BStart           int
	BEnd             int
	Result           textalign.Result
	Matches          int
	AOnly            int
	BOnly            int
	MatchedText      string
	AOnlyText        string
	BOnlyText        string
	HasEvidence      bool
	Complete         bool
	AlignmentLimited bool
	CharacterLocated int
	CharacterTotal   int
	MatchedRanges    []textalign.Range
	Differences      []textalign.Range
	AOnlyBlockIDs    []int
	Frames           []proofreadingOCRDiffFrame
}

type proofreadingOCRStepState struct {
	batch               proofreadingOCRBatch
	manualModeButton    widget.Clickable
	aiModeButton        widget.Clickable
	ocrButton           widget.Clickable
	compareButton       widget.Clickable
	diffPauseButton     widget.Clickable
	diffReplayButton    widget.Clickable
	diffList            widget.List
	pages               map[int]proofreadingOCRPage
	pageSourceHash      map[int]string
	comparisons         map[proofreadingOCRStepPair]proofreadingOCRStepComparison
	results             chan proofreadingOCRStepResult
	progress            chan proofreadingOCRStepProgress
	historyResults      chan proofreadingOCRStepHistoryResult
	generation          uint64
	historyGeneration   uint64
	historySourceHash   string
	historyLoading      bool
	historyLoaded       bool
	historyCancel       context.CancelFunc
	historyInvalidPages map[int]bool
	page                int
	bPage               int
	phase               proofreadingOCRStepPhase
	extracted           int
	total               int
	startedAt           time.Time
	cancel              context.CancelFunc
	overflowCancel      context.CancelFunc
	overflowGeneration  uint64
	overflowRequest     *proofreadingOCROverflowRequest
	boundarySkip        *proofreadingOCRBoundarySkip
	cacheHit            bool
	cacheErr            string
	characterErr        string
	err                 string
	status              string
	sourceHash          string
	diffPair            proofreadingOCRStepPair
	diffDraftHash       string
	diffCursor          int
	diffNextAt          time.Time
	diffRunning         bool
	diffPaused          bool
	diffMatches         int
	diffAOnly           int
	diffBOnly           int
	diffPlayedPairs     map[proofreadingOCRStepPair]string
	autoCompareAfterOCR bool
	circuitCache        proofreadingCircuitCache
	badgeClicksByPair   map[proofreadingOCRStepPair][]widget.Clickable
	badgeAnchors        map[proofreadingOCRStepPair][]proofreadingCenterBadgeAnchor
	badgeConnections    map[proofreadingOCRStepPair][]proofreadingOCRConnection
	currentConnections  []proofreadingOCRConnection
	currentBadges       []proofreadingCenterBadge
	inspector           proofreadingDiffInspector
}

func newProofreadingOCRStepState() *proofreadingOCRStepState {
	return &proofreadingOCRStepState{
		pages:               map[int]proofreadingOCRPage{},
		pageSourceHash:      map[int]string{},
		comparisons:         map[proofreadingOCRStepPair]proofreadingOCRStepComparison{},
		results:             make(chan proofreadingOCRStepResult, 1),
		progress:            make(chan proofreadingOCRStepProgress, 1),
		historyResults:      make(chan proofreadingOCRStepHistoryResult, 1),
		diffPlayedPairs:     map[proofreadingOCRStepPair]string{},
		badgeClicksByPair:   make(map[proofreadingOCRStepPair][]widget.Clickable),
		badgeAnchors:        make(map[proofreadingOCRStepPair][]proofreadingCenterBadgeAnchor),
		badgeConnections:    make(map[proofreadingOCRStepPair][]proofreadingOCRConnection),
		historyInvalidPages: make(map[int]bool),
		inspector:           newProofreadingDiffInspector(),
		page:                -1,
		bPage:               -1,
	}
}

func (s *proofreadingOCRStepState) active() bool {
	if s == nil {
		return false
	}
	switch s.phase {
	case proofreadingOCRStepCheckingCache, proofreadingOCRStepAuthenticating,
		proofreadingOCRStepRendering, proofreadingOCRStepUploading,
		proofreadingOCRStepRunning, proofreadingOCRStepComparing,
		proofreadingOCRStepCheckingBoundary:
		return true
	default:
		return false
	}
}

func (s *proofreadingOCRStepState) close() {
	if s == nil {
		return
	}
	s.cancelTask()
	s.cancelHistoryLoad()
	for {
		select {
		case <-s.results:
		case <-s.progress:
		case <-s.historyResults:
		default:
			return
		}
	}
}

func (s *proofreadingOCRStepState) cancelHistoryLoad() {
	if s == nil {
		return
	}
	s.historyGeneration++
	if s.historyCancel != nil {
		s.historyCancel()
		s.historyCancel = nil
	}
	s.historyLoading = false
}

func (s *proofreadingOCRStepState) cancelTask() {
	if s == nil {
		return
	}
	s.generation++
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.overflowCancel != nil {
		s.overflowCancel()
		s.overflowCancel = nil
	}
	s.overflowGeneration++
	s.overflowRequest = nil
	if s.active() {
		s.phase = proofreadingOCRStepIdle
		s.status = "proofread.ai.step.cancelled"
	}
}

func (a *App) proofreadingOCRStep() *proofreadingOCRStepState {
	if a == nil || a.proofreading == nil {
		return nil
	}
	if a.proofreading.ocrStep == nil {
		a.proofreading.ocrStep = newProofreadingOCRStepState()
	}
	return a.proofreading.ocrStep
}

func (a *App) proofreadingCurrentBPage() int {
	if a == nil || a.proofreading == nil || a.pdfView == nil {
		return 0
	}
	state := a.proofreading
	return max(0, min(len(state.xPages)-1, a.pdfView.page))
}

func (a *App) requestProofreadingOCRStep() {
	if a == nil || a.proofreading == nil || a.pdfDoc == nil {
		return
	}
	state := a.proofreading
	step := a.proofreadingOCRStep()
	if step == nil {
		return
	}
	if step.active() {
		step.autoCompareAfterOCR = false
		step.cancelTask()
		return
	}
	if state.bDocument == nil || !isSupportedAIProofreadingFormat(state.bDocument.formatID()) {
		step.phase = proofreadingOCRStepFailed
		step.err = a.textID("proofread.ai.status.markdown_required")
		return
	}
	view := a.ensurePDFView()
	if view == nil || view.info.Pages <= 0 || state.aSourceHash == "" {
		step.phase = proofreadingOCRStepFailed
		step.err = a.textID("proofread.ai.status.source_required")
		return
	}
	pageIndex := max(0, min(view.info.Pages-1, view.page))
	if len(state.pageFrames) != view.info.Pages {
		step.phase = proofreadingOCRStepFailed
		step.err = a.textID("proofread.ai.step.page_unavailable")
		return
	}
	// This comparison is explicitly A-current-page ↔ B-same-page. An older
	// block link may have left X independently scrolled, which would hide the
	// B anchor and make the cross-pane connector impossible to draw.
	state.xCanvas.Axis = layout.Vertical
	state.xCanvas.Position = view.canvas.Position
	endpoint, err := proofreadingOCREndpointFromEnvironment()
	if err != nil {
		step.phase = proofreadingOCRStepFailed
		step.err = err.Error()
		return
	}
	if step.sourceHash != state.aSourceHash {
		step.pages = map[int]proofreadingOCRPage{}
		step.pageSourceHash = map[int]string{}
		step.comparisons = map[proofreadingOCRStepPair]proofreadingOCRStepComparison{}
		step.historyInvalidPages = make(map[int]bool)
		step.badgeAnchors = make(map[proofreadingOCRStepPair][]proofreadingCenterBadgeAnchor)
		step.badgeConnections = make(map[proofreadingOCRStepPair][]proofreadingOCRConnection)
		step.badgeClicksByPair = make(map[proofreadingOCRStepPair][]widget.Clickable)
	}
	step.sourceHash = state.aSourceHash
	if step.historyInvalidPages == nil {
		step.historyInvalidPages = make(map[int]bool)
	}
	step.historyInvalidPages[pageIndex] = true
	step.cancelTask()
	step.generation++
	generation := step.generation
	ctx, cancel := context.WithCancel(context.Background())
	step.cancel = cancel
	step.page = pageIndex
	step.bPage = a.proofreadingCurrentBPage()
	step.phase = proofreadingOCRStepCheckingCache
	step.status = "proofread.ai.step.cache_checking"
	step.err = ""
	step.cacheHit = false
	step.cacheErr = ""
	step.startedAt = time.Now()
	step.total = 1
	step.extracted = 0
	a.clearProofreadingOCRStepComparisonHistory(step, proofreadingOCRStepPair{APage: pageIndex, BPage: step.bPage})
	if _, exists := step.pages[pageIndex]; exists {
		delete(step.pages, pageIndex)
	}
	window, shutdown := a.window, a.shutdown
	pageCount, sourceHash, document := view.info.Pages, state.aSourceHash, a.pdfDoc
	store := a.proofreadingStore()
	credentials := state.ocrCredentialStore
	if credentials == nil {
		credentials = agentsettings.SystemCredentialStore{}
	}
	if !a.launchManagedBackground(func() {
		publish := func(result proofreadingOCRStepResult) {
			result.generation = generation
			result.sourceHash = sourceHash
			select {
			case step.results <- result:
				if window != nil {
					window.Invalidate()
				}
			case <-shutdown:
			}
		}
		publishProgress := func(phase proofreadingOCRStepPhase, extracted, total int) {
			value := proofreadingOCRStepProgress{generation: generation, phase: phase, extracted: extracted, total: total}
			select {
			case step.progress <- value:
			default:
				select {
				case <-step.progress:
				default:
				}
				select {
				case step.progress <- value:
				default:
				}
			}
			if window != nil {
				window.Invalidate()
			}
		}
		cached, cacheHit, cacheErr := loadProofreadingOCRCachePage(sourceHash, ocr.DefaultModel, ocr.Options{}, pageIndex+1, pageCount)
		if err := ctx.Err(); err != nil {
			return
		}
		if cacheHit {
			publish(proofreadingOCRStepResult{page: pageIndex, ocrPage: &cached, cacheHit: true, cacheErr: cacheErr})
			return
		}
		publishProgress(proofreadingOCRStepAuthenticating, 0, 1)
		_, token, found, lookupErr := loadProofreadingOCRAuthentication(ctx, credentials, store, endpoint)
		if err := ctx.Err(); err != nil {
			return
		}
		if !found {
			publish(proofreadingOCRStepResult{page: pageIndex, needToken: true, endpoint: endpoint, cacheErr: cacheErr, err: lookupErr})
			return
		}
		cacheErr = runProofreadingOCRPageStep(ctx, document, pageIndex, pageCount, sourceHash, token, endpoint, false, credentials, store, publishProgress, publish)
		if cacheErr != nil && !errors.Is(cacheErr, context.Canceled) {
			publish(proofreadingOCRStepResult{page: pageIndex, cacheErr: cacheErr, err: cacheErr})
		}
	}) {
		cancel()
		step.cancel = nil
		step.phase = proofreadingOCRStepFailed
		step.err = a.textID("proofread.ai.step.start_failed")
	}
}

func runProofreadingOCRPageStep(ctx context.Context, document *pdfdoc.Document, pageIndex, pageCount int, sourceHash, token, endpoint string, persistCredential bool, credentials proofreadingOCRCredentialStore, store proofreadingOCRPreferenceStore, publishProgress func(proofreadingOCRStepPhase, int, int), publish func(proofreadingOCRStepResult)) error {
	if document == nil || pageIndex < 0 || pageCount <= pageIndex {
		return errors.New("当前 PDF 页不可用")
	}
	if persistCredential {
		if _, err := saveProofreadingOCRAuthentication(ctx, credentials, store, endpoint, token); err != nil {
			return fmt.Errorf("保存 OCR 令牌失败：%w", err)
		}
	}
	publishProgress(proofreadingOCRStepRendering, 0, 1)
	image, _, err := document.RenderVisualPage(pageIndex, proofreadingOCRStepRenderDPI)
	if err != nil {
		return fmt.Errorf("渲染 PDF 第 %d 页失败：%w", pageIndex+1, err)
	}
	var encoded bytes.Buffer
	if err = png.Encode(&encoded, image); err != nil {
		return fmt.Errorf("编码 PDF 第 %d 页失败：%w", pageIndex+1, err)
	}
	if encoded.Len() == 0 || encoded.Len() > 32<<20 {
		return errors.New("当前页图像为空或超过 32 MiB 上传限制")
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	client := &ocr.Client{Token: token, Endpoint: endpoint}
	publishProgress(proofreadingOCRStepUploading, 0, 1)
	source := ocr.Source{Name: fmt.Sprintf("page-%05d.png", pageIndex+1), Reader: bytes.NewReader(encoded.Bytes()), Size: int64(encoded.Len())}
	job, err := client.Submit(ctx, source, ocr.Options{})
	if err != nil {
		return err
	}
	publishProgress(proofreadingOCRStepRunning, 0, 1)
	completed, err := client.Wait(ctx, job.ID, func(update ocr.Job) {
		publishProgress(proofreadingOCRStepRunning, update.Progress.ExtractedPages, max(1, update.Progress.TotalPages))
	})
	if err != nil {
		return err
	}
	pages, err := client.Results(ctx, completed)
	if err != nil {
		return err
	}
	if len(pages) != 1 {
		return fmt.Errorf("OCR 返回 %d 页，预期当前页 1 页", len(pages))
	}
	pages[0].Number = pageIndex + 1
	if err = saveProofreadingOCRStepPageCache(sourceHash, ocr.DefaultModel, ocr.Options{}, pages[0], pageCount); err != nil {
		// Keep the recognized page available in memory even if the local cache is unavailable.
		cacheErr := err
		publish(proofreadingOCRStepResult{page: pageIndex, ocrPage: &pages[0], cacheErr: cacheErr})
		return nil
	}
	publish(proofreadingOCRStepResult{page: pageIndex, ocrPage: &pages[0]})
	return nil
}

func (a *App) startProofreadingOCRPageStepWithAuthentication(token, endpoint string, persistCredential bool) {
	if a == nil || a.proofreading == nil || a.pdfDoc == nil {
		return
	}
	step := a.proofreadingOCRStep()
	if step == nil || step.page < 0 {
		return
	}
	state := a.proofreading
	view := a.ensurePDFView()
	pageIndex, pageCount, sourceHash, document := step.page, view.info.Pages, state.aSourceHash, a.pdfDoc
	if pageIndex >= pageCount {
		step.phase = proofreadingOCRStepFailed
		step.err = a.textID("proofread.ai.step.page_unavailable")
		return
	}
	step.generation++
	generation := step.generation
	ctx, cancel := context.WithCancel(context.Background())
	step.cancel = cancel
	step.phase = proofreadingOCRStepRendering
	step.startedAt = time.Now()
	step.err = ""
	if step.sourceHash != sourceHash {
		step.pages = map[int]proofreadingOCRPage{}
		step.pageSourceHash = map[int]string{}
		step.comparisons = map[proofreadingOCRStepPair]proofreadingOCRStepComparison{}
		step.historyInvalidPages = make(map[int]bool)
		step.badgeAnchors = make(map[proofreadingOCRStepPair][]proofreadingCenterBadgeAnchor)
		step.badgeConnections = make(map[proofreadingOCRStepPair][]proofreadingOCRConnection)
		step.badgeClicksByPair = make(map[proofreadingOCRStepPair][]widget.Clickable)
	}
	step.sourceHash = sourceHash
	if step.historyInvalidPages == nil {
		step.historyInvalidPages = make(map[int]bool)
	}
	step.historyInvalidPages[pageIndex] = true
	window, shutdown := a.window, a.shutdown
	store := a.proofreadingStore()
	credentials := state.ocrCredentialStore
	if credentials == nil {
		credentials = agentsettings.SystemCredentialStore{}
	}
	if !a.launchManagedBackground(func() {
		publish := func(result proofreadingOCRStepResult) {
			result.generation = generation
			result.sourceHash = sourceHash
			select {
			case step.results <- result:
				if window != nil {
					window.Invalidate()
				}
			case <-shutdown:
			}
		}
		publishProgress := func(phase proofreadingOCRStepPhase, extracted, total int) {
			value := proofreadingOCRStepProgress{generation: generation, phase: phase, extracted: extracted, total: total}
			select {
			case step.progress <- value:
			default:
				select {
				case <-step.progress:
				default:
				}
				select {
				case step.progress <- value:
				default:
				}
			}
			if window != nil {
				window.Invalidate()
			}
		}
		err := runProofreadingOCRPageStep(ctx, document, pageIndex, pageCount, sourceHash, token, endpoint, persistCredential, credentials, store, publishProgress, publish)
		if err != nil && !errors.Is(err, context.Canceled) {
			publish(proofreadingOCRStepResult{page: pageIndex, err: err})
		}
	}) {
		cancel()
		step.cancel = nil
		step.phase = proofreadingOCRStepFailed
		step.err = a.textID("proofread.ai.step.start_failed")
	}
}

func (a *App) compareProofreadingOCRStep() {
	if a == nil || a.proofreading == nil || a.pdfDoc == nil {
		return
	}
	state := a.proofreading
	step := a.proofreadingOCRStep()
	if step == nil {
		return
	}
	view := a.ensurePDFView()
	if step.active() {
		if step.phase != proofreadingOCRStepCheckingBoundary {
			return
		}
		if view != nil && view.page == step.page && a.proofreadingCurrentBPage() == step.bPage {
			return
		}
		step.cancelTask()
	}
	aPage := max(0, min(view.info.Pages-1, view.page))
	bPage := a.proofreadingCurrentBPage()
	if bPage < 0 || bPage >= len(state.xPages) {
		step.phase = proofreadingOCRStepFailed
		step.err = a.textID("proofread.ai.step.page_unavailable")
		return
	}

	b := state.xPages[bPage]
	sourceHash := state.aSourceHash
	pageDraftHash := proofreadingOCRDraftSnapshotHash(b.Content)
	step.page, step.bPage = aPage, bPage
	ocrPage, found := step.pages[aPage]
	if !found || step.pageSourceHash[aPage] != state.aSourceHash || ocrPage.layout.Width <= 0 || ocrPage.layout.Height <= 0 {
		step.autoCompareAfterOCR = true
		a.requestProofreadingOCRStep()
		if !step.active() {
			step.autoCompareAfterOCR = false
		}
		return
	}
	a.clearProofreadingOCRStepComparisonHistory(step, proofreadingOCRStepPair{APage: aPage, BPage: bPage})
	if strings.TrimSpace(b.Content) == "" && a.fillEmptyProofreadingBPageFromOCR(aPage, ocrPage.layout) {
		return
	}
	// Resolve the PDF page boundary from adjacent-page OCR before character comparison.
	if !a.consumeProofreadingOCRBoundarySkip(aPage, sourceHash, pageDraftHash) {
		if a.adjustProofreadingOCRBoundaryFromCachedPages(aPage, b) {
			if a.window != nil {
				a.window.Invalidate()
			}
			return
		}
		if a.startProofreadingOCRBoundaryPrefetch(aPage, b) {
			if a.window != nil {
				a.window.Invalidate()
			}
			return
		}
	}
	exceptions := a.proofreadingDiffExceptionMap()
	if cachedComparison, hit, _ := loadProofreadingOCRComparisonCache(sourceHash, aPage, bPage, pageDraftHash); hit && cachedComparison.Complete && ocrPage.layout.CharacterVersion == ocr.CharacterGeometryVersion {
		if cachedComparison.BText == b.Content && (cachedComparison.BStart != b.ContentStart || cachedComparison.BEnd != b.ContentEnd) {
			rebaseProofreadingOCRStepComparison(&cachedComparison, b.ContentStart)
			_ = saveProofreadingOCRComparisonCache(cachedComparison)
		}
		if proofreadingOCRStepComparisonValid(state, cachedComparison) {
			if len(exceptions) > 0 {
				filterProofreadingOCRComparisonExceptions(&cachedComparison, exceptions)
			}
			a.clearProofreadingOCRStepComparisonHistory(step, proofreadingOCRStepPair{APage: aPage, BPage: bPage})
			step.page, step.bPage = aPage, bPage
			if a.prepareProofreadingOCRStepComparison(cachedComparison) {
				if a.window != nil {
					a.window.Invalidate()
				}
				return
			}
			a.presentProofreadingOCRStepComparison(cachedComparison)
			if a.window != nil {
				a.window.Invalidate()
			}
			return
		}
	}
	step.cancelTask()
	step.generation++
	generation := step.generation
	ctx, cancel := context.WithCancel(context.Background())
	step.cancel = cancel
	step.page, step.bPage = aPage, bPage
	step.phase = proofreadingOCRStepComparing
	step.status = "proofread.ai.step.comparing"
	step.err = ""
	step.startedAt = time.Now()
	a.clearProofreadingOCRStepComparisonHistory(step, proofreadingOCRStepPair{APage: aPage, BPage: bPage})
	layout := ocrPage.layout
	bText, bStart, bEnd := b.Content, b.ContentStart, b.ContentEnd
	window, shutdown := a.window, a.shutdown
	document, pageCount := a.pdfDoc, view.info.Pages
	if !a.launchManagedBackground(func() {
		result := proofreadingOCRStepResult{generation: generation, page: aPage, sourceHash: sourceHash}
		if layout.CharacterVersion != ocr.CharacterGeometryVersion {
			layout, result.characterErr, result.cacheErr = ensureProofreadingOCRCharacterGeometry(ctx, document, aPage, pageCount, sourceHash, layout)
			if ctx.Err() != nil {
				return
			}
			result.characterLayout = &layout
		}
		page, err := alignProofreadingOCRLayoutPage(ctx, aPage+1, layout, bText)
		if err != nil {
			result.err = err
		} else {
			page.result = shiftProofreadingOCRResult(page.result, bStart)
			diff := proofreadingOCRCharacterDiff(layout, bText, bStart, exceptions)
			located, total := proofreadingOCRCharacterCoverage(layout)
			comparison := proofreadingOCRStepComparison{
				APage: aPage, BPage: bPage, SourceHash: sourceHash, DraftHash: pageDraftHash,
				BText: bText, BStart: bStart, BEnd: bEnd, Result: page.result,
				Matches: diff.Matches, AOnly: diff.AOnly, BOnly: diff.BOnly,
				MatchedText: diff.MatchedText,
				AOnlyText:   diff.AOnlyText, BOnlyText: diff.BOnlyText,
				HasEvidence: diff.HasEvidence, Complete: diff.Complete,
				AlignmentLimited: page.result.Status != "matched",
				MatchedRanges:    diff.MatchedRanges,
				Differences:      diff.BOnlyRanges, AOnlyBlockIDs: diff.AOnlyBlockIDs,
				Frames:           diff.Frames,
				CharacterLocated: located,
				CharacterTotal:   total,
			}
			result.comparison = &comparison
		}
		result.generation = generation
		select {
		case step.results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	}) {
		cancel()
		step.cancel = nil
		step.phase = proofreadingOCRStepFailed
		step.err = a.textID("proofread.ai.step.start_failed")
	}
}

func (a *App) processProofreadingOCRStep(gtx layout.Context) {
	step := a.proofreadingOCRStep()
	if step == nil {
		return
	}
	// Leave in-flight results queued while paused. Workers still finish and
	// persist OCR caches, but no boundary mutation or playback may run here.
	if step.batch.suspended {
		return
	}
	if a.proofreading != nil {
		state := a.proofreading
		if state.aSourceHash != "" && step.historySourceHash != state.aSourceHash {
			step.cancelHistoryLoad()
			step.historySourceHash = state.aSourceHash
			step.historyLoaded = false
		}
		if state.aSourceHash != "" && step.sourceHash != "" && step.sourceHash != state.aSourceHash {
			step.cancelTask()
			step.pages = map[int]proofreadingOCRPage{}
			step.pageSourceHash = map[int]string{}
			step.comparisons = map[proofreadingOCRStepPair]proofreadingOCRStepComparison{}
			step.historyInvalidPages = make(map[int]bool)
			step.badgeAnchors = make(map[proofreadingOCRStepPair][]proofreadingCenterBadgeAnchor)
			step.badgeConnections = make(map[proofreadingOCRStepPair][]proofreadingOCRConnection)
			step.badgeClicksByPair = make(map[proofreadingOCRStepPair][]widget.Clickable)
		}
		if state.aSourceHash != "" {
			step.sourceHash = state.aSourceHash
			a.requestProofreadingOCRStepHistory(step)
		}
	}
	a.advanceProofreadingOCRDiffPlayback(gtx, step)
	for {
		select {
		case history := <-step.historyResults:
			if history.generation != step.historyGeneration || a.proofreading == nil || history.sourceHash != a.proofreading.aSourceHash {
				continue
			}
			step.historyLoading = false
			step.historyLoaded = true
			step.historyCancel = nil
			exceptions := a.proofreadingDiffExceptionMap()
			for _, entry := range history.entries {
				if entry.page < 0 || entry.page >= len(a.proofreading.xPages) || entry.comparison.APage != entry.page ||
					entry.comparison.BPage != entry.page ||
					entry.layout.Width <= 0 || entry.layout.Height <= 0 || step.historyInvalidPages[entry.page] {
					continue
				}
				xPage := a.proofreading.xPages[entry.page]
				if entry.comparison.BText == xPage.Content && (entry.comparison.BStart != xPage.ContentStart || entry.comparison.BEnd != xPage.ContentEnd) {
					rebaseProofreadingOCRStepComparison(&entry.comparison, xPage.ContentStart)
				}
				if !proofreadingOCRStepComparisonValid(a.proofreading, entry.comparison) {
					continue
				}
				if len(exceptions) > 0 {
					filterProofreadingOCRComparisonExceptions(&entry.comparison, exceptions)
				}
				step.pages[entry.page] = proofreadingOCRPage{
					layout: entry.layout,
					result: textalign.Result{Page: entry.page + 1, Status: "recognized"},
				}
				step.pageSourceHash[entry.page] = history.sourceHash
				step.comparisons[proofreadingOCRStepPair{APage: entry.page, BPage: entry.page}] = entry.comparison
			}
		default:
			goto historyReady
		}
	}
historyReady:
	for {
		select {
		case progress := <-step.progress:
			if progress.generation != step.generation {
				continue
			}
			step.phase = progress.phase
			step.extracted = progress.extracted
			step.total = max(1, progress.total)
		default:
			goto resultLoop
		}
	}
resultLoop:
	select {
	case result := <-step.results:
		if result.generation != step.generation || result.sourceHash != a.proofreading.aSourceHash {
			return
		}
		if result.overflowPrefetch {
			if result.overflowGeneration != step.overflowGeneration {
				return
			}
			if step.overflowCancel != nil {
				step.overflowCancel()
			}
			step.overflowCancel = nil
			request := step.overflowRequest
			if request == nil || !request.preComparison {
				return
			}
			keepRequest := result.needToken && a.openProofreadingOCROverflowTokenDialog(result.endpoint, request)
			requestCurrent := a.proofreadingOCROverflowRequestCurrent(request)
			if result.needToken && keepRequest {
				gtx.Execute(op.InvalidateCmd{})
				return
			}
			if requestCurrent && result.err != nil {
				// A failed adjacent-page recognition is not evidence that the
				// current boundary is correct. Surface it in the shared page
				// pipeline so callers can retry instead of silently skipping it.
				a.cancelProofreadingOCROverflowRequest(request)
				step.phase = proofreadingOCRStepFailed
				step.err = result.err.Error()
				step.autoCompareAfterOCR = false
				gtx.Execute(op.InvalidateCmd{})
				return
			}
			if result.ocrLayout != nil {
				nextPage := request.comparison.APage + 1
				step.pages[nextPage] = proofreadingOCRPage{
					layout: *result.ocrLayout,
					result: textalign.Result{Page: nextPage + 1, Status: "recognized"},
				}
				step.pageSourceHash[nextPage] = result.sourceHash
				if requestCurrent && result.overflowCandidate != nil &&
					a.applyProofreadingOCRBoundaryPrefetch(request, *result.overflowCandidate, *result.ocrLayout) {
					gtx.Execute(op.InvalidateCmd{})
					return
				}
			} else if requestCurrent {
				a.setProofreadingOCRBoundarySkip(request)
			}
			if step.overflowRequest == request {
				step.overflowRequest = nil
			}
			if step.phase == proofreadingOCRStepCheckingBoundary {
				step.phase = proofreadingOCRStepIdle
			}
			view := a.ensurePDFView()
			if view != nil && view.page == request.comparison.APage && a.proofreadingCurrentBPage() == request.comparison.BPage {
				a.compareProofreadingOCRStep()
			}
			gtx.Execute(op.InvalidateCmd{})
			return
		}
		if step.cancel != nil {
			step.cancel()
			step.cancel = nil
		}
		if result.needToken {
			step.phase = proofreadingOCRStepTokenRequired
			step.err = ""
			step.cacheErr = ""
			if result.cacheErr != nil {
				step.cacheErr = result.cacheErr.Error()
			}
			a.openProofreadingOCRStepTokenDialog(result.endpoint)
		} else if result.err != nil {
			step.phase = proofreadingOCRStepFailed
			step.err = result.err.Error()
			step.autoCompareAfterOCR = false
		} else if result.comparison != nil {
			comparison := *result.comparison
			if comparison.SourceHash != a.proofreading.aSourceHash || comparison.BPage < 0 || comparison.BPage >= len(a.proofreading.xPages) || comparison.BText != a.proofreading.xPages[comparison.BPage].Content || comparison.BStart != a.proofreading.xPages[comparison.BPage].ContentStart || comparison.BEnd != a.proofreading.xPages[comparison.BPage].ContentEnd {
				step.phase = proofreadingOCRStepFailed
				step.err = a.textID("proofread.ai.step.stale")
			} else {
				if result.characterLayout != nil {
					page := step.pages[result.page]
					page.layout = *result.characterLayout
					step.pages[result.page] = page
				}
				step.characterErr = ""
				if result.characterErr != nil {
					step.characterErr = result.characterErr.Error()
				}
				if result.cacheErr != nil {
					step.cacheErr = result.cacheErr.Error()
				}
				step.phase = proofreadingOCRStepCheckingBoundary
				step.err = ""
				if a.prepareProofreadingOCRStepComparison(comparison) {
					gtx.Execute(op.InvalidateCmd{})
					return
				}
				a.presentProofreadingOCRStepComparison(comparison)
			}
		} else if result.ocrPage != nil {
			layout, err := result.ocrPage.Layout()
			if err != nil {
				step.phase = proofreadingOCRStepFailed
				step.err = err.Error()
				step.autoCompareAfterOCR = false
			} else {
				if step.historyInvalidPages == nil {
					step.historyInvalidPages = make(map[int]bool)
				}
				step.historyInvalidPages[result.page] = true
				step.pages[result.page] = proofreadingOCRPage{layout: layout, result: textalign.Result{Page: result.page + 1, Status: "recognized"}}
				step.pageSourceHash[result.page] = result.sourceHash
				step.page = result.page
				step.phase = proofreadingOCRStepRecognized
				step.cacheHit = result.cacheHit
				step.cacheErr = ""
				if result.cacheErr != nil {
					step.cacheErr = result.cacheErr.Error()
				}
				step.err = ""
				step.status = "proofread.ai.step.recognized"
				for pair := range step.comparisons {
					if pair.APage == result.page {
						a.clearProofreadingOCRStepComparisonHistory(step, pair)
					}
				}
				if step.autoCompareAfterOCR {
					step.autoCompareAfterOCR = false
					view := a.ensurePDFView()
					if view != nil && view.page == result.page && a.proofreadingCurrentBPage() == step.bPage {
						a.compareProofreadingOCRStep()
					}
					return
				}
			}
		}
		gtx.Execute(op.InvalidateCmd{})
	default:
	}
}

func (a *App) clearProofreadingOCRStepComparisonHistory(step *proofreadingOCRStepState, pair proofreadingOCRStepPair) {
	if step == nil {
		return
	}
	delete(step.comparisons, pair)
	delete(step.badgeAnchors, pair)
	delete(step.badgeConnections, pair)
	delete(step.badgeClicksByPair, pair)
	if step.inspector.open && step.inspector.pair == pair {
		step.inspector.open = false
	}
	if step.diffPair == pair {
		step.currentConnections = nil
		step.currentBadges = nil
		step.diffCursor = 0
		step.diffDraftHash = ""
		step.diffNextAt = time.Time{}
		step.diffRunning = false
		step.diffPaused = false
		delete(step.diffPlayedPairs, pair)
	}
}

func (a *App) requestProofreadingOCRStepHistory(step *proofreadingOCRStepState) {
	if a == nil || a.proofreading == nil || step == nil || step.historyLoading || step.historyLoaded ||
		step.historySourceHash != a.proofreading.aSourceHash || !a.proofreading.bDraftInitialized || len(a.proofreading.xPages) == 0 {
		return
	}
	view := a.ensurePDFView()
	if view == nil || view.info.Pages <= 0 || view.info.Pages > proofreadingOCRCacheMaxPages {
		step.historyLoaded = true
		return
	}
	pageCount := view.info.Pages
	pages := append([]proofreadingXPage(nil), a.proofreading.xPages...)
	sourceHash := a.proofreading.aSourceHash
	step.historyGeneration++
	generation := step.historyGeneration
	ctx, cancel := context.WithCancel(context.Background())
	step.historyCancel = cancel
	step.historyLoading = true
	window, shutdown := a.window, a.shutdown
	if !a.launchManagedBackground(func() {
		entries := loadProofreadingOCRStepHistory(ctx, sourceHash, pageCount, pages)
		if ctx.Err() != nil {
			return
		}
		result := proofreadingOCRStepHistoryResult{generation: generation, sourceHash: sourceHash, entries: entries}
		select {
		case step.historyResults <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-ctx.Done():
		case <-shutdown:
		}
	}) {
		cancel()
		step.historyCancel = nil
		step.historyLoading = false
		step.historyLoaded = true
	}
}

func (a *App) openProofreadingOCRStepTokenDialog(endpoint string) {
	if a == nil || a.proofreading == nil || a.modal.Visible {
		return
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
				if a.proofreading != nil && a.proofreading.ocrStep != nil {
					a.proofreading.ocrStep.autoCompareAfterOCR = false
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
			a.startProofreadingOCRPageStepWithAuthentication(token, endpoint, true)
		},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
		modalDialogActionSpec{ID: "start", LabelRef: i18n.Message{ID: "proofread.ai.step.recognize"}},
	)
	a.modal.Input.Mask = '•'
	a.modal.CardWidthDp = 470
	a.modal.FocusInputRequested = true
}

func alignProofreadingOCRLayoutPage(ctx context.Context, pageNumber int, layout ocr.Layout, markdown string) (proofreadingOCRPage, error) {
	if pageNumber <= 0 {
		return proofreadingOCRPage{}, errors.New("OCR 页面序号无效")
	}
	page := textalign.Page{Number: pageNumber}
	blocks := append([]ocr.LayoutBlock(nil), layout.Blocks...)
	sort.SliceStable(blocks, func(i, j int) bool {
		left, right := blocks[i].Order, blocks[j].Order
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		return *left < *right
	})
	for _, block := range blocks {
		page.Blocks = append(page.Blocks, textalign.Block{ID: block.ID, Label: block.Label, Text: block.Text, Bounds: block.Bounds})
	}
	result, err := textalign.Align(ctx, []textalign.Page{page}, markdown, textalign.Options{})
	if err != nil {
		return proofreadingOCRPage{}, err
	}
	if len(result) != 1 {
		return proofreadingOCRPage{}, errors.New("文字比对未返回当前页面结果")
	}
	return proofreadingOCRPage{layout: layout, result: result[0]}, nil
}

func shiftProofreadingOCRResult(result textalign.Result, delta int) textalign.Result {
	shiftedEvidence := make(map[*textalign.Evidence]struct{})
	shiftCandidate := func(candidate *textalign.Candidate) {
		if candidate == nil {
			return
		}
		candidate.Range.Start += delta
		candidate.Range.End += delta
		if len(candidate.Evidence) == 0 {
			return
		}
		first := &candidate.Evidence[0]
		if _, shifted := shiftedEvidence[first]; shifted {
			return
		}
		shiftedEvidence[first] = struct{}{}
		for i := range candidate.Evidence {
			candidate.Evidence[i].Range.Start += delta
			candidate.Evidence[i].Range.End += delta
		}
	}
	shiftCandidate(result.Selected)
	for i := range result.Candidates {
		shiftCandidate(&result.Candidates[i])
	}
	for i := range result.PathChoices {
		shiftCandidate(&result.PathChoices[i])
	}
	for i := range result.Anchors {
		result.Anchors[i].Range.Start += delta
		result.Anchors[i].Range.End += delta
	}
	return result
}

func mergeProofreadingOCRRanges(values []textalign.Range) []textalign.Range {
	if len(values) == 0 {
		return nil
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].Start == values[j].Start {
			return values[i].End < values[j].End
		}
		return values[i].Start < values[j].Start
	})
	out := values[:0]
	for _, value := range values {
		if len(out) > 0 && value.Start <= out[len(out)-1].End {
			out[len(out)-1].End = max(out[len(out)-1].End, value.End)
			continue
		}
		out = append(out, value)
	}
	return out
}

func proofreadingOCRStepStatusKeyForPages(step *proofreadingOCRStepState, aPage, bPage int, sourceHash string) string {
	if step == nil {
		return ""
	}
	if step.err != "" && step.page == aPage {
		return "proofread.ai.step.failed"
	}
	if step.active() {
		switch step.phase {
		case proofreadingOCRStepCheckingCache:
			return "proofread.ai.step.cache_checking"
		case proofreadingOCRStepAuthenticating:
			return "proofread.ai.step.authenticating"
		case proofreadingOCRStepRendering:
			return "proofread.ai.step.rendering"
		case proofreadingOCRStepUploading:
			return "proofread.ai.step.uploading"
		case proofreadingOCRStepRunning:
			return "proofread.ai.step.running"
		case proofreadingOCRStepComparing, proofreadingOCRStepCheckingBoundary:
			return "proofread.ai.step.comparing"
		}
	}
	if page, ok := step.pages[aPage]; ok && step.pageSourceHash[aPage] == sourceHash && page.layout.Width > 0 {
		if step.page == aPage && step.cacheHit {
			return "proofread.ai.step.cache_reused"
		}
		if step.page == aPage && step.cacheErr != "" {
			return "proofread.ai.step.cache_save_failed"
		}
		return "proofread.ai.step.recognized"
	}
	if step.page == aPage && step.status == "proofread.ai.step.cancelled" {
		return step.status
	}
	switch step.phase {
	case proofreadingOCRStepTokenRequired:
		if step.page == aPage {
			return "proofread.ai.step.token_required"
		}
	case proofreadingOCRStepFailed:
		if step.page == aPage {
			return "proofread.ai.step.failed"
		}
	default:
		return "proofread.ai.step.ready"
	}
	return "proofread.ai.step.ready"
}

func (a *App) currentProofreadingOCRStepComparison() (proofreadingOCRStepComparison, bool) {
	if a == nil || a.proofreading == nil || a.pdfView == nil {
		return proofreadingOCRStepComparison{}, false
	}
	state := a.proofreading
	step := state.ocrStep
	if step == nil {
		return proofreadingOCRStepComparison{}, false
	}
	view := a.pdfView
	aPage := max(0, min(view.info.Pages-1, view.page))
	bPage := a.proofreadingCurrentBPage()
	comparison, ok := step.comparisons[proofreadingOCRStepPair{APage: aPage, BPage: bPage}]
	if !ok || !proofreadingOCRStepComparisonValid(state, comparison) {
		return proofreadingOCRStepComparison{}, false
	}
	return comparison, true
}

func proofreadingOCRStepComparisonValid(state *proofreadingWorkspace, comparison proofreadingOCRStepComparison) bool {
	if state == nil || comparison.SourceHash != state.aSourceHash || comparison.BPage < 0 || comparison.BPage >= len(state.xPages) {
		return false
	}
	page := state.xPages[comparison.BPage]
	return comparison.BText == page.Content && comparison.BStart == page.ContentStart && comparison.BEnd == page.ContentEnd
}

func proofreadingOCRStepComparisonsForPage(state *proofreadingWorkspace, page int, source bool) []proofreadingOCRStepComparison {
	if state == nil || state.ocrStep == nil {
		return nil
	}
	var comparisons []proofreadingOCRStepComparison
	for _, comparison := range state.ocrStep.comparisons {
		matches := comparison.BPage == page
		if source {
			matches = comparison.APage == page
		}
		if matches && proofreadingOCRStepComparisonValid(state, comparison) {
			comparisons = append(comparisons, comparison)
		}
	}
	sort.Slice(comparisons, func(i, j int) bool {
		if comparisons[i].APage != comparisons[j].APage {
			return comparisons[i].APage < comparisons[j].APage
		}
		return comparisons[i].BPage < comparisons[j].BPage
	})
	return comparisons
}

func drawProofreadingStrikethrough(gtx layout.Context, bounds image.Rectangle, ink color.NRGBA) {
	midY := (bounds.Min.Y + bounds.Max.Y) / 2
	h := max(1, gtx.Dp(1.5))
	line := image.Rect(bounds.Min.X, midY-h/2, bounds.Max.X, midY-h/2+h)
	paint.FillShape(gtx.Ops, ink, clip.Rect(line).Op())
}

func drawProofreadingCaret(gtx layout.Context, bounds image.Rectangle, ink color.NRGBA) {
	caretX := bounds.Min.X
	w := max(2, gtx.Dp(2))
	stem := image.Rect(caretX-w/2, bounds.Min.Y, caretX-w/2+w, bounds.Max.Y)
	paint.FillShape(gtx.Ops, ink, clip.Rect(stem).Op())

	capW := max(6, gtx.Dp(6))
	capH := max(1, gtx.Dp(1.5))
	topCap := image.Rect(caretX-capW/2, bounds.Min.Y, caretX-capW/2+capW, bounds.Min.Y+capH)
	botCap := image.Rect(caretX-capW/2, bounds.Max.Y-capH, caretX-capW/2+capW, bounds.Max.Y)
	paint.FillShape(gtx.Ops, ink, clip.Rect(topCap).Op())
	paint.FillShape(gtx.Ops, ink, clip.Rect(botCap).Op())
}

func (a *App) drawProofreadingOCRStepComparisonHighlights(gtx layout.Context, editor *widget.Editor, page proofreadingXPage, window proofreadingEditorWindow) {
	if a == nil || a.proofreading == nil || editor == nil {
		return
	}
	for _, comparison := range proofreadingOCRStepComparisonsForPage(a.proofreading, page.PageIndex, false) {
		a.drawProofreadingOCRComparisonHighlights(gtx, editor, page, window, comparison)
	}
}

func (a *App) drawProofreadingOCRComparisonHighlights(gtx layout.Context, editor *widget.Editor, page proofreadingXPage, window proofreadingEditorWindow, comparison proofreadingOCRStepComparison) {
	strokeWidth := float32(max(1, gtx.Dp(1.5)))
	diffStroke := color.NRGBA{R: 217, G: 48, B: 37, A: 255}
	diffFill := color.NRGBA{R: 217, G: 48, B: 37, A: 38}
	greenStroke := color.NRGBA{R: 46, G: 160, B: 67, A: 255}
	greenFill := color.NRGBA{R: 46, G: 160, B: 67, A: 45}

	step := a.proofreadingOCRStep()
	if step != nil && (step.diffRunning || step.diffPaused) &&
		step.diffPair == (proofreadingOCRStepPair{APage: comparison.APage, BPage: comparison.BPage}) && step.diffDraftHash == comparison.DraftHash {
		frame, hasFrame := proofreadingOCRCurrentDiffFrame(step, comparison)
		if !hasFrame {
			return
		}
		if frame.Kind == proofreadingOCRDiffAOnly {
			regions := proofreadingOCRDiffAnchorRegions(editor, page, window, frame.BPosition)
			if len(regions) > 0 {
				drawProofreadingCaret(gtx, regions[0].Bounds, proofreadScanInk)
			}
			return
		}
		if frame.BPosition < page.ContentStart || frame.BPosition >= page.ContentEnd {
			return
		}
		start := frame.BPosition - page.ContentStart
		local, valid := proofreadingOCRHighlightLocalRange(proofreadingOCRRangeSegment{PageIndex: page.PageIndex, Start: start, End: start + 1}, window)
		if !valid {
			return
		}
		for _, region := range editor.Regions(local.Start, local.End, nil) {
			paint.FillShape(gtx.Ops, proofreadScanFill, clip.Rect(region.Bounds).Op())
			paint.FillShape(gtx.Ops, proofreadScanInk, clip.Stroke{Path: clip.Rect(region.Bounds).Path(), Width: float32(max(1, gtx.Dp(2)))}.Op())
			if frame.Kind == proofreadingOCRDiffBOnly {
				drawProofreadingStrikethrough(gtx, region.Bounds, proofreadScanInk)
			}
		}
		return
	}

	_, resolvedB := proofreadingOCRResolvedDiffRuns(a.pdfView, a.proofreading, step, comparison)
	seenAOnlyPositions := make(map[int]bool)
	type diffRun struct {
		kind       proofreadingOCRDiffOpKind
		start, end int
	}
	var runs []diffRun
	for _, frame := range comparison.Frames {
		if frame.Kind == proofreadingOCRDiffEqual {
			continue
		}
		if frame.Kind == proofreadingOCRDiffAOnly {
			if !seenAOnlyPositions[frame.BPosition] {
				seenAOnlyPositions[frame.BPosition] = true
				regions := proofreadingOCRDiffAnchorRegions(editor, page, window, frame.BPosition)
				if len(regions) > 0 {
					caretInk := diffStroke
					if resolvedB[frame.BPosition] {
						caretInk = greenStroke
					}
					drawProofreadingCaret(gtx, regions[0].Bounds, caretInk)
				}
			}
			continue
		}
		if frame.BPosition < page.ContentStart || frame.BPosition >= page.ContentEnd {
			continue
		}
		pos := frame.BPosition
		if len(runs) > 0 && runs[len(runs)-1].kind == frame.Kind && runs[len(runs)-1].end == pos {
			runs[len(runs)-1].end = pos + 1
		} else {
			runs = append(runs, diffRun{kind: frame.Kind, start: pos, end: pos + 1})
		}
	}

	for _, r := range runs {
		start, end := r.start-page.ContentStart, r.end-page.ContentStart
		local, valid := proofreadingOCRHighlightLocalRange(proofreadingOCRRangeSegment{PageIndex: page.PageIndex, Start: start, End: end}, window)
		if !valid {
			continue
		}
		isResolved := false
		for p := r.start; p < r.end; p++ {
			if resolvedB[p] {
				isResolved = true
				break
			}
		}
		stroke := diffStroke
		fill := diffFill
		if isResolved {
			stroke = greenStroke
			fill = greenFill
		}
		for _, region := range editor.Regions(local.Start, local.End, nil) {
			paint.FillShape(gtx.Ops, fill, clip.Rect(region.Bounds).Op())
			paint.FillShape(gtx.Ops, stroke, clip.Stroke{Path: clip.Rect(region.Bounds).Path(), Width: strokeWidth}.Op())
			if r.kind == proofreadingOCRDiffBOnly {
				drawProofreadingStrikethrough(gtx, region.Bounds, stroke)
			}
		}
	}
}

func proofreadOCRStepPageLabel(a *App, page int) string {
	if a == nil {
		return fmt.Sprint(page + 1)
	}
	return i18n.FormatNumber(a.localeSnapshot().Resolved, float64(page+1))
}

func (a *App) proofreadingDiffExceptionMap() map[rune]bool {
	if a == nil || a.proofreading == nil || a.proofreading.ocrStep == nil {
		return nil
	}
	return a.proofreading.ocrStep.inspector.exceptions.Map()
}

func filterProofreadingOCRComparisonExceptions(comparison *proofreadingOCRStepComparison, exceptions map[rune]bool) {
	if comparison == nil || len(exceptions) == 0 {
		return
	}
	filtered := make([]proofreadingOCRDiffFrame, 0, len(comparison.Frames))
	aOnly := 0
	bOnly := 0
	matches := 0
	for _, f := range comparison.Frames {
		if (f.AChar != 0 && exceptions[f.AChar]) || (f.BChar != 0 && exceptions[f.BChar]) {
			continue
		}
		filtered = append(filtered, f)
		switch f.Kind {
		case proofreadingOCRDiffEqual:
			matches++
		case proofreadingOCRDiffAOnly:
			aOnly++
		case proofreadingOCRDiffBOnly:
			bOnly++
		case proofreadingOCRDiffReplace:
			aOnly++
			bOnly++
		}
	}
	comparison.Frames = filtered
	comparison.AOnly = aOnly
	comparison.BOnly = bOnly
	comparison.Matches = matches
}

func rebaseProofreadingOCRStepComparison(comparison *proofreadingOCRStepComparison, newStart int) {
	if comparison == nil || comparison.BStart == newStart {
		return
	}
	delta := newStart - comparison.BStart
	comparison.BStart = newStart
	comparison.BEnd += delta
	for i := range comparison.Frames {
		comparison.Frames[i].BPosition += delta
	}
}
