package xls

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readImportedSheet(t *testing.T, reader ImportReader, sheet string) map[string]ImportedCell {
	t.Helper()
	cells := make(map[string]ImportedCell)
	previousRow := 0
	err := reader.VisitRows(sheet, func(row ImportedRow) error {
		if row.Number <= previousRow {
			return fmt.Errorf("rows are not ascending: %d after %d", row.Number, previousRow)
		}
		previousRow = row.Number
		previousColumn := 0
		for _, cell := range row.Cells {
			if cell.Column <= previousColumn {
				return fmt.Errorf("columns are not ascending in row %d: %d after %d", row.Number, cell.Column, previousColumn)
			}
			previousColumn = cell.Column
			cells[fmt.Sprintf("%d:%d", row.Number, cell.Column)] = cell
		}
		return nil
	})
	if err != nil {
		t.Fatalf("VisitRows(%q): %v", sheet, err)
	}
	return cells
}

func TestImportReaderReturnsAllSheetsAndTypedCellMetadata(t *testing.T) {
	reader, err := OpenImportReader("testdata/formula-style-multisheet.xls")
	if err != nil {
		t.Fatalf("OpenImportReader: %v", err)
	}
	defer reader.Close()

	wantSheets := []string{"总览", "明细"}
	gotSheets := reader.SheetNames()
	if fmt.Sprint(gotSheets) != fmt.Sprint(wantSheets) {
		t.Fatalf("SheetNames() = %q, want %q", gotSheets, wantSheets)
	}

	cells := readImportedSheet(t, reader, "总览")
	for _, want := range []struct {
		key   string
		value any
	}{
		{"1:1", "项目"},
		{"2:1", "中文文本"},
		{"1:2", float64(42.5)},
		{"2:2", float64(3)},
		{"1:3", true},
		{"1:4", float64(85)},
	} {
		got, ok := cells[want.key]
		if !ok {
			t.Fatalf("missing cell %s", want.key)
		}
		switch expected := want.value.(type) {
		case float64:
			actual, ok := got.Value.(float64)
			if !ok || math.Abs(actual-expected) > 1e-9 {
				t.Errorf("cell %s value = %#v (%T), want %v (float64)", want.key, got.Value, got.Value, expected)
			}
		default:
			if got.Value != expected {
				t.Errorf("cell %s value = %#v (%T), want %#v (%T)", want.key, got.Value, got.Value, expected, expected)
			}
		}
	}
	if value, ok := cells["1:5"].Value.(CellError); !ok {
		t.Errorf("formula error cache at E1 = %#v (%T), want CellError", cells["1:5"].Value, cells["1:5"].Value)
	} else if value.Code != 0x2A {
		t.Errorf("formula error cache at E1 has BIFF code 0x%02X, want #N/A code 0x2A", value.Code)
	}
	boundary, ok := cells["65536:256"]
	if !ok || boundary.Value != "BIFF8-boundary" {
		t.Errorf("IV65536 = %#v, present=%v; want original sparse coordinate and value", boundary.Value, ok)
	}
	if _, ok := cells["65535:256"]; ok {
		t.Error("BIFF8 boundary cell was shifted to an earlier row")
	}
}

func TestImportReaderPreservesFormulaTokensAndXFIndices(t *testing.T) {
	reader, err := OpenImportReader("testdata/formula-style-multisheet.xls")
	if err != nil {
		t.Fatalf("OpenImportReader: %v", err)
	}
	defer reader.Close()

	cells := readImportedSheet(t, reader, "总览")
	formula := cells["1:4"]
	if len(formula.FormulaToken) == 0 {
		t.Fatal("D1 formula token stream is empty")
	}
	styled := cells["1:1"]
	blankStyled := cells["4:6"]
	if styled.XF == 0 || blankStyled.XF == 0 {
		t.Fatalf("expected non-default XF indices for A1 and blank F4, got %d and %d", styled.XF, blankStyled.XF)
	}
	if blankStyled.Value != nil {
		t.Errorf("blank styled F4 value = %#v, want nil", blankStyled.Value)
	}
}

