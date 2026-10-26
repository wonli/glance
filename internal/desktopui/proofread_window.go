package desktopui

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"

	"context"
	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/format/markdowndoc"
	"glance/internal/format/pdfdoc"
	"glance/internal/proofreading"
	"glance/internal/workspace"
)

type proofreadingWindowRequest struct {
	resumePayload  *proofreadingProjectPayload
	resume         *workspace.ProofreadingProject
	showHistory    bool
	sourcePath     string
	sourceName     string
	sourcePage     int
	sourceSnapshot io.ReadCloser
}

var errProofreadingFileTooLarge = errors.New("proofreading file exceeds 50 MB")

// A is the visual source, B is the selected content document, and X is an
// editable working projection that uses A's page geometry. The loaded B file
// remains an input snapshot; edits live in the proofreading workspace draft.
type proofreadingDocumentRole string

const (
	proofreadingRoleA proofreadingDocumentRole = "A"
	proofreadingRoleB proofreadingDocumentRole = "B"
	proofreadingRoleX proofreadingDocumentRole = "X"
)

type proofreadingUnitKind string

const (
	proofreadingUnitBlock       proofreadingUnitKind = "block"
	proofreadingUnitParagraph   proofreadingUnitKind = "paragraph"
	proofreadingUnitHeading     proofreadingUnitKind = "heading"
	proofreadingUnitList        proofreadingUnitKind = "list"
	proofreadingUnitQuote       proofreadingUnitKind = "quote"
	proofreadingUnitCode        proofreadingUnitKind = "code"
	proofreadingUnitMath        proofreadingUnitKind = "math"
	proofreadingUnitRule        proofreadingUnitKind = "rule"
	proofreadingUnitTableRow    proofreadingUnitKind = "table_row"
	proofreadingUnitUnsupported proofreadingUnitKind = "unsupported"
)

// Content units retain the source adapter's logical structure. X projects the
// normalized B text flow onto page frames copied from A without reading A text.
type proofreadingContentUnit struct {
	ID      string
	Kind    proofreadingUnitKind
	Text    string
	Ordinal int
	Level   int
	Start   int
	End     int
}

type proofreadingAdapterDocument interface {
	FormatID() string
	ContentUnits() []proofreadingContentUnit
	Close() error
}

type proofreadingFormatAdapter interface {
	Open(path string) (proofreadingAdapterDocument, error)
}

type proofreadingObject struct {
	Role    proofreadingDocumentRole
	Path    string
	Name    string
	Adapter proofreadingAdapterDocument
}

func (o *proofreadingObject) formatID() string {
	if o == nil || o.Adapter == nil {
		return ""
	}
	return o.Adapter.FormatID()
}

func (o *proofreadingObject) contentUnits() []proofreadingContentUnit {
	if o == nil || o.Adapter == nil {
		return nil
	}
	return o.Adapter.ContentUnits()
}

type proofreadingPageFrame struct {
	PageIndex        int
	Width            float64
	Height           float64
	FrameUnavailable bool
}

type proofreadingXPage struct {
	PageIndex        int
	BPageIndex       int
	Width            float64
	Height           float64
	FrameUnavailable bool
	ContentStart     int // Rune offsets into the editable B working draft.
	ContentEnd       int
	Content          string
	Blocks           []proofreadingXBlock
}

type proofreadingXBlock struct {
	Kind         proofreadingUnitKind
	Level        int
	Text         string
	ContentStart int
	ContentEnd   int
}

type proofreadingPagePoint struct {
	X float64
	Y float64
}

type proofreadingManualBreak struct {
	Page        *int `json:"page,omitempty"`
	Offset      int
	Point       proofreadingPagePoint
	AtPageStart bool
}

type proofreadingPageFrameProvider interface {
	PageCount() int
	PageFrame(page int) (proofreadingPageFrame, error)
}

