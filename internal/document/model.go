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
	FontSize            float64
	HorizontalAlignment string
	VerticalAlignment   string
	WrapText            bool
	Borders             [4]CellBorder
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
