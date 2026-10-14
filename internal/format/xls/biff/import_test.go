package xls

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func TestColorRGBUsesCompleteBIFF8DefaultPalette(t *testing.T) {
	color, err := (&WorkBook{}).ColorRGB(27)
	if err != nil {
		t.Fatal(err)
	}
	if color != (PaletteColor{Red: 0xCC, Green: 0xFF, Blue: 0xFF}) {
		t.Fatalf("default BIFF8 color 27 = %+v, want #CCFFFF", color)
	}
}

func TestVisitImportRowsPreservesBOOLERRValuesAndXF(t *testing.T) {
	sheet := &WorkSheet{
		Name:   "Sheet1",
		parsed: true,
		rows: map[uint16]*Row{
			2: {info: &rowInfo{Height: 360}, cols: map[uint16]contentHandler{
				1: &BoolErrCol{Col: Col{RowB: 2, FirstColB: 1}, Xf: 7, Value: 0x2A, IsError: 1},
			}},
			5: {cols: map[uint16]contentHandler{
				3: &BoolErrCol{Col: Col{RowB: 5, FirstColB: 3}, Xf: 9, Value: 1, IsError: 0},
			}},
		},
		wb: &WorkBook{},
	}
	var rows []ImportedRow
	if err := sheet.VisitImportRows(func(row ImportedRow) error {
		rows = append(rows, row)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Number != 2 || rows[1].Number != 5 {
		t.Fatalf("rows = %#v, want original ascending BIFF row numbers", rows)
	}
	if got, ok := rows[0].Cells[0].Value.(ErrorCell); !ok || got.Code != 0x2A || rows[0].Cells[0].XF != 7 {
		t.Errorf("error cell = %#v, want #N/A ErrorCell with XF 7", rows[0].Cells[0])
	}
	if got, ok := rows[1].Cells[0].Value.(bool); !ok || !got || rows[1].Cells[0].XF != 9 {
		t.Errorf("boolean cell = %#v, want true with XF 9", rows[1].Cells[0])
	}
	if rows[0].HeightPoints != 18 || rows[1].HeightPoints != 0 {
		t.Errorf("row heights = %v, %v; want 18pt and no explicit height", rows[0].HeightPoints, rows[1].HeightPoints)
	}
}

func TestVisitImportRowsPreservesRowVisibilityOutlineAndStyle(t *testing.T) {
	sheet := &WorkSheet{
		Name:   "Sheet1",
		parsed: true,
		rows: map[uint16]*Row{
			0: {info: &rowInfo{Height: 300, Flags: 0x004700C0}, cols: map[uint16]contentHandler{
				0: &labelCol{BlankCol: BlankCol{Col: Col{RowB: 0, FirstColB: 0}, Xf: 2}, Str: "header"},
			}},
			1: {info: &rowInfo{Height: 300, Flags: 0x00000023}, cols: map[uint16]contentHandler{
				0: &labelCol{BlankCol: BlankCol{Col: Col{RowB: 1, FirstColB: 0}, Xf: 2}, Str: "detail"},
			}},
		},
		wb: &WorkBook{},
	}
	var rows []ImportedRow
	if err := sheet.VisitImportRows(func(row ImportedRow) error { rows = append(rows, row); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %#v, want 2", rows)
	}
	if !rows[0].HasStyle || rows[0].StyleXF != 0x47 || rows[0].Hidden || rows[0].OutlineLevel != 0 {
		t.Errorf("styled row metadata = %+v, want XF 71 and visible/unoutlined", rows[0])
	}
	if !rows[1].Hidden || rows[1].OutlineLevel != 3 || rows[1].HasStyle {
		t.Errorf("hidden outlined row metadata = %+v, want hidden outline level 3", rows[1])
	}
}

func TestVisitImportRowsResolvesSharedAndArrayFormulaGroups(t *testing.T) {
	sharedTokens := []byte{0x44, 0, 0, 0, 0xC0, 0x1E, 1, 0, 0x03} // A1+1, relative.
	arrayTokens := []byte{0x1E, 2, 0, 0x1E, 3, 0, 0x42, 2, 4, 0}  // SUM(2,3).
	sheet := &WorkSheet{
		Name:          "Sheet1",
		parsed:        true,
		rows:          make(map[uint16]*Row),
		formulaGroups: make(map[uint32]formulaGroup),
		wb:            &WorkBook{},
	}
	sharedRecord := make([]byte, 10+len(sharedTokens))
	sharedRecord[2] = 1 // ref last row = 1, range A1:A2.
	sharedRecord[7] = 2 // cUse.
	binary.LittleEndian.PutUint16(sharedRecord[8:10], uint16(len(sharedTokens)))
	copy(sharedRecord[10:], sharedTokens)
	if err := sheet.addFormulaGroup(sharedRecord, formulaGroupShared); err != nil {
		t.Fatalf("add shared formula group: %v", err)
	}
	arrayRecord := make([]byte, 14+len(arrayTokens))
	arrayRecord[0] = 2 // row 3
	arrayRecord[2] = 3 // row 4
	arrayRecord[4] = 1 // first column B
	arrayRecord[5] = 1 // last column B
	binary.LittleEndian.PutUint16(arrayRecord[12:14], uint16(len(arrayTokens)))
	copy(arrayRecord[14:], arrayTokens)
	if err := sheet.addFormulaGroup(arrayRecord, formulaGroupArray); err != nil {
		t.Fatalf("add array formula group: %v", err)
	}

	addFormula := func(row, column, anchorRow, anchorColumn uint16) {
		formula := &FormulaCol{}
		formula.Header.Col = Col{RowB: row, FirstColB: column}
		formula.Header.IndexXf = 3
		formula.Header.Result = [8]byte{0, 0, 0, 0, 0, 0, 0, 0}
		formula.Bts = make([]byte, 7)
		binary.LittleEndian.PutUint16(formula.Bts[:2], 5)
		formula.Bts[2] = 0x01
		binary.LittleEndian.PutUint16(formula.Bts[3:5], anchorRow)
		binary.LittleEndian.PutUint16(formula.Bts[5:7], anchorColumn)
		line := sheet.rows[row]
		if line == nil {
			line = &Row{cols: make(map[uint16]contentHandler)}
			sheet.rows[row] = line
		}
		line.cols[column] = formula
	}
	addFormula(0, 0, 0, 0)
	addFormula(1, 0, 0, 0)
	addFormula(2, 1, 2, 1)
	addFormula(3, 1, 2, 1)

	var got []ImportedCell
	if err := sheet.VisitImportRows(func(row ImportedRow) error {
		got = append(got, row.Cells...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d formula cells, want 4", len(got))
	}
	for index, cell := range got[:2] {
		if cell.FormulaKind != "shared" || cell.FormulaAnchor != "A1" || cell.FormulaRange != "A1:A2" || string(cell.FormulaToken) != string(sharedTokens) {
			t.Errorf("shared cell %d metadata = %#v", index, cell)
		}
	}
	for index, cell := range got[2:] {
		if cell.FormulaKind != "array" || cell.FormulaAnchor != "B3" || cell.FormulaRange != "B3:B4" || string(cell.FormulaToken) != string(arrayTokens) {
			t.Errorf("array cell %d metadata = %#v", index, cell)
		}
	}
}

func TestGetStringRejectsImpossiblePhoneticLength(t *testing.T) {
	workbook := &WorkBook{}
	_, err := workbook.get_string(bytes.NewReader([]byte{0x04, 0xFF, 0xFF, 0xFF, 0xFF}), 0)
	if err == nil || !strings.Contains(err.Error(), "phonetic extension length") {
		t.Fatalf("get_string error = %v, want bounded phonetic-length rejection", err)
	}
}
