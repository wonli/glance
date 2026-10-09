package xlsx

import (
	excelize "github.com/xuri/excelize/v2"
	"glance/internal/document"
	"path/filepath"
	"testing"
)

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