type proofreadingWorkspace struct {
	resumePayload            *proofreadingProjectPayload
	resume                   *workspace.ProofreadingProject
	history                  proofreadingHistoryState
	pendingView              string
	viewSaveAt               time.Time
	restoredConfirmedCurrent map[int]bool
	restoredXOffsets         []float64
	lastSavedView            string
	visibleEditorPages       []int
	navigator                proofreadingNavigator
	xEditorWindows           []proofreadingEditorWindow
	xEditorCarets            []proofreadingEditorCaret
	xPageRenderScales        []float64
	xPageViewportHeights     []float64
	xPageScrollOffsets       []float64
	xRevealCaret             []bool

	pendingXPageWheel      map[int]float64
	xEditorsSnapshot       proofreading.Snapshot
	controller             *application.ProofreadingController
	restoredModel          *proofreading.Snapshot
	modelInitialAssigned   bool
	modelLayoutResults     chan proofreadingModelLayoutResult
	modelLayoutCancel      context.CancelFunc
	modelLayoutGeneration  uint64
	modelLayoutPending     bool
	modelDirtyPages        map[proofreading.PageID]bool
	modelLayoutNeedsFull   bool
	modelLayoutActivePages []proofreading.PageID
	modelRequestedPages    []proofreading.PageID
	xEditorTexts           []string
	xViewport              image.Point

	xPageAdjustmentActive bool
	xPageAdjustmentPage   int
	ocrScan               *proofreadingOCRScan
	ocrStep               *proofreadingOCRStepState
	aiProofreading        bool
	ocrJob                *proofreadingOCRJobRecord
	ocrCredentialStore    proofreadingOCRCredentialStore
	ocrPreview            *proofreadingOCRMissingPageCandidate
	ocrPreviewList        widget.List
	aDocument             *proofreadingObject
	aSourceHash           string
	page                  int
	sourceLoading         bool
	sourceError           string
	sourceResults         chan proofreadingSourceResult
	sourceGen             uint64

	frameLoading                                 bool
	frameError                                   string
	frameResults                                 chan proofreadingPageFrameResult
	frameGen                                     uint64
	pageFrames                                   []proofreadingPageFrame
	xPages                                       []proofreadingXPage
	bDraftText                                   string
	bDraftInitialized                            bool
	bDraftDirty                                  bool
	pageOwnership                                []proofreadingPageOwnership
	pageFilter                                   proofreadingPageFilter
	filteredPageIndices                          []int
	filterReviewedButton, filterUnreviewedButton widget.Clickable
	xPageUp, xPageUnreviewed, xPageReviewed      widget.Clickable
	xPageScrollbars                              []widget.Scrollbar
	xPageEditors                                 []widget.Editor
	xTextSizeStep                                int
	xPagePointerTags                             []proofreadingPagePointerTag
	xPageMenuOpen                                bool
	xPageMenuPage                                int
	xPageMenuPoint                               proofreadingPagePoint
	xPageMenuBounds                              image.Rectangle
	xCanvasScrollPointerTag                      struct{}
	xCanvasScrollRemainder                       float32
	xScrollHighlightPage                         int
	xScrollHighlightUntil                        time.Time

	bDocument           *proofreadingObject
	draftLoading        bool
	draftError          string
	choosingDraft       bool
	draftResults        chan proofreadingDraftResult
	draftGen            uint64
	draftRestoreResults chan proofreadingDraftRestoreResult
	draftSaveResults    chan proofreadingDraftSaveResult
	draftRestoreGen     uint64
	draftRestoreFailed  bool
	draftPersistGen     atomic.Uint64
	projectDocumentID   string
	sourceBHash         string
	draftFilePath       string
	draftFileMu         sync.Mutex
	draftRestoring      bool
	draftSavePending    bool
	draftAutoSaved      bool
	draftPersistError   string
	chooseDraft         widget.Clickable
	xTextSmaller        widget.Clickable
	xTextLarger         widget.Clickable
	xInsertPageBreak    widget.Clickable
	xPageMenuDismiss    widget.Clickable
	xUndoPaginationMenu widget.Clickable
	xUndoPageBreak      widget.Clickable
	xCanvas             widget.List

	toolbarExceptionsBtn widget.Clickable
	exceptionsModalOpen  bool
	exceptionInput       widget.Editor
	addExceptionBtn      widget.Clickable
	closeExceptionsBtn   widget.Clickable
	removeExceptionBtns  []widget.Clickable
}

