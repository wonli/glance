package desktopui

import (
	"sync/atomic"

	"glance/internal/document"
	"glance/internal/format/docx"
	"glance/internal/format/jsondoc"

	"gioui.org/gesture"
	"gioui.org/layout"
	"gioui.org/widget"
)

// documentSessionState holds persistent Gio widgets and view state for one document tab.
type documentSessionState struct {
	closed                              atomic.Bool
	path                                string
	book                                document.Spreadsheet
	word                                *docx.Document
	wordBlocks                          []document.WordBlock
	jsonDoc                             *jsondoc.Document
	sheets                              []string
	sheet                               string
	grid                                [][]document.Cell
	gridList                            widget.List
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
	nameBox                             widget.Editor
	sheetNameEditor                     widget.Editor
	sheetCreateButton                   widget.Clickable
	sheetRenameButton                   widget.Clickable
	sheetDeleteButton                   widget.Clickable
	wordEditor                          widget.Editor
	wordRunEditors                      map[string]*widget.Editor
	wordRunCommitted                    map[string]string
	wordRunDrafts                       map[string]string
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
	wordAppendTarget                    widget.Clickable
	wordInsertionRunning                bool
	wordFlushRequested                  bool
	wordFlushAllDrafts                  bool
	wordForceDraftFlush                 bool
	wordSaveAfterDrafts                 bool
	wordAutoSave                        bool
	wordDeferredToolbarAction           string
	wordFindQuery                       string
	wordFormatPainterActive             bool
	wordFormatPainterSourceRunID        string
	wordFormatPainterPendingRunID       string
	wordFormatPainterPendingParagraphID string
	wordFormatPainterTargetRunID        string
	wordFormatPainterTargetParagraphID  string
	wordInsertAfterDraftFlush           bool
	wordPendingPasteRunID               string
	wordPendingPasteText                string
	jsonEditor                          widget.Editor
	cellButtons                         map[string]*widget.Clickable
	sheetButtons                        map[string]*widget.Clickable
	blockButtons                        map[int]*widget.Clickable
	selectedCell                        string
	selection                           CellSelection
	selectionSet                        bool
	selectionDragging                   bool
	toolbarTab                          spreadsheetToolbarTab
	toolbarTabs                         [8]widget.Clickable
	toolbarActionList                   widget.List
	toolbarActionButtons                [29]widget.Clickable
	toolbarPickerButtons                [9]widget.Clickable
	toolbarPicker                       spreadsheetToolbarPicker
	toolbarPickerSlot                   int
	wordToolbarTab                      wordToolbarTab
	wordToolbarTabs                     [8]widget.Clickable
	wordToolbarTabList                  widget.List
	wordToolbarActionList               widget.List
	wordToolbarActionButtons            map[string]*widget.Clickable
	wordToolbarColorButtons             [8]widget.Clickable
	wordToolbarColorPicker              bool
	wordToolbarHighlightPicker          bool
	wordObjectButtons                   map[string]*widget.Clickable
	wordImageInteractions               map[string]*wordImageInteraction
	wordImageFocusRequestID             string
	wordImageCache                      *wordImageCache
	wordImagePending                    map[string]uint64
	wordImageVersions                   map[string]uint64
	wordImageErrors                     map[string]string
	wordImageDropTargetID               string
	wordImageDropParagraphID            string
	wordClipboardTarget                 struct{}
	wordClipboardRunID                  string
	formatRunning                       bool
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

func newDocumentSessionState(path string) *documentSessionState {
	s := &documentSessionState{path: path, gridList: widget.List{List: layout.List{Axis: layout.Vertical}}, wordListState: widget.List{List: layout.List{Axis: layout.Vertical}}, toolbarTab: toolbarTabHome, wordToolbarTab: wordTabHome, toolbarActionList: widget.List{List: layout.List{Axis: layout.Horizontal}}, wordToolbarTabList: widget.List{List: layout.List{Axis: layout.Horizontal}}, wordToolbarActionList: widget.List{List: layout.List{Axis: layout.Horizontal}}, wordToolbarActionButtons: make(map[string]*widget.Clickable), wordRunEditors: make(map[string]*widget.Editor), wordRunCommitted: make(map[string]string), wordRunDrafts: make(map[string]string), wordObjectButtons: make(map[string]*widget.Clickable), wordImageInteractions: make(map[string]*wordImageInteraction), wordImageCache: newWordImageCache(wordImageCacheLimit), wordImagePending: make(map[string]uint64), wordImageVersions: make(map[string]uint64), wordImageErrors: make(map[string]string), gridPictureCache: newWordImageCache(wordImageCacheLimit), gridPicturePending: make(map[string]uint64), gridPictureVersions: make(map[string]uint64), gridPictureErrors: make(map[string]string), cellButtons: make(map[string]*widget.Clickable), sheetButtons: make(map[string]*widget.Clickable), blockButtons: make(map[int]*widget.Clickable), selectedBlock: -1}
	s.cellEditor.SingleLine = true
	s.cellEditor.Submit = true
	s.formulaEditor.SingleLine = true
	s.formulaEditor.Submit = true
	s.nameBox.SingleLine = true
	s.nameBox.Submit = true
	s.sheetNameEditor.SingleLine = true
	s.sheetNameEditor.Submit = true
	s.wordEditor.SingleLine = false
	s.jsonEditor.SingleLine = false
	return s
}
