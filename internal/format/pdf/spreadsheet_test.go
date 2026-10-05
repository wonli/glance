package pdf

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SalvioniDigitalSolutions/gopdf"

	"glance/internal/document"
)

func TestWriteSpreadsheetPDFBasic(t *testing.T) {
	window := document.SheetWindow{
		StartRow:    1,
		StartColumn: 1,
		Cells: [][]document.Cell{
			{
				{Value: "标题", Style: document.CellStyle{Bold: true, FontSize: 14, HorizontalAlignment: "center", FillColor: "E2F0D9"}},
				{Value: "", Style: document.CellStyle{}},
				{Value: "状态", Style: document.CellStyle{Bold: true}},
			},
			{
				{Value: "第一项", Style: document.CellStyle{}},
				{Value: "100.5", Style: document.CellStyle{HorizontalAlignment: "right"}},
				{Value: "正常", Style: document.CellStyle{FontColor: "008000"}},
			},
			{
				{Value: "第二项", Style: document.CellStyle{}},
				{Value: "200.0", Style: document.CellStyle{HorizontalAlignment: "right"}},
				{Value: "警告", Style: document.CellStyle{FontColor: "FF0000"}},
			},
		},
		RowHeightsPoints:  []float64{24, 18, 18},
		ColumnWidthsChars: []float64{15, 12, 10},
		Merges: []document.MergeRegion{
			{
				Range:  document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 2},
				Anchor: document.Cell{Value: "标题", Style: document.CellStyle{Bold: true, FontSize: 14, HorizontalAlignment: "center", FillColor: "E2F0D9"}},
			},
		},
	}

	var buf bytes.Buffer
	err := WriteSpreadsheetPDF(context.Background(), &buf, window, SpreadsheetPDFOptions{SheetName: "Sheet1"}, nil, nil)
	if err != nil {
		t.Fatalf("WriteSpreadsheetPDF failed: %v", err)
	}

	reader, err := gopdf.NewReader(buf.Bytes())
	if err != nil {
		t.Fatalf("read generated PDF: %v", err)
	}
	if reader.NumPages() != 1 {
		t.Fatalf("pages = %d, want 1", reader.NumPages())
	}
	text, err := reader.PageText(0)
	if err != nil {
		t.Fatalf("extract text: %v", err)
	}
	for _, expected := range []string{"100.5", "200.0"} {
		if !strings.Contains(text, expected) {
			t.Errorf("extracted text %q does not contain %q", text, expected)
		}
	}
}

func TestWriteSpreadsheetPDFPaginatesRows(t *testing.T) {
	const rowCount = 60
	cells := make([][]document.Cell, rowCount)
	rowHeights := make([]float64, rowCount)
	for r := 0; r < rowCount; r++ {
		cells[r] = []document.Cell{
			{Value: "Row item", Style: document.CellStyle{}},
			{Value: "123", Style: document.CellStyle{}},
		}
		rowHeights[r] = 20
	}
	window := document.SheetWindow{
		StartRow:          1,
		StartColumn:       1,
		Cells:             cells,
		RowHeightsPoints:  rowHeights,
		ColumnWidthsChars: []float64{20, 15},
	}

	var buf bytes.Buffer
	err := WriteSpreadsheetPDF(context.Background(), &buf, window, SpreadsheetPDFOptions{SheetName: "Sheet1"}, nil, nil)
	if err != nil {
		t.Fatalf("WriteSpreadsheetPDF failed: %v", err)
	}

	reader, err := gopdf.NewReader(buf.Bytes())
	if err != nil {
		t.Fatalf("read generated PDF: %v", err)
	}
	if reader.NumPages() < 2 {
		t.Fatalf("expected multiple pages for %d rows, got %d", rowCount, reader.NumPages())
	}
}

func TestExportSpreadsheetPDFFileCommitGuardAndAtomicReplace(t *testing.T) {
	tempDir := t.TempDir()
	outPath := filepath.Join(tempDir, "export.pdf")

	window := document.SheetWindow{
		StartRow:    1,
		StartColumn: 1,
		Cells: [][]document.Cell{
			{{Value: "Test cell"}},
		},
	}

	var guardCalled bool
	err := ExportSpreadsheetPDFFileWithCommitGuard(context.Background(), outPath, window, SpreadsheetPDFOptions{SheetName: "Sheet1"}, nil, nil, func() (func(), error) {
		guardCalled = true
		return nil, nil
	})
	if err != nil {
		t.Fatalf("ExportSpreadsheetPDFFileWithCommitGuard failed: %v", err)
	}
	if !guardCalled {
		t.Fatal("expected commit guard to be called")
	}

	reader, err := gopdf.Open(outPath)
	if err != nil {
		t.Fatalf("open exported PDF: %v", err)
	}
	if reader.NumPages() != 1 {
		t.Fatalf("pages = %d, want 1", reader.NumPages())
	}

	// Verify canceled context preserves destination
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = ExportSpreadsheetPDFFileWithCommitGuard(ctx, outPath, window, SpreadsheetPDFOptions{SheetName: "Sheet1"}, nil, nil, nil)
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	// Existing file should still be readable
	data, err := os.ReadFile(outPath)
	if err != nil || len(data) == 0 {
		t.Fatalf("existing file corrupted or missing after cancel: %v", err)
	}
}

