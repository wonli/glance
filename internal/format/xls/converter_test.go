package xls

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
	"glance/internal/fileutil"
	"glance/internal/format/xlsx"
)

type stubImportReader struct {
	sheets   []string
	rows     map[string][]ImportedRow
	features []UnsupportedFeature
	style    excelize.Style
	merges   map[string][][2]string
	hidden   map[string][2]bool
	widths   map[string][]ImportedColumnWidth
}

func (r *stubImportReader) SheetNames() []string { return append([]string(nil), r.sheets...) }
func (r *stubImportReader) ColumnWidths(sheet string) []ImportedColumnWidth {
	return append([]ImportedColumnWidth(nil), r.widths[sheet]...)
}
func (r *stubImportReader) VisitRows(sheet string, visit func(ImportedRow) error) error {
	rows, ok := r.rows[sheet]
	if !ok {
		return fmt.Errorf("unknown sheet %q", sheet)
	}
	for _, row := range rows {
		if err := visit(row); err != nil {
			return err
		}
	}
	return nil
}
func (r *stubImportReader) ExcelizeStyle(uint16) (excelize.Style, error) { return r.style, nil }
func (r *stubImportReader) SheetName(index uint16) (string, error) {
	if int(index) >= len(r.sheets) {
		return "", fmt.Errorf("unknown sheet index %d", index)
	}
	return r.sheets[index], nil
}
func (r *stubImportReader) DefinedName(index uint16) (string, error) {
	return "", fmt.Errorf("defined name %d is unavailable", index)
}
func (r *stubImportReader) UnsupportedFeatures() []UnsupportedFeature {
	return append([]UnsupportedFeature(nil), r.features...)
}
func (r *stubImportReader) MergedCells(sheet string) [][2]string {
	return append([][2]string(nil), r.merges[sheet]...)
}
func (r *stubImportReader) SheetVisibility(sheet string) (bool, bool) {
	return r.hidden[sheet][0], r.hidden[sheet][1]
}
func (r *stubImportReader) Comments(string) []ImportedComment { return nil }
func (r *stubImportReader) Close() error                      { return nil }

func TestConvertXLSPreservesSheetsFormulaCachesAndStyles(t *testing.T) {
	converted, err := ConvertToTempXLSX(context.Background(), "testdata/formula-style-multisheet.xls")
	if err != nil {
		t.Fatalf("ConvertToTempXLSX: %v", err)
	}
	defer converted.Cleanup()

	book, err := xlsx.Open(converted.Path)
	if err != nil {
		t.Fatalf("open converted workbook: %v", err)
	}
	defer book.Close()
	if got, want := fmt.Sprint(book.SheetNames()), "[总览 明细]"; got != want {
		t.Fatalf("sheet order = %s, want %s", got, want)
	}
	window, err := book.Window("总览", 1, 1, 4, 6)
	if err != nil {
		t.Fatalf("read converted grid: %v", err)
	}
	checks := []struct {
		cell    string
		value   string
		formula string
	}{
		{"A1", "项目", ""},
		{"A2", "中文文本", ""},
		{"B1", "42.50", ""},
		{"C1", "TRUE", "TRUE()"},
		{"D1", "85.00", "(B1*2)"},
		{"E1", "#N/A", "NA()"},
	}
	for _, check := range checks {
		column, row, err := excelize.CellNameToCoordinates(check.cell)
		if err != nil {
			t.Fatal(err)
		}
		cell := window.Cells[row-1][column-1]
		if cell.Value != check.value || cell.Formula != check.formula {
			t.Errorf("%s = value %q formula %q; want %q / %q", check.cell, cell.Value, cell.Formula, check.value, check.formula)
		}
	}
	styled := window.Cells[0][0]
	if styled.StyleID == 0 || styled.Style.FillColor == "" || styled.Style.NumberFormat == "" {
		t.Errorf("A1 style = %+v (ID %d), want converted fill and number format", styled.Style, styled.StyleID)
	}
	if len(window.RowHeightsPoints) == 0 || window.RowHeightsPoints[0] != 15 {
		t.Errorf("row 1 height = %#v, want the BIFF height of 15 points", window.RowHeightsPoints)
	}
	if len(window.ColumnWidthsChars) < 2 || window.ColumnWidthsChars[0] < 9.44 || window.ColumnWidthsChars[0] > 9.45 || window.ColumnWidthsChars[1] < 9.44 || window.ColumnWidthsChars[1] > 9.45 {
		t.Errorf("column widths = %#v, want BIFF's custom default width across the sheet", window.ColumnWidthsChars)
	}
	blankStyled := window.Cells[3][5]
	if blankStyled.Value != "" || blankStyled.StyleID == 0 || blankStyled.Style.FillColor == "" {
		t.Errorf("styled blank F4 = %+v (ID %d), want empty value with its style", blankStyled.Style, blankStyled.StyleID)
	}
}

