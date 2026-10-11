package xlsx

import (
	"testing"

	"glance/internal/document"
)

func TestWorkbookRecalculateUpdatesFormulaCacheAndMarksDirty(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()
	sheet := wb.SheetNames()[0]
	if err := wb.SetCellText(sheet, "A1", "1"); err != nil {
		t.Fatal(err)
	}
	if err := wb.SetCellText(sheet, "A2", "2"); err != nil {
		t.Fatal(err)
	}
	if err := wb.SetCellFormula(sheet, "B1", "=SUM(A1:A2)"); err != nil {
		t.Fatal(err)
	}
	wb.SetDirty(false)
	if err := wb.SetCellText(sheet, "A1", "5"); err != nil {
		t.Fatal(err)
	}
	if err := wb.RecalculateFormulas(); err != nil {
		t.Fatalf("RecalculateFormulas: %v", err)
	}
	value, err := wb.Value(sheet, "B1")
	if err != nil || value != "7" {
		t.Fatalf("recalculated formula value = %q, %v; want 7", value, err)
	}
	if !wb.Dirty() {
		t.Fatal("recalculating cached formula values did not mark the workbook dirty")
	}
}

func TestWorkbookSetCellFormulaAndUndo(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]

	_ = wb.SetCellText(sheet, "A1", "10")
	_ = wb.SetCellText(sheet, "A2", "20")

	editor, ok := any(wb).(document.SpreadsheetFormulaEditor)
	if !ok {
		t.Fatal("workbook does not implement SpreadsheetFormulaEditor")
	}

	if err := editor.SetCellFormula(sheet, "A3", "=SUM(A1:A2)"); err != nil {
		t.Fatalf("SetCellFormula failed: %v", err)
	}

	formula, err := editor.GetCellFormula(sheet, "A3")
	if err != nil {
		t.Fatalf("GetCellFormula failed: %v", err)
	}
	if formula != "SUM(A1:A2)" {
		t.Fatalf("expected formula SUM(A1:A2), got %q", formula)
	}

	// Undo
	undone, err := wb.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo failed: undone=%v err=%v", undone, err)
	}

	formulaAfterUndo, _ := editor.GetCellFormula(sheet, "A3")
	if formulaAfterUndo != "" {
		t.Fatalf("expected empty formula after undo, got %q", formulaAfterUndo)
	}

	// Redo
	redone, err := wb.Redo()
	if err != nil || !redone {
		t.Fatalf("Redo failed: redone=%v err=%v", redone, err)
	}
	formulaAfterRedo, _ := editor.GetCellFormula(sheet, "A3")
	if formulaAfterRedo != "SUM(A1:A2)" {
		t.Fatalf("expected restored formula SUM(A1:A2), got %q", formulaAfterRedo)
	}
}