func TestImportReaderPreservesFormulaStringCacheFromBIFF(t *testing.T) {
	reader, err := OpenImportReader("testdata/formula-string-cache.xls")
	if err != nil {
		t.Fatalf("OpenImportReader: %v", err)
	}
	defer reader.Close()

	cells := readImportedSheet(t, reader, "Sheet1")
	cell, ok := cells["1:1"]
	if !ok {
		t.Fatal("formula cell A1 is missing")
	}
	if cell.Value != "cached result 中文" {
		t.Errorf("A1 cached value = %#v (%T), want string cache", cell.Value, cell.Value)
	}
	if len(cell.FormulaToken) == 0 {
		t.Error("A1 formula token stream is empty")
	}
}

func TestImportReaderReportsUnsupportedWorksheetMetadata(t *testing.T) {
	reader, err := OpenImportReader("testdata/unsupported-metadata.xls")
	if err != nil {
		t.Fatalf("OpenImportReader: %v", err)
	}
	defer reader.Close()

	var names []string
	for _, feature := range reader.UnsupportedFeatures() {
		names = append(names, strings.ToLower(feature.Name))
	}
	for _, want := range []string{"hyperlink"} {
		found := false
		for _, name := range names {
			if strings.Contains(name, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("unsupported metadata %q not reported; features=%q", want, names)
		}
	}
	for _, name := range names {
		if strings.Contains(name, "merged") {
			t.Errorf("merged cells should be importable, but were reported unsupported: %q", names)
		}
	}
	merged := reader.MergedCells(reader.SheetNames()[0])
	if len(merged) == 0 {
		t.Fatal("merged ranges were not exposed to the XLSX converter")
	}
}

func TestParseBIFFMergedCellsPreservesCoordinatesAndRejectsInvalidRanges(t *testing.T) {
	payload := make([]byte, 10)
	binary.LittleEndian.PutUint16(payload[0:2], 1)
	binary.LittleEndian.PutUint16(payload[2:4], 1) // First row: 2.
	binary.LittleEndian.PutUint16(payload[4:6], 3) // Last row: 4.
	binary.LittleEndian.PutUint16(payload[6:8], 1) // First column: B.
	binary.LittleEndian.PutUint16(payload[8:10], 2)
	ranges, err := parseBIFFMergedCells(payload)
	if err != nil || len(ranges) != 1 || ranges[0] != [2]string{"B2", "C4"} {
		t.Fatalf("parsed merged ranges = %#v, %v; want B2:C4", ranges, err)
	}
	if mergeOverlaps(ranges, [2]string{"C4", "D5"}) != true {
		t.Error("overlapping merged ranges were accepted")
	}
	if mergeOverlaps(ranges, [2]string{"D4", "E5"}) {
		t.Error("disjoint merged ranges were rejected")
	}
	if _, err := parseBIFFMergedCells(payload[:9]); err == nil {
		t.Error("truncated merged-range payload was accepted")
	}
	payload[2], payload[3], payload[4], payload[5] = 4, 0, 1, 0
	if _, err := parseBIFFMergedCells(payload); err == nil {
		t.Error("inverted merged-row range was accepted")
	}
}

func TestParseBIFFNotePreservesCellAuthorAndVisibility(t *testing.T) {
	payload := make([]byte, 13)
	binary.LittleEndian.PutUint16(payload[0:2], 3) // Row 4.
	binary.LittleEndian.PutUint16(payload[2:4], 4) // Column E.
	binary.LittleEndian.PutUint16(payload[4:6], 2) // Always show.
	binary.LittleEndian.PutUint16(payload[6:8], 7)
	binary.LittleEndian.PutUint16(payload[8:10], 1) // One UTF-16 character.
	payload[10] = 1
	binary.LittleEndian.PutUint16(payload[11:13], 'A')
	comment, objectID, err := parseBIFFNote(payload)
	if err != nil || objectID != 7 || comment.Cell != "E4" || comment.Author != "A" || !comment.Visible {
		t.Fatalf("parsed BIFF note = %+v object=%d err=%v; want E4 author A visible object 7", comment, objectID, err)
	}
}

func TestBoundsheetMetadataReportsHiddenAndNonWorksheetSheets(t *testing.T) {
	for _, test := range []struct {
		name      string
		state     byte
		sheetType byte
		want      string
	}{
		{name: "chart sheet", sheetType: 2, want: "non-worksheet"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, unsupported := unsupportedBoundsheetMetadata(test.state, test.sheetType)
			if !unsupported || !strings.Contains(got, test.want) {
				t.Fatalf("unsupportedBoundsheetMetadata(%d, %d) = %q, %v; want %q", test.state, test.sheetType, got, unsupported, test.want)
			}
		})
	}
	for _, state := range []byte{0, 1, 2} {
		if got, unsupported := unsupportedBoundsheetMetadata(state, 0); unsupported || got != "" {
			t.Errorf("worksheet state %d = %q, %v; visibility should be preserved", state, got, unsupported)
		}
	}
	if got, unsupported := unsupportedBoundsheetMetadata(0, 0); unsupported || got != "" {
		t.Fatalf("visible worksheet metadata = %q, %v; want supported", got, unsupported)
	}
}

