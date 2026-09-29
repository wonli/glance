package xlsx

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"glance/internal/document"

	"github.com/xuri/excelize/v2"
)

func TestSnapshotRestoresEditedWorkbook(t *testing.T) {
	book := NewBlank()
	defer book.Close()
	if err := book.SetCellText("Sheet1", "A1", "draft value"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := book.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := OpenSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	value, err := restored.Value("Sheet1", "A1")
	if err != nil || value != "draft value" {
		t.Fatalf("restored A1 = %q, %v", value, err)
	}
}

func TestOpenSnapshotRejectsMalformedPayload(t *testing.T) {
	if _, err := OpenSnapshot(bytes.Repeat([]byte("x"), 32)); err == nil {
		t.Fatal("expected malformed snapshot error")
	}
}

func TestWorkbookEditSaveRoundTrip(t *testing.T) {
	f := excelize.NewFile()
	if err := f.SetCellValue("Sheet1", "A1", "原始内容"); err != nil {
		t.Fatal(err)
	}
	style, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellStyle("Sheet1", "A1", "A1", style); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "roundtrip.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	wb, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()
	if err := wb.SetCellText("Sheet1", "A1", "修改后"); err != nil {
		t.Fatal(err)
	}
	if err := wb.Save(); err != nil {
		t.Fatal(err)
	}

	reopened, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.GetCellValue("Sheet1", "A1")
	if err != nil || got != "修改后" {
		t.Fatalf("saved value = %q, err = %v", got, err)
	}
	gotStyle, err := reopened.GetCellStyle("Sheet1", "A1")
	if err != nil || gotStyle != style {
		t.Fatalf("saved style = %d, want %d, err = %v", gotStyle, style, err)
	}
}

func TestWorkbookRejectsFormulaLikeText(t *testing.T) {
	f := excelize.NewFile()
	path := filepath.Join(t.TempDir(), "formula.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	wb, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()
	if err := wb.SetCellText("Sheet1", "A1", "=1+1"); err == nil {
		t.Fatal("expected formula-like input to be rejected")
	}
}

func TestWorkbookParsesBasicConstantsAndIgnoresNoOp(t *testing.T) {
	f := excelize.NewFile()
	path := filepath.Join(t.TempDir(), "constants.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	wb, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()
	if err := wb.SetCellText("Sheet1", "A1", ""); err != nil {
		t.Fatal(err)
	}
	if wb.Dirty() {
		t.Fatal("setting an empty cell to empty should be a no-op")
	}
	for _, item := range []struct{ cell, value, want string }{
		{"A1", "42.5", "42.5"},
		{"A2", "TRUE", "TRUE"},
		{"A3", "普通文本", "普通文本"},
	} {
		if err := wb.SetCellText("Sheet1", item.cell, item.value); err != nil {
			t.Fatal(err)
		}
		got, err := wb.Value("Sheet1", item.cell)
		if err != nil || got != item.want {
			t.Fatalf("%s value = %q, want %q, err = %v", item.cell, got, item.want, err)
		}
	}
	if err := wb.SetCellText("Sheet1", "A4", "Inf"); err == nil {
		t.Fatal("expected non-finite numeric input to be rejected")
	}
}

func TestSetCellFormatsPreservesUntouchedPropertiesAndRoundTrips(t *testing.T) {
	f := excelize.NewFile()
	if err := f.SetCellValue("Sheet1", "A1", 12.5); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue("Sheet1", "B1", "untouched"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellFormula("Sheet1", "C1", "SUM(1,2)"); err != nil {
		t.Fatal(err)
	}
	if err := f.MergeCell("Sheet1", "D1", "E1"); err != nil {
		t.Fatal(err)
	}
	format := "0.00"
	styleID, err := f.NewStyle(&excelize.Style{
		Font:         &excelize.Font{Bold: true, Italic: true, Underline: "single", Color: "FF112233"},
		Fill:         excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"FFFF00"}},
		Alignment:    &excelize.Alignment{Horizontal: "center"},
		CustomNumFmt: &format,
		Border:       []excelize.Border{{Type: "left", Color: "000000", Style: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellStyle("Sheet1", "A1", "A1", styleID); err != nil {
		t.Fatal(err)
	}
	if err := f.SetRowHeight("Sheet1", 1, 27); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "format-roundtrip.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	wb, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()
	formatter, ok := any(wb).(document.SpreadsheetFormatter)
	if !ok {
		t.Fatal("XLSX workbook does not expose SpreadsheetFormatter")
	}
	bold := false
	fontColor := "#FF0000"
	if err := formatter.SetCellFormats("Sheet1", document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}, document.CellFormatPatch{Bold: &bold, FontColor: &fontColor}); err != nil {
		t.Fatal(err)
	}
	window, err := wb.Window("Sheet1", 1, 1, 1, 5)
	if err != nil {
		t.Fatal(err)
	}
	got := window.Cells[0][0].Style
	if got.Bold || !got.Italic || !got.Underline || got.FontColor != fontColor || got.FillColor != "#FFFF00" || got.HorizontalAlignment != "center" || got.NumberFormat != "0.00" || got.Borders[3].Style != 1 {
		t.Fatalf("format patch lost or misapplied properties: %+v", got)
	}
	if err := wb.Save(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	window, err = reopened.Window("Sheet1", 1, 1, 1, 5)
	if err != nil {
		t.Fatal(err)
	}
	got = window.Cells[0][0].Style
	if got.Bold || !got.Italic || !got.Underline || got.FontColor != fontColor || got.FillColor != "#FFFF00" || got.HorizontalAlignment != "center" || got.NumberFormat != "0.00" || got.Borders[3].Style != 1 {
		t.Fatalf("reopened format=%+v", got)
	}
	if value, err := reopened.Value("Sheet1", "B1"); err != nil || value != "untouched" {
		t.Fatalf("untouched B1=%q, err=%v", value, err)
	}
	if formula, err := reopened.file.GetCellFormula("Sheet1", "C1"); err != nil || formula != "SUM(1,2)" {
		t.Fatalf("untouched formula=%q, err=%v", formula, err)
	}
	if height, err := reopened.file.GetRowHeight("Sheet1", 1); err != nil || height != 27 {
		t.Fatalf("untouched row height=%v, err=%v", height, err)
	}
	if merges, err := reopened.file.GetMergeCells("Sheet1"); err != nil || len(merges) != 1 {
		t.Fatalf("untouched merges=%v, err=%v", merges, err)
	}
}

func TestSetCellFormatsNoOpAndInvalidRangeStayClean(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()
	wb.SetDirty(false)
	formatter, ok := any(wb).(document.SpreadsheetFormatter)
	if !ok {
		t.Fatal("XLSX workbook does not expose SpreadsheetFormatter")
	}
	bold := false
	if err := formatter.SetCellFormats("Sheet1", document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}, document.CellFormatPatch{Bold: &bold}); err != nil {
		t.Fatal(err)
	}
	if wb.Dirty() {
		t.Fatal("applying an existing default format should not dirty the workbook")
	}
	if err := formatter.SetCellFormats("Sheet1", document.CellRange{StartRow: 2, StartColumn: 1, EndRow: 1, EndColumn: 1}, document.CellFormatPatch{Bold: &bold}); err == nil {
		t.Fatal("expected invalid range to fail")
	}
	if err := formatter.SetCellFormats("Sheet1", document.CellRange{StartRow: 1, StartColumn: 1, EndRow: maxClipboardCells + 1, EndColumn: 1}, document.CellFormatPatch{Bold: &bold}); err == nil {
		t.Fatal("expected oversized range to fail before scanning cells")
	}
	if wb.Dirty() {
		t.Fatal("invalid range should not dirty the workbook")
	}
}

func TestSetCellFormatsAppliesAllSupportedPropertiesAndClears(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()
	wb.SetDirty(false)
	formatter := document.SpreadsheetFormatter(wb)
	trueValue := true
	fill := "#AABBCC"
	center := "center"
	date := "yyyy-mm-dd"
	allBorders := "all"
	for _, item := range []struct {
		cell  string
		patch document.CellFormatPatch
	}{
		{"A1", document.CellFormatPatch{Italic: &trueValue}},
		{"B1", document.CellFormatPatch{Underline: &trueValue}},
		{"C1", document.CellFormatPatch{FillColor: &fill}},
		{"D1", document.CellFormatPatch{HorizontalAlignment: &center}},
		{"E1", document.CellFormatPatch{NumberFormat: &date}},
		{"F1", document.CellFormatPatch{BorderPreset: &allBorders}},
	} {
		if err := formatter.SetCellFormats("Sheet1", singleCellRange(item.cell), item.patch); err != nil {
			t.Fatalf("format %s: %v", item.cell, err)
		}
	}
	window, err := wb.Window("Sheet1", 1, 1, 1, 6)
	if err != nil {
		t.Fatal(err)
	}
	if !window.Cells[0][0].Style.Italic || !window.Cells[0][1].Style.Underline || window.Cells[0][2].Style.FillColor != fill || window.Cells[0][3].Style.HorizontalAlignment != center || window.Cells[0][4].Style.NumberFormat != date {
		t.Fatalf("supported formats were not applied: %+v", window.Cells[0])
	}
	for index, border := range window.Cells[0][5].Style.Borders {
		if border.Style != 1 {
			t.Fatalf("border %d was not set: %+v", index, window.Cells[0][5].Style.Borders)
		}
	}
	if err := formatter.SetCellFormats("Sheet1", singleCellRange("F1"), document.CellFormatPatch{ClearFormat: true}); err != nil {
		t.Fatal(err)
	}
	window, err = wb.Window("Sheet1", 1, 6, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if style := window.Cells[0][0].Style; style.Bold || style.Italic || style.Underline || style.FillColor != "" || style.FontColor != "" || style.Borders != ([4]document.CellBorder{}) || style.NumberFormat != "General" {
		t.Fatalf("clear format left style properties: %+v", style)
	}
}

func singleCellRange(cell string) document.CellRange {
	column, row, err := excelize.CellNameToCoordinates(cell)
	if err != nil {
		panic(err)
	}
	return document.CellRange{StartRow: row, StartColumn: column, EndRow: row, EndColumn: column}
}

func TestOfficeXlsxSamplesOpenAndSaveWithoutChangingSource(t *testing.T) {
	dir := os.Getenv("GLANCE_OFFICE_TEST_DIR")
	if dir == "" {
		t.Skip("set GLANCE_OFFICE_TEST_DIR to the local office sample directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || filepath.Ext(entry.Name()) != ".xlsx" {
			continue
		}
		count++
		path := filepath.Join(dir, entry.Name())
		original, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		wb, err := Open(path)
		if err != nil {
			t.Errorf("open %s: %v", entry.Name(), err)
			continue
		}
		if len(wb.SheetNames()) == 0 {
			t.Errorf("%s contains no visible worksheet names", entry.Name())
			_ = wb.Close()
			continue
		}
		beforeNames := wb.SheetNames()
		beforeRanges := make(map[string][][]document.Cell, len(beforeNames))
		for _, name := range beforeNames {
			preview, err := wb.Range(name, 300, 16)
			if err != nil {
				t.Errorf("read preview of %s/%s: %v", entry.Name(), name, err)
				continue
			}
			beforeRanges[name] = preview
		}
		if entry.Name() == "人事表格-员工绩效表.xlsx" {
			preview := beforeRanges["月绩效"]
			if len(preview) < 10 || preview[0][0].Value == "" || preview[3][0].Value != "考核项" {
				t.Errorf("human-resources sample did not expose expected visible cells: rows=%d A1=%q A4=%q", len(preview), preview[0][0].Value, preview[3][0].Value)
			}
			nonEmpty := 0
			for _, row := range preview {
				for _, cell := range row {
					if cell.Value != "" {
						nonEmpty++
					}
				}
			}
			if nonEmpty < 20 {
				t.Errorf("human-resources sample exposed only %d non-empty cells", nonEmpty)
			}
		}
		copyPath := filepath.Join(t.TempDir(), entry.Name())
		if err := wb.SaveAs(copyPath); err != nil {
			t.Errorf("save copy of %s: %v", entry.Name(), err)
		}
		if err := wb.Close(); err != nil {
			t.Errorf("close %s: %v", entry.Name(), err)
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != string(original) {
			t.Errorf("opening/saving a copy modified source file %s", entry.Name())
		}
		copyBook, err := Open(copyPath)
		if err != nil {
			t.Errorf("reopen copy of %s: %v", entry.Name(), err)
			continue
		}
		if !reflect.DeepEqual(copyBook.SheetNames(), beforeNames) {
			t.Errorf("sheet names changed in copy of %s", entry.Name())
		}
		for name, before := range beforeRanges {
			after, err := copyBook.Range(name, 300, 16)
			if err != nil || !reflect.DeepEqual(after, before) {
				t.Errorf("preview changed after save/reopen for %s/%s (err=%v)", entry.Name(), name, err)
			}
		}
		_ = copyBook.Close()
	}
	if count == 0 {
		t.Fatal("no .xlsx samples found in GLANCE_OFFICE_TEST_DIR")
	}
}

func TestWorkbookWindowIncludesStylesMergesAndDimensions(t *testing.T) {
	f := excelize.NewFile()
	if err := f.SetCellValue("Sheet1", "A1", "styled"); err != nil {
		t.Fatal(err)
	}
	style, err := f.NewStyle(&excelize.Style{
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"FFFF00"}},
		Font:      &excelize.Font{Bold: true, Italic: true, Size: 13, Color: "FF112233", Family: "宋体"},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border:    []excelize.Border{{Type: "left", Color: "010203", Style: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellStyle("Sheet1", "A1", "A1", style); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue("Sheet1", "C2", "merged anchor"); err != nil {
		t.Fatal(err)
	}
	if err := f.MergeCell("Sheet1", "C2", "D3"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetColWidth("Sheet1", "B", "B", 22); err != nil {
		t.Fatal(err)
	}
	if err := f.SetRowHeight("Sheet1", 2, 31); err != nil {
		t.Fatal(err)
	}
	path := saveTestWorkbook(t, f)
	wb, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()
	window, err := wb.Window("Sheet1", 1, 1, 3, 4)
	if err != nil {
		t.Fatal(err)
	}
	if window.StartRow != 1 || window.StartColumn != 1 || len(window.Cells) != 3 || len(window.Cells[0]) != 4 {
		t.Fatalf("window shape/origin = (%d,%d) %dx%d; want (1,1) 3x4", window.StartRow, window.StartColumn, len(window.Cells), len(window.Cells[0]))
	}
	cell := window.Cells[0][0]
	if cell.Value != "styled" || cell.StyleID != style {
		t.Fatalf("styled cell = %#v; want value %q and style %d", cell, "styled", style)
	}
	if got := cell.Style; got.FillColor != "#FFFF00" || got.FontColor != "#112233" || got.FontFamily != "宋体" || !got.Bold || !got.Italic || got.FontSize != 13 || got.HorizontalAlignment != "center" || got.VerticalAlignment != "center" || !got.WrapText {
		t.Fatalf("neutral style = %#v; missing translated style attributes", got)
	}
	if got := cell.Style.Borders[3]; got.Color != "#010203" || got.Style != 1 {
		t.Fatalf("left border = %#v; want color #010203 and style 1", got)
	}
	if len(window.Merges) != 1 || window.Merges[0].Range != (document.CellRange{StartRow: 2, StartColumn: 3, EndRow: 3, EndColumn: 4}) || window.Merges[0].Anchor.Value != "merged anchor" {
		t.Fatalf("merges = %#v; want C2:D3 with its anchor value", window.Merges)
	}
	if len(window.RowHeightsPoints) != 3 || window.RowHeightsPoints[1] != 31 {
		t.Fatalf("row heights = %#v; want row 2 to be 31pt", window.RowHeightsPoints)
	}
	if len(window.ColumnWidthsChars) != 4 || window.ColumnWidthsChars[1] != 22 {
		t.Fatalf("column widths = %#v; want column B to be 22 chars", window.ColumnWidthsChars)
	}
}

func TestWorkbookWindowReadsBlankCellsOutsideUsedRange(t *testing.T) {
	f := excelize.NewFile()
	path := saveTestWorkbook(t, f)
	wb, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()
	window, err := wb.Window("Sheet1", 100, 40, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if window.StartRow != 100 || window.StartColumn != 40 || len(window.Cells) != 2 || len(window.Cells[0]) != 3 {
		t.Fatalf("blank window origin/shape = (%d,%d) %dx%d; want (100,40) 2x3", window.StartRow, window.StartColumn, len(window.Cells), len(window.Cells[0]))
	}
	for r, row := range window.Cells {
		for c, cell := range row {
			if cell.Value != "" || cell.StyleID != 0 {
				t.Errorf("blank cell [%d][%d] = %#v; want empty style 0", r, c, cell)
			}
		}
	}
}

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

func TestWorkbookWindowRejectsInvalidBounds(t *testing.T) {
	f := excelize.NewFile()
	path := saveTestWorkbook(t, f)
	wb, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()
	for _, item := range []struct {
		row, column, rows, columns int
	}{{0, 1, 1, 1}, {1, 0, 1, 1}, {1, 1, 0, 1}, {1, 1, 1, 0}, {1_048_576, 1, 2, 1}, {1, 16_384, 1, 2}, {1_048_577, 1, 1, 1}, {1, 16_385, 1, 1}} {
		if _, err := wb.Window("Sheet1", item.row, item.column, item.rows, item.columns); err == nil {
			t.Errorf("Window(%d,%d,%d,%d) should fail", item.row, item.column, item.rows, item.columns)
		}
	}
	for _, cell := range []string{"A1048577", "XFE1"} {
		if err := wb.SetCellText("Sheet1", cell, "out of bounds"); err == nil {
			t.Errorf("SetCellText(%q) should fail outside XLSX limits", cell)
		}
	}
	if err := wb.SetRowHeight("Sheet1", 1, 410); err == nil {
		t.Error("row height above Excel maximum should fail")
	}
	if err := wb.SetColumnWidth("Sheet1", 1, 256); err == nil {
		t.Error("column width above Excel maximum should fail")
	}
}

func TestWorkbookWindowFallsBackForUnsupportedStyleColors(t *testing.T) {
	f := excelize.NewFile()
	if err := f.SetCellValue("Sheet1", "A1", "theme"); err != nil {
		t.Fatal(err)
	}
	themeColor := 6
	style, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{ColorTheme: &themeColor}})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellStyle("Sheet1", "A1", "A1", style); err != nil {
		t.Fatal(err)
	}
	path := saveTestWorkbook(t, f)
	wb, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()
	window, err := wb.Window("Sheet1", 1, 1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := window.Cells[0][0].Style.FontColor; got != "" {
		t.Fatalf("unsupported theme font color = %q; want default empty color", got)
	}
}

func TestWorkbookFailedSaveKeepsDirtyAndOriginal(t *testing.T) {
	f := excelize.NewFile()
	if err := f.SetCellValue("Sheet1", "A1", "original"); err != nil {
		t.Fatal(err)
	}
	path := saveTestWorkbook(t, f)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wb, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()
	if err := wb.SetCellText("Sheet1", "A1", "edited"); err != nil {
		t.Fatal(err)
	}
	backup := path + ".backup"
	if err := os.Rename(path, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		_ = os.Rename(backup, path)
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(path)
		_ = os.Rename(backup, path)
	}()
	if err := wb.Save(); err == nil {
		t.Fatal("Save should fail when the destination path is a directory")
	}
	if !wb.Dirty() {
		t.Fatal("failed save should retain dirty state")
	}
	after, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, original) {
		t.Fatal("failed save changed original workbook bytes")
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

func saveTestWorkbook(t *testing.T, file *excelize.File) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "window.xlsx")
	if err := file.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

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
