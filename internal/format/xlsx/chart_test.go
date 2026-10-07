package xlsx

import (
	"path/filepath"
	"testing"

	"glance/internal/document"
)

func TestWorkbookAddChartAndUndo(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]

	// Populate test data
	_ = wb.SetCellText(sheet, "A1", "Item")
	_ = wb.SetCellText(sheet, "B1", "Sales")
	_ = wb.SetCellText(sheet, "A2", "Apples")
	_ = wb.SetCellText(sheet, "B2", "100")
	_ = wb.SetCellText(sheet, "A3", "Oranges")
	_ = wb.SetCellText(sheet, "B3", "150")

	// 1. Column chart
	err := wb.AddChart(document.SpreadsheetChartSpec{
		Sheet: sheet,
		Cell:  "E2",
		Kind:  document.SpreadsheetChartColumn,
		Title: "销量柱状图",
		Range: document.CellRange{
			StartRow:    1,
			StartColumn: 1,
			EndRow:      3,
			EndColumn:   2,
		},
	})
	if err != nil {
		t.Fatalf("AddChart column failed: %v", err)
	}
	if !wb.Dirty() {
		t.Fatalf("expected workbook to be dirty after AddChart")
	}

	// Undo
	undone, err := wb.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo failed: %v", err)
	}

	// Redo
	redone, err := wb.Redo()
	if err != nil || !redone {
		t.Fatalf("Redo failed: %v", err)
	}

	// 2. Line chart
	err = wb.AddChart(document.SpreadsheetChartSpec{
		Sheet: sheet,
		Cell:  "E15",
		Kind:  document.SpreadsheetChartLine,
		Title: "趋势折线图",
		Range: document.CellRange{
			StartRow:    1,
			StartColumn: 1,
			EndRow:      3,
			EndColumn:   2,
		},
	})
	if err != nil {
		t.Fatalf("AddChart line failed: %v", err)
	}

	// 3. Pie chart
	err = wb.AddChart(document.SpreadsheetChartSpec{
		Sheet: sheet,
		Cell:  "E28",
		Kind:  document.SpreadsheetChartPie,
		Title: "占比饼图",
		Range: document.CellRange{
			StartRow:    1,
			StartColumn: 1,
			EndRow:      3,
			EndColumn:   2,
		},
	})
	if err != nil {
		t.Fatalf("AddChart pie failed: %v", err)
	}

	// Save and verify reopen
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "chart_test.xlsx")
	if err := wb.SaveAs(path); err != nil {
		t.Fatalf("SaveAs failed: %v", err)
	}

	wb2, err := Open(path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer wb2.Close()
}