func TestBIFFRowFlagsSeparatesPreservableMetadataFromUnsupportedFeatures(t *testing.T) {
	for _, test := range []struct {
		name  string
		flags uint32
		want  bool
	}{
		{name: "ordinary row height", flags: 0x000F0100},
		{name: "manually sized row", flags: 0x00000040},
		{name: "hidden row", flags: 0x00000020},
		{name: "collapsed outline", flags: 0x00000010, want: true},
		{name: "outline level", flags: 0x00000003},
		{name: "row style", flags: 0x00000080},
		{name: "phonetic row", flags: 0x40000000, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := unsupportedBIFFRowFlags(test.flags); got != test.want {
				t.Fatalf("unsupportedBIFFRowFlags(%08x) = %v, want %v", test.flags, got, test.want)
			}
		})
	}
}

func TestParseBIFFColumnInfoPreservesWidthAndRejectsVisibilityOrStyle(t *testing.T) {
	payload := []byte{0, 0, 0, 1, 0x72, 0x09, 0x0F, 0, 0, 0, 0, 0}
	width, unsupported, err := parseBIFFColumnInfo(payload)
	if err != nil || unsupported != "" {
		t.Fatalf("parse default COLINFO = %+v, %q, %v", width, unsupported, err)
	}
	if width.FirstColumn != 1 || width.LastColumn != 256 || math.Abs(width.WidthChars-9.4453125) > 1e-9 {
		t.Errorf("parsed column width = %+v, want columns 1:256 at 9.4453125 characters", width)
	}
	rangePayload := append([]byte(nil), payload...)
	rangePayload[0], rangePayload[1], rangePayload[2], rangePayload[3] = 2, 0, 4, 0 // Zero-based C:E, inclusive.
	ranged, unsupported, err := parseBIFFColumnInfo(rangePayload)
	if err != nil || unsupported != "" || ranged.FirstColumn != 3 || ranged.LastColumn != 5 {
		t.Errorf("parsed bounded COLINFO = %+v, %q, %v; want columns 3:5", ranged, unsupported, err)
	}

	for _, test := range []struct {
		name      string
		mutate    func([]byte)
		check     func(ImportedColumnWidth) bool
		wantError string
	}{
		{name: "hidden", mutate: func(b []byte) { b[8] = 1 }, check: func(g ImportedColumnWidth) bool { return g.Hidden }},
		{name: "best fit", mutate: func(b []byte) { b[8] = 4 }, check: func(g ImportedColumnWidth) bool { return g.BestFit }},
		{name: "outline level", mutate: func(b []byte) { b[9] = 1 }, check: func(g ImportedColumnWidth) bool { return g.OutlineLevel == 1 }},
		{name: "column style", mutate: func(b []byte) { b[6] = 14 }, check: func(g ImportedColumnWidth) bool { return g.StyleXF == 14 }},
		{name: "phonetic", mutate: func(b []byte) { b[8] = 8 }, wantError: "phonetic"},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := append([]byte(nil), payload...)
			test.mutate(input)
			got, unsupported, err := parseBIFFColumnInfo(input)
			if err != nil || (test.wantError == "" && unsupported != "") || (test.wantError != "" && !strings.Contains(unsupported, test.wantError)) {
				t.Fatalf("parse COLINFO = %+v, %q, %v; want unsupported %q", got, unsupported, err, test.wantError)
			}
			if test.wantError == "" && !test.check(got) {
				t.Errorf("parsed COLINFO = %+v, expected %s metadata", got, test.name)
			}
		})
	}

	styledWidth := append([]byte(nil), payload...)
	styledWidth[6], styledWidth[7] = 72, 0
	styledWidth[8], styledWidth[9] = 6, 0 // Manually sized and best-fit; preserve the resolved width.
	styled, unsupported, err := parseBIFFColumnInfo(styledWidth)
	if err != nil || unsupported != "" || styled.WidthChars == 0 || styled.StyleXF != 72 || !styled.BestFit {
		t.Fatalf("parse styled BIFF column = %+v, %q, %v; want supported resolved width", styled, unsupported, err)
	}
}

