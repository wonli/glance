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
	finalized                  atomic.Bool
	wordRevisionAtomic         atomic.Uint64
	wordDraftEpoch             atomic.Uint64
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
	wordLayoutPendingKey       *wordLayoutKey
	wordLayoutAcceptedKey      *wordLayoutKey
	wordLayoutErrorKey         *wordLayoutKey
	wordLayoutResult           *wordlayout.Result
	wordLayoutError            string
	wordLayoutCancel           context.CancelFunc
	wordLayoutWarningConfirmed bool
	wordInsertionRunning       bool
	wordFlushRequested         bool
	wordFlushAllDrafts         bool
	wordForceDraftFlush        bool
	wordSaveAfterDrafts        bool
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
	formatRunning              bool
}

func newSharedDocumentSession(path string) *sharedDocumentSession {
	session := &sharedDocumentSession{
		path:             path,
		wordRevision:     1,
		wordRunCommitted: make(map[string]string),
		wordRunDrafts:    make(map[string]string),
	}
	session.wordRevisionAtomic.Store(session.wordRevision)
	return session
}

func newDocumentSessionView(shared *sharedDocumentSession, windowID uint64) *documentSessionState {
	if shared == nil {
		shared = newSharedDocumentSession("")
	}
	if shared.wordRunCommitted == nil {
		shared.wordRunCommitted = make(map[string]string)
	}
	if shared.wordRunDrafts == nil {
		shared.wordRunDrafts = make(map[string]string)
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
	return state
}

func newDocumentSessionState(path string) *documentSessionState {
	return newDocumentSessionView(newSharedDocumentSession(path), 0)
}
