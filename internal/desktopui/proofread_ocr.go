package desktopui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"glance/internal/i18n"
	"glance/internal/ocr"
	"glance/internal/textalign"
)

type proofreadingOCRStage uint8

const (
	proofreadingOCRIdle proofreadingOCRStage = iota
	proofreadingOCRUploading
	proofreadingOCRPending
	proofreadingOCRRunning
	proofreadingOCRMatching
	proofreadingOCRComplete
	proofreadingOCRCancelled
	proofreadingOCRFailed
)

type proofreadingOCRProgress struct {
	generation uint64
	stage      proofreadingOCRStage
	current    int
	total      int
	extracted  int
}

type proofreadingOCRPage struct {
	layout ocr.Layout
	result textalign.Result
}

type proofreadingOCRResult struct {
	generation uint64
	pages      []proofreadingOCRPage
	err        error
	cancelled  bool
}

type proofreadingOCRScan struct {
	button              widget.Clickable
	progress            chan proofreadingOCRProgress
	results             chan proofreadingOCRResult
	generation          uint64
	cancel              context.CancelFunc
	stage               proofreadingOCRStage
	current             int
	total               int
	extracted           int
	startedAt           time.Time
	markdownSnapshot    string
	markdownSnapshotSet bool
	pages               []proofreadingOCRPage
	err                 string
	notice              string
}

func newProofreadingOCRScan() *proofreadingOCRScan {
	return &proofreadingOCRScan{
		progress: make(chan proofreadingOCRProgress, 1),
		results:  make(chan proofreadingOCRResult, 1),
		current:  -1,
	}
}

func (s *proofreadingOCRScan) active() bool {
	if s == nil {
		return false
	}
	return s.stage == proofreadingOCRUploading || s.stage == proofreadingOCRPending || s.stage == proofreadingOCRRunning || s.stage == proofreadingOCRMatching
}

func (s *proofreadingOCRScan) close() {
	if s == nil {
		return
	}
	s.cancelTask()
	for {
		select {
		case <-s.progress:
		default:
			goto drainResults
		}
	}
drainResults:
	for {
		select {
		case <-s.results:
		default:
			return
		}
	}
}

func (s *proofreadingOCRScan) cancelTask() {
	if s == nil {
		return
	}
	s.generation++
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.markdownSnapshot = ""
	s.markdownSnapshotSet = false
	s.stage = proofreadingOCRCancelled
}

func (a *App) openProofreadingOCRTokenDialog() {
	if a == nil || a.proofreading == nil || a.modal.Visible {
		return
	}
	state := a.proofreading
	if state.bDocument == nil || state.bDocument.formatID() != "markdown" {
		state.ocrScan.notice = "proofread.ai.status.markdown_required"
		state.ocrScan.err = ""
		return
	}
	if a.pdfDoc == nil {
		state.ocrScan.notice = "proofread.ai.status.source_required"
		return
	}
	markdown := state.bDraftText
	if len(markdown) > 8<<20 {
		state.ocrScan.notice = "proofread.ai.status.markdown_too_large"
		return
	}
	state.ocrScan.notice = ""
	bodyID := "proofread.ai.token.body"
	if strings.TrimSpace(os.Getenv("PADDLEOCR_ENDPOINT")) != "" {
		bodyID = "proofread.ai.token.body.custom"
	}
	a.modal.ShowInputMessage(
		"proofread.ocr.token",
		i18n.Message{ID: "proofread.ai.token.title"},
		i18n.Message{ID: bodyID},
		"",
		i18n.Message{ID: "proofread.ai.token.hint"},
		func(action string) {
			token := strings.TrimSpace(a.modal.Input.Text())
			a.modal.Input.SetText("")
			a.modal.Input.Mask = 0
			if action == "cancel" {
				a.modal.Hide()
				return
			}
			if action != "start" && action != "confirm" {
				return
			}
			if token == "" {
				a.modal.SetMessageRef(i18n.Message{ID: "proofread.ai.token.required"})
				a.modal.Input.Mask = '•'
				a.modal.FocusInputRequested = true
				return
			}
			a.modal.Hide()
			a.startProofreadingOCRScan(token)
		},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
		modalDialogActionSpec{ID: "start", LabelRef: i18n.Message{ID: "proofread.ai.scan.start"}},
	)
	a.modal.Input.Mask = '•'
	a.modal.CardWidthDp = 470
	a.modal.FocusInputRequested = true
}