func TestImportReaderReportsUnsupportedFeatures(t *testing.T) {
	reader, err := OpenImportReader("testdata/unsupported-chart.xls")
	if err != nil {
		t.Fatalf("OpenImportReader: %v", err)
	}
	defer reader.Close()

	features := reader.UnsupportedFeatures()
	if len(features) == 0 {
		t.Fatal("chart workbook returned no unsupported-feature report")
	}
	found := false
	for _, feature := range features {
		if strings.Contains(strings.ToLower(feature.Name), "chart") || feature.RecordID == 0x00EC || feature.RecordID == 0x005D {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("unsupported features do not identify the chart records: %+v", features)
	}
}

func TestOpenImportReaderRejectsOverBudgetBeforeWorkbookParsing(t *testing.T) {
	reader, err := OpenImportReader("testdata/resource-over-budget.xls")
	if reader != nil {
		_ = reader.Close()
		t.Fatal("OpenImportReader returned a reader for an over-budget workbook")
	}
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "budget") {
		t.Fatalf("OpenImportReader error = %v, want pre-parse cell-budget rejection", err)
	}
}

func TestImportReaderRejectsEncryptedWorkbook(t *testing.T) {
	_, err := OpenImportReader("testdata/encrypted.xls")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "encrypt") {
		t.Fatalf("OpenImportReader encrypted workbook error = %v, want an encryption error", err)
	}
}

func TestImportReaderRejectsCorruptWorkbook(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.xls")
	if err := os.WriteFile(path, []byte("not a compound document"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenImportReader(path); err == nil {
		t.Fatal("OpenImportReader accepted a corrupt non-OLE file")
	}
}

func TestImportReaderRejectsTruncatedWorkbook(t *testing.T) {
	data, err := os.ReadFile("testdata/formula-style-multisheet.xls")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "truncated.xls")
	if err := os.WriteFile(path, data[:512], 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = OpenImportReader(path)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("OpenImportReader truncated workbook error = %v, want a parse error", err)
	}
}

func TestImportReaderReadsSharedStringsAcrossBIFFContinuations(t *testing.T) {
	reader, err := OpenImportReader("testdata/sst-continuations.xls")
	if err != nil {
		t.Fatalf("OpenImportReader: %v", err)
	}
	defer reader.Close()

	count := 0
	var first, last string
	err = reader.VisitRows(reader.SheetNames()[0], func(row ImportedRow) error {
		count += len(row.Cells)
		if row.Number == 1 {
			first, _ = row.Cells[0].Value.(string)
		}
		if row.Number == 3000 {
			last, _ = row.Cells[0].Value.(string)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("VisitRows: %v", err)
	}
	if count != 3000 {
		t.Fatalf("imported %d cells, want all 3000 shared-string cells", count)
	}
	if first != "row-0001-中文-共享字符串" || last != "row-3000-中文-共享字符串" {
		t.Fatalf("shared-string endpoints = %q / %q, want original Chinese text", first, last)
	}
}
