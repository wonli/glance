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
	family := "Glance Synthetic Font"
	if err := formatter.SetCellFormats("Sheet1", document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}, document.CellFormatPatch{Bold: &bold, FontColor: &fontColor, FontFamily: &family}); err != nil {
		t.Fatal(err)
	}
	window, err := wb.Window("Sheet1", 1, 1, 1, 5)
	if err != nil {
		t.Fatal(err)
	}
	got := window.Cells[0][0].Style
	if got.FontFamily != family || got.Bold || !got.Italic || !got.Underline || got.FontColor != fontColor || got.FillColor != "#FFFF00" || got.HorizontalAlignment != "center" || got.NumberFormat != "0.00" || got.Borders[3].Style != 1 {
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
	if got.FontFamily != family || got.Bold || !got.Italic || !got.Underline || got.FontColor != fontColor || got.FillColor != "#FFFF00" || got.HorizontalAlignment != "center" || got.NumberFormat != "0.00" || got.Borders[3].Style != 1 {
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

func TestSetCellFormatsAppliesInsideBordersToSelectedRange(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()
	inside := "inside"
	bounds := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 2, EndColumn: 2}
	if err := wb.SetCellFormats("Sheet1", bounds, document.CellFormatPatch{BorderPreset: &inside}); err != nil {
		t.Fatalf("apply inside borders: %v", err)
	}
	window, err := wb.Window("Sheet1", 1, 1, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := [2][2][4]int{
		{{0, 1, 1, 0}, {0, 0, 1, 0}},
		{{0, 1, 0, 0}, {0, 0, 0, 0}},
	}
	for row := range want {
		for column := range want[row] {
			for edge, style := range window.Cells[row][column].Style.Borders {
				if style.Style != want[row][column][edge] {
					t.Fatalf("cell (%d,%d) edge %d style=%d; want %d", row+1, column+1, edge, style.Style, want[row][column][edge])
				}
			}
		}
	}
	for _, item := range []struct {
		cell   string
		preset string
		edge   int
	}{
		{cell: "A4", preset: "top", edge: 0},
		{cell: "B4", preset: "right", edge: 1},
		{cell: "C4", preset: "bottom", edge: 2},
		{cell: "D4", preset: "left", edge: 3},
	} {
		preset := item.preset
		if err := wb.SetCellFormats("Sheet1", singleCellRange(item.cell), document.CellFormatPatch{BorderPreset: &preset}); err != nil {
			t.Fatalf("apply %s border to %s: %v", preset, item.cell, err)
		}
	}
	path := filepath.Join(t.TempDir(), "inside-borders.xlsx")
	if err := wb.SaveAs(path); err != nil {
		t.Fatalf("save border styles: %v", err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen border styles: %v", err)
	}
	defer reopened.Close()
	window, err = reopened.Window("Sheet1", 1, 1, 4, 4)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		row, column int
		edge        int
	}{
		{row: 3, column: 0, edge: 0},
		{row: 3, column: 1, edge: 1},
		{row: 3, column: 2, edge: 2},
		{row: 3, column: 3, edge: 3},
	} {
		if got := window.Cells[item.row][item.column].Style.Borders[item.edge].Style; got != 1 {
			t.Fatalf("reopened border at row %d column %d edge %d = %d; want 1", item.row+1, item.column+1, item.edge, got)
		}
	}
}

func TestSetCellFormatsRejectsInsideBordersOnWholeRowsOrColumns(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()
	wb.SetDirty(false)
	inside := "inside"
	for _, bounds := range []document.CellRange{
		{StartRow: 2, StartColumn: 1, EndRow: 3, EndColumn: maxXLSXColumns},
		{StartRow: 1, StartColumn: 2, EndRow: maxXLSXRows, EndColumn: 3},
	} {
		if err := wb.SetCellFormats("Sheet1", bounds, document.CellFormatPatch{BorderPreset: &inside}); err == nil {
			t.Fatalf("inside borders unexpectedly accepted for unbounded range %+v", bounds)
		}
	}
	if wb.Dirty() {
		t.Fatal("rejected whole-row/column border changes dirtied workbook")
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

func TestSetCellFormatsRoundTripsExcelKeyboardShortcutFormats(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()
	formats := []string{"#,##0.00", "h:mm AM/PM", "m/d/yy", `"¥"#,##0.00`, "0%", "0.00E+00", "General"}
	for index, format := range formats {
		cell := CellName(index+1, 1)
		if err := wb.SetCellText("Sheet1", cell, "1234.5"); err != nil {
			t.Fatalf("set %s value: %v", cell, err)
		}
		if format == "General" {
			initial := "0.00"
			if err := wb.SetCellFormats("Sheet1", singleCellRange(cell), document.CellFormatPatch{NumberFormat: &initial}); err != nil {
				t.Fatalf("set initial format for %s: %v", cell, err)
			}
		}
		if err := wb.SetCellFormats("Sheet1", singleCellRange(cell), document.CellFormatPatch{NumberFormat: &format}); err != nil {
			t.Fatalf("set %s format %q: %v", cell, format, err)
		}
	}

	path := filepath.Join(t.TempDir(), "excel-shortcut-number-formats.xlsx")
	if err := wb.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	window, err := reopened.Window("Sheet1", 1, 1, 1, len(formats))
	if err != nil {
		t.Fatal(err)
	}
	for index, want := range formats {
		if got := window.Cells[0][index].Style.NumberFormat; got != want {
			t.Errorf("%s number format after save/reopen = %q; want %q", CellName(index+1, 1), got, want)
		}
	}
}

func TestExcelHomeFormatPatchesRoundTrip(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	thousandths := "#,##0.00"
	if err := wb.SetCellFormats("Sheet1", singleCellRange("A1"), document.CellFormatPatch{NumberFormat: &thousandths}); err != nil {
		t.Fatal(err)
	}
	increaseDecimal := 1
	if err := wb.SetCellFormats("Sheet1", singleCellRange("A1"), document.CellFormatPatch{DecimalPlacesDelta: &increaseDecimal}); err != nil {
		t.Fatal(err)
	}

	initialSize := 13.0
	fontFamily := "Arial"
	fontColor := "#2196F3"
	fillColor := "#DDEBF7"
	bold := true
	if err := wb.SetCellFormats("Sheet1", singleCellRange("B1"), document.CellFormatPatch{
		FontSize: &initialSize, FontFamily: &fontFamily, FontColor: &fontColor, FillColor: &fillColor, Bold: &bold,
	}); err != nil {
		t.Fatal(err)
	}
	decreaseFontSize := -1
	if err := wb.SetCellFormats("Sheet1", singleCellRange("B1"), document.CellFormatPatch{FontSizeStep: &decreaseFontSize}); err != nil {
		t.Fatal(err)
	}
	increaseDefaultFontSize := 1
	if err := wb.SetCellFormats("Sheet1", singleCellRange("C1"), document.CellFormatPatch{FontSizeStep: &increaseDefaultFontSize}); err != nil {
		t.Fatal(err)
	}
	bottomBorder := "bottom"
	if err := wb.SetCellFormats("Sheet1", singleCellRange("D1"), document.CellFormatPatch{BorderPreset: &bottomBorder}); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "excel-home-format-actions.xlsx")
	if err := wb.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	window, err := reopened.Window("Sheet1", 1, 1, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	if got := window.Cells[0][0].Style.NumberFormat; got != "#,##0.000" {
		t.Errorf("A1 decimal-step format after save/reopen = %q; want %q", got, "#,##0.000")
	}
	if style := window.Cells[0][1].Style; style.FontSize != 12 || style.FontFamily != fontFamily || style.FontColor != fontColor || style.FillColor != fillColor || !style.Bold {
		t.Errorf("B1 Home font changes after save/reopen = %+v", style)
	}
	if got := window.Cells[0][2].Style.FontSize; got != 12 {
		t.Errorf("C1 stepped workbook-default font size after save/reopen = %v; want 12", got)
	}
	if style := window.Cells[0][3].Style; style.Borders[2].Style != 1 || style.Borders[2].Color != "#000000" {
		t.Errorf("D1 bottom-border picker choice after save/reopen = %+v; want a black bottom border", style.Borders[2])
	}
}

func TestSetCellFormatsVerticalAlignmentRoundTrips(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()
	wrap := true
	horizontal := "center"
	if _, err := wb.SetCellFormatsChanged("Sheet1", document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}, document.CellFormatPatch{
		WrapText: &wrap, HorizontalAlignment: &horizontal,
	}); err != nil {
		t.Fatal(err)
	}
	vertical := "center"
	changed, err := wb.SetCellFormatsChanged("Sheet1", document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}, document.CellFormatPatch{VerticalAlignment: &vertical})
	if err != nil || !changed {
		t.Fatalf("set vertical alignment changed=%t, err=%v; want true, nil", changed, err)
	}
	window, err := wb.Window("Sheet1", 1, 1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if style := window.Cells[0][0].Style; style.VerticalAlignment != vertical || style.HorizontalAlignment != horizontal || !style.WrapText {
		t.Fatalf("style after vertical alignment = %+v; want vertical=%q, horizontal=%q, wrap=true", style, vertical, horizontal)
	}
	if changed, err := wb.SetCellFormatsChanged("Sheet1", document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}, document.CellFormatPatch{VerticalAlignment: &vertical}); err != nil || changed {
		t.Fatalf("repeat vertical alignment changed=%t, err=%v; want false, nil", changed, err)
	}
	invalid := "middle"
	if changed, err := wb.SetCellFormatsChanged("Sheet1", document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}, document.CellFormatPatch{VerticalAlignment: &invalid}); err == nil || changed {
		t.Fatalf("invalid vertical alignment changed=%t, err=%v; want false and an error", changed, err)
	}
	if applied, err := wb.Undo(); err != nil || !applied {
		t.Fatalf("undo vertical alignment = %t, %v; want true, nil", applied, err)
	}
	window, err = wb.Window("Sheet1", 1, 1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if style := window.Cells[0][0].Style; style.VerticalAlignment != "" || style.HorizontalAlignment != horizontal || !style.WrapText {
		t.Fatalf("style after undo = %+v; want vertical default and existing horizontal/wrap", style)
	}
	if applied, err := wb.Redo(); err != nil || !applied {
		t.Fatalf("redo vertical alignment = %t, %v; want true, nil", applied, err)
	}

	path := filepath.Join(t.TempDir(), "vertical-alignment.xlsx")
	if err := wb.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	window, err = reopened.Window("Sheet1", 1, 1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if style := window.Cells[0][0].Style; style.VerticalAlignment != vertical || style.HorizontalAlignment != horizontal || !style.WrapText {
		t.Fatalf("reopened style = %+v; want vertical=%q, horizontal=%q, wrap=true", style, vertical, horizontal)
	}
}