func (a *App) startProofreadingOCRScan(token string) {
	state := a.proofreading
	if state == nil || a.pdfDoc == nil || state.bDocument == nil || state.bDocument.formatID() != "markdown" {
		return
	}
	scan := state.ocrScan
	if scan == nil {
		scan = newProofreadingOCRScan()
		state.ocrScan = scan
	}
	markdown := state.bDraftText
	if len(markdown) > 8<<20 {
		scan.stage = proofreadingOCRFailed
		scan.notice = "proofread.ai.status.markdown_too_large"
		return
	}
	reader, _, _, _, err := a.pdfDoc.SnapshotReaderWithRevision()
	if err != nil {
		scan.stage = proofreadingOCRFailed
		scan.err = err.Error()
		return
	}
	info := a.pdfDoc.Info()
	pdfName := filepath.Base(state.aDocument.Path)
	if pdfName == "" || pdfName == "." {
		pdfName = "source.pdf"
	}
	scan.generation++
	generation := scan.generation
	ctx, cancel := context.WithCancel(context.Background())
	scan.cancel = cancel
	scan.stage = proofreadingOCRUploading
	scan.current = 0
	scan.total = max(1, info.Pages)
	scan.extracted = 0
	scan.startedAt = time.Now()
	scan.markdownSnapshot = markdown
	scan.markdownSnapshotSet = true
	scan.pages = nil
	scan.err = ""
	scan.notice = ""
	state.pageFilter = proofreadingPageFilterAll
	a.followProofreadingOCRPage(0)

	window, shutdown := a.window, a.shutdown
	progressChannel, resultChannel := scan.progress, scan.results
	if !a.launchManagedBackground(func() {
		defer reader.Close()
		defer func() { token = "" }()
		publish := func(progress proofreadingOCRProgress) {
			select {
			case <-ctx.Done():
				return
			default:
			}
			progress.generation = generation
			select {
			case progressChannel <- progress:
			default:
				select {
				case <-progressChannel:
				default:
				}
				select {
				case progressChannel <- progress:
				default:
				}
			}
			if window != nil {
				window.Invalidate()
			}
		}
		result := proofreadingOCRResult{generation: generation}
		defer func() {
			select {
			case resultChannel <- result:
				if window != nil {
					window.Invalidate()
				}
			case <-shutdown:
			}
		}()

		publish(proofreadingOCRProgress{stage: proofreadingOCRUploading, current: 0, total: max(1, info.Pages)})
		client := ocr.Client{Token: token, Endpoint: os.Getenv("PADDLEOCR_ENDPOINT")}
		job, submitErr := client.Submit(ctx, ocr.Source{Name: pdfName, Reader: reader, Size: int64(info.Bytes)}, ocr.Options{})
		if submitErr != nil {
			result.err = submitErr
			result.cancelled = errors.Is(ctx.Err(), context.Canceled)
			return
		}
		publish(proofreadingOCRProgress{stage: proofreadingOCRPending, total: max(1, info.Pages)})
		completedJob, waitErr := client.Wait(ctx, job.ID, func(update ocr.Job) {
			stage := proofreadingOCRPending
			if update.State == "running" || update.State == "done" {
				stage = proofreadingOCRRunning
			}
			if update.State == "done" {
				stage = proofreadingOCRMatching
			}
			total := update.Progress.TotalPages
			if total <= 0 {
				total = max(1, info.Pages)
			}
			current := proofreadingOCRCurrentPage(update.Progress.ExtractedPages, total)
			publish(proofreadingOCRProgress{stage: stage, current: current, total: total, extracted: update.Progress.ExtractedPages})
		})
		if waitErr != nil {
			result.err = waitErr
			result.cancelled = errors.Is(ctx.Err(), context.Canceled)
			return
		}
		publish(proofreadingOCRProgress{stage: proofreadingOCRMatching, current: max(0, info.Pages-1), total: max(1, info.Pages), extracted: max(0, info.Pages)})
		recognized, err := client.Results(ctx, completedJob)
		if err != nil {
			result.err = err
			result.cancelled = errors.Is(ctx.Err(), context.Canceled)
			return
		}
		if info.Pages > 0 && len(recognized) != info.Pages {
			result.err = fmt.Errorf("OCR 返回 %d 页，PDF 共 %d 页；页码无法可靠对应", len(recognized), info.Pages)
			return
		}
		layouts, alignedPages, err := proofreadingOCRAlignmentPages(recognized)
		if err != nil {
			result.err = err
			return
		}
		aligned, err := textalign.Align(ctx, alignedPages, markdown, textalign.Options{})
		if err != nil {
			result.err = err
			result.cancelled = errors.Is(ctx.Err(), context.Canceled)
			return
		}
		result.pages = make([]proofreadingOCRPage, len(layouts))
		for i := range layouts {
			result.pages[i] = proofreadingOCRPage{layout: layouts[i], result: aligned[i]}
		}
	}) {
		cancel()
		_ = reader.Close()
		scan.cancel = nil
		scan.stage = proofreadingOCRFailed
		scan.err = "无法启动 OCR 扫描任务"
	}
}

