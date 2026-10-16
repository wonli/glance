package xlsx

import (
	"path/filepath"
	"strings"
	"testing"

	"glance/internal/document"
)

func TestWorkbookSetCellFormulasChangedIsOneUndoableBatch(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()
	sheet := wb.SheetNames()[0]
	for cell, value := range map[string]string{"A1": "1", "A2": "2", "B1": "3", "B2": "4"} {
		if err := wb.SetCellText(sheet, cell, value); err != nil {
			t.Fatalf("SetCellText(%s): %v", cell, err)
		}
	}
	wb.SetDirty(false)
	beforeState := wb.HistoryState()
	changed, err := wb.SetCellFormulasChanged(sheet, nil)
	if err != nil || changed || wb.HistoryState() != beforeState {
		t.Fatalf("empty formula batch = changed %v err %v history %d; want no-op at %d", changed, err, wb.HistoryState(), beforeState)
	}

	edits := []document.CellFormulaEdit{
		{Row: 3, Column: 1, Formula: "=SUM(A1:A2)"},
		{Row: 3, Column: 2, Formula: "=AVERAGE(B1:B2)"},
	}
	changed, err = wb.SetCellFormulasChanged(sheet, edits)
	if err != nil || !changed {
		t.Fatalf("SetCellFormulasChanged: changed=%v err=%v; want true, nil", changed, err)
	}
	if got := wb.HistoryState(); got != beforeState+1 {
		t.Fatalf("batch history state = %d; want one step from %d", got, beforeState)
	}
	if !wb.Dirty() {
		t.Fatal("formula batch did not mark workbook dirty")
	}
	for _, tc := range []struct{ cell, want string }{{"A3", "SUM(A1:A2)"}, {"B3", "AVERAGE(B1:B2)"}} {
		formula, formulaErr := wb.GetCellFormula(sheet, tc.cell)
		if formulaErr != nil || formula != tc.want {
			t.Fatalf("formula %s = %q, %v; want %q", tc.cell, formula, formulaErr, tc.want)
		}
	}

	changed, err = wb.SetCellFormulasChanged(sheet, []document.CellFormulaEdit{
		{Row: 3, Column: 1, Formula: "SUM(A1:A2)"},
		{Row: 3, Column: 2, Formula: "AVERAGE(B1:B2)"},
	})
	if err != nil || changed {
		t.Fatalf("identical formula batch = changed %v, err %v; want false, nil", changed, err)
	}
	if got := wb.HistoryState(); got != beforeState+1 {
		t.Fatalf("no-op batch changed history state to %d; want %d", got, beforeState+1)
	}

	undone, err := wb.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo formula batch: undone=%v err=%v", undone, err)
	}
	for _, cell := range []string{"A3", "B3"} {
		formula, formulaErr := wb.GetCellFormula(sheet, cell)
		if formulaErr != nil || formula != "" {
			t.Fatalf("formula %s after one undo = %q, %v; want empty", cell, formula, formulaErr)
		}
	}
	redone, err := wb.Redo()
	if err != nil || !redone {
		t.Fatalf("Redo formula batch: redone=%v err=%v", redone, err)
	}
	for _, tc := range []struct{ cell, want string }{{"A3", "SUM(A1:A2)"}, {"B3", "AVERAGE(B1:B2)"}} {
		formula, formulaErr := wb.GetCellFormula(sheet, tc.cell)
		if formulaErr != nil || formula != tc.want {
			t.Fatalf("formula %s after redo = %q, %v; want %q", tc.cell, formula, formulaErr, tc.want)
		}
	}
	if err := wb.SetCellText(sheet, "A1", "5"); err != nil {
		t.Fatalf("edit formula source: %v", err)
	}
	if err := wb.RecalculateFormulas(); err != nil {
		t.Fatalf("recalculate formula batch after source edit: %v", err)
	}
	if value, err := wb.Value(sheet, "A3"); err != nil || value != "7" {
		t.Fatalf("SUM result after source edit = %q, %v; want 7", value, err)
	}

	path := filepath.Join(t.TempDir(), "formulas.xlsx")
	if err := wb.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open saved formula workbook: %v", err)
	}
	defer reopened.Close()
	for _, tc := range []struct{ cell, want string }{{"A3", "SUM(A1:A2)"}, {"B3", "AVERAGE(B1:B2)"}} {
		formula, formulaErr := reopened.GetCellFormula(sheet, tc.cell)
		if formulaErr != nil || formula != tc.want {
			t.Fatalf("reopened formula %s = %q, %v; want %q", tc.cell, formula, formulaErr, tc.want)
		}
	}
}

