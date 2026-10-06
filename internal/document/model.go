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
	FontSize            *float64
	FontFamily          *string
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

// CellMatch represents a cell matched by a search query. Coordinates are 1-based.
type CellMatch struct {
	Row    int
	Column int
	Value  string
}

// SpreadsheetSearcher is an optional capability for searching cell values in a worksheet.
type SpreadsheetSearcher interface {
	SearchSheet(sheet, query string) ([]CellMatch, error)
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

// SpreadsheetPageLayout describes the format-neutral print page setup for a worksheet.
type SpreadsheetPageLayout struct {
	Orientation  string  // "portrait" or "landscape"
	PaperSize    int     // 1=Letter, 9=A4, 8=A3, 5=Legal
	MarginTop    float64 // in inches (e.g. 0.75)
	MarginRight  float64 // in inches (e.g. 0.7)
	MarginBottom float64 // in inches (e.g. 0.75)
	MarginLeft   float64 // in inches (e.g. 0.7)
	PrintArea    string  // e.g. "$A$1:$D$10" or empty
	PrintTitles  string  // e.g. "$1:$1" or empty
}

// SpreadsheetPageLayoutEditor is an optional capability for editing worksheet page setup.
type SpreadsheetPageLayoutEditor interface {
	GetSheetPageLayout(sheet string) (SpreadsheetPageLayout, error)
	SetSheetPageOrientation(sheet string, orientation string) error
	SetSheetPageMargins(sheet string, top, right, bottom, left float64) error
	SetSheetPageSize(sheet string, paperSize int) error
	SetSheetPrintArea(sheet string, printArea string) error
	SetSheetPrintTitles(sheet string, printTitles string) error
}

// SpreadsheetGridlinesEditor is an optional capability for reading and toggling worksheet gridlines.
type SpreadsheetGridlinesEditor interface {
	GetSheetGridlines(sheet string) (bool, error)
	SetSheetGridlines(sheet string, show bool) error
}

// CellHyperlink describes a hyperlink attached to a worksheet cell.
type CellHyperlink struct {
	Target  string // e.g. "https://example.com" or "Sheet2!A1"
	Tooltip string // optional hover text
}

// SpreadsheetHyperlinkEditor is an optional capability for managing worksheet hyperlinks.
type SpreadsheetHyperlinkEditor interface {
	GetCellHyperlink(sheet, cell string) (CellHyperlink, bool, error)
	SetCellHyperlink(sheet, cell, target, tooltip string) error
	RemoveCellHyperlink(sheet, cell string) error
}

// SpreadsheetPaneFreeze describes frozen rows and columns in a worksheet.
type SpreadsheetPaneFreeze struct {
	Frozen bool
	XSplit int // number of frozen columns on the left (0 = none)
	YSplit int // number of frozen rows at the top (0 = none)
}

// SpreadsheetPanesEditor is an optional capability for configuring frozen panes.
type SpreadsheetPanesEditor interface {
	GetSheetPanes(sheet string) (SpreadsheetPaneFreeze, error)
	SetSheetPanes(sheet string, freeze SpreadsheetPaneFreeze) error
}

// SpreadsheetDuplicatesResult reports the outcome of a duplicate row removal operation.
type SpreadsheetDuplicatesResult struct {
	RemovedCount   int
	RemainingCount int
}

// SpreadsheetDuplicatesEditor is an optional capability for deduplicating rows in a range.
type SpreadsheetDuplicatesEditor interface {
	RemoveDuplicates(sheet string, bounds CellRange, keyColumn int) (SpreadsheetDuplicatesResult, error)
}

// SpreadsheetSheetView describes the view mode and element visibility for a worksheet.
type SpreadsheetSheetView struct {
	ViewMode  string // "normal", "pageBreakPreview", "pageLayout"
	ShowRuler bool
}

// SpreadsheetViewsEditor is an optional capability for configuring worksheet views and ruler.
type SpreadsheetViewsEditor interface {
	GetSheetView(sheet string) (SpreadsheetSheetView, error)
	SetSheetViewMode(sheet string, mode string) error
	SetSheetRuler(sheet string, show bool) error
}

// SpreadsheetProtectionEditor is an optional capability for protecting worksheets.
type SpreadsheetProtectionEditor interface {
	GetSheetProtection(sheet string) (bool, error)
	ProtectSheet(sheet string, password string) error
	UnprotectSheet(sheet string, password string) error
}

// SpreadsheetWorkbookProtectionEditor is an optional capability for protecting workbooks.
type SpreadsheetWorkbookProtectionEditor interface {
	GetWorkbookProtection() (bool, error)
	ProtectWorkbook(password string) error
	UnprotectWorkbook(password string) error
}

// SpreadsheetTextToColumnsResult reports the outcome of a text-to-columns split operation.
type SpreadsheetTextToColumnsResult struct {
	RowsSplit int
	MaxCols   int
}

// SpreadsheetTextToColumnsEditor is an optional capability for splitting text in a column into adjacent columns.
type SpreadsheetTextToColumnsEditor interface {
	SplitTextToColumns(sheet string, bounds CellRange, keyColumn int, delimiter string) (SpreadsheetTextToColumnsResult, error)
}

// SpreadsheetTableSpec specifies options for creating a spreadsheet table.
type SpreadsheetTableSpec struct {
	Sheet     string
	Range     CellRange
	Name      string
	StyleName string
}

// SpreadsheetTableEditor is an optional capability for formatting or inserting structured tables.
type SpreadsheetTableEditor interface {
	AddTable(spec SpreadsheetTableSpec) error
}

// SpreadsheetRecalculateEditor is an optional capability for recalculating formulas and updating linked values.
type SpreadsheetRecalculateEditor interface {
	RecalculateFormulas() error
}

// SpreadsheetFormulaEditor provides formula read and write capabilities.
type SpreadsheetFormulaEditor interface {
	SetCellFormula(sheet, cell, formula string) error
	GetCellFormula(sheet, cell string) (string, error)
}

// SpreadsheetDefinedName represents a named range or formula in a workbook.
type SpreadsheetDefinedName struct {
	Name    string
	Ref     string
	Scope   string
	Comment string
}

// SpreadsheetDefinedNameEditor provides reading, adding, and deleting defined names.
type SpreadsheetDefinedNameEditor interface {
	GetDefinedNames() ([]SpreadsheetDefinedName, error)
	SetDefinedName(name SpreadsheetDefinedName) error
	DeleteDefinedName(name string, scope string) error
}

// SpreadsheetFlashFillResult describes the result of a flash fill / series fill operation.
type SpreadsheetFlashFillResult struct {
	RowsFilled int
}

// SpreadsheetFlashFillEditor supports filling down series or repeating patterns into selection.
type SpreadsheetFlashFillEditor interface {
	FlashFill(sheet string, selection CellRange) (SpreadsheetFlashFillResult, error)
}


