package xlsx

import (
	excelize "github.com/xuri/excelize/v2"
	"glance/internal/document"
	"path/filepath"
	"testing"
)

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

func TestSetCellFormatsWrapTextAlignmentAndPercent(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()
	wb.SetDirty(false)
	formatter := document.SpreadsheetFormatter(wb)

	trueVal := true
	left := "left"
	right := "right"
	pct := "0%"
	pctDec := "0.00%"

	tests := []struct {
		cell  string
		patch document.CellFormatPatch
	}{
		{"A1", document.CellFormatPatch{WrapText: &trueVal}},
		{"B1", document.CellFormatPatch{HorizontalAlignment: &left}},
		{"C1", document.CellFormatPatch{HorizontalAlignment: &right}},
		{"D1", document.CellFormatPatch{NumberFormat: &pct}},
		{"E1", document.CellFormatPatch{NumberFormat: &pctDec}},
	}
	for _, item := range tests {
		if err := formatter.SetCellFormats("Sheet1", singleCellRange(item.cell), item.patch); err != nil {
			t.Fatalf("format %s: %v", item.cell, err)
		}
	}

	window, err := wb.Window("Sheet1", 1, 1, 1, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !window.Cells[0][0].Style.WrapText {
		t.Fatalf("A1 WrapText want true, got %v", window.Cells[0][0].Style.WrapText)
	}
	if window.Cells[0][1].Style.HorizontalAlignment != "left" {
		t.Fatalf("B1 HorizontalAlignment want left, got %v", window.Cells[0][1].Style.HorizontalAlignment)
	}
	if window.Cells[0][2].Style.HorizontalAlignment != "right" {
		t.Fatalf("C1 HorizontalAlignment want right, got %v", window.Cells[0][2].Style.HorizontalAlignment)
	}
	if window.Cells[0][3].Style.NumberFormat != "0%" {
		t.Fatalf("D1 NumberFormat want 0%%, got %v", window.Cells[0][3].Style.NumberFormat)
	}
	if window.Cells[0][4].Style.NumberFormat != "0.00%" {
		t.Fatalf("E1 NumberFormat want 0.00%%, got %v", window.Cells[0][4].Style.NumberFormat)
	}

	if err := formatter.SetCellFormats("Sheet1", singleCellRange("A1"), document.CellFormatPatch{ClearFormat: true}); err != nil {
		t.Fatal(err)
	}
	window, err = wb.Window("Sheet1", 1, 1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if window.Cells[0][0].Style.WrapText {
		t.Fatalf("A1 WrapText after clear want false, got %v", window.Cells[0][0].Style.WrapText)
	}
}

func TestSetCellFormatsFontSizeAndFamily(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()
	formatter, ok := any(wb).(document.SpreadsheetFormatter)
	if !ok {
		t.Fatal("workbook does not implement SpreadsheetFormatter")
	}

	size := 16.0
	family := "Arial"
	if err := formatter.SetCellFormats("Sheet1", singleCellRange("A1"), document.CellFormatPatch{FontSize: &size, FontFamily: &family}); err != nil {
		t.Fatalf("SetCellFormats with size and family: %v", err)
	}

	window, err := wb.Window("Sheet1", 1, 1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if window.Cells[0][0].Style.FontSize != 16.0 {
		t.Fatalf("A1 FontSize want 16.0, got %v", window.Cells[0][0].Style.FontSize)
	}
	if window.Cells[0][0].Style.FontFamily != "Arial" {
		t.Fatalf("A1 FontFamily want Arial, got %q", window.Cells[0][0].Style.FontFamily)
	}

	badSize := -1.0
	if err := formatter.SetCellFormats("Sheet1", singleCellRange("A1"), document.CellFormatPatch{FontSize: &badSize}); err == nil {
		t.Fatal("expected error for negative font size")
	}
	emptyFamily := "   "
	if err := formatter.SetCellFormats("Sheet1", singleCellRange("A1"), document.CellFormatPatch{FontFamily: &emptyFamily}); err == nil {
		t.Fatal("expected error for empty font family")
	}
}