type proofreadingSourceResult struct {
	generation uint64
	object     *proofreadingObject
	sourceHash string
	err        error
}

type proofreadingDraftResult struct {
	generation uint64
	object     *proofreadingObject
	err        error
}

type proofreadingDraftRestoreResult struct {
	generation uint64
	draft      *proofreadingPersistedDraft
	found      bool
	err        error
}

type proofreadingDraftSaveResult struct {
	generation uint64
	ocrJobID   string
	err        error
}

type proofreadingPageFrameResult struct {
	generation uint64
	pages      []proofreadingPageFrame
	err        error
}

func newProofreadingWorkspace(request proofreadingWindowRequest) *proofreadingWorkspace {
	state := &proofreadingWorkspace{
		resume:              request.resume,
		resumePayload:       request.resumePayload,
		page:                max(0, request.sourcePage),
		sourceLoading:       true,
		sourceResults:       make(chan proofreadingSourceResult, 1),
		frameResults:        make(chan proofreadingPageFrameResult, 1),
		draftResults:        make(chan proofreadingDraftResult, 1),
		modelLayoutResults:  make(chan proofreadingModelLayoutResult, 1),
		draftRestoreResults: make(chan proofreadingDraftRestoreResult, 1),
		draftSaveResults:    make(chan proofreadingDraftSaveResult, 8),
		ocrScan:             newProofreadingOCRScan(),
		ocrStep:             newProofreadingOCRStepState(),
		aDocument:           &proofreadingObject{Role: proofreadingRoleA, Path: request.sourcePath, Name: request.sourceName},
	}
	state.xCanvas.Axis = layout.Vertical
	return state
}

func (a *App) openProofreadingWindow() error {
	if a.pdfDoc == nil {
		return fmt.Errorf("当前没有打开的 PDF 文档")
	}
	entry := a.ensureProofreadingPDFEntry()
	if entry.busy {
		return nil
	}
	view := a.ensurePDFView()
	reader, _, _, _, err := a.pdfDoc.SnapshotReaderWithRevision()
	if err != nil {
		return fmt.Errorf("读取 PDF 快照失败：%w", err)
	}
	hashReader, _, _, _, err := a.pdfDoc.SnapshotReaderWithRevision()
	if err != nil {
		reader.Close()
		return fmt.Errorf("读取 PDF 快照失败：%w", err)
	}
	path := sessionPath(a.documentSessionState)
	name := filepath.Base(path)
	if path == "" || name == "." {
		name = a.textID("pdf.proofread.source_a_untitled")
	}
	request := proofreadingWindowRequest{
		sourcePath:     path,
		sourceName:     name,
		sourcePage:     view.page,
		sourceSnapshot: reader,
	}
	store := a.proofreadingStore()
	if store == nil {
		reader.Close()
		hashReader.Close()
		return fmt.Errorf("校对记录数据库不可用")
	}
	entry.busy = true
	results := entry.results
	window, shutdown := a.window, a.shutdown
	if !a.launchManagedBackground(func() {
		result := inspectProofreadingPDFEntry(store, request, hashReader)
		select {
		case results <- result:
			if window != nil && !isAppShuttingDown(shutdown) {
				window.Invalidate()
			}
		case <-shutdown:
			if request.sourceSnapshot != nil {
				_ = request.sourceSnapshot.Close()
			}
		}
	}) {
		entry.busy = false
		reader.Close()
		hashReader.Close()
		return fmt.Errorf("无法启动校对记录检查")
	}
	return nil
}

