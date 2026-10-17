package xlsx

import (
	"path/filepath"
	"testing"

	"glance/internal/document"
)

func TestSetCellFormulaRangeChangedAdjustsRelativeReferencesAndUndoesAsOneEdit(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()
	sheet := wb.SheetNames()[0]
	bounds := document.CellRange{StartRow: 2, StartColumn: 2, EndRow: 3, EndColumn: 3}
	changed, err := wb.SetCellFormulaRangeChanged(sheet, document.CellCoordinate{Row: 2, Column: 2}, bounds, "=SUM(A1:A2)")
	if err != nil || !changed {
		t.Fatalf("SetCellFormulaRangeChanged changed=%t, err=%v; want true, nil", changed, err)
	}
	for address, want := range map[string]string{
		"B2": "SUM(A1:A2)", "C2": "SUM(B1:B2)",
		"B3": "SUM(A2:A3)", "C3": "SUM(B2:B3)",
	} {
		if got, err := wb.GetCellFormula(sheet, address); err != nil || got != want {
			t.Errorf("%s formula = %q, %v; want %s", address, got, err, want)
		}
	}
	if changed, err := wb.Undo(); err != nil || !changed {
		t.Fatalf("Undo formula range entry = changed %t, err %v; want one undoable edit", changed, err)
	}
	for _, address := range []string{"B2", "C2", "B3", "C3"} {
		if got, err := wb.GetCellFormula(sheet, address); err != nil || got != "" {
			t.Errorf("%s formula after undo = %q, %v; want empty", address, got, err)
		}
	}
}

func TestSetCellFormulaRangeChangedRejectsInvalidReferenceWithoutPartialEdit(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()
	sheet := wb.SheetNames()[0]
	if err := wb.SetCellText(sheet, "A1", "keep"); err != nil {
		t.Fatal(err)
	}
	if err := wb.SetCellText(sheet, "A2", "untouched"); err != nil {
		t.Fatal(err)
	}
	bounds := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 2, EndColumn: 1}
	changed, err := wb.SetCellFormulaRangeChanged(sheet, document.CellCoordinate{Row: 1, Column: 1}, bounds, "=A1048576")
	if err == nil || changed {
		t.Fatalf("out-of-bounds relative reference changed=%t, err=%v; want rejection without mutation", changed, err)
	}
	for address, want := range map[string]string{"A1": "keep", "A2": "untouched"} {
		if got, err := wb.Value(sheet, address); err != nil || got != want {
			t.Errorf("%s after rejected fill = %q, %v; want %q", address, got, err, want)
		}
		if formula, err := wb.GetCellFormula(sheet, address); err != nil || formula != "" {
			t.Errorf("%s formula after rejected fill = %q, %v; want empty", address, formula, err)
		}
	}
}

func TestSetCellFormulaRangeChangedKeepsAbsoluteReferenceComponentsFixed(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()
	sheet := wb.SheetNames()[0]
	bounds := document.CellRange{StartRow: 2, StartColumn: 2, EndRow: 3, EndColumn: 3}
	changed, err := wb.SetCellFormulaRangeChanged(sheet, document.CellCoordinate{Row: 2, Column: 2}, bounds, "=SUM($A1:B$2,$C$4)")
	if err != nil || !changed {
		t.Fatalf("SetCellFormulaRangeChanged changed=%t, err=%v; want true, nil", changed, err)
	}
	for address, want := range map[string]string{
		"B2": "SUM($A1:B$2,$C$4)", "C2": "SUM($A1:C$2,$C$4)",
		"B3": "SUM($A2:B$2,$C$4)", "C3": "SUM($A2:C$2,$C$4)",
	} {
		if got, err := wb.GetCellFormula(sheet, address); err != nil || got != want {
			t.Errorf("%s formula = %q, %v; want %s", address, got, err, want)
		}
	}
	path := filepath.Join(t.TempDir(), "formula-range.xlsx")
	if err := wb.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open saved formula range: %v", err)
	}
	defer reopened.Close()
	for address, want := range map[string]string{
		"B2": "SUM($A1:B$2,$C$4)", "C2": "SUM($A1:C$2,$C$4)",
		"B3": "SUM($A2:B$2,$C$4)", "C3": "SUM($A2:C$2,$C$4)",
	} {
		if got, err := reopened.GetCellFormula(sheet, address); err != nil || got != want {
			t.Errorf("%s formula after save/reopen = %q, %v; want %s", address, got, err, want)
		}
	}
}
