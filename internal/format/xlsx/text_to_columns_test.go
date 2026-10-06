package xlsx

import (
	"testing"

	"glance/internal/document"
)

func TestWorkbookSplitTextToColumnsAndUndo(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	_ = wb.SetCellText(sheet, "A1", "Alice,Smith,Engineer")
	_ = wb.SetCellText(sheet, "A2", "Bob,Jones,Designer")
	_ = wb.SetCellText(sheet, "A3", "Carol,Taylor")

	editor, ok := any(wb).(document.SpreadsheetTextToColumnsEditor)
	if !ok {
		t.Fatal("workbook does not implement SpreadsheetTextToColumnsEditor")
	}

	bounds := document.CellRange{
		StartRow:    1,
		StartColumn: 1,
		EndRow:      3,
		EndColumn:   1,
	}

	res, err := editor.SplitTextToColumns(sheet, bounds, 1, ",")
	if err != nil {
		t.Fatalf("SplitTextToColumns failed: %v", err)
	}
	if res.RowsSplit != 3 || res.MaxCols != 3 {
		t.Fatalf("expected RowsSplit=3, MaxCols=3, got %+v", res)
	}

	// Verify split cells
	vA1, _ := wb.Value(sheet, "A1")
	vB1, _ := wb.Value(sheet, "B1")
	vC1, _ := wb.Value(sheet, "C1")
	if vA1 != "Alice" || vB1 != "Smith" || vC1 != "Engineer" {
		t.Fatalf("unexpected row 1 values: A1=%q, B1=%q, C1=%q", vA1, vB1, vC1)
	}

	vA2, _ := wb.Value(sheet, "A2")
	vB2, _ := wb.Value(sheet, "B2")
	vC2, _ := wb.Value(sheet, "C2")
	if vA2 != "Bob" || vB2 != "Jones" || vC2 != "Designer" {
		t.Fatalf("unexpected row 2 values: A2=%q, B2=%q, C2=%q", vA2, vB2, vC2)
	}

	vA3, _ := wb.Value(sheet, "A3")
	vB3, _ := wb.Value(sheet, "B3")
	vC3, _ := wb.Value(sheet, "C3")
	if vA3 != "Carol" || vB3 != "Taylor" || vC3 != "" {
		t.Fatalf("unexpected row 3 values: A3=%q, B3=%q, C3=%q", vA3, vB3, vC3)
	}

	// Undo
	undone, err := wb.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo failed: undone=%v err=%v", undone, err)
	}
	vA1Original, _ := wb.Value(sheet, "A1")
	vB1Original, _ := wb.Value(sheet, "B1")
	if vA1Original != "Alice,Smith,Engineer" || vB1Original != "" {
		t.Fatalf("unexpected values after Undo: A1=%q, B1=%q", vA1Original, vB1Original)
	}

	// Redo
	redone, err := wb.Redo()
	if err != nil || !redone {
		t.Fatalf("Redo failed: redone=%v err=%v", redone, err)
	}
	vA1Redone, _ := wb.Value(sheet, "A1")
	vB1Redone, _ := wb.Value(sheet, "B1")
	vC1Redone, _ := wb.Value(sheet, "C1")
	if vA1Redone != "Alice" || vB1Redone != "Smith" || vC1Redone != "Engineer" {
		t.Fatalf("unexpected values after Redo: A1=%q, B1=%q, C1=%q", vA1Redone, vB1Redone, vC1Redone)
	}
}

func TestWorkbookSplitTextToColumnsEdgeCases(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	_ = wb.SetCellText(sheet, "A1", "One Two Three")

	editor, ok := any(wb).(document.SpreadsheetTextToColumnsEditor)
	if !ok {
		t.Fatal("workbook does not implement SpreadsheetTextToColumnsEditor")
	}

	bounds := document.CellRange{
		StartRow:    1,
		StartColumn: 1,
		EndRow:      1,
		EndColumn:   1,
	}

	// Empty delimiter error
	_, err := editor.SplitTextToColumns(sheet, bounds, 1, "")
	if err == nil {
		t.Fatal("expected error for empty delimiter")
	}

	// Delimiter not found in any cell -> 0 rows split
	res, err := editor.SplitTextToColumns(sheet, bounds, 1, ";")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.RowsSplit != 0 {
		t.Fatalf("expected 0 rows split, got %d", res.RowsSplit)
	}

	// Space delimiter
	resSpace, err := editor.SplitTextToColumns(sheet, bounds, 1, " ")
	if err != nil {
		t.Fatalf("split by space failed: %v", err)
	}
	if resSpace.RowsSplit != 1 || resSpace.MaxCols != 3 {
		t.Fatalf("expected 1 row split with 3 cols, got %+v", resSpace)
	}
	vA1, _ := wb.Value(sheet, "A1")
	vB1, _ := wb.Value(sheet, "B1")
	vC1, _ := wb.Value(sheet, "C1")
	if vA1 != "One" || vB1 != "Two" || vC1 != "Three" {
		t.Fatalf("unexpected space-split values: A1=%q, B1=%q, C1=%q", vA1, vB1, vC1)
	}
}