func (a *App) startProofreadingSourceSnapshot(reader io.ReadCloser) {
	if a == nil || a.proofreading == nil || reader == nil {
		if reader != nil {
			_ = reader.Close()
		}
		return
	}
	state := a.proofreading
	state.sourceGen++
	generation := state.sourceGen
	sourcePath, sourceName := state.aDocument.Path, state.aDocument.Name
	window, shutdown, resultChannel := a.window, a.shutdown, state.sourceResults
	if !a.launchManagedBackground(func() {
		defer reader.Close()
		a.loadProofreadingSavedIdentity()
		sourceHasher := sha256.New()
		doc, err := pdfdoc.OpenSnapshotStream(func(w io.Writer) error {
			_, copyErr := io.Copy(io.MultiWriter(w, sourceHasher), reader)
			return copyErr
		})
		var object *proofreadingObject
		if err == nil {
			object = &proofreadingObject{Role: proofreadingRoleA, Path: sourcePath, Name: sourceName, Adapter: &proofreadingPDFAdapter{doc: doc}}
		}
		result := proofreadingSourceResult{generation: generation, object: object, sourceHash: hex.EncodeToString(sourceHasher.Sum(nil)), err: err}
		select {
		case resultChannel <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
			if object != nil {
				_ = object.Adapter.Close()
			}
		}
	}) {
		_ = reader.Close()
		state.sourceLoading = false
		state.sourceError = "无法启动 PDF 快照读取"
	}
}

