package xlsx

import (
	excelize "github.com/xuri/excelize/v2"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestWorkbookResizeSaveReopen(t *testing.T) {
	f := excelize.NewFile()
	if err := f.SetCellValue("Sheet1", "A1", "keep"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellFormula("Sheet1", "F1", "SUM(1,2)"); err != nil {
		t.Fatal(err)
	}
	path := saveTestWorkbook(t, f)
	wb, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()
	if err := wb.SetRowHeight("Sheet1", 3, 42); err != nil {
		t.Fatal(err)
	}
	if err := wb.SetColumnWidth("Sheet1", 4, 25); err != nil {
		t.Fatal(err)
	}
	if !wb.Dirty() {
		t.Fatal("dimension changes should mark workbook dirty")
	}
	if err := wb.Save(); err != nil {
		t.Fatal(err)
	}
	if wb.Dirty() {
		t.Fatal("successful save should clear dirty state")
	}
	reopened, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got, err := reopened.GetRowHeight("Sheet1", 3); err != nil || got != 42 {
		t.Errorf("saved row height = %v, %v; want 42, nil", got, err)
	}
	if got, err := reopened.GetColWidth("Sheet1", "D"); err != nil || got != 25 {
		t.Errorf("saved column width = %v, %v; want 25, nil", got, err)
	}
	if got, err := reopened.GetCellValue("Sheet1", "A1"); err != nil || got != "keep" {
		t.Errorf("untouched A1 = %q, %v; want %q", got, err, "keep")
	}
	if got, err := reopened.GetCellFormula("Sheet1", "F1"); err != nil || got != "SUM(1,2)" {
		t.Errorf("untouched F1 formula = %q, %v; want SUM(1,2)", got, err)
	}
}

func TestEmployeeWorkbookResizePreservesStylesAndMerges(t *testing.T) {
	dir := os.Getenv("GLANCE_OFFICE_TEST_DIR")
	if dir == "" {
		t.Skip("set GLANCE_OFFICE_TEST_DIR to the local office sample directory")
	}
	path := filepath.Join(dir, "人事表格-员工绩效表.xlsx")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wb, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	window, err := wb.Window("月绩效", 1, 1, 14, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(window.Merges) != 8 {
		t.Fatalf("sample merge count = %d; want 8", len(window.Merges))
	}
	if window.Cells[0][0].StyleID == 0 {
		t.Fatal("sample A1 should retain a non-default style")
	}
	untouched := window.Cells[3][1]
	copyPath := filepath.Join(t.TempDir(), filepath.Base(path))
	if err := wb.SaveAs(copyPath); err != nil {
		t.Fatal(err)
	}
	if err := wb.Close(); err != nil {
		t.Fatal(err)
	}
	wb, err = Open(copyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()
	if err := wb.SetCellText("月绩效", "Z500", "新增空白区内容"); err != nil {
		t.Fatal(err)
	}
	if err := wb.SetRowHeight("月绩效", 14, 31); err != nil {
		t.Fatal(err)
	}
	if err := wb.SetColumnWidth("月绩效", 26, 18); err != nil {
		t.Fatal(err)
	}
	if err := wb.Save(); err != nil {
		t.Fatal(err)
	}
	if err := wb.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(copyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got, err := reopened.Value("月绩效", "Z500"); err != nil || got != "新增空白区内容" {
		t.Fatalf("Z500 after save = %q, %v", got, err)
	}
	if got, err := reopened.file.GetRowHeight("月绩效", 14); err != nil || got != 31 {
		t.Errorf("row 14 height after save = %v, %v; want 31", got, err)
	}
	if got, err := reopened.file.GetColWidth("月绩效", "Z"); err != nil || got != 18 {
		t.Errorf("column Z width after save = %v, %v; want 18", got, err)
	}
	after, err := reopened.Window("月绩效", 1, 1, 14, 7)
	if err != nil {
		t.Fatal(err)
	}
	if after.Cells[3][1] != untouched {
		t.Errorf("untouched B4 changed: before=%#v after=%#v", untouched, after.Cells[3][1])
	}
	if !reflect.DeepEqual(after.Merges, window.Merges) {
		t.Errorf("sample merges changed after edit/resize: before=%#v after=%#v", window.Merges, after.Merges)
	}
	if got, err := os.ReadFile(path); err != nil || !reflect.DeepEqual(got, original) {
		t.Errorf("source sample changed: readErr=%v", err)
	}
}