func TestConvertXLSPreservesRealBIFFStringFormulaCache(t *testing.T) {
	converted, err := ConvertToTempXLSX(context.Background(), "testdata/formula-string-cache.xls")
	if err != nil {
		t.Fatalf("ConvertToTempXLSX: %v", err)
	}
	defer converted.Cleanup()

	book, err := xlsx.Open(converted.Path)
	if err != nil {
		t.Fatalf("open converted workbook: %v", err)
	}
	defer book.Close()
	window, err := book.Window("Sheet1", 1, 1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	cell := window.Cells[0][0]
	if cell.Value != "cached result 中文" {
		t.Errorf("A1 value = %q, want real BIFF cached string result", cell.Value)
	}
	if cell.Formula == "" {
		t.Error("A1 formula is empty after conversion")
	}
}

func TestConvertXLSSafelyRejectsUnpreservedWorksheetMetadata(t *testing.T) {
	converted, err := ConvertToTempXLSX(context.Background(), "testdata/unsupported-metadata.xls")
	if converted != nil {
		_ = converted.Cleanup()
		t.Fatal("ConvertToTempXLSX returned a workbook with unpreserved worksheet metadata")
	}
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "cannot be preserved") {
		t.Fatalf("ConvertToTempXLSX error = %v, want fail-closed unsupported metadata error", err)
	}
}