func (a *App) startProofreadingPageFrameLoad(source proofreadingPageFrameProvider) {
	state := a.proofreading
	if state == nil || source == nil || state.frameLoading {
		return
	}
	state.frameLoading = true
	state.frameError = ""
	state.frameGen++
	generation := state.frameGen
	window, shutdown, resultChannel := a.window, a.shutdown, state.frameResults
	if !a.launchManagedBackground(func() {
		pageCount := source.PageCount()
		if pageCount < 0 {
			pageCount = 0
		}
		pages := make([]proofreadingPageFrame, pageCount)
		var firstError error
		if pageCount == 0 {
			firstError = fmt.Errorf("A 未包含可读取页面")
		}
		for pageIndex := 0; pageIndex < pageCount; pageIndex++ {
			page, err := source.PageFrame(pageIndex)
			if err != nil {
				if firstError == nil {
					firstError = err
				}
				pages[pageIndex] = proofreadingPageFrame{PageIndex: pageIndex, FrameUnavailable: true}
				continue
			}
			pages[pageIndex] = page
		}
		result := proofreadingPageFrameResult{generation: generation, pages: pages, err: firstError}
		select {
		case resultChannel <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	}) {
		state.frameLoading = false
		state.frameError = "无法启动页面尺寸读取"
	}
}

func (a *App) startProofreadingDraftLoad(path string) {
	state := a.proofreading
	if state == nil || state.sourceLoading || state.draftLoading {
		return
	}
	state.choosingDraft = false
	state.draftLoading = true
	state.draftError = ""
	state.draftGen++
	generation := state.draftGen
	window, shutdown, resultChannel := a.window, a.shutdown, state.draftResults
	if !a.launchManagedBackground(func() {
		object, err := loadProofreadingObject(path)
		result := proofreadingDraftResult{generation: generation, object: object, err: err}
		select {
		case resultChannel <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
			if object != nil {
				_ = object.Adapter.Close()
			}
		}
	}) {
		state.draftLoading = false
		state.draftError = "无法启动对象读取"
	}
}

func (a *App) processProofreadingResults() {
	state := a.proofreading
	if state == nil {
		return
	}
	select {
	case result := <-state.sourceResults:
		if result.generation != state.sourceGen {
			if result.object != nil && result.object.Adapter != nil {
				_ = result.object.Adapter.Close()
			}
			break
		}
		state.sourceLoading = false
		if result.err != nil {
			state.sourceError = result.err.Error()
			break
		}
		if result.object == nil || result.object.Adapter == nil {
			state.sourceError = "PDF 快照不可用"
			break
		}
		pdfAdapter, ok := result.object.Adapter.(*proofreadingPDFAdapter)
		if !ok || pdfAdapter.doc == nil {
			_ = result.object.Adapter.Close()
			state.sourceError = "PDF 快照不可用"
			break
		}
		a.clearProofreadingOCRLink(true)
		state.aDocument = result.object
		state.aSourceHash = result.sourceHash
		a.pdfDoc = pdfAdapter.doc
		if a.documentSessionState != nil {
			a.documentSessionState.pdfDoc = a.pdfDoc
		}
		view := a.ensurePDFView()
		view.visualOnly = true
		view.page = min(state.page, max(0, view.info.Pages-1))
		view.canvas.Position.First = view.page
		view.fit = "page"
		view.mode = "read"
		view.thumbnails = true
		view.thumbList.Axis = layout.Horizontal
		view.followPDFThumbnailPage(view.page)
		a.startProofreadingPageFrameLoad(pdfAdapter)
		if state.resume != nil {
			a.startProofreadingDraftLoad(state.resume.SourceB)
		}
	case result := <-state.frameResults:
		if result.generation != state.frameGen {
			break
		}
		state.frameLoading = false
		state.frameError = ""
		if result.err != nil {
			state.frameError = result.err.Error()
		}
		state.pageFrames = result.pages
		a.rebuildProofreadingXPages()
	case result := <-state.draftResults:
		if result.generation != state.draftGen {
			if result.object != nil && result.object.Adapter != nil {
				_ = result.object.Adapter.Close()
			}
			break
		}
		state.draftLoading = false
		state.choosingDraft = false
		if result.err != nil {
			if errors.Is(result.err, errProofreadingFileTooLarge) {
				state.draftError = a.textID("proofread.file.too_large")
			} else {
				state.draftError = result.err.Error()
			}
			if result.object != nil {
				_ = result.object.Adapter.Close()
			}
			break
		}
		if result.object == nil || result.object.Adapter == nil {
			state.draftError = "校对对象不可用"
			break
		}
		if state.ocrScan != nil {
			state.ocrScan.cancelTask()
			state.ocrScan.pages = nil
			state.ocrScan.err = ""
			state.ocrScan.notice = ""
			state.ocrScan.stage = proofreadingOCRIdle
		}
		if state.bDocument != nil && state.bDocument.Adapter != nil {
			_ = state.bDocument.Adapter.Close()
		}
		state.bDocument = result.object
		if state.modelLayoutCancel != nil {
			state.modelLayoutCancel()
		}
		state.controller = nil
		state.pendingXPageWheel = nil
		state.navigator = proofreadingNavigator{}
		state.restoredModel = nil
		state.modelInitialAssigned = false
		state.modelLayoutGeneration++
		state.modelLayoutPending = false
		state.xEditorTexts = nil
		state.xPageEditors = nil
		state.xPageScrollbars = nil
		state.xPageScrollOffsets = nil

		state.bDraftText, _ = proofreadingProjectionContent(state.bDocument.contentUnits())
		state.bDraftInitialized = true
		state.bDraftDirty = false
		state.pageOwnership = nil
		state.draftFilePath = ""
		state.draftSavePending = false
		state.draftPersistGen.Add(1)
		state.projectDocumentID, state.sourceBHash = proofreadingProjectDocumentID(state.aDocument.Path, state.aSourceHash, state.bDocument.Path, state.bDraftText)
		if state.resume != nil {
			state.projectDocumentID = state.resume.ID
		}
		if a.proofreadingTabHost != nil {
			host := a.proofreadingTabHost
			for _, tab := range host.proofreadingTabs {
				if tab.editor != a {
					continue
				}
				owner := host.registerProofreadingTab(tab, state.projectDocumentID)
				if owner.host != host || owner.id != tab.id {
					state.draftRestoreFailed = true
					tab.duplicate = true
					state.draftError = a.textID("proofread.workbench.already_open")
					owner.host.requestProofreadingTabFocus(owner.id, nil)
				}
			}
			if state.draftRestoreFailed {
				break
			}
		}
		state.draftPersistError = ""
		state.draftRestoreFailed = false
		state.draftAutoSaved = false
		state.draftError = ""
		a.startProofreadingDraftRestore()
	case result := <-state.draftRestoreResults:
		if result.generation != state.draftRestoreGen {
			break
		}
		state.draftRestoring = false
		if result.err != nil {
			state.draftPersistError = result.err.Error()
			state.draftRestoreFailed = true
			state.draftError = result.err.Error()
		}
		if result.found && result.draft != nil {
			state.bDraftText = result.draft.Text
			state.restoredModel = result.draft.Model
			state.pageOwnership = append([]proofreadingPageOwnership(nil), result.draft.Pages...)
			state.xTextSizeStep = max(-3, min(proofreadingXTextSizeMaxStep, result.draft.TextSizeStep))
			state.bDraftDirty = true
			state.draftAutoSaved = result.err == nil
			if result.err == nil {
				state.draftPersistError = ""
			}
		}
		a.rebuildProofreadingXPages()
		if result.err == nil && !result.found {
			a.queueProofreadingDraftPersistence()
		}
	case result := <-state.draftSaveResults:
		if result.generation != state.draftPersistGen.Load() {
			break
		}
		state.draftSavePending = false
		if result.err != nil {
			state.draftPersistError = result.err.Error()
			state.draftAutoSaved = false
		} else {
			state.draftPersistError = ""
			state.draftAutoSaved = true
		}
		if result.ocrJobID != "" && state.ocrScan != nil && state.ocrScan.awaitingPersistJob == result.ocrJobID {
			state.ocrScan.awaitingPersistJob = ""
			if result.err != nil {
				state.ocrScan.notice = "proofread.ai.status.resume_unavailable"
			}
			sendProofreadingOCRPersistAck(state.ocrScan, proofreadingOCRJobPersistAck{jobID: result.ocrJobID, err: result.err})
		}
	default:
	}
}

// rebuildProofreadingXPages maps editable B-flow pages onto A's fixed page frames.
// It deliberately does not read A's text or infer A's text positions.
func (a *App) rebuildProofreadingXPages() {
	a.refreshProofreadingModelView()
}

func (a *App) reflowProofreadingXPageBreaksForTextSize() {
	state := a.proofreading
	if state == nil || len(state.pageFrames) == 0 || state.bDocument == nil {
		return
	}
	a.startProofreadingModelLayout(true)
}

func (state *proofreadingWorkspace) pageSize(pageIndex int) (float64, float64, bool) {
	if state == nil || pageIndex < 0 || pageIndex >= len(state.pageFrames) {
		return 0, 0, false
	}
	page := state.pageFrames[pageIndex]
	if page.PageIndex != pageIndex || page.Width <= 0 || page.Height <= 0 {
		return 0, 0, false
	}
	return page.Width, page.Height, true
}

func (a *App) closeProofreadingDraft() {
	if a == nil || a.proofreading == nil {
		return
	}
	state := a.proofreading
	if state.ocrScan != nil {
		state.ocrScan.close()
	}
	if state.ocrStep != nil {
		state.ocrStep.close()
	}
	if state.bDocument != nil && state.bDocument.Adapter != nil {
		_ = state.bDocument.Adapter.Close()
		state.bDocument.Adapter = nil
	}
	if state.aDocument != nil && state.aDocument.Adapter != nil {
		if adapter, ok := state.aDocument.Adapter.(*proofreadingPDFAdapter); ok && adapter.doc == a.pdfDoc {
			a.pdfDoc = nil
		}
		_ = state.aDocument.Adapter.Close()
		state.aDocument.Adapter = nil
	}
	for {
		select {
		case result := <-state.sourceResults:
			if result.object != nil && result.object.Adapter != nil {
				_ = result.object.Adapter.Close()
			}
		default:
			goto drainDraftResults
		}
	}

drainDraftResults:
	for {
		select {
		case result := <-state.draftResults:
			if result.object != nil && result.object.Adapter != nil {
				_ = result.object.Adapter.Close()
			}
		default:
			return
		}
	}
}

func loadProofreadingObject(path string) (*proofreadingObject, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > 50<<20 {
		return nil, errProofreadingFileTooLarge
	}
	adapter := proofreadingFormatAdapters[strings.ToLower(filepath.Ext(path))]
	if adapter == nil {
		return nil, fmt.Errorf("此文件暂不能作为校对对象读取")
	}
	documentAdapter, err := adapter.Open(path)
	if err != nil {
		return nil, err
	}
	return &proofreadingObject{Role: proofreadingRoleB, Path: path, Name: filepath.Base(path), Adapter: documentAdapter}, nil
}

// Register additional extension adapters here without changing A/B roles or
// the geometry projection contract.
var proofreadingFormatAdapters = map[string]proofreadingFormatAdapter{
	".md":       proofreadingMarkdownAdapter{},
	".markdown": proofreadingMarkdownAdapter{},
	".txt":      proofreadingPlainTextAdapter{},
	".docx":     proofreadingWordAdapter{},
	".doc":      proofreadingWordAdapter{},
}

type proofreadingPlainTextAdapter struct{}

func (proofreadingPlainTextAdapter) Open(path string) (proofreadingAdapterDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := strings.TrimPrefix(strings.ToValidUTF8(string(data), "�"), "\uFEFF")
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	units := make([]proofreadingContentUnit, 0, len(lines))
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		units = append(units, proofreadingContentUnit{
			ID:      fmt.Sprintf("paragraph-%d", index),
			Kind:    proofreadingUnitParagraph,
			Text:    trimmed,
			Ordinal: len(units),
		})
	}
	return &proofreadingPlainTextDocument{units: units}, nil
}

