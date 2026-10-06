package xlsx

import (
	"testing"

	"glance/internal/document"
)

func TestWorkbookRemoveDuplicatesAndUndo(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	// Populate 5 rows:
	// A1: "Alice", B1: "Engineering"
	// A2: "Bob",   B2: "Marketing"
	// A3: "Alice", B3: "Engineering" (duplicate of row 1)
	// A4: "Carol", B4: "Finance"
	// A5: "Bob",   B5: "Marketing"   (duplicate of row 2)
	_ = wb.SetCellText(sheet, "A1", "Alice")
	_ = wb.SetCellText(sheet, "B1", "Engineering")
	_ = wb.SetCellText(sheet, "A2", "Bob")
	_ = wb.SetCellText(sheet, "B2", "Marketing")
	_ = wb.SetCellText(sheet, "A3", "Alice")
	_ = wb.SetCellText(sheet, "B3", "Engineering")
	_ = wb.SetCellText(sheet, "A4", "Carol")
	_ = wb.SetCellText(sheet, "B4", "Finance")
	_ = wb.SetCellText(sheet, "A5", "Bob")
	_ = wb.SetCellText(sheet, "B5", "Marketing")

	editor, ok := any(wb).(document.SpreadsheetDuplicatesEditor)
	if !ok {
		t.Fatal("workbook does not implement SpreadsheetDuplicatesEditor")
	}

	bounds := document.CellRange{
		StartRow:    1,
		StartColumn: 1,
		EndRow:      5,
		EndColumn:   2,
	}

	res, err := editor.RemoveDuplicates(sheet, bounds, 0)
	if err != nil {
		t.Fatalf("RemoveDuplicates failed: %v", err)
	}
	if res.RemovedCount != 2 || res.RemainingCount != 3 {
		t.Fatalf("expected removed=2, remaining=3, got %+v", res)
	}

	// Verify remaining rows:
	// Row 1: Alice, Engineering
	// Row 2: Bob, Marketing
	// Row 3: Carol, Finance
	v1, _ := wb.Value(sheet, "A1")
	v2, _ := wb.Value(sheet, "A2")
	v3, _ := wb.Value(sheet, "A3")
	if v1 != "Alice" || v2 != "Bob" || v3 != "Carol" {
		t.Fatalf("unexpected values after deduplication: A1=%q, A2=%q, A3=%q", v1, v2, v3)
	}

	// Test Undo
	history := any(wb).(document.SpreadsheetHistory)
	undone, err := history.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo failed: undone=%v err=%v", undone, err)
	}
	v1, _ = wb.Value(sheet, "A1")
	v2, _ = wb.Value(sheet, "A2")
	v3, _ = wb.Value(sheet, "A3")
	v4, _ := wb.Value(sheet, "A4")
	v5, _ := wb.Value(sheet, "A5")
	if v1 != "Alice" || v2 != "Bob" || v3 != "Alice" || v4 != "Carol" || v5 != "Bob" {
		t.Fatalf("expected rows restored after undo, got A1=%q A2=%q A3=%q A4=%q A5=%q", v1, v2, v3, v4, v5)
	}

	// Test Redo
	redone, err := history.Redo()
	if err != nil || !redone {
		t.Fatalf("Redo failed: redone=%v err=%v", redone, err)
	}
	v1, _ = wb.Value(sheet, "A1")
	v2, _ = wb.Value(sheet, "A2")
	v3, _ = wb.Value(sheet, "A3")
	if v1 != "Alice" || v2 != "Bob" || v3 != "Carol" {
		t.Fatalf("unexpected values after redo: A1=%q, A2=%q, A3=%q", v1, v2, v3)
	}
}
