package desktopui

import (
	"context"
	"sync"
	"sync/atomic"

	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/format/docx"
	"glance/internal/format/jsondoc"
	"glance/internal/format/markdowndoc"
	"glance/internal/format/pdfdoc"
	"glance/internal/format/pptx"
	"glance/internal/i18n"
	"glance/internal/wordlayout"

	"gioui.org/layout"
	"gioui.org/widget"
)

// liveDocumentState contains the canonical document identity, revision, and
// format-backed data shared by the application Runtime adapter and every
// desktop view. It deliberately contains no Gio widgets or window state.
type liveDocumentState struct {
	adapterMu                    sync.RWMutex
	spreadsheetZoomMu            sync.RWMutex
	spreadsheetZooms             map[string]int
	spreadsheetZoomRevision      atomic.Uint64
	spreadsheetZoomSavedRevision atomic.Uint64
	documentID                   string
	wordRevision                 uint64
	path                         string
	sourceFormat                 application.Format
	saveAsRequired               bool
	book                         document.Spreadsheet
	word                         *docx.Document
	wordBlocks                   []document.WordBlock
	jsonDoc                      *jsondoc.Document
	markdownDoc                  *markdowndoc.Document
	pdfDoc                       *pdfdoc.Document
	pptxDoc                      *pptx.Document
	sheets                       []string
	wordRevisionAtomic           atomic.Uint64
	publishedRevision            atomic.Uint64
}

// sharedDocumentSession owns lifecycle, draft synchronization, and work that
// must be consistent across every window displaying the same document.
// Canonical document state is grouped in liveDocumentState; view widgets stay
// in documentSessionState.
type sharedDocumentSession struct {
	liveDocumentState
	operationMu                  sync.Mutex
	wordLayoutCancelMu           sync.Mutex
	wordBlocksMu                 sync.RWMutex
	wordDraftMu                  sync.RWMutex
	presentationDraftMu          sync.RWMutex
	viewsMu                      sync.RWMutex
	operationGeneration          uint64
	activeOperation              sessionOperationToken
	operationActive              bool
	operationChanged             chan struct{}
	finalized                    atomic.Bool
	adaptersClosed               atomic.Bool
	wordLayoutReleased           atomic.Bool
	spreadsheetDraftViews        atomic.Int32
	markdownDraftViews           atomic.Int32
	markdownCompositionViews     atomic.Int32
	wordDraftEpoch               atomic.Uint64
	wordTextResetEpoch           atomic.Uint64
	wordRestoreNoticeMu          sync.Mutex
	wordRestoreError             string
	wordRestoreMessage           i18n.Message
	wordRestoreErrorEpoch        uint64
	wordRestoreLaunchMu          sync.Mutex
	wordRestoreLaunchClaimed     bool
	wordLayoutPageCount          atomic.Int64
	wordExportCommitMu           sync.Mutex
	spreadsheetExportCommitMu    sync.Mutex
	wordRunCommitted             map[string]string
	wordRunDrafts                map[string]string
	presentationDraftDirty       map[uint64]map[string]bool
	views                        map[uint64]*documentSessionState
	ownerRuntime                 *desktopRuntime
	wordLayoutPendingKey         *wordLayoutKey
	wordLayoutAcceptedKey        *wordLayoutKey
	wordLayoutErrorKey           *wordLayoutKey
	wordLayoutResult             *wordlayout.Result
	wordSupportNoticeShown       bool
	wordLayoutError              string
	wordLayoutCancel             context.CancelFunc
	wordLayoutPendingView        *documentSessionState
	wordLayoutWarningConfirmed   bool
	wordInsertionRunning         bool
	wordFlushRequested           atomic.Bool
	wordFlushAllDrafts           atomic.Bool
	wordForceDraftFlush          atomic.Bool
	wordRestoreAfterDraftsID     int64
	wordSaveAfterDrafts          bool
	wordSaveAfterDraftsReason    wordSaveReason
	wordPDFExportAfterDrafts     bool
	wordPDFExportAfterLayout     bool
	wordPDFExportRunning         bool
	wordPDFExportCancel          context.CancelFunc
	spreadsheetPDFExportRunning  bool
	spreadsheetPDFExportCancel   context.CancelFunc
	presentationPDFExportRunning bool
	spreadsheetPrintRunning      bool
	spreadsheetPrintCancel       context.CancelFunc
	wordPrintAfterDrafts         bool
	wordPrintAfterLayout         bool
	wordPrintPreparing           bool
	wordPrintRunning             bool
	wordPrintCancel              context.CancelFunc
	wordAutoSave                 bool
	formatRunning                atomic.Bool
	saveRunning                  atomic.Bool
}