func proofreadingOCRAlignmentPages(recognized []ocr.Page) ([]ocr.Layout, []textalign.Page, error) {
	layouts := make([]ocr.Layout, 0, len(recognized))
	pages := make([]textalign.Page, 0, len(recognized))
	for pageIndex, result := range recognized {
		layoutResult, err := result.Layout()
		if err != nil {
			return nil, nil, err
		}
		sort.SliceStable(layoutResult.Blocks, func(i, j int) bool {
			left, right := layoutResult.Blocks[i].Order, layoutResult.Blocks[j].Order
			if left == nil {
				return false
			}
			if right == nil {
				return true
			}
			return *left < *right
		})
		page := textalign.Page{Number: pageIndex + 1}
		for _, block := range layoutResult.Blocks {
			page.Blocks = append(page.Blocks, textalign.Block{ID: block.ID, Label: block.Label, Text: block.Text, Bounds: block.Bounds})
		}
		layouts = append(layouts, layoutResult)
		pages = append(pages, page)
	}
	return layouts, pages, nil
}

func (a *App) processProofreadingOCRScan(gtx layout.Context) {
	if a == nil || a.proofreading == nil || a.proofreading.ocrScan == nil {
		return
	}
	scan := a.proofreading.ocrScan
	if scan.markdownSnapshotSet && (scan.active() || scan.stage == proofreadingOCRComplete) && scan.markdownSnapshot != a.proofreading.bDraftText {
		scan.cancelTask()
		scan.pages = nil
		scan.err = ""
		scan.notice = "proofread.ai.status.draft_changed"
	}
	for {
		select {
		case progress := <-scan.progress:
			if progress.generation != scan.generation {
				continue
			}
			oldPage := scan.current
			scan.stage = progress.stage
			scan.current = max(0, progress.current)
			scan.total = max(1, progress.total)
			scan.extracted = max(0, progress.extracted)
			if scan.current != oldPage {
				a.followProofreadingOCRPage(scan.current)
			}
		default:
			goto readResult
		}
	}
readResult:
	select {
	case result := <-scan.results:
		if result.generation != scan.generation {
			return
		}
		if scan.cancel != nil {
			scan.cancel()
			scan.cancel = nil
		}
		if result.cancelled {
			scan.stage = proofreadingOCRCancelled
			scan.err = ""
			scan.notice = ""
		} else if result.err != nil {
			scan.stage = proofreadingOCRFailed
			scan.err = result.err.Error()
			scan.notice = ""
		} else {
			scan.stage = proofreadingOCRComplete
			scan.pages = result.pages
			scan.err = ""
			scan.notice = ""
		}
		gtx.Execute(op.InvalidateCmd{})
	default:
	}
}

func (a *App) followProofreadingOCRPage(page int) {
	state := a.proofreading
	if state == nil || a.pdfDoc == nil {
		return
	}
	view := a.ensurePDFView()
	page = max(0, min(max(0, view.info.Pages-1), page))
	row, clearFilter := proofreadingOCRPageRow(state, max(1, view.info.Pages), page)
	if row < 0 {
		return
	}
	if clearFilter {
		state.pageFilter = proofreadingPageFilterAll
	}
	view.pageMap = a.proofreadingPageIndices(max(1, view.info.Pages))
	view.canvas.Position.First = row
	view.canvas.Position.Offset = 0
	view.canvas.Position.OffsetLast = 0
	view.canvas.Position.BeforeEnd = false
	if view.syncPage(page) {
		view.trimCacheWindow()
		state.navigator.initialized = false
	}
	view.followPDFThumbnailPage(page)
	state.page = page
	state.xCanvas.Position = view.canvas.Position
	if a.window != nil {
		a.window.Invalidate()
	}
}

func proofreadingOCRPageRow(state *proofreadingWorkspace, pageCount, page int) (int, bool) {
	pages := proofreadingFilteredPages(state.pageOwnership, pageCount, state.pageFilter)
	if row := proofreadingFilterRowForPage(pages, page); row >= 0 {
		return row, false
	}
	pages = proofreadingFilteredPages(state.pageOwnership, pageCount, proofreadingPageFilterAll)
	return proofreadingFilterRowForPage(pages, page), true
}

func proofreadingOCRCurrentPage(extractedPages, totalPages int) int {
	totalPages = max(1, totalPages)
	if extractedPages <= 0 {
		return 0
	}
	return min(totalPages-1, extractedPages-1)
}
