package xlsx

import (
	"testing"

	"github.com/xuri/excelize/v2"
	"glance/internal/document"
)

func TestTransferCellRangeCopiesTypesStylesAndAdjustsFormulaReferences(t *testing.T) {
	f := excelize.NewFile()
	if _, err := f.NewSheet("Other Sheet"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellFormula("Sheet1", "A1", "B1+$C$1+A$1+$A1+'Other Sheet'!B1"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue("Sheet1", "B1", 42.5); err != nil {
		t.Fatal(err)
	}
	styleID, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellStyle("Sheet1", "A1", "B1", styleID); err != nil {
		t.Fatal(err)
	}
	path := saveTestWorkbook(t, f)
	wb, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()

	source := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 2}
	cells, err := wb.ReadClipboardRange("Sheet1", source)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := wb.TransferCellRangeChanged("Sheet1", source, 3, 4, cells)
	if err != nil || !changed {
		t.Fatalf("TransferCellRangeChanged changed=%v, err=%v", changed, err)
	}
	formula, err := wb.file.GetCellFormula("Sheet1", "D3")
	if err != nil || formula != "E3+$C$1+D$1+$A3+'Other Sheet'!E3" {
		t.Fatalf("pasted formula=%q, err=%v", formula, err)
	}
	value, err := wb.file.GetCellValue("Sheet1", "E3", excelize.Options{RawCellValue: true})
	if err != nil || value != "42.5" {
		t.Fatalf("pasted raw numeric value=%q, err=%v", value, err)
	}
	for _, cell := range []string{"D3", "E3"} {
		got, err := wb.file.GetCellStyle("Sheet1", cell)
		if err != nil || got != styleID {
			t.Fatalf("pasted style for %s=%d, err=%v; want %d", cell, got, err, styleID)
		}
	}
	snapshot, err := wb.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got, err := reopened.file.GetCellFormula("Sheet1", "D3"); err != nil || got != "E3+$C$1+D$1+$A3+'Other Sheet'!E3" {
		t.Fatalf("saved copied formula=%q err=%v", got, err)
	}
	if got, err := reopened.file.GetCellStyle("Sheet1", "E3"); err != nil || got != styleID {
		t.Fatalf("saved copied style=%d err=%v; want %d", got, err, styleID)
	}
	changed, err = wb.TransferCellRangeChanged("Sheet1", source, 3, 4, cells)
	if err != nil || changed {
		t.Fatalf("pasting identical range should be a no-op: changed=%v err=%v", changed, err)
	}
	if ok, err := wb.Undo(); err != nil || !ok {
		t.Fatalf("Undo=%v err=%v", ok, err)
	}
	for _, cell := range []string{"D3", "E3"} {
		formula, err := wb.file.GetCellFormula("Sheet1", cell)
		value, valueErr := wb.file.GetCellValue("Sheet1", cell)
		style, styleErr := wb.file.GetCellStyle("Sheet1", cell)
		if err != nil || valueErr != nil || styleErr != nil || formula != "" || value != "" || style != 0 {
			t.Fatalf("Undo left %s formula=%q value=%q style=%d errors=%v/%v/%v", cell, formula, value, style, err, valueErr, styleErr)
		}
	}
}

func TestTransferCellRangeRejectsOutOfBoundsWithoutChanges(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()
	if err := wb.file.SetCellFormula("Sheet1", "A1", "SUM(B1:B2)"); err != nil {
		t.Fatal(err)
	}
	cells, err := wb.ReadClipboardRange("Sheet1", document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := wb.TransferCellRangeChanged("Sheet1", document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}, 1_048_576, 1, cells)
	if err == nil || changed {
		t.Fatalf("out-of-bounds transfer changed=%v err=%v; want rejection", changed, err)
	}
	if formula, _ := wb.file.GetCellFormula("Sheet1", "A1"); formula != "SUM(B1:B2)" {
		t.Fatalf("source formula changed after rejected transfer: %q", formula)
	}
}

func TestTransferCellRangePreservesFormulaGroupSafety(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()
	formulaType, formulaRange := excelize.STCellFormulaTypeArray, "A1:A2"
	if err := wb.file.SetCellFormula("Sheet1", "A1", "ROW(A1:A2)", excelize.FormulaOpts{Type: &formulaType, Ref: &formulaRange}); err != nil {
		t.Fatal(err)
	}
	if err := wb.file.SetCellValue("Sheet1", "C1", "plain"); err != nil {
		t.Fatal(err)
	}
	groupRange := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 2, EndColumn: 1}
	groupCells, err := wb.ReadClipboardRange("Sheet1", groupRange)
	if err != nil {
		t.Fatal(err)
	}
	if groupCells[0][0].FormulaType != "array" {
		t.Fatalf("array formula type=%q; want array", groupCells[0][0].FormulaType)
	}
	if changed, err := wb.TransferCellRangeChanged("Sheet1", groupRange, 1, 4, groupCells); err == nil || changed {
		t.Fatalf("copying an array formula group changed=%v err=%v; want safe rejection", changed, err)
	}
	plainCells, err := wb.ReadClipboardRange("Sheet1", document.CellRange{StartRow: 1, StartColumn: 3, EndRow: 1, EndColumn: 3})
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := wb.TransferCellRangeChanged("Sheet1", document.CellRange{StartRow: 1, StartColumn: 3, EndRow: 1, EndColumn: 3}, 2, 1, plainCells); err == nil || changed {
		t.Fatalf("paste into an array formula group changed=%v err=%v; want safe rejection", changed, err)
	}
}

func TestTranslateClipboardFormulaAdjustsOnlyRelativeReferences(t *testing.T) {
	got, err := translateClipboardFormula("SUM(A1:B2)+$C$4+MyNamedRange", 2, 3, map[string]bool{"mynamedrange": true})
	if err != nil {
		t.Fatal(err)
	}
	if want := "SUM(D3:E4)+$C$4+MyNamedRange"; got != want {
		t.Fatalf("translated formula=%q; want %q", got, want)
	}
}