type proofreadingPlainTextDocument struct {
	units []proofreadingContentUnit
}

func (*proofreadingPlainTextDocument) FormatID() string { return "txt" }
func (d *proofreadingPlainTextDocument) ContentUnits() []proofreadingContentUnit {
	if d == nil {
		return nil
	}
	return append([]proofreadingContentUnit(nil), d.units...)
}
func (*proofreadingPlainTextDocument) Close() error { return nil }

type proofreadingMarkdownAdapter struct{}

func (proofreadingMarkdownAdapter) Open(path string) (proofreadingAdapterDocument, error) {
	loaded, err := loadDocument(path)
	if err != nil {
		return nil, err
	}
	if loaded.markdownDoc == nil {
		closeLoadedDocument(loaded)
		return nil, fmt.Errorf("所选文件无法读取为可预览的文档")
	}
	parsed, err := markdowndoc.Parse(loaded.markdownDoc.Text())
	if err != nil {
		closeLoadedDocument(loaded)
		return nil, err
	}
	units := make([]proofreadingContentUnit, 0, len(parsed.Blocks))
	for index, block := range parsed.Blocks {
		if block.Kind == "table" {
			for rowIndex, row := range block.Rows {
				units = append(units, proofreadingContentUnit{ID: fmt.Sprintf("block-%d-row-%d", index, rowIndex), Kind: proofreadingUnitTableRow, Text: strings.Join(row, "\t"), Ordinal: len(units)})
			}
			continue
		}
		text := block.Text
		if block.Prefix != "" {
			text = block.Prefix + text
		}
		if block.Kind == "rule" {
			text = "────────"
		}
		kind := proofreadingUnitBlock
		switch block.Kind {
		case "heading":
			kind = proofreadingUnitHeading
		case "paragraph":
			kind = proofreadingUnitParagraph
		case "list":
			kind = proofreadingUnitList
		case "quote":
			kind = proofreadingUnitQuote
		case "code":
			kind = proofreadingUnitCode
		case "math":
			kind = proofreadingUnitMath
		case "rule":
			kind = proofreadingUnitRule
		}
		units = append(units, proofreadingContentUnit{ID: fmt.Sprintf("block-%d", index), Kind: kind, Text: text, Ordinal: len(units), Level: block.Level, Start: block.SourceStart, End: block.SourceEnd})
	}
	return &proofreadingLoadedAdapter{format: "markdown", loaded: loaded, units: units}, nil
}

