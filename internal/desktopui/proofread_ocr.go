package desktopui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"glance/internal/agentsettings"
	"glance/internal/i18n"
	"glance/internal/ocr"
	"glance/internal/textalign"
)

type proofreadingOCRStage uint8

const (
	proofreadingOCRIdle proofreadingOCRStage = iota
	proofreadingOCRCheckingCache
	proofreadingOCRAuthenticating
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
	layout            ocr.Layout
	result            textalign.Result
	missingPage       *proofreadingOCRMissingPageCandidate
	missingPageButton widget.Clickable
}

type proofreadingOCRResult struct {
	generation       uint64
	pages            []proofreadingOCRPage
	err              error
	cacheErr         error
	cacheHit         bool
	persistenceErr   error
	terminalJobState string
	authRejected     bool
	cancelled        bool
}

type proofreadingOCRCredentialResult struct {
	generation uint64
	endpoint   string
	job        *proofreadingOCRJobRecord
	auth       proofreadingOCRAuthentication
	token      string
	found      bool
	cacheErr   error
	err        error
}

type proofreadingOCRJobSubmission struct {
	generation uint64
	record     proofreadingOCRJobRecord
}

type proofreadingOCRJobPersistAck struct {
	jobID string
	err   error
}

type proofreadingOCRJobClient interface {
	Submit(context.Context, ocr.Source, ocr.Options) (ocr.Job, error)
	Wait(context.Context, string, func(ocr.Job)) (ocr.Job, error)
	Results(context.Context, ocr.Job) ([]ocr.Page, error)
}

type proofreadingOCRJobRun struct {
	CompletedJob   ocr.Job
	Pages          []ocr.Page
	PersistenceErr error
}

func runProofreadingOCRJob(ctx context.Context, client proofreadingOCRJobClient, existingJobID string, source ocr.Source, onSubmitted func(ocr.Job) error, progress func(ocr.Job)) (proofreadingOCRJobRun, error) {
	var run proofreadingOCRJobRun
	jobID := existingJobID
	if jobID == "" {
		job, err := client.Submit(ctx, source, ocr.Options{})
		if err != nil {
			return run, err
		}
		if job.ID == "" {
			return run, errors.New("OCR 服务未返回任务 ID")
		}
		jobID = job.ID
		if onSubmitted != nil {
			run.PersistenceErr = onSubmitted(job)
		}
	}
	completed, err := client.Wait(ctx, jobID, progress)
	if err != nil {
		return run, err
	}
	run.CompletedJob = completed
	pages, err := client.Results(ctx, completed)
	if err != nil {
		return run, err
	}
	run.Pages = pages
	return run, nil
}

type proofreadingOCRScan struct {
	button              widget.Clickable
	progress            chan proofreadingOCRProgress
	results             chan proofreadingOCRResult
	credentials         chan proofreadingOCRCredentialResult
	submissions         chan proofreadingOCRJobSubmission
	persistAcks         chan proofreadingOCRJobPersistAck
	generation          uint64
	cancel              context.CancelFunc
	stage               proofreadingOCRStage
	current             int
	total               int
	extracted           int
	startedAt           time.Time
	markdownSnapshot    string
	markdownSnapshotSet bool
	sourceAHash         string
	sourceBHash         string
	pages               []proofreadingOCRPage
	err                 string
	notice              string
	authLookup          bool
	authRejected        bool
	awaitingPersistJob  string
	currentJobID        string
	cacheHit            bool
	cacheErr            string
}

func newProofreadingOCRScan() *proofreadingOCRScan {
	return &proofreadingOCRScan{
		progress:    make(chan proofreadingOCRProgress, 1),
		results:     make(chan proofreadingOCRResult, 1),
		credentials: make(chan proofreadingOCRCredentialResult, 1),
		submissions: make(chan proofreadingOCRJobSubmission, 1),
		persistAcks: make(chan proofreadingOCRJobPersistAck, 1),
		current:     -1,
	}
}

