package xlsx

import (
	"testing"

	"glance/internal/document"
)

func TestWorkbookAddPivotTableAndUndo(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]

	// Fill sample data in A1:C5
	_ = wb.SetCellText(sheet, "A1", "部门")
	_ = wb.SetCellText(sheet, "B1", "季度")
	_ = wb.SetCellText(sheet, "C1", "销售额")

	_ = wb.SetCellText(sheet, "A2", "研发")
	_ = wb.SetCellText(sheet, "B2", "Q1")
	_ = wb.SetCellText(sheet, "C2", "100")

	_ = wb.SetCellText(sheet, "A3", "市场")
	_ = wb.SetCellText(sheet, "B3", "Q1")
	_ = wb.SetCellText(sheet, "C3", "200")

	_ = wb.SetCellText(sheet, "A4", "研发")
	_ = wb.SetCellText(sheet, "B4", "Q2")
	_ = wb.SetCellText(sheet, "C4", "150")

	_ = wb.SetCellText(sheet, "A5", "市场")
	_ = wb.SetCellText(sheet, "B5", "Q2")
	_ = wb.SetCellText(sheet, "C5", "250")

	editor, ok := any(wb).(document.SpreadsheetPivotTableEditor)
	if !ok {
		t.Fatal("workbook does not implement SpreadsheetPivotTableEditor")
	}

	spec := document.SpreadsheetPivotTableSpec{
		Sheet:     sheet,
		DataRange: document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 5, EndColumn: 3},
		Rows:      []string{"部门"},
		Columns:   []string{"季度"},
		Data:      []string{"销售额"},
	}

	if err := editor.AddPivotTable(spec); err != nil {
		t.Fatalf("AddPivotTable failed: %v", err)
	}

	if !wb.Dirty() {
		t.Fatal("expected workbook to be dirty after AddPivotTable")
	}

	// Test Undo
	undone, err := wb.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo failed: %v", err)
	}

	// Test Redo
	redone, err := wb.Redo()
	if err != nil || !redone {
		t.Fatalf("Redo failed: %v", err)
	}
}
