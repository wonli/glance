package pdf

import "glance/internal/document"

// SpreadsheetPDFOptions controls worksheet page setup and default layout.
type SpreadsheetPDFOptions struct {
	SheetName       string
	Orientation     string // "portrait" or "landscape" (or empty for auto)
	PageWidthPT     float64
	PageHeightPT    float64
	MarginPT        float64
	DefaultFontKey  string
	DefaultFontSize float64
	FitToPageWidth  bool
}

// SpreadsheetImageSource returns over-cell picture metadata and original image bytes.
type SpreadsheetImageSource interface {
	SpreadsheetImages(sheet string) ([]document.SpreadsheetImage, error)
	SpreadsheetImageBytes(imageID string) ([]byte, error)
}

// SpreadsheetFontResolver is an optional extension on FontSource that picks
// a suitable system font key for a given cell style and text content.
type SpreadsheetFontResolver interface {
	ResolveSpreadsheetFont(style document.CellStyle, text string) (string, error)
}

// SpreadsheetBand represents a half-open range [Start, End) of column or row indices.
type SpreadsheetBand struct {
	Start int
	End   int
}

// SpreadsheetPagination contains computed page dimensions, bands, and trimmed window.
type SpreadsheetPagination struct {
	TrimmedWindow   document.SheetWindow
	Orientation     string
	PageWidthPT     float64
	PageHeightPT    float64
	MarginPT        float64
	TotalGridWidth  float64
	TotalGridHeight float64
	Scale           float64
	ColWidths       []float64
	RowHeights      []float64
	ColBands        []SpreadsheetBand
	RowBands        []SpreadsheetBand
	TotalPages      int
	UsedRows        int
	UsedCols        int
}
