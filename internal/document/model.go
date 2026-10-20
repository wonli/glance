package document

import (
	"context"
	"errors"
	"time"
)

var ErrReadOnly = errors.New("document is read-only")

type Cell struct {
	Value string
	// ValueKind is the source value type used for display behavior such as General alignment.
	ValueKind          CellValueKind `json:"value_kind,omitempty"`
	Formula            string
	FormulaType        string `json:"formula_type,omitempty"`
	StyleID            int
	Style              CellStyle
	TypedValue         *CellValue             `json:"typed_value,omitempty"`
	ConditionalDisplay CellConditionalDisplay `json:"-"`
	RichText           []CellRichTextRun      `json:"rich_text,omitempty"`
}

// CellRichTextRun exposes one formatted run of inline text within a spreadsheet cell.
type CellRichTextRun struct {
	Text       string  `json:"text"`
	Bold       bool    `json:"bold,omitempty"`
	Italic     bool    `json:"italic,omitempty"`
	Underline  bool    `json:"underline,omitempty"`
	FontFamily string  `json:"font_family,omitempty"`
	FontSize   float64 `json:"font_size,omitempty"`
	FontColor  string  `json:"font_color,omitempty"`
}

// CellConditionalDisplay carries format-neutral, computed worksheet effects.
// It is deliberately excluded from JSON payloads because it is presentation
// data derived from the workbook's conditional-format rules.
type CellConditionalDisplay struct {
	FillColor    string
	FontColor    string
	DataBarColor string
	DataBarStart float64
	DataBarEnd   float64
	HasDataBar   bool
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

type CellValueKind string

const (
	CellValueText    CellValueKind = "text"
	CellValueNumber  CellValueKind = "number"
	CellValueBoolean CellValueKind = "boolean"
)

// CellValue preserves the JSON value type supplied by an Agent. Desktop
// editing keeps its existing text inference path and does not set an explicit
// kind.
type CellValue struct {
	Kind    CellValueKind
	Text    string
	Number  float64
	Boolean bool
}

// CellFormatPatch represents changes to a subset of cell formatting. Nil
// fields leave the existing property untouched; ClearFormat resets all style.
// DecimalPlacesDelta is a standalone +/-1 precision command applied to each
// cell's existing supported numeric format. FontSizeStep moves each selected
// cell to the adjacent common spreadsheet font size.
type CellFormatPatch struct {
	Bold                *bool    `json:"bold,omitempty"`
	Italic              *bool    `json:"italic,omitempty"`
	Underline           *bool    `json:"underline,omitempty"`
	FontColor           *string  `json:"font_color,omitempty"`
	FillColor           *string  `json:"fill_color,omitempty"`
	HorizontalAlignment *string  `json:"horizontal_alignment,omitempty"`
	VerticalAlignment   *string  `json:"vertical_alignment,omitempty"`
	NumberFormat        *string  `json:"number_format,omitempty"`
	DecimalPlacesDelta  *int     `json:"decimal_places_delta,omitempty"`
	FontSizeStep        *int     `json:"font_size_step,omitempty"`
	BorderPreset        *string  `json:"border_preset,omitempty"`
	WrapText            *bool    `json:"wrap_text,omitempty"`
	FontSize            *float64 `json:"font_size,omitempty"`
	FontFamily          *string  `json:"font_family,omitempty"`
	ClearFormat         bool     `json:"clear_format,omitempty"`
}

// CellBorder describes one cell edge. Borders are ordered top, right,
// bottom, left in CellStyle.Borders.
type CellBorder struct {
	Color string
	Style int
}

// CellRange uses one-based inclusive worksheet coordinates.
type CellRange struct {
	StartRow    int `json:"start_row"`
	StartColumn int `json:"start_column"`
	EndRow      int `json:"end_row"`
	EndColumn   int `json:"end_column"`
}

// CellCoordinate is a one-based active-cell position.
type CellCoordinate struct {
	Row    int
	Column int
}

type SpreadsheetDirection uint8

const (
	SpreadsheetDirectionUp SpreadsheetDirection = iota + 1
	SpreadsheetDirectionDown
	SpreadsheetDirectionLeft
	SpreadsheetDirectionRight
)

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
	Panes             SpreadsheetPaneFreeze
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

type SpreadsheetDimensionAxis uint8

const (
	SpreadsheetDimensionRow SpreadsheetDimensionAxis = iota + 1
	SpreadsheetDimensionColumn
)

type SpreadsheetDimensionCell struct {
	// DimensionIndex is the one-based row or column being visited.
	DimensionIndex int
	// DimensionSize is the current row height in points or column width in
	// character units for DimensionIndex.
	DimensionSize float64
	// Index is the one-based row or column coordinate perpendicular to Axis.
	Index int
	Cell  Cell
	// ColumnWidthChars is populated for row visits so row AutoFit can measure
	// against the cell's actual worksheet column width, even off viewport.
	ColumnWidthChars float64
}

// SpreadsheetDimensionCellReader visits populated, unmerged cells in a row
// or column. Adapters may hold their document lock while calling visit, so the
// callback must not call back into the same adapter.
type SpreadsheetDimensionCellReader interface {
	VisitDimensionCells(sheet string, axis SpreadsheetDimensionAxis, index int, visit func(SpreadsheetDimensionCell) error) error
}

// SpreadsheetDimensionRangeCellReader visits populated, unmerged cells across
// an inclusive range of rows or columns without expanding the worksheet grid.
type SpreadsheetDimensionRangeCellReader interface {
	VisitDimensionRangeCells(sheet string, axis SpreadsheetDimensionAxis, start, end int, visit func(SpreadsheetDimensionCell) error) error
}

// SpreadsheetDimensionSize is a requested row height in points or column
// width in character units. Index is one-based.
type SpreadsheetDimensionSize struct {
	Index int
	Value float64
}

// SpreadsheetDimensionBatchEditor applies several row heights or column
// widths as one undoable edit.
type SpreadsheetDimensionBatchEditor interface {
	SetDimensionSizes(sheet string, axis SpreadsheetDimensionAxis, sizes []SpreadsheetDimensionSize) error
}

// SpreadsheetDimensionEditGrouper joins the automatic row-height adjustment
// caused by a cell edit to that edit's latest undo/redo step.
type SpreadsheetDimensionEditGrouper interface {
	SetRowHeightAsPartOfLatestEdit(sheet string, row int, heightPoints float64) error
}

// SpreadsheetDataRegionReader moves from a cell to the edge of the next
// contiguous non-empty run in one worksheet direction. If no later run exists,
// it returns the corresponding worksheet edge.
type SpreadsheetDataRegionReader interface {
	DataRegionEdge(sheet string, current CellCoordinate, direction SpreadsheetDirection) (CellCoordinate, error)
}

// SpreadsheetCurrentDataRegionReader reports the blank-row/blank-column
// bounded region around a cell. When the active cell is outside populated data,
// implementations may anchor the region at the nearest populated cell on its
// row or column. The bool is false when no populated region can be identified.
type SpreadsheetCurrentDataRegionReader interface {
	CurrentDataRegion(sheet string, current CellCoordinate) (CellRange, CellCoordinate, bool, error)
}

// SpreadsheetNextNonEmptyCellReader implements the one-shot End-key movement
// to the next occupied cell, or to the worksheet edge when none remains.
type SpreadsheetNextNonEmptyCellReader interface {
	NextNonEmptyCell(sheet string, current CellCoordinate, direction SpreadsheetDirection) (CellCoordinate, error)
}

// SpreadsheetDimensionReader reports the current in-memory used range, including
// cell formatting. Values are returned in the order (maxColumn, maxRow), both
// one-based. Implementations may scan a package, so callers should use a worker
// for large workbooks.
type SpreadsheetDimensionReader interface {
	Dimension(sheet string) (maxColumn, maxRow int, err error)
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

type SpreadsheetFormatChangeEditor interface {
	SetCellFormatsChanged(sheet string, bounds CellRange, patch CellFormatPatch) (bool, error)
}

// SpreadsheetCellFormatPainter copies a source cell's complete XLSX style to
// a target range without changing cell values.
type SpreadsheetCellFormatPainter interface {
	PaintCellFormat(sheet, sourceCell string, targetBounds CellRange) (bool, error)
}

type SpreadsheetHistory interface {
	Undo() (bool, error)
	Redo() (bool, error)
	RebaseHistory()
}

type CellEdit struct {
	Row        int        `json:"row"`
	Column     int        `json:"column"`
	Value      string     `json:"value"`
	TypedValue *CellValue `json:"-"`
}

type SpreadsheetCellValueEditor interface {
	SetCellValueChanged(sheet, cell string, value CellValue) (bool, error)
}

// SpreadsheetCellDateEditor inserts a static date value into one cell.
type SpreadsheetCellDateEditor interface {
	SetCellDateChanged(sheet, cell string, date time.Time) (bool, error)
}

// SpreadsheetCellTimeEditor inserts a static, minute-precision time value into one cell.
type SpreadsheetCellTimeEditor interface {
	SetCellTimeChanged(sheet, cell string, value time.Time) (bool, error)
}

type SpreadsheetBatchEditor interface {
	SetCells(sheet string, edits []CellEdit) error
}

// SpreadsheetBatchChangeEditor reports whether a bounded batch changed the
// workbook. Adapters use it to advance the shared document revision only when
// content actually changed.
type SpreadsheetBatchChangeEditor interface {
	SetCellsChanged(sheet string, edits []CellEdit) (bool, error)
}

// SpreadsheetCellTransferEditor applies a captured rectangular cell payload to
// a target position as one undoable edit. Source coordinates are retained so
// relative formula references can be adjusted like a native spreadsheet paste.
type SpreadsheetCellTransferEditor interface {
	TransferCellRangeChanged(sheet string, source CellRange, targetRow, targetColumn int, cells [][]Cell) (bool, error)
}

// SpreadsheetFillDownEditor fills a bounded selection from its top row as one
// undoable edit, translating relative formula references for each row.
type SpreadsheetFillDownEditor interface {
	FillDownChanged(sheet string, bounds CellRange) (bool, error)
}

// SpreadsheetFillRightEditor fills a bounded selection from its leftmost
// column as one undoable edit, translating relative formula references by
// column.
type SpreadsheetFillRightEditor interface {
	FillRightChanged(sheet string, bounds CellRange) (bool, error)
}

// SpreadsheetMergeResult reports a merge preview or the completed merge action.
// A non-empty confirmation token is a content-bound approval for discarding
// non-anchor values; it must be checked again atomically with the mutation.
type SpreadsheetMergeResult struct {
	Changed              bool   `json:"changed"`
	ConfirmationRequired bool   `json:"confirmation_required"`
	DiscardedCellCount   int    `json:"discarded_cell_count"`
	ConfirmationToken    string `json:"confirm_discard_token,omitempty"`
	Unmerged             bool   `json:"unmerged"`
}

// SpreadsheetMergeCellsEditor merges a bounded range and centers its anchor,
// or unmerges when the selection identifies an existing merged cell/range.
type SpreadsheetMergeCellsEditor interface {
	MergeCellsChanged(sheet string, bounds CellRange, confirmationToken string) (SpreadsheetMergeResult, error)
}

// CellFormulaEdit describes one formula written to a spreadsheet cell.
type CellFormulaEdit struct {
	Row     int    `json:"row"`
	Column  int    `json:"column"`
	Formula string `json:"formula"`
}

// SpreadsheetFormulaBatchEditor applies a formula batch as one document edit.
type SpreadsheetFormulaBatchEditor interface {
	SetCellFormulasChanged(sheet string, edits []CellFormulaEdit) (bool, error)
}

// SpreadsheetFormulaRangeEntryEditor writes one formula across a selected
// rectangle, shifting relative references from the active cell to each target.
type SpreadsheetFormulaRangeEntryEditor interface {
	SetCellFormulaRangeChanged(sheet string, active CellCoordinate, bounds CellRange, formula string) (bool, error)
}

// SpreadsheetRangeContentEditor clears cell values and formulas while preserving formatting.
type SpreadsheetRangeContentEditor interface {
	ClearCellRange(sheet string, bounds CellRange) (changed bool, err error)
}

type SpreadsheetRangeReader interface {
	ReadRange(sheet string, bounds CellRange) ([][]Cell, error)
}

// SpreadsheetSelectionStats summarizes a selected worksheet range. Count is
// the number of cells in the selection, including blanks. Sum and NumericCount
// use numeric cell values; formulas with empty results count as selected cells
// but do not affect the numeric statistics.
type SpreadsheetSelectionStats struct {
	Count        int64
	NumericCount int64
	Sum          float64
}

// SpreadsheetSelectionStatsReader provides a sparse, cancellation-aware
// summary for the spreadsheet status bar without expanding a whole-row or
// whole-column selection into a dense grid.
type SpreadsheetSelectionStatsReader interface {
	SelectionStats(ctx context.Context, sheet string, bounds CellRange) (SpreadsheetSelectionStats, error)
}

// SpreadsheetClipboardRangeReader includes transfer metadata that ordinary
// range consumers do not need, such as formula-group type information.
type SpreadsheetClipboardRangeReader interface {
	ReadClipboardRange(sheet string, bounds CellRange) ([][]Cell, error)
}

// CellMatch represents a cell matched by a search query. Coordinates are 1-based.
type CellMatch struct {
	Row    int    `json:"row"`
	Column int    `json:"column"`
	Value  string `json:"value"`
}

// SpreadsheetSearcher is an optional capability for searching cell values in a worksheet.
type SpreadsheetSearcher interface {
	SearchSheet(sheet, query string) ([]CellMatch, error)
}

// SpreadsheetCellValueKindReader lets editing workflows preserve cell types
// without exposing format-library values to the UI.
type SpreadsheetCellValueKindReader interface {
	StoredCellValueKind(sheet, cell string) (CellValueKind, error)
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
	Sheet      string    `json:"sheet"`
	Bounds     CellRange `json:"bounds"`
	KeyColumn  int       `json:"key_column"`
	Descending bool      `json:"descending"`
}
type SpreadsheetSorter interface {
	SortRange(request SortRequest) error
}

type SpreadsheetSortChangeEditor interface {
	SortRangeChanged(request SortRequest) (bool, error)
}

// SpreadsheetPageLayout describes the format-neutral print page setup for a worksheet.
type SpreadsheetPageLayout struct {
	Orientation  string  `json:"orientation"`   // "portrait" or "landscape"
	PaperSize    int     `json:"paper_size"`    // 1=Letter, 9=A4, 8=A3, 5=Legal
	MarginTop    float64 `json:"margin_top"`    // in inches (e.g. 0.75)
	MarginRight  float64 `json:"margin_right"`  // in inches (e.g. 0.7)
	MarginBottom float64 `json:"margin_bottom"` // in inches (e.g. 0.75)
	MarginLeft   float64 `json:"margin_left"`   // in inches (e.g. 0.7)
	PrintArea    string  `json:"print_area"`    // e.g. "$A$1:$D$10" or empty
	PrintTitles  string  `json:"print_titles"`  // e.g. "$1:$1" or empty
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
	Target  string `json:"target"`  // e.g. "https://example.com" or "Sheet2!A1"
	Tooltip string `json:"tooltip"` // optional hover text
}

// SpreadsheetHyperlinkEditor is an optional capability for managing worksheet hyperlinks.
type SpreadsheetHyperlinkEditor interface {
	GetCellHyperlink(sheet, cell string) (CellHyperlink, bool, error)
	SetCellHyperlink(sheet, cell, target, tooltip string) error
	RemoveCellHyperlink(sheet, cell string) error
}

// SpreadsheetPaneFreeze describes frozen rows and columns in a worksheet.
type SpreadsheetPaneFreeze struct {
	Frozen bool `json:"frozen"`
	XSplit int  `json:"x_split"` // number of frozen columns on the left (0 = none)
	YSplit int  `json:"y_split"` // number of frozen rows at the top (0 = none)
}

// SpreadsheetPanesEditor is an optional capability for configuring frozen panes.
type SpreadsheetPanesEditor interface {
	GetSheetPanes(sheet string) (SpreadsheetPaneFreeze, error)
	SetSheetPanes(sheet string, freeze SpreadsheetPaneFreeze) error
}

// SpreadsheetDuplicatesResult reports the outcome of a duplicate row removal operation.
type SpreadsheetDuplicatesResult struct {
	RemovedCount   int `json:"removed_count"`
	RemainingCount int `json:"remaining_count"`
}

// SpreadsheetDuplicatesEditor is an optional capability for deduplicating rows in a range.
type SpreadsheetDuplicatesEditor interface {
	RemoveDuplicates(sheet string, bounds CellRange, keyColumn int) (SpreadsheetDuplicatesResult, error)
}

// SpreadsheetSheetView describes display settings for a worksheet.
type SpreadsheetSheetView struct {
	ViewMode    string `json:"view_mode"` // "normal", "pageBreakPreview", "pageLayout"
	ShowRuler   bool   `json:"show_ruler"`
	ZoomPercent int    `json:"zoom_percent"`
}

// SpreadsheetViewsEditor is an optional capability for reading and configuring worksheet views.
type SpreadsheetViewsEditor interface {
	GetSheetView(sheet string) (SpreadsheetSheetView, error)
	SetSheetViewMode(sheet string, mode string) error
	SetSheetRuler(sheet string, show bool) error
	SetSheetZoom(sheet string, percent int) error
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
	RowsSplit int `json:"rows_split"`
	MaxCols   int `json:"max_columns"`
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
	Name    string `json:"name"`
	Ref     string `json:"ref"`
	Scope   string `json:"scope"`
	Comment string `json:"comment"`
}

// SpreadsheetDefinedNameEditor provides reading, adding, and deleting defined names.
type SpreadsheetDefinedNameEditor interface {
	GetDefinedNames() ([]SpreadsheetDefinedName, error)
	SetDefinedName(name SpreadsheetDefinedName) error
	DeleteDefinedName(name string, scope string) error
}

// SpreadsheetFlashFillResult describes the result of a flash fill / series fill operation.
type SpreadsheetFlashFillResult struct {
	RowsFilled int `json:"rows_filled"`
}

// SpreadsheetFlashFillEditor supports filling down series or repeating patterns into selection.
type SpreadsheetFlashFillEditor interface {
	FlashFill(sheet string, selection CellRange) (SpreadsheetFlashFillResult, error)
}

// SpreadsheetDataValidationType represents validation rule kinds.
type SpreadsheetDataValidationType string

const (
	DataValidationTypeList   SpreadsheetDataValidationType = "list"
	DataValidationTypeNumber SpreadsheetDataValidationType = "number"
)

// SpreadsheetDataValidationSpec specifies a data validation constraint for a range.
type SpreadsheetDataValidationSpec struct {
	Sheet     string
	Range     CellRange
	Type      SpreadsheetDataValidationType
	DropList  []string
	MinNumber *float64
	MaxNumber *float64
}

// SpreadsheetDataValidationEditor provides adding and removing cell data validations.
type SpreadsheetDataValidationEditor interface {
	AddDataValidation(spec SpreadsheetDataValidationSpec) (changed bool, err error)
	DeleteDataValidation(sheet string, bounds CellRange) (changed bool, err error)
}

// SpreadsheetComment represents a cell note/comment.
type SpreadsheetComment struct {
	Author string `json:"author"`
	Cell   string `json:"cell"`
	Text   string `json:"text"`
}

// SpreadsheetCommentEditor provides adding, reading, and deleting cell comments.
type SpreadsheetCommentEditor interface {
	GetComments(sheet string) ([]SpreadsheetComment, error)
	AddComment(sheet string, comment SpreadsheetComment) error
	DeleteComment(sheet string, cell string) error
}

// SpreadsheetConditionalFormatType represents preset conditional format kinds.
type SpreadsheetConditionalFormatType string

const (
	ConditionalFormatDataBar     SpreadsheetConditionalFormatType = "data_bar"
	ConditionalFormat3ColorScale SpreadsheetConditionalFormatType = "3_color_scale"
	ConditionalFormatDuplicate   SpreadsheetConditionalFormatType = "duplicate"
	ConditionalFormatAboveAvg    SpreadsheetConditionalFormatType = "above_average"
)

// SpreadsheetConditionalFormatSpec specifies a conditional formatting rule for a range.
type SpreadsheetConditionalFormatSpec struct {
	Sheet string
	Range CellRange
	Type  SpreadsheetConditionalFormatType
}

// SpreadsheetConditionalFormatEditor provides adding and clearing conditional formatting.
type SpreadsheetConditionalFormatEditor interface {
	SetConditionalFormat(spec SpreadsheetConditionalFormatSpec) (changed bool, err error)
	ClearConditionalFormat(sheet string, bounds CellRange) (changed bool, err error)
}

// SpreadsheetShapeSpec defines parameters for adding a shape or text box to a worksheet.
type SpreadsheetShapeSpec struct {
	Sheet  string
	Cell   string // Top-left anchor cell, e.g. "B2"
	Type   string // Shape type: "rect", "roundRect", "ellipse", "rightArrow", "star5", etc.
	Width  uint   // Width in pixels (default e.g. 160)
	Height uint   // Height in pixels (default e.g. 90)
	Text   string // Optional inner text
}

// SpreadsheetShapeEditor provides adding shapes and text boxes to a worksheet.
type SpreadsheetShapeEditor interface {
	AddShape(spec SpreadsheetShapeSpec) error
}

// SpreadsheetChartKind represents supported chart types.
type SpreadsheetChartKind string

const (
	SpreadsheetChartColumn SpreadsheetChartKind = "col"
	SpreadsheetChartLine   SpreadsheetChartKind = "line"
	SpreadsheetChartPie    SpreadsheetChartKind = "pie"
)

// SpreadsheetChartSpec defines parameters for generating a chart in a worksheet.
type SpreadsheetChartSpec struct {
	Sheet string
	Cell  string // Top-left anchor cell, e.g. "E2"
	Kind  SpreadsheetChartKind
	Title string
	Range CellRange // Data range, e.g. A1:B5
}

// SpreadsheetChartEditor provides adding charts to a worksheet.
type SpreadsheetChartEditor interface {
	AddChart(spec SpreadsheetChartSpec) error
}

// SpreadsheetPivotTableSpec defines parameters for generating a pivot table in a worksheet.
type SpreadsheetPivotTableSpec struct {
	Sheet           string
	DataRange       CellRange // Source data range, e.g. A1:D10
	PivotTableRange string    // Target destination range or anchor, e.g. "F2:J15"
	Rows            []string  // Row fields
	Columns         []string  // Column fields
	Data            []string  // Data/value fields
}

// SpreadsheetPivotTableEditor provides adding pivot tables to a worksheet.
type SpreadsheetPivotTableEditor interface {
	AddPivotTable(spec SpreadsheetPivotTableSpec) error
}

// SpreadsheetCellRichTextEditor provides capabilities to read and format inline rich text runs of a cell.
type SpreadsheetCellRichTextEditor interface {
	GetCellRichText(sheet, cell string) ([]CellRichTextRun, error)
	SetCellRichText(sheet, cell string, runs []CellRichTextRun) error
	SetCellTextSelectionFormat(sheet, cell, text string, startRune, endRune int, patch CellFormatPatch) error
}
