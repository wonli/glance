package desktopui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	proofreadingOCRStepCompared
	proofreadingOCRStepFailed
)

type proofreadingOCRStepResult struct {
	generation uint64
	page       int
	sourceHash string
	ocrPage    *ocr.Page
	comparison *proofreadingOCRStepComparison
	cacheHit   bool
	cacheErr   error
	needToken  bool
	endpoint   string
	err        error
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
	APage         int
	BPage         int
	SourceHash    string
	DraftHash     string
	BText         string
	BStart        int
	BEnd          int
	Result        textalign.Result
	Matches       int
	AOnly         int
	BOnly         int
	MatchedText   string
	AOnlyText     string
	BOnlyText     string
	HasEvidence   bool
	Complete      bool
	MatchedRanges []textalign.Range
	Differences   []textalign.Range
	AOnlyBlockIDs []int
}

type proofreadingOCRStepState struct {
	manualModeButton widget.Clickable
	aiModeButton     widget.Clickable
	ocrButton        widget.Clickable
	compareButton    widget.Clickable
	diffList         widget.List
	pages            map[int]proofreadingOCRPage
	pageSourceHash   map[int]string
	comparisons      map[proofreadingOCRStepPair]proofreadingOCRStepComparison
	results          chan proofreadingOCRStepResult
	progress         chan proofreadingOCRStepProgress
	generation       uint64
	page             int
	bPage            int
	phase            proofreadingOCRStepPhase
	extracted        int
	total            int
	startedAt        time.Time
	cancel           context.CancelFunc
	cacheHit         bool
	cacheErr         string
	err              string
	status           string
	sourceHash       string
}

func newProofreadingOCRStepState() *proofreadingOCRStepState {
	return &proofreadingOCRStepState{
		pages:          map[int]proofreadingOCRPage{},
		pageSourceHash: map[int]string{},
		comparisons:    map[proofreadingOCRStepPair]proofreadingOCRStepComparison{},
		results:        make(chan proofreadingOCRStepResult, 1),
		progress:       make(chan proofreadingOCRStepProgress, 1),
		page:           -1,
		bPage:          -1,
	}
}

