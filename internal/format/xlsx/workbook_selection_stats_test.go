package xlsx

import (
	"context"
	"math"
	"testing"

	"github.com/xuri/excelize/v2"
	"glance/internal/document"
)

func TestWorkbookSelectionStatsCountsSelectedCellsAndSumsNumericCells(t *testing.T) {
	file := excelize.NewFile()
	for cell, value := range map[string]any{
		"A1": 2.5,
		"B1": "note",
		"B2": true,
		"C2": 0,
	} {
		if err := file.SetCellValue("Sheet1", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := file.SetCellFormula("Sheet1", "C1", "1+3"); err != nil {
		t.Fatal(err)
	}
	if err := file.SetCellFormula("Sheet1", "A2", `""`); err != nil {
		t.Fatal(err)
	}
	book := NewFromExcelize(file, "")
	defer book.Close()
	reader, ok := any(book).(document.SpreadsheetSelectionStatsReader)
	if !ok {
		t.Fatal("XLSX workbook does not provide selection statistics")
	}
	got, err := reader.SelectionStats(context.Background(), "Sheet1", document.CellRange{
		StartRow: 1, StartColumn: 1, EndRow: 2, EndColumn: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Count != 6 || got.NumericCount != 3 || math.Abs(got.Sum-6.5) > 1e-9 {
		t.Fatalf("selection stats = %+v; want Count=6 NumericCount=3 Sum=6.5", got)
	}
}

func TestWorkbookSelectionStatsCountsBlankCellsInSelection(t *testing.T) {
	book := NewBlank()
	defer book.Close()
	reader, ok := any(book).(document.SpreadsheetSelectionStatsReader)
	if !ok {
		t.Fatal("XLSX workbook does not provide selection statistics")
	}
	got, err := reader.SelectionStats(context.Background(), "Sheet1", document.CellRange{
		StartRow: 1, StartColumn: 1, EndRow: 3, EndColumn: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Count != 12 || got.NumericCount != 0 || got.Sum != 0 {
		t.Fatalf("blank selection stats = %+v; want Count=12 NumericCount=0 Sum=0", got)
	}
}

func TestWorkbookSelectionStatsHonorsCancellation(t *testing.T) {
	book := NewBlank()
	defer book.Close()
	reader, ok := any(book).(document.SpreadsheetSelectionStatsReader)
	if !ok {
		t.Fatal("XLSX workbook does not provide selection statistics")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reader.SelectionStats(ctx, "Sheet1", document.CellRange{
		StartRow: 1, StartColumn: 1, EndRow: 1_048_576, EndColumn: 16_384,
	}); err == nil {
		t.Fatal("canceled whole-sheet summary should stop before scanning")
	}
}

func TestWorkbookSelectionStatsDoesNotCountNumericLookingText(t *testing.T) {
	file := excelize.NewFile()
	if err := file.SetCellValue("Sheet1", "A1", "123"); err != nil {
		t.Fatal(err)
	}
	if err := file.SetCellFormula("Sheet1", "B1", `"456"`); err != nil {
		t.Fatal(err)
	}
	book := NewFromExcelize(file, "")
	defer book.Close()
	reader, ok := any(book).(document.SpreadsheetSelectionStatsReader)
	if !ok {
		t.Fatal("XLSX workbook does not provide selection statistics")
	}
	got, err := reader.SelectionStats(context.Background(), "Sheet1", document.CellRange{
		StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Count != 2 || got.NumericCount != 0 || got.Sum != 0 {
		t.Fatalf("numeric-looking text selection stats = %+v; want Count=2 NumericCount=0 Sum=0", got)
	}
}