type proofreadingWordAdapter struct{}

func (proofreadingWordAdapter) Open(path string) (proofreadingAdapterDocument, error) {
	loaded, err := loadDocument(path)
	if err != nil {
		return nil, err
	}
	if loaded.word == nil {
		closeLoadedDocument(loaded)
		return nil, fmt.Errorf("所选文件无法读取为可预览的文档")
	}
	format := "docx"
	if loaded.sourceFormat == application.FormatDOC {
		format = "doc"
	}
	return &proofreadingLoadedAdapter{format: format, loaded: loaded, units: proofreadingWordUnits(loaded.wordBlocks)}, nil
}

type proofreadingLoadedAdapter struct {
	format string
	loaded loadedDocument
	units  []proofreadingContentUnit
}

func (a *proofreadingLoadedAdapter) FormatID() string { return a.format }
func (a *proofreadingLoadedAdapter) ContentUnits() []proofreadingContentUnit {
	return append([]proofreadingContentUnit(nil), a.units...)
}
func (a *proofreadingLoadedAdapter) Close() error {
	closeLoadedDocument(a.loaded)
	a.loaded = loadedDocument{}
	return nil
}

type proofreadingPDFAdapter struct{ doc *pdfdoc.Document }

func (*proofreadingPDFAdapter) FormatID() string                        { return "pdf" }
func (*proofreadingPDFAdapter) ContentUnits() []proofreadingContentUnit { return nil }
func (a *proofreadingPDFAdapter) PageCount() int {
	if a == nil || a.doc == nil {
		return 0
	}
	return a.doc.Info().Pages
}
func (a *proofreadingPDFAdapter) PageFrame(index int) (proofreadingPageFrame, error) {
	if a == nil || a.doc == nil {
		return proofreadingPageFrame{}, fmt.Errorf("PDF 页面不可用")
	}
	width, height, err := a.doc.PageDimensions(index)
	if err != nil {
		return proofreadingPageFrame{}, err
	}
	return proofreadingPageFrame{PageIndex: index, Width: width, Height: height}, nil
}
func (a *proofreadingPDFAdapter) Close() error {
	if a == nil || a.doc == nil {
		return nil
	}
	return a.doc.Close()
}

