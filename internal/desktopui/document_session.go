package desktopui

import (
	"context"
	"sync"
	"sync/atomic"

	"glance/internal/document"
	"glance/internal/format/docx"
	"glance/internal/format/jsondoc"
	"glance/internal/wordlayout"

	"gioui.org/layout"
	"gioui.org/widget"
)

// sharedDocumentSession owns document data and work that must be consistent
// across every window displaying the same logical document.
type sharedDocumentSession struct {
	adapterMu                  sync.RWMutex
	operationMu                sync.Mutex
	wordLayoutCancelMu         sync.Mutex
	wordBlocksMu               sync.RWMutex
	wordDraftMu                sync.RWMutex
	viewsMu                    sync.RWMutex
	operationGeneration        uint64
	activeOperation            sessionOperationToken
	operationActive            bool
	operationChanged           chan struct{}
	finalized                  atomic.Bool
	adaptersClosed             atomic.Bool
	wordLayoutReleased         atomic.Bool
	wordRevisionAtomic         atomic.Uint64
	publishedRevision          atomic.Uint64
	wordDraftEpoch             atomic.Uint64
	wordTextResetEpoch         atomic.Uint64
	wordRestoreNoticeMu        sync.Mutex
	wordRestoreError           string
	wordRestoreErrorEpoch      uint64
	wordRestoreLaunchMu        sync.Mutex
	wordRestoreLaunchClaimed   bool
	wordLayoutPageCount        atomic.Int64
	wordExportCommitMu         sync.Mutex
	documentID                 string
	wordRevision               uint64
	path                       string
	book                       document.Spreadsheet
	word                       *docx.Document
	wordBlocks                 []document.WordBlock
	jsonDoc                    *jsondoc.Document
	sheets                     []string
	wordRunCommitted           map[string]string
	wordRunDrafts              map[string]string
	views                      map[uint64]*documentSessionState
	ownerRuntime               *desktopRuntime
	wordLayoutPendingKey       *wordLayoutKey
	wordLayoutAcceptedKey      *wordLayoutKey
	wordLayoutErrorKey         *wordLayoutKey
	wordLayoutResult           *wordlayout.Result
	wordLayoutError            string
	wordLayoutCancel           context.CancelFunc
	wordLayoutPendingView      *documentSessionState
	wordLayoutWarningConfirmed bool
	wordInsertionRunning       bool
	wordFlushRequested         atomic.Bool
	wordFlushAllDrafts         atomic.Bool
	wordForceDraftFlush        atomic.Bool
	wordRestoreAfterDraftsID   int64
	wordSaveAfterDrafts        bool
	wordSaveAfterDraftsReason  wordSaveReason
	wordPDFExportAfterDrafts   bool
	wordPDFExportAfterLayout   bool
	wordPDFExportRunning       bool
	wordPDFExportCancel        context.CancelFunc
	wordPrintAfterDrafts       bool
	wordPrintAfterLayout       bool
	wordPrintPreparing         bool
	wordPrintRunning           bool
	wordPrintCancel            context.CancelFunc
	wordAutoSave               bool
	formatRunning              atomic.Bool
	saveRunning                atomic.Bool
}

func newSharedDocumentSession(path string) *sharedDocumentSession {
	idle := make(chan struct{})
	close(idle)
	session := &sharedDocumentSession{
		path:             path,
		wordRevision:     1,
		operationChanged: idle,
		wordRunCommitted: make(map[string]string),
		wordRunDrafts:    make(map[string]string),
		views:            make(map[uint64]*documentSessionState),
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
		gridList:                 widget.List{List: layout.List{Axis: layout.Vertical}},
		wordListState:            widget.List{List: layout.List{Axis: layout.Vertical}},
		wordPageListState:        widget.List{List: layout.List{Axis: layout.Vertical}},
		wordNavigationList:       widget.List{List: layout.List{Axis: layout.Vertical}},
		toolbarTab:               toolbarTabHome,
		wordToolbarTab:           wordTabHome,
		toolbarActionList:        widget.List{List: layout.List{Axis: layout.Horizontal}},
		wordToolbarTabList:       widget.List{List: layout.List{Axis: layout.Horizontal}},
		wordToolbarActionList:    widget.List{List: layout.List{Axis: layout.Horizontal}},
		wordToolbarActionButtons: make(map[string]*widget.Clickable),
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