func newSharedDocumentSession(path string) *sharedDocumentSession {
	idle := make(chan struct{})
	close(idle)
	session := &sharedDocumentSession{
		liveDocumentState: liveDocumentState{
			path:             path,
			wordRevision:     1,
			spreadsheetZooms: make(map[string]int),
		},
		operationChanged:       idle,
		wordRunCommitted:       make(map[string]string),
		wordRunDrafts:          make(map[string]string),
		presentationDraftDirty: make(map[uint64]map[string]bool),
		views:                  make(map[uint64]*documentSessionState),
	}
	session.wordRevisionAtomic.Store(session.wordRevision)
	session.publishedRevision.Store(session.wordRevision)
	return session
}

func newDocumentSessionView(shared *sharedDocumentSession, windowID uint64) *documentSessionState {
	if shared == nil {
		shared = newSharedDocumentSession("")
	}
	state := &documentSessionState{
		sharedDocumentSession:    shared,
		windowID:                 windowID,
		wordViewMode:             wordViewContinuous,
		wordLayoutZoom:           1,
		wordTargetRestorePage:    -1,
		spreadsheetZoom:          1,
		wordListState:            widget.List{List: layout.List{Axis: layout.Vertical}},
		wordPageListState:        widget.List{List: layout.List{Axis: layout.Vertical}},
		wordNavigationList:       widget.List{List: layout.List{Axis: layout.Vertical}},
		sheetTabList:             widget.List{List: layout.List{Axis: layout.Horizontal}},
		toolbarTab:               toolbarTabHome,
		wordToolbarTab:           wordTabHome,
		toolbarActionList:        widget.List{List: layout.List{Axis: layout.Horizontal}},
		toolbarGroupList:         widget.List{List: layout.List{Axis: layout.Horizontal}},
		wordToolbarTabList:       widget.List{List: layout.List{Axis: layout.Horizontal}},
		wordToolbarActionList:    widget.List{List: layout.List{Axis: layout.Horizontal}},
		wordToolbarGroupList:     widget.List{List: layout.List{Axis: layout.Horizontal}},
		wordToolbarActionButtons: make(map[string]*widget.Clickable),
		jsonToolbarActionList:    widget.List{List: layout.List{Axis: layout.Horizontal}},
		jsonToolbarActionButtons: make(map[string]*widget.Clickable),
		wordNavigationButtons:    make(map[string]*widget.Clickable),
		wordRunEditors:           make(map[string]*widget.Editor),
		wordObjectButtons:        make(map[string]*widget.Clickable),
		wordImageInteractions:    make(map[string]*wordImageInteraction),
		wordImageCache:           newWordImageCache(wordImageCacheLimit),
		wordImagePending:         make(map[string]uint64),
		wordImageVersions:        make(map[string]uint64),
		wordImageErrors:          make(map[string]string),
		gridPictureCache:         newWordImageCache(wordImageCacheLimit),
		gridPicturePending:       make(map[string]uint64),
		gridPictureVersions:      make(map[string]uint64),
		gridPictureErrors:        make(map[string]string),
		cellButtons:              make(map[string]*widget.Clickable),
		sheetButtons:             make(map[string]*widget.Clickable),
		blockButtons:             make(map[int]*widget.Clickable),
		selectedBlock:            -1,
	}
	shared.attachView(windowID, state)
	state.cellEdit.session = shared
	state.cellEditor.SingleLine = true
	state.cellEditor.Submit = true
	state.formulaEditor.SingleLine = true
	state.formulaEditor.Submit = true
	state.nameBox.SingleLine = true
	state.nameBox.Submit = true
	state.sheetNameEditor.SingleLine = true
	state.sheetNameEditor.Submit = true
	state.wordNavigationSearchEditor.SingleLine = true
	state.wordEditor.SingleLine = false
	state.jsonEditor.SingleLine = false
	state.observedSessionRevision.Store(shared.wordRevisionAtomic.Load())
	state.appliedSessionRevision.Store(shared.wordRevisionAtomic.Load())
	state.appliedWordDraftEpoch.Store(shared.wordDraftEpoch.Load())
	state.appliedWordTextResetEpoch.Store(shared.wordTextResetEpoch.Load())
	state.appliedWordRestoreErrorEpoch.Store(shared.wordRestoreNoticeEpoch())
	return state
}

func newDocumentSessionState(path string) *documentSessionState {
	return newDocumentSessionView(newSharedDocumentSession(path), 0)
}

// releaseWordViewResources drops view-local widgets and clears bounded image
// caches when a window no longer owns this view.
func (state *documentSessionState) releaseWordViewResources() {
	if state == nil || !state.wordViewResourcesReleased.CompareAndSwap(false, true) {
		return
	}
	state.cancelJSONWorkers()
	state.cancelSpreadsheetSelectionStats()
	if state.sharedDocumentSession != nil && state.sharedDocumentSession.cancelWordLayoutForView(state) && state.ownerRuntime != nil {
		state.ownerRuntime.invalidateSessionViews(state.documentID)
	}
	state.wordImageDecodeActive.Store(0)
	state.wordRunEditors = nil
	state.wordNavLayouts = nil
	state.wordNavLayoutsNext = nil
	state.wordPageVisibleInteractions = nil
	state.wordPageVisibleInteractionsNext = nil
	state.wordPageInteractionTags = nil
	state.wordPageInteractionTagsNext = nil
	state.wordPageSelectionDrag = nil
	state.wordPageTextSelection = nil
	state.wordImageInteractions = nil
	state.wordObjectButtons = nil
	state.wordNavigationEntryCache = wordNavigationEntryCache{}
	if state.wordImageCache != nil {
		state.wordImageCache.Clear()
		state.wordImageCache = nil
	}
	if state.gridPictureCache != nil {
		state.gridPictureCache.Clear()
		state.gridPictureCache = nil
	}
	state.wordImagePending = nil
	state.wordImageVersions = nil
	state.wordImageErrors = nil
	state.gridPicturePending = nil
	state.gridPictureVersions = nil
	state.gridPictureErrors = nil
}

