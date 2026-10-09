package xlsx

import (
	excelize "github.com/xuri/excelize/v2"
	"glance/internal/document"
	"path/filepath"
	"testing"
)

// Test when guard fails

// Test when guard succeeds

func TestWorkbookRejectsFormulaLikeText(t *testing.T) {
	f := excelize.NewFile()
	path := filepath.Join(t.TempDir(), "formula.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	wb, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()
	if err := wb.SetCellText("Sheet1", "A1", "=1+1"); err == nil {
		t.Fatal("expected formula-like input to be rejected")
	}
}

// Verify clear format on wrap text

// Validation tests

func singleCellRange(cell string) document.CellRange {
	column, row, err := excelize.CellNameToCoordinates(cell)
	if err != nil {
		panic(err)
	}
	return document.CellRange{StartRow: row, StartColumn: column, EndRow: row, EndColumn: column}
}

func saveTestWorkbook(t *testing.T, file *excelize.File) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "window.xlsx")
	if err := file.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
