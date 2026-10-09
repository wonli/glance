package xlsx

import (
	excelize "github.com/xuri/excelize/v2"
	"glance/internal/document"
	"testing"
)

func TestSpreadsheetHistoryUndoRedoAndSaveRebase(t *testing.T) {
	path := saveTestWorkbook(t, excelize.NewFile())
	book, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	book.SetDirty(false)
	bold := true
	if err := book.SetCellFormats("Sheet1", singleCellRange("A1"), document.CellFormatPatch{Bold: &bold}); err != nil {
		t.Fatal(err)
	}
	if applied, err := book.Undo(); err != nil || !applied {
		t.Fatalf("undo=(%t,%v)", applied, err)
	}
	if book.Dirty() {
		t.Fatal("undo back to the saved state should clear dirty")
	}
	window, err := book.Window("Sheet1", 1, 1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if window.Cells[0][0].Style.Bold {
		t.Fatal("undo left bold formatting applied")
	}
	if applied, err := book.Redo(); err != nil || !applied {
		t.Fatalf("redo=(%t,%v)", applied, err)
	}
	window, err = book.Window("Sheet1", 1, 1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !window.Cells[0][0].Style.Bold {
		t.Fatal("redo did not restore bold formatting")
	}
	if err := book.Save(); err != nil {
		t.Fatal(err)
	}
	if applied, err := book.Undo(); err != nil || !applied {
		t.Fatalf("undo after save rebase=(%t,%v); want true,nil", applied, err)
	}
	if !book.Dirty() {
		t.Fatal("undo past the saved state should mark the workbook dirty")
	}
	if applied, err := book.Redo(); err != nil || !applied {
		t.Fatalf("redo back to save baseline=(%t,%v)", applied, err)
	}
	if book.Dirty() {
		t.Fatal("redo back to saved state should clear dirty")
	}
	italic := true
	if err := book.SetCellFormats("Sheet1", singleCellRange("A1"), document.CellFormatPatch{Italic: &italic}); err != nil {
		t.Fatal(err)
	}
	if applied, err := book.Undo(); err != nil || !applied {
		t.Fatalf("undo after save rebase and new edit=(%t,%v)", applied, err)
	}
}

func TestSetCellsBatchPreservesStylesAndIsUndoable(t *testing.T) {
	f := excelize.NewFile()
	style, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.SetCellStyle("Sheet1", "A1", "A1", style); err != nil {
		t.Fatal(err)
	}
	path := saveTestWorkbook(t, f)
	book, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	book.SetDirty(false)
	if err := book.SetCells("Sheet1", []document.CellEdit{{Row: 1, Column: 1, Value: "new"}, {Row: 1, Column: 2, Value: "2"}}); err != nil {
		t.Fatal(err)
	}
	window, err := book.Window("Sheet1", 1, 1, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if window.Cells[0][0].Value != "new" || !window.Cells[0][0].Style.Bold || window.Cells[0][1].Value != "2" {
		t.Fatalf("batch window=%#v", window.Cells)
	}
	if applied, err := book.Undo(); err != nil || !applied {
		t.Fatalf("undo=(%t,%v)", applied, err)
	}
	value, err := book.Value("Sheet1", "A1")
	if err != nil || value != "" {
		t.Fatalf("A1=%q,%v after undo", value, err)
	}
}

func TestSetCellsRejectsFormulaAtomically(t *testing.T) {
	book := NewBlank()
	defer book.Close()
	book.SetDirty(false)
	err := book.SetCells("Sheet1", []document.CellEdit{{Row: 1, Column: 1, Value: "first"}, {Row: 1, Column: 2, Value: "=1+1"}})
	if err == nil {
		t.Fatal("formula input should be rejected")
	}
	value, _ := book.Value("Sheet1", "A1")
	if value != "" || book.Dirty() {
		t.Fatalf("failed batch changed workbook: A1=%q dirty=%t", value, book.Dirty())
	}
}

func TestSetCellsSaveReopenPreservesUntouchedWorkbookContent(t *testing.T) {
	f := excelize.NewFile()
	_ = f.SetCellValue("Sheet1", "A1", "styled")
	style, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		t.Fatal(err)
	}
	_ = f.SetCellStyle("Sheet1", "A1", "A1", style)
	_ = f.SetCellFormula("Sheet1", "B1", "1+1")
	_ = f.SetCellValue("Sheet1", "C1", "merge")
	_ = f.MergeCell("Sheet1", "C1", "D1")
	path := saveTestWorkbook(t, f)
	book, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := book.SetCells("Sheet1", []document.CellEdit{{Row: 2, Column: 1, Value: "copied"}, {Row: 2, Column: 2, Value: "42"}}); err != nil {
		t.Fatal(err)
	}
	if err := book.Save(); err != nil {
		t.Fatal(err)
	}
	_ = book.Close()
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	window, err := reopened.Window("Sheet1", 1, 1, 2, 4)
	if err != nil {
		t.Fatal(err)
	}
	formula, formulaErr := reopened.file.GetCellFormula("Sheet1", "B1")
	if window.Cells[0][0].Value != "styled" || !window.Cells[0][0].Style.Bold || formulaErr != nil || formula == "" || window.Cells[1][0].Value != "copied" || window.Cells[1][1].Value != "42" {
		t.Fatalf("reopened clipboard region=%#v formula=%q err=%v", window.Cells, formula, formulaErr)
	}
	if len(window.Merges) != 1 || window.Merges[0].Range.StartColumn != 3 {
		t.Fatalf("untouched merges=%#v", window.Merges)
	}
}
