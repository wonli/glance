package desktopui

import (
	"errors"
	"testing"

	"glance/internal/document"
)

func TestCellEditBeginCommitCancel(t *testing.T) {
	s := cellEditState{}
	s.selectCell("C4", "原值")
	if !s.begin() {
		t.Fatal("begin failed")
	}
	s.setDraft("草稿")
	if s.Original != "原值" || s.Draft != "草稿" {
		t.Fatalf("state=%+v", s)
	}
	s.cancel()
	if s.Editing || s.Draft != "原值" || s.Original != "原值" {
		t.Fatalf("cancel=%+v", s)
	}
	_ = s.begin()
	s.setDraft("更新")
	calls := 0
	if err := s.commit(func(address, value string) error {
		calls++
		if address != "C4" || value != "更新" {
			t.Fatalf("%s=%s", address, value)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || s.Editing || s.Original != "更新" {
		t.Fatalf("commit=%+v calls=%d", s, calls)
	}
}
func TestCellEditNoOpDoesNotDirty(t *testing.T) {
	s := cellEditState{}
	s.selectCell("A1", "same")
	_ = s.begin()
	calls := 0
	if err := s.commit(func(string, string) error { calls++; return nil }); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("no-op called apply %d times", calls)
	}
}
func TestCellEditRejectsFormulaAndRetainsDraftOnError(t *testing.T) {
	s := cellEditState{}
	s.selectCell("A1", "10")
	_ = s.begin()
	s.setDraft("=SUM(A1:A2)")
	want := errors.New("formulas unsupported")
	err := s.commit(func(_, value string) error {
		if value != "=SUM(A1:A2)" {
			t.Fatalf("draft=%q", value)
		}
		return want
	})
	if !errors.Is(err, want) || !s.Editing || s.Draft != "=SUM(A1:A2)" || s.Original != "10" {
		t.Fatalf("state=%+v err=%v", s, err)
	}
}
func TestCellSelectionSyncsNameAndValue(t *testing.T) {
	s := cellEditState{}
	s.selectCell("B7", "员工")
	if s.Address != "B7" || s.Draft != "员工" {
		t.Fatalf("state=%+v", s)
	}
}

func TestParseCellRangeAddress(t *testing.T) {
	tests := []struct {
		input     string
		wantStart CellPosition
		wantEnd   CellPosition
		wantOK    bool
	}{
		{input: "B3", wantStart: CellPosition{Row: 2, Column: 1}, wantEnd: CellPosition{Row: 2, Column: 1}, wantOK: true},
		{input: " $b$3 : $D$7 ", wantStart: CellPosition{Row: 2, Column: 1}, wantEnd: CellPosition{Row: 6, Column: 3}, wantOK: true},
		{input: "D7:B3", wantStart: CellPosition{Row: 6, Column: 3}, wantEnd: CellPosition{Row: 2, Column: 1}, wantOK: true},
		{input: "A1048576:XFD1048576", wantStart: CellPosition{Row: xlsxRows - 1, Column: 0}, wantEnd: CellPosition{Row: xlsxRows - 1, Column: xlsxColumns - 1}, wantOK: true},
		{input: "A1048577", wantOK: false},
		{input: "XFE1", wantOK: false},
		{input: "A1:B2:C3", wantOK: false},
		{input: "Sheet1!A1", wantOK: false},
		{input: "", wantOK: false},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			gotStart, gotEnd, gotOK := parseCellRangeAddress(test.input)
			if gotStart != test.wantStart || gotEnd != test.wantEnd || gotOK != test.wantOK {
				t.Fatalf("parseCellRangeAddress(%q) = %+v, %+v, %t; want %+v, %+v, %t", test.input, gotStart, gotEnd, gotOK, test.wantStart, test.wantEnd, test.wantOK)
			}
		})
	}
}

func TestParseSpreadsheetGoToReference(t *testing.T) {
	names := []document.SpreadsheetDefinedName{
		{Name: "SalesData", Ref: "'Data!2026'!$B$2:$D$4", Scope: "Workbook"},
		{Name: "LocalSales", Ref: "'Sheet1'!$C$3:$C$8", Scope: "Sheet1"},
		{Name: "ScopedElsewhere", Ref: "'Sheet2'!$A$1:$A$2", Scope: "Sheet2"},
		{Name: "Shadowed", Ref: "'Sheet2'!$A$1", Scope: "Workbook"},
		{Name: "Shadowed", Ref: "'Sheet1'!$E$5", Scope: "Sheet1"},
		{Name: "FormulaName", Ref: "=TODAY()", Scope: "Workbook"},
	}
	tests := []struct {
		name      string
		input     string
		current   string
		wantSheet string
		wantStart CellPosition
		wantEnd   CellPosition
		wantOK    bool
	}{
		{name: "current sheet cell", input: "B3", current: "Sheet1", wantSheet: "Sheet1", wantStart: CellPosition{Row: 2, Column: 1}, wantEnd: CellPosition{Row: 2, Column: 1}, wantOK: true},
		{name: "qualified range", input: "'Data Sheet'!$B$2:$D$4", current: "Sheet1", wantSheet: "Data Sheet", wantStart: CellPosition{Row: 1, Column: 1}, wantEnd: CellPosition{Row: 3, Column: 3}, wantOK: true},
		{name: "quoted bang in sheet name", input: "'Data!2026'!$B$2:$D$4", current: "Sheet1", wantSheet: "Data!2026", wantStart: CellPosition{Row: 1, Column: 1}, wantEnd: CellPosition{Row: 3, Column: 3}, wantOK: true},
		{name: "workbook defined name", input: "salesdata", current: "Sheet1", wantSheet: "Data!2026", wantStart: CellPosition{Row: 1, Column: 1}, wantEnd: CellPosition{Row: 3, Column: 3}, wantOK: true},
		{name: "sheet-scoped name", input: "LocalSales", current: "Sheet1", wantSheet: "Sheet1", wantStart: CellPosition{Row: 2, Column: 2}, wantEnd: CellPosition{Row: 7, Column: 2}, wantOK: true},
		{name: "local scope shadows workbook", input: "Shadowed", current: "Sheet1", wantSheet: "Sheet1", wantStart: CellPosition{Row: 4, Column: 4}, wantEnd: CellPosition{Row: 4, Column: 4}, wantOK: true},
		{name: "other-sheet local name is unavailable", input: "ScopedElsewhere", current: "Sheet1", wantOK: false},
		{name: "formula name is not a range", input: "FormulaName", current: "Sheet1", wantOK: false},
		{name: "qualified address retains target sheet for caller validation", input: "Missing!A1", current: "Sheet1", wantSheet: "Missing", wantOK: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotSheet, gotStart, gotEnd, gotOK := parseSpreadsheetGoToReference(test.input, test.current, names)
			if gotSheet != test.wantSheet || gotStart != test.wantStart || gotEnd != test.wantEnd || gotOK != test.wantOK {
				t.Fatalf("parseSpreadsheetGoToReference(%q, %q) = %q, %+v, %+v, %t; want %q, %+v, %+v, %t", test.input, test.current, gotSheet, gotStart, gotEnd, gotOK, test.wantSheet, test.wantStart, test.wantEnd, test.wantOK)
			}
		})
	}
}