func (s *proofreadingOCRStepState) active() bool {
	if s == nil {
		return false
	}
	switch s.phase {
	case proofreadingOCRStepCheckingCache, proofreadingOCRStepAuthenticating,
		proofreadingOCRStepRendering, proofreadingOCRStepUploading,
		proofreadingOCRStepRunning, proofreadingOCRStepComparing:
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
	for {
		select {
		case <-s.results:
		case <-s.progress:
		default:
			return
		}
	}
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
		step.cancelTask()
		return
	}
	if state.bDocument == nil || state.bDocument.formatID() != "markdown" {
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
	}
	step.sourceHash = state.aSourceHash
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
	delete(step.comparisons, proofreadingOCRStepPair{APage: pageIndex, BPage: step.bPage})
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
	}
	step.sourceHash = sourceHash
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
	if step == nil || step.active() {
		return
	}
	view := a.ensurePDFView()
	aPage := max(0, min(view.info.Pages-1, view.page))
	bPage := a.proofreadingCurrentBPage()
	ocrPage, found := step.pages[aPage]
	if !found || step.pageSourceHash[aPage] != state.aSourceHash {
		step.phase = proofreadingOCRStepFailed
		step.err = a.textID("proofread.ai.step.ocr_required")
		return
	}
	if bPage < 0 || bPage >= len(state.xPages) {
		step.phase = proofreadingOCRStepFailed
		step.err = a.textID("proofread.ai.step.page_unavailable")
		return
	}
	if ocrPage.layout.Width <= 0 || ocrPage.layout.Height <= 0 {
		step.phase = proofreadingOCRStepFailed
		step.err = a.textID("proofread.ai.step.ocr_required")
		return
	}
	b := state.xPages[bPage]
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
	delete(step.comparisons, proofreadingOCRStepPair{APage: aPage, BPage: bPage})
	layout := ocrPage.layout
	bText, bStart, bEnd := b.Content, b.ContentStart, b.ContentEnd
	sourceHash := state.aSourceHash
	draftHash := proofreadingOCRDraftSnapshotHash(state.bDraftText)
	window, shutdown := a.window, a.shutdown
	if !a.launchManagedBackground(func() {
		result := proofreadingOCRStepResult{generation: generation, page: aPage, sourceHash: sourceHash}
		page, err := alignProofreadingOCRLayoutPage(ctx, aPage+1, layout, bText)
		if err != nil {
			result.err = err
		} else {
			page.result = shiftProofreadingOCRResult(page.result, bStart)
			diff := proofreadingOCRCharacterDiff(layout, page.result, bText, bStart)
			comparison := proofreadingOCRStepComparison{
				APage: aPage, BPage: bPage, SourceHash: sourceHash, DraftHash: draftHash,
				BText: bText, BStart: bStart, BEnd: bEnd, Result: page.result,
				Matches: diff.Matches, AOnly: diff.AOnly, BOnly: diff.BOnly,
				MatchedText: diff.MatchedText,
				AOnlyText:   diff.AOnlyText, BOnlyText: diff.BOnlyText,
				HasEvidence: diff.HasEvidence, Complete: diff.Complete, MatchedRanges: diff.MatchedRanges,
				Differences: diff.BOnlyRanges, AOnlyBlockIDs: diff.AOnlyBlockIDs,
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
	if a.proofreading != nil && step.sourceHash != "" && step.sourceHash != a.proofreading.aSourceHash {
		step.cancelTask()
		step.pages = map[int]proofreadingOCRPage{}
		step.pageSourceHash = map[int]string{}
		step.comparisons = map[proofreadingOCRStepPair]proofreadingOCRStepComparison{}
		step.sourceHash = a.proofreading.aSourceHash
	}
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
		} else if result.comparison != nil {
			comparison := *result.comparison
			if comparison.SourceHash != a.proofreading.aSourceHash || comparison.DraftHash != proofreadingOCRDraftSnapshotHash(a.proofreading.bDraftText) || comparison.BPage < 0 || comparison.BPage >= len(a.proofreading.xPages) || comparison.BText != a.proofreading.xPages[comparison.BPage].Content || comparison.BStart != a.proofreading.xPages[comparison.BPage].ContentStart || comparison.BEnd != a.proofreading.xPages[comparison.BPage].ContentEnd {
				step.phase = proofreadingOCRStepFailed
				step.err = a.textID("proofread.ai.step.stale")
			} else {
				step.comparisons[proofreadingOCRStepPair{APage: comparison.APage, BPage: comparison.BPage}] = comparison
				step.phase = proofreadingOCRStepCompared
				step.err = ""
				step.status = "proofread.ai.step.compared"
			}
		} else if result.ocrPage != nil {
			layout, err := result.ocrPage.Layout()
			if err != nil {
				step.phase = proofreadingOCRStepFailed
				step.err = err.Error()
			} else {
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
						delete(step.comparisons, pair)
					}
				}
			}
		}
		gtx.Execute(op.InvalidateCmd{})
	default:
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

func proofreadingOCRComparisonCandidate(result textalign.Result) *textalign.Candidate {
	if result.Selected != nil {
		return result.Selected
	}
	if len(result.PathChoices) > 0 {
		return &result.PathChoices[0]
	}
	return nil
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
		case proofreadingOCRStepComparing:
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
	if state == nil || comparison.SourceHash != state.aSourceHash || comparison.DraftHash != proofreadingOCRDraftSnapshotHash(state.bDraftText) || comparison.BPage < 0 || comparison.BPage >= len(state.xPages) {
		return false
	}
	page := state.xPages[comparison.BPage]
	return comparison.BText == page.Content && comparison.BStart == page.ContentStart && comparison.BEnd == page.ContentEnd
}

func (a *App) drawProofreadingOCRStepComparisonHighlights(gtx layout.Context, editor *widget.Editor, page proofreadingXPage, window proofreadingEditorWindow) {
	if a == nil || editor == nil {
		return
	}
	comparison, ok := a.currentProofreadingOCRStepComparison()
	if !ok || comparison.BPage != page.PageIndex {
		return
	}
	matchColor := color.NRGBA{R: 57, G: 145, B: 100, A: 88}
	if comparison.Result.Status == "ambiguous" {
		matchColor = color.NRGBA{R: 213, G: 151, B: 49, A: 72}
	}
	for _, matched := range comparison.MatchedRanges {
		start, end := matched.Start-page.ContentStart, matched.End-page.ContentStart
		local, valid := proofreadingOCRHighlightLocalRange(proofreadingOCRRangeSegment{PageIndex: page.PageIndex, Start: start, End: end}, proofreadingEditorWindow{Start: window.Start, End: window.End})
		if !valid {
			continue
		}
		for _, region := range editor.Regions(local.Start, local.End, nil) {
			paint.FillShape(gtx.Ops, matchColor, clip.Rect(region.Bounds).Op())
		}
	}
	diffColor := color.NRGBA{R: 51, G: 124, B: 205, A: 112}
	for _, diff := range comparison.Differences {
		start, end := diff.Start-page.ContentStart, diff.End-page.ContentStart
		local, valid := proofreadingOCRHighlightLocalRange(proofreadingOCRRangeSegment{PageIndex: page.PageIndex, Start: start, End: end}, window)
		if !valid {
			continue
		}
		for _, region := range editor.Regions(local.Start, local.End, nil) {
			paint.FillShape(gtx.Ops, diffColor, clip.Rect(region.Bounds).Op())
		}
	}
}

func proofreadOCRStepPageLabel(a *App, page int) string {
	if a == nil {
		return fmt.Sprint(page + 1)
	}
	return i18n.FormatNumber(a.localeSnapshot().Resolved, float64(page+1))
}
