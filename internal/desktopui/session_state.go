package desktopui

import (
	"image/color"
	"sync"
	"sync/atomic"

	"glance/internal/document"

	"gioui.org/gesture"
	"gioui.org/widget"
)

// documentSessionState holds persistent Gio widgets and view state for one window's document view.
type documentSessionState struct {
	pdfView  *pdfViewState
	pptxView *presentationViewState
	*sharedDocumentSession
	windowID                            uint64
	closed                              atomic.Bool
	wordViewResourcesReleased           atomic.Bool
	jsonWorkerMu                        sync.Mutex
	jsonWorkerDone                      chan struct{}
	jsonWorkersClosed                   bool
	jsonLarge                           atomic.Bool
	observedSessionRevision             atomic.Uint64
	appliedSessionRevision              atomic.Uint64
	appliedWordDraftEpoch               atomic.Uint64
	appliedWordTextResetEpoch           atomic.Uint64
	appliedWordRestoreErrorEpoch        atomic.Uint64
	wordViewMode                        wordViewMode
	wordPageFitMode                     bool
	wordMultiplePagesMode               bool
	wordReadingMode                     bool
	wordReadingExitButton               widget.Clickable
	wordNavigationVisible               bool
	wordNavigationSearchEditor          widget.Editor
	wordNavigationSearchQuery           string
	wordNavigationList                  widget.List
	wordNavigationCloseButton           widget.Clickable
	wordNavigationButtons               map[string]*widget.Clickable
	wordNavigationEntryCache            wordNavigationEntryCache
	wordRulerHidden                     bool
	wordGridlinesVisible                bool
	wordProtected                       bool
	wordTrackRevisions                  bool
	wordLayoutActivePage                int
	wordLayoutZoom                      float64
	wordPageListState                   widget.List
	wordPageVisibleInteractions         []wordVisiblePageInteraction
	wordPageVisibleInteractionsNext     []wordVisiblePageInteraction
	wordPageInteractionTags             map[int]*wordPageInteractionTag
	wordPageInteractionTagsNext         map[int]*wordPageInteractionTag
	wordPageSelectionDrag               *wordPageSelectionDrag
	wordPageTextSelection               *wordPageTextSelection
	wordPagePendingScrollRunID          string
	wordPageFocusedMetaRunID            string
	wordPageFocusedMetaRevision         uint64
	wordPageFocusedParagraph            document.WordParagraph
	wordPagePreferredVerticalX          float64
	wordPagePreferredVerticalXSet       bool
	wordPageScrollX                     float64
	wordPageFitWidth                    int
	wordPageFitHeight                   int
	wordPageFitPageSetupKey             string
	wordPageFitColumns                  int
	wordStatusPrintLayoutButton         widget.Clickable
	wordStatusWebLayoutButton           widget.Clickable
	wordStatusZoomOutButton             widget.Clickable
	wordStatusZoomInButton              widget.Clickable
	wordStatusZoomDrag                  gesture.Drag
	spreadsheetZoom                     float64
	spreadsheetStatusZoomOutButton      widget.Clickable
	spreadsheetStatusZoomInButton       widget.Clickable
	spreadsheetStatusZoomResetBtn       widget.Clickable
	spreadsheetStatusZoomDrag           gesture.Drag
	sheet                               string
	wordListState                       widget.List
	wordBodyScroll                      gesture.Scroll
	wordCanvasBlankClick                gesture.Click
	wordCanvasContentClickHandled       bool
	wordCanvasResizeDrag                gesture.Drag
	wordRulerIndentDrag                 gesture.Drag
	wordCanvasWidthDP                   int
	wordCanvasResizeSide                int
	wordCanvasResizeStartX              float32
	wordCanvasResizeStartWidthDP        int
	wordRulerIndentDragSide             int
	wordRulerIndentDragStartX           float32
	wordRulerIndentDragStartEMU         int64
	wordRulerIndentDragPreviewEMU       int64
	wordRulerIndentDragParagraphID      string
	wordRulerIndentIntent               *wordParagraphIndentIntent
	cellEditor                          widget.Editor
	formulaEditor                       widget.Editor
	formulaReference                    formulaReferenceState
	nameBox                             widget.Editor
	sheetNameEditor                     widget.Editor
	sheetCreateButton                   widget.Clickable
	sheetRenameButton                   widget.Clickable
	sheetDeleteButton                   widget.Clickable
	wordEditor                          widget.Editor
	wordRunEditors                      map[string]*widget.Editor
	wordFocusedRunID                    string
	wordLayoutFocusedRunID              string
	wordPendingFocusRunID               string
	wordPendingCaretRunID               string
	wordPendingCaretOffset              int
	wordPendingCaretByX                 bool
	wordPendingCaretX                   int
	wordNavLayouts                      map[string]wordNavigationGeometry
	wordNavLayoutsNext                  map[string]wordNavigationGeometry
	wordPreferredVerticalX              int
	wordPreferredVerticalXSet           bool
	wordNavigationVerticalHandledRunID  string
	wordParagraphSplitIntent            *wordParagraphSplitIntent
	wordParagraphJoinIntent             *wordParagraphJoinIntent
	wordPageBreakIntent                 *wordPageBreakIntent
	wordAppendTarget                    widget.Clickable
	wordDeferredToolbarAction           string
	wordFindQuery                       string
	wordFormatPainterActive             bool
	wordFormatPainterSourceRunID        string
	wordFormatPainterPendingRunID       string
	wordFormatPainterPendingParagraphID string
	wordFormatPainterTargetRunID        string
	wordFormatPainterTargetParagraphID  string
	wordInsertAfterDraftFlush           bool
	wordInsertAfterParagraphID          string
	wordPendingPasteRunID               string
	wordPendingPasteText                string
	markdownView                        *markdownViewState
	jsonEditor                          widget.Editor
	jsonView                            *jsonWorkspaceView
	jsonToolbarActionList               widget.List
	jsonToolbarActionButtons            map[string]*widget.Clickable
	cellButtons                         map[string]*widget.Clickable
	sheetButtons                        map[string]*widget.Clickable
	blockButtons                        map[int]*widget.Clickable
	selectedCell                        string
	excelFindQuery                      string
	selection                           CellSelection
	selectionSet                        bool
	selectionDragging                   bool
	excelFormatPainterActive            bool
	excelFormatPainterPatch             *document.CellFormatPatch
	excelFormatPainterSourceCell        string
	spreadsheetGridlinesHidden          bool
	spreadsheetViewMode                 string
	spreadsheetRulerVisible             bool
	spreadsheetSheetProtected           bool
	spreadsheetWorkbookProtected        bool
	toolbarTab                          spreadsheetToolbarTab
	toolbarTabs                         [9]widget.Clickable
	toolbarActionList                   widget.List
	toolbarGroupList                    widget.List
	toolbarActionButtons                [toolbarActionLast + 1]widget.Clickable
	toolbarPickerButtons                [9]widget.Clickable
	toolbarPicker                       spreadsheetToolbarPicker
	toolbarPickerSlot                   int
	officeRibbonMenu                    officeRibbonDropdownState
	wordToolbarTab                      wordToolbarTab
	wordToolbarTabs                     [9]widget.Clickable
	wordToolbarTabList                  widget.List
	wordToolbarActionList               widget.List
	wordToolbarGroupList                widget.List
	wordToolbarActionButtons            map[string]*widget.Clickable
	wordToolbarColorButtons             [8]widget.Clickable
	wordToolbarColorPicker              bool
	wordToolbarHighlightPicker          bool
	wordBackgroundColor                 *color.NRGBA
	wordObjectButtons                   map[string]*widget.Clickable
	wordImageInteractions               map[string]*wordImageInteraction
	wordImageFocusRequestID             string
	wordImageCache                      *wordImageCache
	wordImageDecodeActive               atomic.Int32
	wordImagePending                    map[string]uint64
	wordImageVersions                   map[string]uint64
	wordImageErrors                     map[string]string
	wordImageDropTargetID               string
	wordImageDropParagraphID            string
	wordClipboardTarget                 struct{}
	wordClipboardRunID                  string
	valueFilter                         *ValueFilter
	clipboardTarget                     struct{}
	selectedBlock                       int
	selectedBlockEditable               bool
	selectedWordRunID                   string
	selectedWordParagraphID             string
	selectedWordImageID                 string
	gridPictures                        []document.SpreadsheetImage
	gridPicturesSheet                   string
	gridPicturesRequestedSheet          string
	gridPicturesLoading                 bool
	gridPicturesError                   string
	selectedGridPictureID               string
	gridPictureGesture                  gridPictureGesture
	gridPictureCommitID                 string
	gridPictureCommitPreview            gridPictureGeometry
	gridPictureCommitActive             bool
	gridPictureCache                    *wordImageCache
	gridPicturePending                  map[string]uint64
	gridPictureVersions                 map[string]uint64
	gridPictureErrors                   map[string]string
	cellEdit                            cellEditState
	lastGridWidth, lastGridHeight       float64
	virtualGrid                         *gridViewport
}

type ValueFilter struct {
	Sheet            string
	Bounds           document.CellRange
	Column           int
	Value            string
	VisibleRows      map[int]bool
	OriginalHeights  map[int]float64
	RowHeightsPoints []float64
}
