package xlsx

import (
	"testing"

	"glance/internal/document"
)

func TestWorkbookFlashFillAndUndo(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]

	// Set A1=1, A2=2
	_ = wb.SetCellText(sheet, "A1", "1")
	_ = wb.SetCellText(sheet, "A2", "2")
	// Set B1="Item-01"
	_ = wb.SetCellText(sheet, "B1", "Item-01")

	// 1. Fill numeric series A1:A5
	rng := document.CellRange{
		StartColumn: 1, StartRow: 1,
		EndColumn: 1, EndRow: 5,
	}
	res, err := wb.FlashFill(sheet, rng)
	if err != nil {
		t.Fatalf("FlashFill A1:A5: %v", err)
	}
	if res.RowsFilled != 3 {
		t.Fatalf("expected 3 rows filled, got %d", res.RowsFilled)
	}

	v3, _ := wb.Value(sheet, "A3")
	v4, _ := wb.Value(sheet, "A4")
	v5, _ := wb.Value(sheet, "A5")
	if v3 != "3" || v4 != "4" || v5 != "5" {
		t.Fatalf("unexpected filled values: A3=%s, A4=%s, A5=%s", v3, v4, v5)
	}

	// 2. Fill text series B1:B3
	rngB := document.CellRange{
		StartColumn: 2, StartRow: 1,
		EndColumn: 2, EndRow: 3,
	}
	resB, err := wb.FlashFill(sheet, rngB)
	if err != nil {
		t.Fatalf("FlashFill B1:B3: %v", err)
	}
	if resB.RowsFilled != 2 {
		t.Fatalf("expected 2 rows filled, got %d", resB.RowsFilled)
	}
	b2, _ := wb.Value(sheet, "B2")
	b3, _ := wb.Value(sheet, "B3")
	if b2 != "Item-02" || b3 != "Item-03" {
		t.Fatalf("unexpected text series: B2=%s, B3=%s", b2, b3)
	}

	// 3. Test Undo
	undone, err := wb.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo failed: undone=%v err=%v", undone, err)
	}
	b2AfterUndo, _ := wb.Value(sheet, "B2")
	if b2AfterUndo != "" {
		t.Fatalf("expected B2 to be empty after undo, got %q", b2AfterUndo)
	}

	// 4. Test Redo
	redone, err := wb.Redo()
	if err != nil || !redone {
		t.Fatalf("Redo failed: redone=%v err=%v", redone, err)
	}
	b2AfterRedo, _ := wb.Value(sheet, "B2")
	if b2AfterRedo != "Item-02" {
		t.Fatalf("expected B2 to be restored after redo, got %q", b2AfterRedo)
	}
}
