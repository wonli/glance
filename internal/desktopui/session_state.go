package desktopui

import (
	"sync/atomic"

	"glance/internal/document"
	"glance/internal/format/docx"
	"glance/internal/format/jsondoc"

	"gioui.org/layout"
	"gioui.org/widget"
)

// documentSessionState holds persistent Gio widgets and view state for one document tab.
type documentSessionState struct {
	closed                        atomic.Bool
	path                          string
	book                          document.Spreadsheet
	word                          *docx.Document
	jsonDoc                       *jsondoc.Document
	sheets                        []string
	sheet                         string
	grid                          [][]document.Cell
	gridList                      widget.List
	wordListState                 widget.List
	cellEditor                    widget.Editor
	formulaEditor                 widget.Editor
	nameBox                       widget.Editor
	sheetNameEditor               widget.Editor
	sheetCreateButton             widget.Clickable
	sheetRenameButton             widget.Clickable
	sheetDeleteButton             widget.Clickable
	wordEditor                    widget.Editor
	jsonEditor                    widget.Editor
	cellButtons                   map[string]*widget.Clickable
	sheetButtons                  map[string]*widget.Clickable
	blockButtons                  map[int]*widget.Clickable
	selectedCell                  string
	selection                     CellSelection
	selectionSet                  bool
	selectionDragging             bool
	toolbarTab                    spreadsheetToolbarTab
	toolbarTabs                   [2]widget.Clickable
	toolbarActionList             widget.List
	toolbarActionButtons          [25]widget.Clickable
	toolbarPickerButtons          [9]widget.Clickable
	toolbarPicker                 spreadsheetToolbarPicker
	toolbarPickerSlot             int
	formatRunning                 bool
	valueFilter                   *ValueFilter
	clipboardTarget               struct{}
	selectedBlock                 int
	selectedBlockEditable         bool
	cellEdit                      cellEditState
	lastGridWidth, lastGridHeight float64
	virtualGrid                   *gridViewport
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
	s := &documentSessionState{path: path, gridList: widget.List{List: layout.List{Axis: layout.Vertical}}, wordListState: widget.List{List: layout.List{Axis: layout.Vertical}}, toolbarTab: toolbarTabHome, toolbarActionList: widget.List{List: layout.List{Axis: layout.Horizontal}}, cellButtons: make(map[string]*widget.Clickable), sheetButtons: make(map[string]*widget.Clickable), blockButtons: make(map[int]*widget.Clickable), selectedBlock: -1}
	s.cellEditor.SingleLine = true
	s.cellEditor.Submit = true
	s.formulaEditor.SingleLine = true
	s.formulaEditor.Submit = true
	s.nameBox.SingleLine = true
	s.nameBox.Submit = true
	s.sheetNameEditor.SingleLine = true
	s.sheetNameEditor.Submit = true
	s.wordEditor.SingleLine = true
	s.jsonEditor.SingleLine = false
	return s
}