// releaseSpreadsheetViewResources drops spreadsheet-specific view models, grid caches,
// and pending selection computations when a window no longer displays this spreadsheet.
func (state *documentSessionState) releaseSpreadsheetViewResources() {
	if state == nil {
		return
	}
	state.cancelSpreadsheetSelectionStats()
	if state.virtualGrid != nil {
		state.virtualGrid.CellPaintCache = nil
		state.virtualGrid.CellPaintFIFO = nil
		state.virtualGrid.Window = document.SheetWindow{}
		state.virtualGrid.TopWindow = document.SheetWindow{}
		state.virtualGrid.LeftWindow = document.SheetWindow{}
		state.virtualGrid.CornerWindow = document.SheetWindow{}
		state.virtualGrid.Rows = nil
		state.virtualGrid.Columns = nil
		state.virtualGrid = nil
	}
	state.cellEditor = widget.Editor{}
	state.formulaEditor = widget.Editor{}
	state.nameBox = widget.Editor{}
	state.sheetNameEditor = widget.Editor{}
	state.cellButtons = nil
	state.sheetButtons = nil
	state.blockButtons = nil
	state.sheetActiveCells = nil
	state.spreadsheetAutoFitRowBasePoints = nil
	state.valueFilter = nil
	state.clipboardPasteTarget = nil
	state.gridPictures = nil
	if state.gridPictureCache != nil {
		state.gridPictureCache.Clear()
		state.gridPictureCache = nil
	}
	state.gridPicturePending = nil
	state.gridPictureVersions = nil
	state.gridPictureErrors = nil
	state.selection = CellSelection{}
	state.selectionSet = false
	state.selectedCell = ""
	requestMemoryRelease()
}

// releaseViewResources drops view-local resources across all document types.
func (state *documentSessionState) releaseViewResources() {
	if state == nil {
		return
	}
	state.releaseJSONViewResources()
	state.releaseSpreadsheetViewResources()
	state.releaseWordViewResources()
	state.releasePDFViewResources()
	state.pptxView = nil
	state.markdownView = nil
}

func (state *sharedDocumentSession) cancelWordLayout() {
	if state == nil {
		return
	}
	state.wordLayoutCancelMu.Lock()
	if state.wordLayoutCancel != nil {
		state.wordLayoutCancel()
		state.wordLayoutCancel = nil
	}
	state.wordLayoutPendingKey = nil
	state.wordLayoutPendingView = nil
	state.wordLayoutCancelMu.Unlock()
}

func (state *sharedDocumentSession) cancelWordLayoutForView(view *documentSessionState) bool {
	if state == nil || view == nil {
		return false
	}
	state.wordLayoutCancelMu.Lock()
	defer state.wordLayoutCancelMu.Unlock()
	if state.wordLayoutPendingView != view {
		return false
	}
	if state.wordLayoutCancel != nil {
		state.wordLayoutCancel()
		state.wordLayoutCancel = nil
	}
	state.wordLayoutPendingKey = nil
	state.wordLayoutPendingView = nil
	return true
}

func (state *sharedDocumentSession) releaseWordLayout() {
	if state == nil || !state.wordLayoutReleased.CompareAndSwap(false, true) {
		return
	}
	state.cancelWordLayout()
	state.wordLayoutCancelMu.Lock()
	state.wordLayoutResult = nil
	state.wordLayoutAcceptedKey = nil
	state.wordLayoutErrorKey = nil
	state.wordLayoutError = ""
	state.wordLayoutWarningConfirmed = false
	state.wordLayoutPageCount.Store(0)
	state.wordLayoutCancelMu.Unlock()
}

func (s *sharedDocumentSession) attachView(windowID uint64, state *documentSessionState) {
	if s == nil || windowID == 0 || state == nil {
		return
	}
	s.viewsMu.Lock()
	if s.views == nil {
		s.views = make(map[uint64]*documentSessionState)
	}
	s.views[windowID] = state
	s.viewsMu.Unlock()
}

func (s *sharedDocumentSession) bumpWordDraftEpoch() {
	if s == nil {
		return
	}
	s.wordDraftEpoch.Add(1)
	if s.ownerRuntime != nil {
		s.ownerRuntime.invalidateSessionViews(s.documentID)
	}
}