func (s *proofreadingOCRScan) active() bool {
	if s == nil {
		return false
	}
	return s.stage == proofreadingOCRCheckingCache || s.stage == proofreadingOCRAuthenticating || s.stage == proofreadingOCRUploading || s.stage == proofreadingOCRPending || s.stage == proofreadingOCRRunning || s.stage == proofreadingOCRMatching
}

func (s *proofreadingOCRScan) close() {
	if s == nil {
		return
	}
	s.cancelTask()
	for {
		select {
		case <-s.progress:
		case <-s.results:
		case result := <-s.credentials:
			result.token = ""
		case <-s.submissions:
		case <-s.persistAcks:
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
	s.sourceAHash = ""
	s.sourceBHash = ""
	s.authLookup = false
	s.awaitingPersistJob = ""
	s.cacheHit = false
	s.cacheErr = ""
	s.stage = proofreadingOCRCancelled
}

func (a *App) openProofreadingOCRTokenDialog() {
	if a == nil || a.proofreading == nil {
		return
	}
	if a.proofreading.ocrScan == nil {
		a.proofreading.ocrScan = newProofreadingOCRScan()
	}
	endpoint, err := proofreadingOCREndpointFromEnvironment()
	if err != nil {
		if a != nil && a.proofreading != nil {
			a.proofreading.ocrScan.err = err.Error()
		}
		return
	}
	var job *proofreadingOCRJobRecord
	if a != nil && a.proofreading != nil && a.proofreading.ocrJob != nil {
		if candidate, ok := proofreadingOCRResumeJob(a.proofreading.ocrJob, a.proofreading.aSourceHash, a.proofreading.sourceBHash, a.proofreading.bDraftText); ok {
			job = candidate
			endpoint = candidate.Endpoint
		}
	}
	a.openProofreadingOCRTokenDialogFor(endpoint, job)
}

func (a *App) openProofreadingOCRTokenDialogFor(endpoint string, job *proofreadingOCRJobRecord) {
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
	endpoint, err := normalizeProofreadingOCREndpoint(endpoint)
	if err != nil {
		state.ocrScan.err = err.Error()
		return
	}
	var resumeRecord *proofreadingOCRJobRecord
	if job != nil {
		copy := *job
		resumeRecord = &copy
	}
	state.ocrScan.notice = ""
	bodyID := "proofread.ai.token.body"
	if endpoint != strings.TrimRight(ocr.DefaultEndpoint, "/") {
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
			a.startProofreadingOCRScanWithAuthentication(token, endpoint, resumeRecord, true)
		},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
		modalDialogActionSpec{ID: "start", LabelRef: i18n.Message{ID: "proofread.ai.scan.start"}},
	)
	a.modal.Input.Mask = '•'
	a.modal.CardWidthDp = 470
	a.modal.FocusInputRequested = true
}

func (a *App) startProofreadingOCRScan(token string) {
	endpoint, err := proofreadingOCREndpointFromEnvironment()
	if err != nil {
		if a != nil && a.proofreading != nil {
			a.proofreading.ocrScan.err = err.Error()
		}
		return
	}
	a.startProofreadingOCRScanWithAuthentication(token, endpoint, nil, true)
}

func proofreadingOCREndpointFromEnvironment() (string, error) {
	return normalizeProofreadingOCREndpoint(os.Getenv("PADDLEOCR_ENDPOINT"))
}

func (a *App) requestProofreadingOCRScan() {
	if a == nil || a.proofreading == nil {
		return
	}
	state := a.proofreading
	if state.ocrScan == nil {
		state.ocrScan = newProofreadingOCRScan()
	}
	if state.bDocument == nil || state.bDocument.formatID() != "markdown" {
		state.ocrScan.notice = "proofread.ai.status.markdown_required"
		state.ocrScan.err = ""
		return
	}
	if a.pdfDoc == nil {
		state.ocrScan.notice = "proofread.ai.status.source_required"
		return
	}
	if len(state.bDraftText) > 8<<20 {
		state.ocrScan.notice = "proofread.ai.status.markdown_too_large"
		return
	}
	endpoint, err := proofreadingOCREndpointFromEnvironment()
	if err != nil {
		state.ocrScan.err = err.Error()
		return
	}
	var resumeRecord *proofreadingOCRJobRecord
	if state.ocrJob != nil {
		if candidate, ok := proofreadingOCRResumeJob(state.ocrJob, state.aSourceHash, state.sourceBHash, state.bDraftText); ok {
			resumeRecord = candidate
			endpoint = candidate.Endpoint
		}
	}
	store := a.proofreadingStore()
	info := a.pdfDoc.Info()
	markdown := state.bDraftText
	sourceAHash := state.aSourceHash
	if state.ocrScan.cancel != nil {
		state.ocrScan.cancel()
	}
	scan := state.ocrScan
	scan.generation++
	generation := scan.generation
	ctx, cancel := context.WithCancel(context.Background())
	scan.cancel = cancel
	scan.authLookup = true
	scan.authRejected = false
	scan.stage = proofreadingOCRCheckingCache
	scan.notice = ""
	scan.err = ""
	scan.cacheHit = false
	scan.cacheErr = ""
	scan.current = -1
	scan.total = max(1, info.Pages)
	scan.extracted = 0
	scan.startedAt = time.Now()
	scan.currentJobID = ""
	scan.pages = nil
	a.refreshProofreadingOCRMissingPageCandidates()
	scan.markdownSnapshot = markdown
	scan.markdownSnapshotSet = true
	scan.sourceAHash = sourceAHash
	scan.sourceBHash = state.sourceBHash
	credentials := state.ocrCredentialStore
	if credentials == nil {
		credentials = agentsettings.SystemCredentialStore{}
	}
	resultChannel, window, shutdown := scan.credentials, a.window, a.shutdown
	if !a.launchManagedBackground(func() {
		finishScan := func(result proofreadingOCRResult) {
			result.generation = generation
			select {
			case scan.results <- result:
				if window != nil {
					window.Invalidate()
				}
			case <-shutdown:
			}
		}
		publishProgress := func(progress proofreadingOCRProgress) {
			progress.generation = generation
			select {
			case scan.progress <- progress:
			default:
				select {
				case <-scan.progress:
				default:
				}
				select {
				case scan.progress <- progress:
				default:
				}
			}
			if window != nil {
				window.Invalidate()
			}
		}
		options, model := ocr.Options{}, ocr.DefaultModel
		cachedPages, cacheHit, cacheErr := loadProofreadingOCRPageCache(sourceAHash, model, options, info.Pages)
		if err := ctx.Err(); err != nil {
			return
		}
		if cacheHit {
			publishProgress(proofreadingOCRProgress{stage: proofreadingOCRMatching, current: 0, total: len(cachedPages), extracted: len(cachedPages)})
			aligned, alignErr := alignProofreadingOCRPages(ctx, cachedPages, markdown)
			result := proofreadingOCRResult{cacheHit: true, cacheErr: cacheErr, err: alignErr, pages: aligned, cancelled: errors.Is(ctx.Err(), context.Canceled)}
			finishScan(result)
			return
		}
		publishProgress(proofreadingOCRProgress{stage: proofreadingOCRAuthenticating, total: max(1, info.Pages)})
		var auth proofreadingOCRAuthentication
		var token string
		var found bool
		var lookupErr error
		if store == nil {
			lookupErr = errors.New("OCR credential storage is unavailable")
		} else {
			auth, token, found, lookupErr = loadProofreadingOCRAuthentication(ctx, credentials, store, endpoint)
		}
		result := proofreadingOCRCredentialResult{generation: generation, endpoint: endpoint, job: resumeRecord, auth: auth, token: token, found: found, cacheErr: cacheErr, err: lookupErr}
		select {
		case resultChannel <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	}) {
		cancel()
		scan.cancel = nil
		scan.authLookup = false
		scan.stage = proofreadingOCRIdle
		a.openProofreadingOCRTokenDialogFor(endpoint, resumeRecord)
	}
}

func (a *App) startProofreadingOCRScanWithAuthentication(token, endpoint string, resumeRecord *proofreadingOCRJobRecord, persistCredential bool) {
	if a == nil {
		return
	}
	state := a.proofreading
	if state == nil || a.pdfDoc == nil || state.bDocument == nil || state.bDocument.formatID() != "markdown" || strings.TrimSpace(token) == "" {
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
	endpoint, err := normalizeProofreadingOCREndpoint(endpoint)
	if err != nil {
		scan.stage = proofreadingOCRFailed
		scan.err = err.Error()
		return
	}
	draftHash := proofreadingOCRDraftSnapshotHash(markdown)
	sourceAHash, sourceBHash := state.aSourceHash, state.sourceBHash
	existingJobID := ""
	if resumeRecord != nil {
		if !resumeRecord.validFor(state.aSourceHash, state.sourceBHash, draftHash) {
			scan.stage = proofreadingOCRFailed
			scan.notice = "proofread.ai.status.resume_source_changed"
			scan.err = ""
			return
		}
		endpoint, err = normalizeProofreadingOCREndpoint(resumeRecord.Endpoint)
		if err != nil {
			scan.stage = proofreadingOCRFailed
			scan.err = err.Error()
			return
		}
		existingJobID = resumeRecord.JobID
	}
	info := a.pdfDoc.Info()
	var reader io.ReadCloser
	if existingJobID == "" {
		reader, _, _, _, err = a.pdfDoc.SnapshotReaderWithRevision()
		if err != nil {
			scan.stage = proofreadingOCRFailed
			scan.err = err.Error()
			return
		}
	}
	pdfName := filepath.Base(state.aDocument.Path)
	if pdfName == "" || pdfName == "." {
		pdfName = "source.pdf"
	}
	if scan.cancel != nil {
		scan.cancel()
	}
	scan.generation++
	generation := scan.generation
	ctx, cancel := context.WithCancel(context.Background())
	scan.cancel = cancel
	scan.stage = proofreadingOCRUploading
	if existingJobID != "" {
		scan.stage = proofreadingOCRPending
	}
	scan.current = 0
	scan.total = max(1, info.Pages)
	scan.extracted = 0
	scan.startedAt = time.Now()
	scan.markdownSnapshot = markdown
	scan.markdownSnapshotSet = true
	scan.sourceAHash = sourceAHash
	scan.sourceBHash = sourceBHash
	scan.currentJobID = existingJobID
	scan.pages = nil
	scan.err = ""
	scan.notice = ""
	state.pageFilter = proofreadingPageFilterAll
	a.followProofreadingOCRPage(0)

	window, shutdown := a.window, a.shutdown
	progressChannel, resultChannel, submissionChannel, persistAcks := scan.progress, scan.results, scan.submissions, scan.persistAcks
	store := a.proofreadingStore()
	credentialStore := state.ocrCredentialStore
	if credentialStore == nil {
		credentialStore = agentsettings.SystemCredentialStore{}
	}
	if !a.launchManagedBackground(func() {
		if reader != nil {
			defer reader.Close()
		}
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

		if persistCredential {
			if store == nil {
				result.err = errors.New("OCR workspace credential storage is unavailable")
				return
			}
			if _, saveErr := saveProofreadingOCRAuthentication(ctx, credentialStore, store, endpoint, token); saveErr != nil {
				result.err = saveErr
				result.cancelled = errors.Is(ctx.Err(), context.Canceled)
				return
			}
		}
		if existingJobID == "" {
			publish(proofreadingOCRProgress{stage: proofreadingOCRUploading, current: 0, total: max(1, info.Pages)})
		} else {
			publish(proofreadingOCRProgress{stage: proofreadingOCRPending, total: max(1, info.Pages)})
		}
		client := &ocr.Client{Token: token, Endpoint: endpoint}
		source := ocr.Source{}
		if reader != nil {
			source = ocr.Source{Name: pdfName, Reader: reader, Size: int64(info.Bytes)}
		}
		onSubmitted := func(job ocr.Job) error {
			record := proofreadingOCRJobRecord{
				JobID: job.ID, Endpoint: endpoint, Model: ocr.DefaultModel,
				SourceAHash: sourceAHash, SourceBHash: sourceBHash, DraftHash: draftHash,
				CreatedAt: time.Now().UTC(), State: job.State,
			}
			select {
			case submissionChannel <- proofreadingOCRJobSubmission{generation: generation, record: record}:
			case <-ctx.Done():
				return ctx.Err()
			case <-shutdown:
				return errors.New("application is shutting down")
			}
			if window != nil {
				window.Invalidate()
			}
			for {
				select {
				case ack := <-persistAcks:
					if ack.jobID == job.ID {
						return ack.err
					}
				case <-ctx.Done():
					return ctx.Err()
				case <-shutdown:
					return errors.New("application is shutting down")
				}
			}
		}
		jobRun, runErr := runProofreadingOCRJob(ctx, client, existingJobID, source, onSubmitted, func(update ocr.Job) {
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
		result.persistenceErr = jobRun.PersistenceErr
		if runErr != nil {
			result.err = runErr
			result.authRejected = errors.Is(runErr, ocr.ErrAuthentication)
			switch {
			case errors.Is(runErr, ocr.ErrJobFailed):
				result.terminalJobState = "failed"
			case errors.Is(runErr, ocr.ErrJobUnavailable):
				result.terminalJobState = "expired"
			}
			result.cancelled = errors.Is(ctx.Err(), context.Canceled)
			return
		}
		publish(proofreadingOCRProgress{stage: proofreadingOCRMatching, current: max(0, info.Pages-1), total: max(1, info.Pages), extracted: max(0, info.Pages)})
		recognized := jobRun.Pages
		if info.Pages > 0 && len(recognized) != info.Pages {
			result.err = fmt.Errorf("OCR 返回 %d 页，PDF 共 %d 页；页码无法可靠对应", len(recognized), info.Pages)
			return
		}
		expectedPages := info.Pages
		if expectedPages <= 0 {
			expectedPages = len(recognized)
		}
		if cacheErr := saveProofreadingOCRPageCache(sourceAHash, ocr.DefaultModel, jobRun.CompletedJob.ID, ocr.Options{}, recognized, expectedPages); cacheErr != nil {
			result.cacheErr = cacheErr
		}
		result.pages, result.err = alignProofreadingOCRPages(ctx, recognized, markdown)
		result.cancelled = errors.Is(ctx.Err(), context.Canceled)
	}) {
		cancel()
		if reader != nil {
			_ = reader.Close()
		}
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

func alignProofreadingOCRPages(ctx context.Context, recognized []ocr.Page, markdown string) ([]proofreadingOCRPage, error) {
	layouts, pages, err := proofreadingOCRAlignmentPages(recognized)
	if err != nil {
		return nil, err
	}
	aligned, err := textalign.Align(ctx, pages, markdown, textalign.Options{})
	if err != nil {
		return nil, err
	}
	result := make([]proofreadingOCRPage, len(layouts))
	for i := range layouts {
		result[i] = proofreadingOCRPage{layout: layouts[i], result: aligned[i]}
	}
	return result, nil
}

func (a *App) processProofreadingOCRScan(gtx layout.Context) {
	if a == nil || a.proofreading == nil || a.proofreading.ocrScan == nil {
		return
	}
	scan := a.proofreading.ocrScan
	if scan.markdownSnapshotSet && (scan.active() || scan.stage == proofreadingOCRComplete) {
		staleDraft := scan.markdownSnapshot != a.proofreading.bDraftText
		staleSource := scan.sourceAHash != a.proofreading.aSourceHash || scan.sourceBHash != a.proofreading.sourceBHash
		if staleDraft || staleSource {
			scan.cancelTask()
			scan.pages = nil
			scan.err = ""
			if staleDraft {
				scan.notice = "proofread.ai.status.draft_changed"
			} else {
				scan.notice = "proofread.ai.status.resume_source_changed"
			}
		}
	}
	for {
		select {
		case result := <-scan.credentials:
			if result.generation != scan.generation {
				result.token = ""
				continue
			}
			scan.authLookup = false
			scan.cacheErr = ""
			if result.cacheErr != nil {
				scan.cacheErr = result.cacheErr.Error()
			}
			if scan.cancel != nil {
				scan.cancel()
				scan.cancel = nil
			}
			if result.err != nil || !result.found {
				scan.stage = proofreadingOCRIdle
				if result.err != nil {
					scan.notice = "proofread.ai.status.credential_unavailable"
				}
				a.openProofreadingOCRTokenDialogFor(result.endpoint, result.job)
				result.token = ""
				continue
			}
			scan.authRejected = false
			a.startProofreadingOCRScanWithAuthentication(result.token, result.endpoint, result.job, false)
			result.token = ""
		case submission := <-scan.submissions:
			if submission.generation != scan.generation {
				sendProofreadingOCRPersistAck(scan, proofreadingOCRJobPersistAck{jobID: submission.record.JobID, err: context.Canceled})
				continue
			}
			record := submission.record
			scan.currentJobID = record.JobID
			scan.awaitingPersistJob = record.JobID
			a.proofreading.ocrJob = &record
			if a.proofreading.projectDocumentID == "" || a.proofreading.controller == nil || !a.proofreading.modelInitialAssigned || !a.proofreading.bDraftInitialized {
				persistErr := errors.New("OCR task cannot be saved before proofreading recovery state is ready")
				scan.notice = "proofread.ai.status.resume_unavailable"
				scan.awaitingPersistJob = ""
				sendProofreadingOCRPersistAck(scan, proofreadingOCRJobPersistAck{jobID: record.JobID, err: persistErr})
				continue
			}
			if !a.queueProofreadingDraftPersistence() {
				persistErr := errors.New("OCR task recovery record could not be queued for saving")
				scan.notice = "proofread.ai.status.resume_unavailable"
				scan.awaitingPersistJob = ""
				sendProofreadingOCRPersistAck(scan, proofreadingOCRJobPersistAck{jobID: record.JobID, err: persistErr})
			}
		default:
			goto readProgress
		}
	}
readProgress:
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
		scan.authLookup = false
		scan.cacheHit = result.cacheHit
		scan.cacheErr = ""
		if result.cacheErr != nil {
			scan.cacheErr = result.cacheErr.Error()
		}
		if result.cancelled {
			scan.stage = proofreadingOCRCancelled
			scan.err = ""
			if scan.notice != "proofread.ai.status.resume_unavailable" {
				scan.notice = ""
			}
		} else if result.err != nil {
			scan.stage = proofreadingOCRFailed
			scan.err = result.err.Error()
			scan.authRejected = result.authRejected
			if result.terminalJobState != "" {
				scan.err = ""
				scan.authRejected = false
				scan.notice = "proofread.ai.status.job_terminal"
				if a.proofreading.ocrJob != nil && scan.currentJobID != "" && a.proofreading.ocrJob.JobID == scan.currentJobID {
					record := *a.proofreading.ocrJob
					record.State = result.terminalJobState
					a.proofreading.ocrJob = &record
					if !a.queueProofreadingDraftPersistence() {
						scan.notice = "proofread.ai.status.resume_unavailable"
					}
				}
			} else if result.authRejected {
				scan.notice = "proofread.ai.status.authentication_rejected"
			} else if scan.notice != "proofread.ai.status.resume_unavailable" {
				scan.notice = ""
			}
		} else {
			scan.stage = proofreadingOCRComplete
			scan.pages = result.pages
			a.refreshProofreadingOCRMissingPageCandidates()
			scan.err = ""
			scan.authRejected = false
			if result.persistenceErr != nil {
				scan.notice = "proofread.ai.status.resume_unavailable"
			} else {
				scan.notice = ""
			}
		}
		gtx.Execute(op.InvalidateCmd{})
	default:
	}
}

func sendProofreadingOCRPersistAck(scan *proofreadingOCRScan, ack proofreadingOCRJobPersistAck) {
	if scan == nil || scan.persistAcks == nil {
		return
	}
	select {
	case scan.persistAcks <- ack:
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