func TestConvertXLSKeepsMergedRangesAndHiddenSheets(t *testing.T) {
	reader := &stubImportReader{
		sheets: []string{"Visible", "Hidden"},
		rows: map[string][]ImportedRow{
			"Visible": {{Number: 1, Cells: []ImportedCell{{Column: 1, Value: "title"}}}},
			"Hidden":  {{Number: 1, Cells: []ImportedCell{{Column: 1, Value: "private"}}}},
		},
		merges: map[string][][2]string{"Visible": {{"A1", "B1"}}},
		hidden: map[string][2]bool{"Hidden": {true, false}},
	}
	converted, err := convertImportReader(context.Background(), reader, 10)
	if err != nil {
		t.Fatalf("convert workbook with merge and hidden sheet: %v", err)
	}
	defer converted.Cleanup()
	book, err := excelize.OpenFile(converted.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	visible, err := book.GetSheetVisible("Hidden")
	if err != nil || visible {
		t.Fatalf("Hidden sheet visible=%v, err=%v; want hidden", visible, err)
	}
	merges, err := book.GetMergeCells("Visible")
	if err != nil || len(merges) != 1 || merges[0].GetStartAxis() != "A1" || merges[0].GetEndAxis() != "B1" {
		t.Fatalf("merged cells=%v, err=%v; want A1:B1", merges, err)
	}
}

func TestConvertXLSPreservesColumnAndRowMetadata(t *testing.T) {
	reader := &stubImportReader{
		sheets: []string{"Sheet1"},
		rows: map[string][]ImportedRow{
			"Sheet1": {{Number: 1, HasStyle: true, StyleXF: 4, OutlineLevel: 1, Hidden: true, Cells: []ImportedCell{{Column: 1, XF: 4, Value: "header"}}}},
		},
		widths: map[string][]ImportedColumnWidth{
			"Sheet1": {{FirstColumn: 1, LastColumn: 1, WidthChars: 20, StyleXF: 5, Hidden: true, OutlineLevel: 2, BestFit: true, Collapsed: true}},
		},
		style: excelize.Style{Font: &excelize.Font{Bold: true}},
	}
	converted, err := convertImportReader(context.Background(), reader, 10)
	if err != nil {
		t.Fatalf("convert workbook with row/column metadata: %v", err)
	}
	defer converted.Cleanup()
	book, err := excelize.OpenFile(converted.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	visible, err := book.GetColVisible("Sheet1", "A")
	if err != nil || visible {
		t.Fatalf("column A visible=%v, err=%v; want hidden", visible, err)
	}
	columnOutline, err := book.GetColOutlineLevel("Sheet1", "A")
	if err != nil || columnOutline != 2 {
		t.Fatalf("column A outline=%d, err=%v; want 2", columnOutline, err)
	}
	columnStyle, err := book.GetColStyle("Sheet1", "A")
	if err != nil || columnStyle == 0 {
		t.Fatalf("column A style=%d, err=%v; want mapped style", columnStyle, err)
	}
	rowVisible, err := book.GetRowVisible("Sheet1", 1)
	if err != nil || rowVisible {
		t.Fatalf("row 1 visible=%v, err=%v; want hidden", rowVisible, err)
	}
	rowOutline, err := book.GetRowOutlineLevel("Sheet1", 1)
	if err != nil || rowOutline != 1 {
		t.Fatalf("row 1 outline=%d, err=%v; want 1", rowOutline, err)
	}
	zipReader, err := zip.OpenReader(converted.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer zipReader.Close()
	for _, entry := range zipReader.File {
		if entry.Name != "xl/worksheets/sheet1.xml" {
			continue
		}
		content, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, readErr := io.ReadAll(content)
		closeErr := content.Close()
		if err := errors.Join(readErr, closeErr); err != nil {
			t.Fatal(err)
		}
		decoder := xml.NewDecoder(strings.NewReader(string(data)))
		rowStylePresent := false
		for {
			token, err := decoder.Token()
			if err != nil {
				break
			}
			if start, ok := token.(xml.StartElement); ok && start.Name.Local == "row" && xmlAttribute(start.Attr, "r") == "1" && xmlAttribute(start.Attr, "s") != "" {
				rowStylePresent = true
			}
		}
		if !rowStylePresent {
			t.Fatalf("row style is missing from worksheet XML")
		}
		if !strings.Contains(string(data), `bestFit="true"`) || !strings.Contains(string(data), `collapsed="true"`) {
			t.Fatalf("column best-fit/outline metadata is missing from worksheet XML")
		}
		return
	}
	t.Fatal("converted worksheet XML missing")
}

func TestConvertXLSPreservesSharedStringsAcrossBIFFContinuations(t *testing.T) {
	converted, err := ConvertToTempXLSX(context.Background(), "testdata/sst-continuations.xls")
	if err != nil {
		t.Fatalf("ConvertToTempXLSX: %v", err)
	}
	defer converted.Cleanup()
	book, err := xlsx.Open(converted.Path)
	if err != nil {
		t.Fatalf("open converted workbook: %v", err)
	}
	defer book.Close()
	window, err := book.Window(book.SheetNames()[0], 3000, 1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := window.Cells[0][0].Value; got != "row-3000-中文-共享字符串" {
		t.Fatalf("A3000 = %q, want original Unicode shared string", got)
	}
}

func TestConvertXLSPreservesBIFF8BoundaryCoordinates(t *testing.T) {
	converted, err := ConvertToTempXLSX(context.Background(), "testdata/formula-style-multisheet.xls")
	if err != nil {
		t.Fatalf("ConvertToTempXLSX: %v", err)
	}
	defer converted.Cleanup()
	book, err := xlsx.Open(converted.Path)
	if err != nil {
		t.Fatalf("open converted workbook: %v", err)
	}
	defer book.Close()
	window, err := book.Window("总览", 65536, 256, 1, 1)
	if err != nil {
		t.Fatalf("read IV65536: %v", err)
	}
	if got := window.Cells[0][0].Value; got != "BIFF8-boundary" {
		t.Fatalf("IV65536 = %q, want original value", got)
	}
}

func TestConvertXLSRejectsUnsupportedWorkbookFeature(t *testing.T) {
	_, err := ConvertToTempXLSX(context.Background(), "testdata/unsupported-chart.xls")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "obj") {
		t.Fatalf("unsupported feature error = %v, want explicit chart/object rejection", err)
	}
}

func TestConvertXLSRejectsUnsupportedFormulaWithoutReplacingItWithValue(t *testing.T) {
	reader := &stubImportReader{
		sheets: []string{"Sheet1"},
		rows: map[string][]ImportedRow{
			"Sheet1": {{Number: 1, Cells: []ImportedCell{{Column: 1, Value: float64(99), FormulaToken: []byte{0xFF}}}}},
		},
	}
	_, err := convertImportReader(context.Background(), reader, 10)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "formula") {
		t.Fatalf("unsupported formula error = %v, want explicit formula rejection", err)
	}
}

func TestConvertXLSPreservesSharedFormulaExpressions(t *testing.T) {
	reader := &stubImportReader{
		sheets: []string{"Sheet1"},
		rows: map[string][]ImportedRow{
			"Sheet1": {
				{Number: 2, Cells: []ImportedCell{{Column: 2, Value: float64(4), FormulaToken: []byte{0x44, 0, 0, 0, 0xC0, 0x44, 0, 0, 1, 0x80, 0x03}, FormulaKind: "shared", FormulaAnchor: "B2", FormulaRange: "B2:B3"}}},
				{Number: 3, Cells: []ImportedCell{{Column: 2, Value: float64(6), FormulaToken: []byte{0x44, 0, 0, 0, 0xC0, 0x44, 0, 0, 1, 0x80, 0x03}, FormulaKind: "shared", FormulaAnchor: "B2", FormulaRange: "B2:B3"}}},
			},
		},
	}
	converted, err := convertImportReader(context.Background(), reader, 10)
	if err != nil {
		t.Fatalf("convert shared formulas: %v", err)
	}
	defer converted.Cleanup()
	book, err := excelize.OpenFile(converted.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	for _, check := range []struct{ cell, formula string }{{"B2", "(A1+$B1)"}, {"B3", "(A2+$B2)"}} {
		got, err := book.GetCellFormula("Sheet1", check.cell)
		if err != nil || got != check.formula {
			t.Errorf("%s formula = %q, %v; want %q", check.cell, got, err, check.formula)
		}
	}
}

func TestConvertXLSKeepsArrayFormulaKindAndRange(t *testing.T) {
	reader := &stubImportReader{
		sheets: []string{"Sheet1"},
		rows: map[string][]ImportedRow{
			"Sheet1": {
				{Number: 1, Cells: []ImportedCell{{Column: 1, Value: float64(2), FormulaToken: []byte{0x1E, 2, 0}, FormulaKind: "array", FormulaAnchor: "A1", FormulaRange: "A1:A2"}}},
				{Number: 2, Cells: []ImportedCell{{Column: 1, Value: float64(3), FormulaToken: []byte{0x1E, 2, 0}, FormulaKind: "array", FormulaAnchor: "A1", FormulaRange: "A1:A2"}}},
			},
		},
	}
	converted, err := convertImportReader(context.Background(), reader, 10)
	if err != nil {
		t.Fatalf("convert array formula: %v", err)
	}
	defer converted.Cleanup()
	archive, err := zip.OpenReader(converted.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	found := false
	for _, entry := range archive.File {
		if entry.Name != "xl/worksheets/sheet1.xml" {
			continue
		}
		file, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		decoder := xml.NewDecoder(file)
		for {
			token, err := decoder.Token()
			if err != nil {
				break
			}
			start, ok := token.(xml.StartElement)
			if !ok || start.Name.Local != "f" {
				continue
			}
			if xmlAttribute(start.Attr, "t") == "array" && xmlAttribute(start.Attr, "ref") == "A1:A2" {
				found = true
			}
		}
		_ = file.Close()
	}
	if !found {
		t.Fatal("converted array formula lost its t=array/ref range metadata")
	}
	book, err := excelize.OpenFile(converted.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	formula, err := book.GetCellFormula("Sheet1", "A1")
	if err != nil || formula != "2" {
		t.Fatalf("A1 array formula = %q, %v; want 2", formula, err)
	}
	follower, err := book.GetCellFormula("Sheet1", "A2")
	if err != nil || follower != "" {
		t.Fatalf("A2 follower formula = %q, %v; want no duplicate formula", follower, err)
	}
}

func TestConvertXLSPreservesFormulaCachedStringBooleanAndError(t *testing.T) {
	reader := &stubImportReader{
		sheets: []string{"Sheet1"},
		rows: map[string][]ImportedRow{
			"Sheet1": {{Number: 1, Cells: []ImportedCell{
				{Column: 1, Value: "cached text", FormulaToken: []byte{0x1E, 2, 0}},
				{Column: 2, Value: true, FormulaToken: []byte{0x1E, 2, 0}},
				{Column: 3, Value: CellError{Code: 0x2A}, FormulaToken: []byte{0x1E, 2, 0}},
			}}},
		},
	}
	converted, err := convertImportReader(context.Background(), reader, 10)
	if err != nil {
		t.Fatalf("convert stub formulas: %v", err)
	}
	defer converted.Cleanup()
	book, err := excelize.OpenFile(converted.Path)
	if err != nil {
		t.Fatalf("open converted workbook: %v", err)
	}
	defer book.Close()
	want := []string{"cached text", "TRUE", "#N/A"}
	for index, value := range want {
		ref := cellName(index+1, 1)
		got, err := book.GetCellValue("Sheet1", ref)
		if err != nil {
			t.Fatal(err)
		}
		formula, err := book.GetCellFormula("Sheet1", ref)
		if err != nil {
			t.Fatal(err)
		}
		if got != value || formula != "2" {
			t.Errorf("%s = value %q formula %q, want %q / 2", ref, got, formula, value)
		}
	}
}

func TestConvertXLSCancellationCleansTempDirectory(t *testing.T) {
	before := legacyImportTempDirs(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := convertImportReader(ctx, &stubImportReader{sheets: []string{"Sheet1"}}, 10)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled conversion error = %v, want context.Canceled", err)
	}
	assertLegacyImportTempDirsUnchanged(t, before)
}

func TestConvertXLSFailureCleansTempDirectory(t *testing.T) {
	before := legacyImportTempDirs(t)
	reader := &stubImportReader{
		sheets: []string{"Sheet1"},
		rows: map[string][]ImportedRow{
			"Sheet1": {{Number: 1, Cells: []ImportedCell{{Column: 1, FormulaToken: []byte{0xFF}}}}},
		},
	}
	if _, err := convertImportReader(context.Background(), reader, 10); err == nil {
		t.Fatal("conversion accepted an unsupported formula")
	}
	assertLegacyImportTempDirsUnchanged(t, before)
}

func TestConvertXLSResourceBudgetRejectsWithoutTruncating(t *testing.T) {
	reader := &stubImportReader{
		sheets: []string{"Sheet1"},
		rows: map[string][]ImportedRow{
			"Sheet1": {{Number: 1, Cells: []ImportedCell{
				{Column: 1, Value: "one"},
				{Column: 2, Value: "two"},
				{Column: 5, XF: 1}, // Styled blanks count against the same budget.
			}}},
		},
	}
	_, err := convertImportReader(context.Background(), reader, 2)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "budget") {
		t.Fatalf("cell-budget error = %v, want explicit resource-budget rejection", err)
	}
}

func legacyImportTempDirs(t *testing.T) map[string]struct{} {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(fileutil.DefaultTempDir(), "glance-xls-import-*"))
	if err != nil {
		t.Fatal(err)
	}
	result := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		result[match] = struct{}{}
	}
	return result
}

func assertLegacyImportTempDirsUnchanged(t *testing.T, before map[string]struct{}) {
	t.Helper()
	after := legacyImportTempDirs(t)
	for path := range after {
		if _, existed := before[path]; !existed {
			t.Errorf("failed conversion leaked temporary import directory %s", path)
		}
	}
}