func proofreadingWordUnits(blocks []document.WordBlock) []proofreadingContentUnit {
	units := make([]proofreadingContentUnit, 0, len(blocks))
	appendParagraph := func(paragraph document.WordParagraph) string {
		var out strings.Builder
		for _, inline := range paragraph.Inlines {
			switch {
			case inline.Text != nil:
				out.WriteString(inline.Text.Text)
			case inline.Tab != nil:
				out.WriteByte('\t')
			case inline.PageBreak != nil:
				out.WriteByte('\n')
			case inline.FieldCode != nil:
				out.WriteString(inline.FieldCode.Value)
			case inline.Image != nil:
				out.WriteString("[图片]")
			case inline.Unsupported != nil:
				out.WriteString("[暂未预览的内容]")
			}
		}
		return out.String()
	}
	for blockIndex, block := range blocks {
		switch {
		case block.Paragraph != nil:
			units = append(units, proofreadingContentUnit{ID: block.ID, Kind: proofreadingUnitParagraph, Text: appendParagraph(*block.Paragraph), Ordinal: len(units)})
		case block.Table != nil:
			for rowIndex, row := range block.Table.Rows {
				cells := make([]string, 0, len(row.Cells))
				for _, cell := range row.Cells {
					paragraphs := make([]string, 0, len(cell.Paragraphs))
					for _, paragraph := range cell.Paragraphs {
						paragraphs = append(paragraphs, appendParagraph(paragraph))
					}
					cells = append(cells, strings.Join(paragraphs, " "))
				}
				units = append(units, proofreadingContentUnit{ID: fmt.Sprintf("%s-row-%d", block.ID, rowIndex), Kind: proofreadingUnitTableRow, Text: strings.Join(cells, "\t"), Ordinal: len(units)})
			}
		case block.Unsupported != nil:
			units = append(units, proofreadingContentUnit{ID: block.Unsupported.ID, Kind: proofreadingUnitUnsupported, Text: "[暂未预览的内容]", Ordinal: len(units)})
		default:
			units = append(units, proofreadingContentUnit{ID: fmt.Sprintf("block-%d", blockIndex), Kind: proofreadingUnitUnsupported, Text: "[暂未预览的内容]", Ordinal: len(units)})
		}
	}
	return units
}