func TestTrimSheetWindow(t *testing.T) {
	// A 10x10 window where only 3 rows and 4 columns have content
	cells := make([][]document.Cell, 10)
	for r := 0; r < 10; r++ {
		cells[r] = make([]document.Cell, 10)
	}
	cells[0][0] = document.Cell{Value: "Header A"}
	cells[0][1] = document.Cell{Value: "Header B"}
	cells[0][2] = document.Cell{Value: "Header C"}
	cells[0][3] = document.Cell{Value: "Header D"}
	cells[1][0] = document.Cell{Value: "Row 1"}
	cells[2][3] = document.Cell{Value: "Row 2 Col 4"}

	window := document.SheetWindow{
		StartRow:          1,
		StartColumn:       1,
		Cells:             cells,
		RowHeightsPoints:  make([]float64, 10),
		ColumnWidthsChars: make([]float64, 10),
	}

	trimmed, rows, cols := TrimSheetWindow(window, nil)
	if rows != 3 {
		t.Errorf("trimmed rows = %d, want 3", rows)
	}
	if cols != 4 {
		t.Errorf("trimmed cols = %d, want 4", cols)
	}
	if len(trimmed.Cells) != 3 {
		t.Errorf("len(trimmed.Cells) = %d, want 3", len(trimmed.Cells))
	}
	if len(trimmed.Cells[0]) != 4 {
		t.Errorf("len(trimmed.Cells[0]) = %d, want 4", len(trimmed.Cells[0]))
	}
}

func TestComputeSpreadsheetPaginationLandscapeFitsSinglePage(t *testing.T) {
	// A table with 6 columns, each 15 chars wide (~105 pt each). Total width = 630 pt.
	// A4 Portrait printable width is 595 - 72 = 523 pt (630 > 523).
	// In portrait without fit-to-width, it splits into 2 column bands (2 pages).
	// In landscape, printable width is 842 - 72 = 770 pt (630 <= 770).
	// In landscape, it fits on 1 single page!
	cells := make([][]document.Cell, 10)
	for r := 0; r < 10; r++ {
		cells[r] = make([]document.Cell, 10)
		if r >= 5 {
			continue
		}
		for c := 0; c < 6; c++ {
			cells[r][c] = document.Cell{Value: "Data"}
		}
	}
	colWidths := []float64{15, 15, 15, 15, 15, 15, 8, 8, 8, 8}
	window := document.SheetWindow{
		StartRow:          1,
		StartColumn:       1,
		Cells:             cells,
		ColumnWidthsChars: colWidths,
		RowHeightsPoints:  []float64{20, 20, 20, 20, 20},
	}

	// In portrait without fit-to-width:
	portraitOptions := SpreadsheetPDFOptions{Orientation: "portrait", SheetName: "Sheet1"}
	pPortrait := ComputeSpreadsheetPagination(window, portraitOptions, nil)
	if pPortrait.TotalPages < 2 {
		t.Errorf("expected portrait to paginate wide table to >= 2 pages, got %d", pPortrait.TotalPages)
	}

	// In landscape:
	landscapeOptions := SpreadsheetPDFOptions{Orientation: "landscape", SheetName: "Sheet1"}
	pLandscape := ComputeSpreadsheetPagination(window, landscapeOptions, nil)
	if pLandscape.TotalPages != 1 {
		t.Errorf("expected landscape to fit table on 1 page, got %d", pLandscape.TotalPages)
	}
	if pLandscape.UsedRows != 5 || pLandscape.UsedCols != 6 {
		t.Errorf("landscape exported area = %dx%d, want 5x6", pLandscape.UsedRows, pLandscape.UsedCols)
	}
	if len(pLandscape.ColBands) != 1 {
		t.Errorf("expected 1 column band in landscape, got %d", len(pLandscape.ColBands))
	}
	if pLandscape.PageWidthPT != 842.0 || pLandscape.PageHeightPT != 595.0 {
		t.Errorf("expected landscape dimensions 842x595, got %vx%v", pLandscape.PageWidthPT, pLandscape.PageHeightPT)
	}

	// Export with landscape and check generated PDF
	var buf bytes.Buffer
	err := WriteSpreadsheetPDF(context.Background(), &buf, window, landscapeOptions, nil, nil)
	if err != nil {
		t.Fatalf("WriteSpreadsheetPDF landscape failed: %v", err)
	}
	reader, err := gopdf.NewReader(buf.Bytes())
	if err != nil {
		t.Fatalf("read landscape PDF: %v", err)
	}
	if reader.NumPages() != 1 {
		t.Errorf("landscape PDF pages = %d, want 1", reader.NumPages())
	}
	size, err := reader.PageSize(0)
	if err != nil {
		t.Fatalf("get page size: %v", err)
	}
	if size.W != 842 || size.H != 595 {
		t.Errorf("page dimensions = %vx%v, want 842x595", size.W, size.H)
	}
}

