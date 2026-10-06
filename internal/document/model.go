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
	WrapText            *bool
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

// SpreadsheetImage describes an ordinary image object anchored above cells.
// Row/Column are one-based; offsets and rendered extents are pixels at 96 DPI.
type SpreadsheetImage struct {
	ID, Sheet, Cell   string
	Row, Column       int
	OffsetX, OffsetY  int
	WidthPx, HeightPx int
	MediaType         string
	Name, AltText     string
	MoveWithCells     bool
	SizeWithCells     bool
	LockAspectRatio   bool
	Editable          bool
	ReadOnlyReason    string
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

// SpreadsheetImageReader is an optional capability for over-cell picture
// objects. Image bytes are separate so a UI can load visible images
// asynchronously without placing format-library values in widget state.
type SpreadsheetImageReader interface {
	SpreadsheetImages(sheet string) ([]SpreadsheetImage, error)
	SpreadsheetImageBytes(imageID string) ([]byte, error)
}

// SpreadsheetImagePlacement describes a newly inserted over-cell picture.
// Coordinates and rendered dimensions use one-based cells and 96-DPI pixels.
type SpreadsheetImagePlacement struct {
	Sheet, MediaType, Name, AltText string
	Row, Column                     int
	OffsetX, OffsetY                int
	WidthPx, HeightPx               int
}

// SpreadsheetImageEditor is an optional capability for safe picture-object
// mutations. Resize takes the target width and preserves the current ratio.
type SpreadsheetImageEditor interface {
	InsertSpreadsheetImage(placement SpreadsheetImagePlacement, data []byte) (imageID string, err error)
	ReplaceSpreadsheetImage(imageID, mediaType string, data []byte) error
	MoveSpreadsheetImage(imageID string, row, column, offsetX, offsetY int) error
	ResizeSpreadsheetImage(imageID string, widthPx int) error
	DeleteSpreadsheetImage(imageID string) error
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
