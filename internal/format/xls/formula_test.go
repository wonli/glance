package xls

import (
	"fmt"
	"strings"
	"testing"
)

type testFormulaResolver struct {
	sheets map[uint16]string
	names  map[uint16]string
}

func (r testFormulaResolver) SheetName(index uint16) (string, error) {
	name, ok := r.sheets[index]
	if !ok {
		return "", fmt.Errorf("unknown sheet index %d", index)
	}
	return name, nil
}

func (r testFormulaResolver) DefinedName(index uint16) (string, error) {
	name, ok := r.names[index]
	if !ok {
		return "", fmt.Errorf("unknown defined name index %d", index)
	}
	return name, nil
}

func TestDecodeBIFFFormulaOperatorsReferencesAndFunctions(t *testing.T) {
	resolver := testFormulaResolver{}
	tests := []struct {
		name  string
		token []byte
		want  string
	}{
		{
			name:  "precedence-preserving arithmetic",
			token: []byte{0x1E, 2, 0, 0x1E, 3, 0, 0x1E, 4, 0, 0x07, 0x03},
			want:  "(2+(3^4))",
		},
		{
			name:  "relative and absolute references",
			token: []byte{0x44, 0, 0, 1, 0xC0, 0x44, 2, 0, 2, 0, 0x03},
			want:  "(B1+$C$3)",
		},
		{
			name:  "area reference",
			token: []byte{0x45, 0, 0, 1, 0, 0, 0xC0, 1, 0xC0},
			want:  "A1:B2",
		},
		{
			name:  "unary and percent",
			token: []byte{0x1E, 2, 0, 0x13, 0x14},
			want:  "(-(2))%",
		},
		{
			name:  "fixed arity function",
			token: []byte{0x44, 0, 0, 1, 0xC0, 0x41, 15, 0},
			want:  "SIN(B1)",
		},
		{
			name:  "variable arity function",
			token: []byte{0x1E, 2, 0, 0x1E, 3, 0, 0x42, 2, 4, 0},
			want:  "SUM(2,3)",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := decodeBIFFFormula(test.token, "D1", resolver)
			if err != nil {
				t.Fatalf("decodeBIFFFormula: %v", err)
			}
			if got != test.want {
				t.Errorf("formula = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDecodeBIFFFormulaCrossSheetReference(t *testing.T) {
	token := []byte{0x5A, 0, 0, 0, 0, 1, 0xC0}
	resolver := testFormulaResolver{sheets: map[uint16]string{0: "总览"}}
	got, err := decodeBIFFFormula(token, "A1", resolver)
	if err != nil {
		t.Fatalf("decodeBIFFFormula: %v", err)
	}
	if got != "'总览'!B1" {
		t.Fatalf("formula = %q, want %q", got, "'总览'!B1")
	}
}

func TestDecodeBIFFFormulaDefinedName(t *testing.T) {
	token := []byte{0x43, 1, 0, 0, 0}
	resolver := testFormulaResolver{names: map[uint16]string{1: "GrossMargin"}}
	got, err := decodeBIFFFormula(token, "A1", resolver)
	if err != nil {
		t.Fatalf("decodeBIFFFormula: %v", err)
	}
	if got != "GrossMargin" {
		t.Fatalf("formula = %q, want GrossMargin", got)
	}
}

func TestDecodeBIFFFormulaUnknownNameReturnsError(t *testing.T) {
	_, err := decodeBIFFFormula([]byte{0x43, 2, 0, 0, 0}, "A1", testFormulaResolver{})
	if err == nil || !strings.Contains(err.Error(), "defined name") {
		t.Fatalf("unknown name error = %v, want defined-name context", err)
	}
}

func TestDecodeBIFFSharedFormulaAtEachCell(t *testing.T) {
	// Shared master B2 contains A1+$B1. Relative row/column flags are retained;
	// expanding the same token stream at B3 changes only relative references.
	tokens := []byte{
		0x44, 0, 0, 0, 0xC0,
		0x44, 0, 0, 1, 0x80,
		0x03,
	}
	for _, test := range []struct{ cell, want string }{
		{"B2", "(A1+$B1)"},
		{"B3", "(A2+$B2)"},
	} {
		got, err := decodeBIFFSharedFormula(tokens, "B2", test.cell, testFormulaResolver{})
		if err != nil {
			t.Fatalf("decode shared formula at %s: %v", test.cell, err)
		}
		if got != test.want {
			t.Errorf("formula at %s = %q, want %q", test.cell, got, test.want)
		}
	}
}

func TestDecodeBIFFArrayFormula(t *testing.T) {
	token := []byte{0x1E, 2, 0, 0x1E, 3, 0, 0x42, 2, 4, 0}
	got, err := decodeBIFFArrayFormula(token, "C1", "C1:C2", testFormulaResolver{})
	if err != nil {
		t.Fatalf("decode array formula: %v", err)
	}
	if got != "SUM(2,3)" {
		t.Fatalf("array formula = %q, want SUM(2,3)", got)
	}
}

func TestDecodeBIFFFormulaUnsupportedTokenReturnsError(t *testing.T) {
	_, err := decodeBIFFFormula([]byte{0xFF}, "A1", testFormulaResolver{})
	if err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("unknown token error = %v, want explicit token error", err)
	}
}

func TestDecodeBIFFFormulaUsesRealXLSFixtureTokens(t *testing.T) {
	reader, err := OpenImportReader("testdata/formula-style-multisheet.xls")
	if err != nil {
		t.Fatalf("OpenImportReader: %v", err)
	}
	defer reader.Close()
	cells := readImportedSheet(t, reader, "总览")
	for _, test := range []struct {
		key, cell, want string
	}{
		{"1:4", "D1", "(B1*2)"},
		{"1:5", "E1", "NA()"},
	} {
		formula, err := decodeBIFFFormula(cells[test.key].FormulaToken, test.cell, nil)
		if err != nil {
			t.Fatalf("decode %s: %v", test.cell, err)
		}
		if formula != test.want {
			t.Errorf("formula at %s = %q, want %q", test.cell, formula, test.want)
		}
	}
	crossSheetCells := readImportedSheet(t, reader, "明细")
	resolver, ok := reader.(FormulaResolver)
	if !ok {
		t.Fatal("import reader does not implement FormulaResolver")
	}
	crossSheet, err := decodeBIFFFormula(crossSheetCells["1:1"].FormulaToken, "A1", resolver)
	if err != nil {
		t.Fatalf("decode cross-sheet fixture formula: %v", err)
	}
	if crossSheet != "('总览'!B1*3)" {
		t.Errorf("cross-sheet formula = %q, want %q", crossSheet, "('总览'!B1*3)")
	}
}
