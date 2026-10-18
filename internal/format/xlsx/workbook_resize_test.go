package xlsx

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	excelize "github.com/xuri/excelize/v2"
	"glance/internal/document"
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

func TestWorkbookDimensionChangesUndoRedo(t *testing.T) {
	f := excelize.NewFile()
	path := saveTestWorkbook(t, f)
	wb, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()

	if err := wb.SetRowHeight("Sheet1", 2, 31); err != nil {
		t.Fatal(err)
	}
	if !wb.Dirty() {
		t.Fatal("row-height change should mark workbook dirty")
	}
	if applied, err := wb.Undo(); err != nil || !applied {
		t.Fatalf("undo row-height change = %v, %v; want true, nil", applied, err)
	}
	if got, err := wb.file.GetRowHeight("Sheet1", 2); err != nil || got != 15 {
		t.Fatalf("row height after undo = %v, %v; want default 15 pt", got, err)
	}
	if wb.Dirty() {
		t.Fatal("undoing the only dimension change should restore the saved state")
	}
	if applied, err := wb.Redo(); err != nil || !applied {
		t.Fatalf("redo row-height change = %v, %v; want true, nil", applied, err)
	}
	if got, err := wb.file.GetRowHeight("Sheet1", 2); err != nil || got != 31 {
		t.Fatalf("row height after redo = %v, %v; want 31 pt", got, err)
	}

	originalColumnWidth, err := wb.file.GetColWidth("Sheet1", "C")
	if err != nil {
		t.Fatal(err)
	}
	if err := wb.SetColumnWidth("Sheet1", 3, 18); err != nil {
		t.Fatal(err)
	}
	if applied, err := wb.Undo(); err != nil || !applied {
		t.Fatalf("undo column-width change = %v, %v; want true, nil", applied, err)
	}
	if got, err := wb.file.GetColWidth("Sheet1", "C"); err != nil || got != originalColumnWidth {
		t.Fatalf("column width after undo = %v, %v; want original %v characters", got, err, originalColumnWidth)
	}
	if applied, err := wb.Redo(); err != nil || !applied {
		t.Fatalf("redo column-width change = %v, %v; want true, nil", applied, err)
	}
	if got, err := wb.file.GetColWidth("Sheet1", "C"); err != nil || got != 18 {
		t.Fatalf("column width after redo = %v, %v; want 18 characters", got, err)
	}
}

func TestWorkbookNoOpDimensionDoesNotCreateUndo(t *testing.T) {
	f := excelize.NewFile()
	path := saveTestWorkbook(t, f)
	wb, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()

	rowHeight, err := wb.file.GetRowHeight("Sheet1", 2)
	if err != nil {
		t.Fatal(err)
	}
	columnWidth, err := wb.file.GetColWidth("Sheet1", "C")
	if err != nil {
		t.Fatal(err)
	}
	if err := wb.SetRowHeight("Sheet1", 2, rowHeight); err != nil {
		t.Fatal(err)
	}
	if err := wb.SetColumnWidth("Sheet1", 3, columnWidth); err != nil {
		t.Fatal(err)
	}
	if applied, err := wb.Undo(); err != nil || applied {
		t.Fatalf("undo after no-op dimensions = %v, %v; want false, nil", applied, err)
	}
	if wb.Dirty() {
		t.Fatal("no-op dimension changes should not dirty the workbook")
	}
}

