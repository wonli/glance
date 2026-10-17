package xlsx

import (
	excelize "github.com/xuri/excelize/v2"
	"glance/internal/document"
	"testing"
)

func TestEditStructureSheetsRowsColumnsAndUndo(t *testing.T) {
	f := excelize.NewFile()
	_ = f.SetCellValue("Sheet1", "A1", "keep")
	_ = f.SetCellValue("Sheet1", "B2", "right")
	path := saveTestWorkbook(t, f)
	book, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	if err := book.EditStructure(document.StructureEdit{Kind: document.StructureInsertRows, Sheet: "Sheet1", Index: 1, Count: 1}); err != nil {
		t.Fatal(err)
	}
	if got, _ := book.Value("Sheet1", "A2"); got != "keep" {
		t.Fatalf("row insert moved A1 to %q", got)
	}
	if applied, err := book.Undo(); err != nil || !applied {
		t.Fatalf("undo row insert=(%t,%v)", applied, err)
	}
	if got, _ := book.Value("Sheet1", "A1"); got != "keep" {
		t.Fatalf("undo left A1=%q", got)
	}
	if err := book.EditStructure(document.StructureEdit{Kind: document.StructureCreateSheet}); err != nil {
		t.Fatal(err)
	}
	if !contains(book.SheetNames(), "Sheet2") {
		t.Fatalf("sheet list=%v", book.SheetNames())
	}
	if err := book.EditStructure(document.StructureEdit{Kind: document.StructureRenameSheet, Sheet: "Sheet2", NewName: "汇总"}); err != nil {
		t.Fatal(err)
	}
	if err := book.EditStructure(document.StructureEdit{Kind: document.StructureDeleteSheet, Sheet: "汇总"}); err != nil {
		t.Fatal(err)
	}
	if err := book.EditStructure(document.StructureEdit{Kind: document.StructureDeleteSheet, Sheet: "Sheet1"}); err == nil {
		t.Fatal("deleting last sheet should fail")
	}
	if err := book.EditStructure(document.StructureEdit{Kind: document.StructureInsertColumns, Sheet: "Sheet1", Index: 1, Count: 1}); err != nil {
		t.Fatal(err)
	}
	if got, _ := book.Value("Sheet1", "B1"); got != "keep" {
		t.Fatalf("column insert moved A1 to %q", got)
	}
	if err := book.EditStructure(document.StructureEdit{Kind: document.StructureDeleteColumns, Sheet: "Sheet1", Index: 1, Count: 1}); err != nil {
		t.Fatal(err)
	}
	if got, _ := book.Value("Sheet1", "A1"); got != "keep" {
		t.Fatalf("column delete left A1=%q", got)
	}
}

func TestEditStructureRejectsInvalidSheetNamesAndBounds(t *testing.T) {
	book := NewBlank()
	defer book.Close()
	for _, edit := range []document.StructureEdit{{Kind: document.StructureCreateSheet, NewName: "bad/name"}, {Kind: document.StructureRenameSheet, Sheet: "Sheet1", NewName: ""}, {Kind: document.StructureInsertRows, Sheet: "Sheet1", Index: maxXLSXRows, Count: 2}} {
		if err := book.EditStructure(edit); err == nil {
			t.Errorf("expected rejection for %+v", edit)
		}
	}
}

func TestEditStructureSaveReopenPreservesStylesFormulasAndMerges(t *testing.T) {
	f := excelize.NewFile()
	_ = f.SetCellValue("Sheet1", "A1", "styled")
	style, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		t.Fatal(err)
	}
	_ = f.SetCellStyle("Sheet1", "A1", "A1", style)
	_ = f.SetCellFormula("Sheet1", "B2", "1+1")
	_ = f.SetCellValue("Sheet1", "C3", "merged")
	_ = f.MergeCell("Sheet1", "C3", "D3")
	path := saveTestWorkbook(t, f)
	book, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := book.EditStructure(document.StructureEdit{Kind: document.StructureInsertRows, Sheet: "Sheet1", Index: 1, Count: 1}); err != nil {
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
	window, err := reopened.Window("Sheet1", 1, 1, 4, 4)
	if err != nil {
		t.Fatal(err)
	}
	formula, formulaErr := reopened.file.GetCellFormula("Sheet1", "B3")
	if window.Cells[1][0].Value != "styled" || !window.Cells[1][0].Style.Bold || formulaErr != nil || formula == "" {
		t.Fatalf("row insert lost content/styles/formula: %#v formula=%q err=%v", window.Cells, formula, formulaErr)
	}
	if len(window.Merges) != 1 || window.Merges[0].Range.StartRow != 4 {
		t.Fatalf("row insert changed merged range unexpectedly: %#v", window.Merges)
	}
}

func TestEditStructureMultiRowAndColumnCountsSaveAndUndo(t *testing.T) {
	f := excelize.NewFile()
	for cell, value := range map[string]string{"A1": "top", "A2": "row two", "A3": "row three", "B1": "left", "C1": "right"} {
		if err := f.SetCellValue("Sheet1", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	path := saveTestWorkbook(t, f)
	book, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()

	if err := book.EditStructure(document.StructureEdit{Kind: document.StructureInsertRows, Sheet: "Sheet1", Index: 2, Count: 3}); err != nil {
		t.Fatal(err)
	}
	if err := book.EditStructure(document.StructureEdit{Kind: document.StructureInsertColumns, Sheet: "Sheet1", Index: 2, Count: 2}); err != nil {
		t.Fatal(err)
	}
	for cell, want := range map[string]string{"A1": "top", "A5": "row two", "A6": "row three", "D1": "left", "E1": "right"} {
		if got, err := book.Value("Sheet1", cell); err != nil || got != want {
			t.Fatalf("%s after multi-row/column inserts = %q (err=%v); want %q", cell, got, err, want)
		}
	}
	if applied, err := book.Undo(); err != nil || !applied {
		t.Fatalf("undo multi-column insert = (%t,%v)", applied, err)
	}
	if got, _ := book.Value("Sheet1", "B1"); got != "left" {
		t.Fatalf("B1 after undoing two-column insert = %q; want left", got)
	}
	if applied, err := book.Undo(); err != nil || !applied {
		t.Fatalf("undo multi-row insert = (%t,%v)", applied, err)
	}
	if got, _ := book.Value("Sheet1", "A2"); got != "row two" {
		t.Fatalf("A2 after undoing three-row insert = %q; want row two", got)
	}
	if applied, err := book.Redo(); err != nil || !applied {
		t.Fatalf("redo multi-row insert = (%t,%v)", applied, err)
	}
	if applied, err := book.Redo(); err != nil || !applied {
		t.Fatalf("redo multi-column insert = (%t,%v)", applied, err)
	}
	if err := book.Save(); err != nil {
		t.Fatal(err)
	}
	if err := book.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	for cell, want := range map[string]string{"A5": "row two", "A6": "row three", "D1": "left", "E1": "right"} {
		if got, err := reopened.Value("Sheet1", cell); err != nil || got != want {
			t.Fatalf("%s after save/reopen = %q (err=%v); want %q", cell, got, err, want)
		}
	}
}
