package xlsx

import (
	excelize "github.com/xuri/excelize/v2"
	"glance/internal/document"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSortRangeNumericTextBlankAndUndo(t *testing.T) {
	f := excelize.NewFile()
	_ = f.SetCellValue("Sheet1", "A1", "10")
	_ = f.SetCellValue("Sheet1", "B1", "ten")
	_ = f.SetCellValue("Sheet1", "A2", "2")
	_ = f.SetCellValue("Sheet1", "B2", "two")
	_ = f.SetCellValue("Sheet1", "A3", "apple")
	_ = f.SetCellValue("Sheet1", "B3", "text")
	style, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		t.Fatal(err)
	}
	_ = f.SetCellStyle("Sheet1", "A2", "B2", style)
	path := saveTestWorkbook(t, f)
	book, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	req := document.SortRequest{Sheet: "Sheet1", Bounds: document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 4, EndColumn: 2}, KeyColumn: 1}
	if err := book.SortRange(req); err != nil {
		t.Fatal(err)
	}
	w, err := book.Window("Sheet1", 1, 1, 4, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{w.Cells[0][0].Value, w.Cells[1][0].Value, w.Cells[2][0].Value, w.Cells[3][0].Value}; !reflect.DeepEqual(got, []string{"2", "10", "apple", ""}) {
		t.Fatalf("ascending keys=%v", got)
	}
	if !w.Cells[0][0].Style.Bold || w.Cells[0][1].Value != "two" {
		t.Fatalf("row style/content didn't move with key: %#v", w.Cells[0])
	}
	if applied, err := book.Undo(); err != nil || !applied {
		t.Fatalf("undo=(%t,%v)", applied, err)
	}
	if err := book.SortRange(document.SortRequest{Sheet: "Sheet1", Bounds: req.Bounds, KeyColumn: 1, Descending: true}); err != nil {
		t.Fatal(err)
	}
	if got, _ := book.Value("Sheet1", "A1"); got != "apple" {
		t.Fatalf("descending A1=%q", got)
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
	if got, _ := reopened.Value("Sheet1", "A1"); got != "apple" {
		t.Fatalf("saved descending order A1=%q", got)
	}
}

func TestSortRangeRejectsFormulaAndMergedRangesAtomically(t *testing.T) {
	f := excelize.NewFile()
	_ = f.SetCellValue("Sheet1", "A1", "2")
	_ = f.SetCellFormula("Sheet1", "B1", "1+1")
	_ = f.SetCellValue("Sheet1", "A2", "1")
	path := saveTestWorkbook(t, f)
	book, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	book.SetDirty(false)
	b := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 2, EndColumn: 2}
	if err := book.SortRange(document.SortRequest{Sheet: "Sheet1", Bounds: b, KeyColumn: 3}); err == nil {
		t.Fatal("sort key outside selection should be rejected")
	}
	if err := book.SortRange(document.SortRequest{Sheet: "Sheet1", Bounds: b, KeyColumn: 1}); err == nil {
		t.Fatal("formula-containing sort should be rejected")
	}
	if got, _ := book.Value("Sheet1", "A1"); got != "2" || book.Dirty() {
		t.Fatalf("rejected sort changed workbook: A1=%q dirty=%t", got, book.Dirty())
	}
	f2 := excelize.NewFile()
	_ = f2.SetCellValue("Sheet1", "A1", "one")
	_ = f2.MergeCell("Sheet1", "A1", "B1")
	path2 := saveTestWorkbook(t, f2)
	merged, err := Open(path2)
	if err != nil {
		t.Fatal(err)
	}
	defer merged.Close()
	if err := merged.SortRange(document.SortRequest{Sheet: "Sheet1", Bounds: document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 2, EndColumn: 2}, KeyColumn: 1}); err == nil {
		t.Fatal("merged-cell sort should be rejected")
	}
}

func TestSortRangePreservesNativeCellTypes(t *testing.T) {
	f := excelize.NewFile()
	if err := f.SetCellValue("Sheet1", "A1", 10.0); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue("Sheet1", "B1", true); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue("Sheet1", "A2", 2.0); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue("Sheet1", "B2", false); err != nil {
		t.Fatal(err)
	}
	path := saveTestWorkbook(t, f)
	book, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	initialType, _ := book.file.GetCellType("Sheet1", "A1")
	if initialType != excelize.CellTypeUnset {
		t.Fatalf("fixture A1 type=%v", initialType)
	}
	if err := book.SortRange(document.SortRequest{Sheet: "Sheet1", Bounds: document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 2, EndColumn: 2}, KeyColumn: 1}); err != nil {
		t.Fatal(err)
	}
	typ, err := book.file.GetCellType("Sheet1", "A1")
	if err != nil || typ != excelize.CellTypeUnset {
		t.Fatalf("sorted numeric cell type=%v err=%v", typ, err)
	}
	value, err := book.file.GetCellValue("Sheet1", "A1", excelize.Options{RawCellValue: true})
	if err != nil || value != "2" {
		t.Fatalf("sorted number raw=%q err=%v", value, err)
	}
	typ, err = book.file.GetCellType("Sheet1", "B1")
	if err != nil || typ != excelize.CellTypeBool {
		t.Fatalf("sorted bool cell type=%v err=%v", typ, err)
	}
	copyPath := filepath.Join(t.TempDir(), "sorted.xlsx")
	if err := book.SaveAs(copyPath); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(copyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if raw, err := reopened.file.GetCellValue("Sheet1", "A1", excelize.Options{RawCellValue: true}); err != nil || raw != "2" {
		t.Fatalf("reopened numeric cell raw=%q err=%v", raw, err)
	}
	if typ, err := reopened.file.GetCellType("Sheet1", "B1"); err != nil || typ != excelize.CellTypeBool {
		t.Fatalf("reopened bool cell type=%v err=%v", typ, err)
	}
}