func TestWorkbookSetDimensionSizesIsOneUndoRedoStep(t *testing.T) {
	f := excelize.NewFile()
	if err := f.SetRowHeight("Sheet1", 2, 21); err != nil {
		t.Fatal(err)
	}
	if err := f.SetRowHeight("Sheet1", 3, 22); err != nil {
		t.Fatal(err)
	}
	if err := f.SetColWidth("Sheet1", "B", "B", 11); err != nil {
		t.Fatal(err)
	}
	if err := f.SetColWidth("Sheet1", "C", "C", 12); err != nil {
		t.Fatal(err)
	}
	path := saveTestWorkbook(t, f)
	wb, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()
	batch, ok := any(wb).(interface {
		SetDimensionSizes(string, document.SpreadsheetDimensionAxis, []document.SpreadsheetDimensionSize) error
	})
	if !ok {
		t.Fatal("Workbook does not implement batch dimension editing")
	}

	if err := batch.SetDimensionSizes("Sheet1", document.SpreadsheetDimensionRow, []document.SpreadsheetDimensionSize{{Index: 2, Value: 31}, {Index: 3, Value: 32}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := wb.file.GetRowHeight("Sheet1", 2); got != 31 {
		t.Errorf("row 2 height = %v; want 31", got)
	}
	if got, _ := wb.file.GetRowHeight("Sheet1", 3); got != 32 {
		t.Errorf("row 3 height = %v; want 32", got)
	}
	if applied, err := wb.Undo(); err != nil || !applied {
		t.Fatalf("undo row batch = %v, %v; want true, nil", applied, err)
	}
	if got, _ := wb.file.GetRowHeight("Sheet1", 2); got != 21 {
		t.Errorf("row 2 height after undo = %v; want 21", got)
	}
	if got, _ := wb.file.GetRowHeight("Sheet1", 3); got != 22 {
		t.Errorf("row 3 height after undo = %v; want 22", got)
	}
	if applied, err := wb.Undo(); err != nil || applied {
		t.Fatalf("second undo after row batch = %v, %v; want false, nil", applied, err)
	}
	if applied, err := wb.Redo(); err != nil || !applied {
		t.Fatalf("redo row batch = %v, %v; want true, nil", applied, err)
	}
	if got, _ := wb.file.GetRowHeight("Sheet1", 2); got != 31 {
		t.Errorf("row 2 height after redo = %v; want 31", got)
	}
	if got, _ := wb.file.GetRowHeight("Sheet1", 3); got != 32 {
		t.Errorf("row 3 height after redo = %v; want 32", got)
	}

	if err := batch.SetDimensionSizes("Sheet1", document.SpreadsheetDimensionColumn, []document.SpreadsheetDimensionSize{{Index: 2, Value: 17}, {Index: 3, Value: 18}}); err != nil {
		t.Fatal(err)
	}
	if applied, err := wb.Undo(); err != nil || !applied {
		t.Fatalf("undo column batch = %v, %v; want true, nil", applied, err)
	}
	if got, _ := wb.file.GetColWidth("Sheet1", "B"); got != 11 {
		t.Errorf("column B width after undo = %v; want 11", got)
	}
	if got, _ := wb.file.GetColWidth("Sheet1", "C"); got != 12 {
		t.Errorf("column C width after undo = %v; want 12", got)
	}
	if applied, err := wb.Redo(); err != nil || !applied {
		t.Fatalf("redo column batch = %v, %v; want true, nil", applied, err)
	}
	if got, _ := wb.file.GetColWidth("Sheet1", "B"); got != 17 {
		t.Errorf("column B width after redo = %v; want 17", got)
	}
	if got, _ := wb.file.GetColWidth("Sheet1", "C"); got != 18 {
		t.Errorf("column C width after redo = %v; want 18", got)
	}
}

func TestWorkbookSetDimensionSizesRejectsInvalidBatchesAtomically(t *testing.T) {
	f := excelize.NewFile()
	if err := f.SetRowHeight("Sheet1", 3, 23); err != nil {
		t.Fatal(err)
	}
	path := saveTestWorkbook(t, f)
	wb, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()
	batch, ok := any(wb).(interface {
		SetDimensionSizes(string, document.SpreadsheetDimensionAxis, []document.SpreadsheetDimensionSize) error
	})
	if !ok {
		t.Fatal("Workbook does not implement batch dimension editing")
	}
	invalidBatches := []struct {
		name  string
		sizes []document.SpreadsheetDimensionSize
	}{
		{name: "invalid index", sizes: []document.SpreadsheetDimensionSize{{Index: 3, Value: 41}, {Index: 0, Value: 42}}},
		{name: "duplicate index", sizes: []document.SpreadsheetDimensionSize{{Index: 3, Value: 41}, {Index: 3, Value: 42}}},
		{name: "invalid size", sizes: []document.SpreadsheetDimensionSize{{Index: 3, Value: 0}}},
	}
	for _, test := range invalidBatches {
		t.Run(test.name, func(t *testing.T) {
			if err := batch.SetDimensionSizes("Sheet1", document.SpreadsheetDimensionRow, test.sizes); err == nil {
				t.Fatal("invalid dimension batch unexpectedly succeeded")
			}
			if got, _ := wb.file.GetRowHeight("Sheet1", 3); got != 23 {
				t.Fatalf("row 3 changed after invalid batch: got %v, want 23", got)
			}
			if applied, err := wb.Undo(); err != nil || applied {
				t.Fatalf("undo after invalid batch = %v, %v; want false, nil", applied, err)
			}
		})
	}
}

func TestWorkbookSetDimensionSizesNoOpBatchDoesNotCreateUndo(t *testing.T) {
	f := excelize.NewFile()
	if err := f.SetRowHeight("Sheet1", 3, 23); err != nil {
		t.Fatal(err)
	}
	if err := f.SetColWidth("Sheet1", "C", "C", 12); err != nil {
		t.Fatal(err)
	}
	path := saveTestWorkbook(t, f)
	wb, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()
	batch, ok := any(wb).(interface {
		SetDimensionSizes(string, document.SpreadsheetDimensionAxis, []document.SpreadsheetDimensionSize) error
	})
	if !ok {
		t.Fatal("Workbook does not implement batch dimension editing")
	}
	if err := batch.SetDimensionSizes("Sheet1", document.SpreadsheetDimensionRow, []document.SpreadsheetDimensionSize{{Index: 3, Value: 23}}); err != nil {
		t.Fatal(err)
	}
	if err := batch.SetDimensionSizes("Sheet1", document.SpreadsheetDimensionColumn, []document.SpreadsheetDimensionSize{{Index: 3, Value: 12}}); err != nil {
		t.Fatal(err)
	}
	if applied, err := wb.Undo(); err != nil || applied {
		t.Fatalf("undo after no-op batch = %v, %v; want false, nil", applied, err)
	}
	if wb.Dirty() {
		t.Fatal("no-op dimension batch should not dirty the workbook")
	}
}

func TestWorkbookCellEditRowHeightStaysInOneUndoStep(t *testing.T) {
	f := excelize.NewFile()
	path := saveTestWorkbook(t, f)
	wb, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()

	if err := wb.SetCellText("Sheet1", "A1", "wrapped text"); err != nil {
		t.Fatal(err)
	}
	rowHeightEdit, ok := any(wb).(interface {
		SetRowHeightAsPartOfLatestEdit(sheet string, row int, heightPoints float64) error
	})
	if !ok {
		t.Fatal("workbook cannot group automatic row height with the cell edit")
	}
	if err := rowHeightEdit.SetRowHeightAsPartOfLatestEdit("Sheet1", 1, 31); err != nil {
		t.Fatal(err)
	}
	if applied, err := wb.Undo(); err != nil || !applied {
		t.Fatalf("undo cell edit and its row height = %v, %v; want true, nil", applied, err)
	}
	if got, err := wb.file.GetCellValue("Sheet1", "A1"); err != nil || got != "" {
		t.Fatalf("cell value after undo = %q, %v; want empty", got, err)
	}
	if got, err := wb.file.GetRowHeight("Sheet1", 1); err != nil || got != 15 {
		t.Fatalf("row height after undo = %v, %v; want default 15 pt", got, err)
	}
	if applied, err := wb.Redo(); err != nil || !applied {
		t.Fatalf("redo cell edit and its row height = %v, %v; want true, nil", applied, err)
	}
	if got, err := wb.file.GetCellValue("Sheet1", "A1"); err != nil || got != "wrapped text" {
		t.Fatalf("cell value after redo = %q, %v; want wrapped text", got, err)
	}
	if got, err := wb.file.GetRowHeight("Sheet1", 1); err != nil || got != 31 {
		t.Fatalf("row height after redo = %v, %v; want 31 pt", got, err)
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
	if !reflect.DeepEqual(after.Cells[3][1], untouched) {
		t.Errorf("untouched B4 changed: before=%#v after=%#v", untouched, after.Cells[3][1])
	}
	if !reflect.DeepEqual(after.Merges, window.Merges) {
		t.Errorf("sample merges changed after edit/resize: before=%#v after=%#v", window.Merges, after.Merges)
	}
	if got, err := os.ReadFile(path); err != nil || !reflect.DeepEqual(got, original) {
		t.Errorf("source sample changed: readErr=%v", err)
	}
}
