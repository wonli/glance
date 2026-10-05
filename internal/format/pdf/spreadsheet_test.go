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