func TestSpreadsheetPDFPaginationMatchesRenderedPages(t *testing.T) {
	cells := make([][]document.Cell, 42)
	for r := range cells {
		cells[r] = make([]document.Cell, 8)
		for c := range cells[r] {
			cells[r][c].Value = "data"
		}
	}
	window := document.SheetWindow{StartRow: 1, StartColumn: 1, Cells: cells,
		ColumnWidthsChars: []float64{14, 14, 14, 14, 14, 14, 14, 14}}
	opts := SpreadsheetPDFOptions{SheetName: "Sheet1", Orientation: "landscape", FitToPageWidth: true}
	pagination := ComputeSpreadsheetPagination(window, opts, nil)
	var buf bytes.Buffer
	if err := WriteSpreadsheetPDF(context.Background(), &buf, window, opts, nil, nil); err != nil {
		t.Fatal(err)
	}
	reader, err := gopdf.NewReader(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if reader.NumPages() != pagination.TotalPages {
		t.Fatalf("rendered %d pages, preview calculated %d", reader.NumPages(), pagination.TotalPages)
	}
}

func TestSpreadsheetPDFKeepsWrappedCellLines(t *testing.T) {
	window := document.SheetWindow{StartRow: 1, StartColumn: 1,
		Cells:             [][]document.Cell{{{Value: "first line\nsecond line", Style: document.CellStyle{WrapText: true}}}},
		ColumnWidthsChars: []float64{12}, RowHeightsPoints: []float64{18},
	}
	pagination := ComputeSpreadsheetPagination(window, SpreadsheetPDFOptions{SheetName: "Sheet1"}, nil)
	if pagination.RowHeights[0] <= 18 {
		t.Fatalf("wrapped row height = %v, want space for both lines", pagination.RowHeights[0])
	}
	var buf bytes.Buffer
	if err := WriteSpreadsheetPDF(context.Background(), &buf, window, SpreadsheetPDFOptions{SheetName: "Sheet1"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	reader, err := gopdf.NewReader(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	content, err := reader.PageText(0)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.ReplaceAll(content, "\n", "")
	if !strings.Contains(joined, "first line") || !strings.Contains(joined, "second line") {
		t.Fatalf("PDF lost wrapped cell text: %q", content)
	}
}

func TestMergedCellOuterBordersUseEdgeCells(t *testing.T) {
	cells := [][]document.Cell{{
		{Value: "merged", Style: document.CellStyle{Borders: [4]document.CellBorder{{Style: 1}, {}, {}, {Style: 1}}}},
		{Style: document.CellStyle{Borders: [4]document.CellBorder{{}, {Style: 1}, {Style: 1}, {}}}},
	}}
	borders := mergedCellOuterBorders(cells, 0, 0, 0, 1)
	for side, border := range borders {
		if border.Style == 0 {
			t.Fatalf("merged outer border %d missing: %+v", side, borders)
		}
	}
}

func TestSpreadsheetCellLinesKeepChineseCharacters(t *testing.T) {
	value := "第一行中文内容\n第二行需要自动换行的文字"
	lines := spreadsheetCellLines(value, true, 36, 10)
	if len(lines) < 3 {
		t.Fatalf("wrapped lines = %v, want explicit and automatic breaks", lines)
	}
	if joined := strings.Join(lines, ""); joined != strings.ReplaceAll(value, "\n", "") {
		t.Fatalf("wrapped text lost characters: %q", joined)
	}
}

// Covered cells may repeat the anchor value in adapter snapshots. Only the
// visible anchor text should contribute to the merged row's height.
func TestSpreadsheetPaginationIgnoresCoveredMergeText(t *testing.T) {
	text := strings.Repeat("操作说明", 30)
	window := document.SheetWindow{StartRow: 1, StartColumn: 1,
		Cells:             [][]document.Cell{make([]document.Cell, 7)},
		ColumnWidthsChars: []float64{30, 50, 22, 7, 10, 10, 10},
		RowHeightsPoints:  []float64{57},
		Merges:            []document.MergeRegion{{Range: document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 7}}},
	}
	for c := range window.Cells[0] {
		window.Cells[0][c] = document.Cell{Value: text, Style: document.CellStyle{FontSize: 10, WrapText: true}}
	}
	p := ComputeSpreadsheetPagination(window, SpreadsheetPDFOptions{Orientation: "portrait", FitToPageWidth: true}, nil)
	if p.RowHeights[0] > 60 {
		t.Fatalf("merged instruction row height = %.2f, want <= 60pt", p.RowHeights[0])
	}
}

func TestMergedCellOuterBordersExcludeInteriorEdges(t *testing.T) {
	cells := [][]document.Cell{{
		{Style: document.CellStyle{Borders: [4]document.CellBorder{{}, {Style: 2}, {}, {Style: 1}}}},
		{Style: document.CellStyle{Borders: [4]document.CellBorder{{}, {Style: 1}, {}, {Style: 2}}}},
	}}
	borders := mergedCellOuterBorders(cells, 0, 0, 0, 1)
	if borders[1].Style != 1 || borders[3].Style != 1 {
		t.Fatalf("outer borders = %+v; interior edge must not replace perimeter", borders)
	}
}