func TestWorkbookSetCellFormulasChangedValidatesEntireBatchBeforeMutation(t *testing.T) {
	tests := []struct {
		name  string
		sheet string
		edits []document.CellFormulaEdit
	}{
		{name: "missing sheet", sheet: "Missing", edits: []document.CellFormulaEdit{{Row: 1, Column: 1, Formula: "=1"}}},
		{name: "zero row", sheet: "Sheet1", edits: []document.CellFormulaEdit{{Row: 0, Column: 1, Formula: "=1"}}},
		{name: "column over limit", sheet: "Sheet1", edits: []document.CellFormulaEdit{{Row: 1, Column: 16385, Formula: "=1"}}},
		{name: "invalid later destination after valid edit", sheet: "Sheet1", edits: []document.CellFormulaEdit{{Row: 2, Column: 1, Formula: "=1"}, {Row: 0, Column: 2, Formula: "=2"}}},
		{name: "duplicate destination", sheet: "Sheet1", edits: []document.CellFormulaEdit{{Row: 1, Column: 1, Formula: "=1"}, {Row: 1, Column: 1, Formula: "=2"}}},
		{name: "empty formula", sheet: "Sheet1", edits: []document.CellFormulaEdit{{Row: 1, Column: 1, Formula: " = "}}},
		{name: "formula beyond UTF-16 limit", sheet: "Sheet1", edits: []document.CellFormulaEdit{{Row: 1, Column: 1, Formula: "=" + strings.Repeat("😀", 4096) + "1"}}},
		{name: "invalid UTF-8 formula validates before writes", sheet: "Sheet1", edits: []document.CellFormulaEdit{{Row: 2, Column: 1, Formula: "=1"}, {Row: 2, Column: 2, Formula: string([]byte{0xff})}}},
		{name: "oversized batch", sheet: "Sheet1", edits: oversizedFormulaBatch()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wb := NewBlank()
			defer wb.Close()
			sheet := wb.SheetNames()[0]
			if err := wb.SetCellText(sheet, "A1", "existing"); err != nil {
				t.Fatal(err)
			}
			wb.SetDirty(false)
			beforeState, beforeDirty := wb.HistoryState(), wb.Dirty()
			changed, err := wb.SetCellFormulasChanged(tc.sheet, tc.edits)
			if err == nil || changed {
				t.Fatalf("invalid batch result = changed %v, err %v; want false and error", changed, err)
			}
			if wb.HistoryState() != beforeState || wb.Dirty() != beforeDirty {
				t.Fatalf("invalid batch changed workbook state: history %d -> %d, dirty %v -> %v", beforeState, wb.HistoryState(), beforeDirty, wb.Dirty())
			}
			for _, edit := range tc.edits {
				if edit.Row < 1 || edit.Row > maxXLSXRows || edit.Column < 1 || edit.Column > maxXLSXColumns {
					continue
				}
				cell := CellName(edit.Column, edit.Row)
				formula, formulaErr := wb.GetCellFormula(sheet, cell)
				if formulaErr != nil || formula != "" {
					t.Fatalf("invalid batch partially wrote %s formula %q, err %v", cell, formula, formulaErr)
				}
			}
		})
	}
}

func oversizedFormulaBatch() []document.CellFormulaEdit {
	edits := make([]document.CellFormulaEdit, 10001)
	for i := range edits {
		edits[i] = document.CellFormulaEdit{Row: i + 1, Column: 1, Formula: "=1"}
	}
	return edits
}
