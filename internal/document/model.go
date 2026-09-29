package document

import "errors"

var ErrReadOnly = errors.New("document is read-only")

type Cell struct {
	Value   string
	StyleID int
	Style   CellStyle
}

// CellStyle is a format-neutral subset that can be drawn by a lightweight
// spreadsheet UI. Colors use #RRGGBB; empty means the document/theme default.
type CellStyle struct {
	FillColor           string
	FontColor           string
	FontFamily          string
	Bold                bool
	Italic              bool
	Underline           bool
	FontSize            float64
	NumberFormat        string
	HorizontalAlignment string
	VerticalAlignment   string
	WrapText            bool
	Borders             [4]CellBorder
}

// CellFormatPatch represents changes to a subset of cell formatting. Nil
// fields leave the existing property untouched; ClearFormat resets all style.
type CellFormatPatch struct {
	Bold                *bool
	Italic              *bool
	Underline           *bool
	FontColor           *string
	FillColor           *string
	HorizontalAlignment *string
	NumberFormat        *string
	BorderPreset        *string
	ClearFormat         bool
}

// CellBorder describes one cell edge. Borders are ordered top, right,
// bottom, left in CellStyle.Borders.
type CellBorder struct {
	Color string
	Style int
}

// CellRange uses one-based inclusive worksheet coordinates.
type CellRange struct {
	StartRow    int
	StartColumn int
	EndRow      int
	EndColumn   int
}

// MergeRegion includes the anchor value/style even when the anchor is outside
// the requested visible window.
type MergeRegion struct {
	Range  CellRange
	Anchor Cell
}

// SheetWindow is a bounded rectangular worksheet snapshot. The cell matrix
// and dimension vectors are limited to the requested viewport region.
type SheetWindow struct {
	StartRow          int
	StartColumn       int
	Cells             [][]Cell
	Merges            []MergeRegion
	RowHeightsPoints  []float64
	ColumnWidthsChars []float64
}

type Spreadsheet interface {
	SheetNames() []string
	Range(sheet string, maxRows, maxColumns int) ([][]Cell, error)
	SetCellText(sheet, cell, value string) error
	Save() error
	Dirty() bool
	Editable() bool
	Close() error
}

// GridReader is an optional capability for native two-dimensional editable
// spreadsheet surfaces. Legacy formats may implement Spreadsheet only.
type GridReader interface {
	Window(sheet string, startRow, startColumn, rowCount, columnCount int) (SheetWindow, error)
	SetRowHeight(sheet string, row int, heightPoints float64) error
	SetColumnWidth(sheet string, column int, widthChars float64) error
}

// SpreadsheetFormatter is an optional capability for editing cell styles in
// a bounded range without exposing format-library types to the UI.
type SpreadsheetFormatter interface {
	SetCellFormats(sheet string, bounds CellRange, patch CellFormatPatch) error
}

type SpreadsheetHistory interface {
	Undo() (bool, error)
	Redo() (bool, error)
	RebaseHistory()
}

type CellEdit struct {
	Row, Column int
	Value       string
}

type SpreadsheetBatchEditor interface {
	SetCells(sheet string, edits []CellEdit) error
}
type SpreadsheetRangeReader interface {
	ReadRange(sheet string, bounds CellRange) ([][]Cell, error)
}

type StructureEditKind uint8

const (
	StructureInsertRows StructureEditKind = iota + 1
	StructureDeleteRows
	StructureInsertColumns
	StructureDeleteColumns
	StructureCreateSheet
	StructureRenameSheet
	StructureDeleteSheet
)

type StructureEdit struct {
	Kind           StructureEditKind
	Sheet, NewName string
	Index, Count   int
}
type SpreadsheetStructureEditor interface {
	EditStructure(edit StructureEdit) error
}

type SortRequest struct {
	Sheet      string
	Bounds     CellRange
	KeyColumn  int
	Descending bool
}
type SpreadsheetSorter interface {
	SortRange(request SortRequest) error
}
