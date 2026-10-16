package desktopui

import (
	"math"
	"testing"
	"time"

	"gioui.org/layout"
	"glance/internal/document"
	"glance/internal/format/xlsx"
)

func TestFormatPatchForToolbarActionTogglesAndClears(t *testing.T) {
	for _, item := range []struct {
		slot  int
		style document.CellStyle
		want  *bool
	}{
		{slot: 4, style: document.CellStyle{Bold: false}, want: boolPointer(true)},
		{slot: 5, style: document.CellStyle{Italic: true}, want: boolPointer(false)},
		{slot: 6, style: document.CellStyle{Underline: false}, want: boolPointer(true)},
	} {
		patch, ok := formatPatchForToolbarAction(item.slot, item.style)
		if !ok || patch.Bold != nil && *patch.Bold != *item.want || patch.Italic != nil && *patch.Italic != *item.want || patch.Underline != nil && *patch.Underline != *item.want {
			t.Fatalf("format action %d returned %+v, ok=%t", item.slot, patch, ok)
		}
	}
	patch, ok := formatPatchForToolbarAction(16, document.CellStyle{})
	if !ok || !patch.ClearFormat {
		t.Fatalf("clear-format action returned %+v, ok=%t", patch, ok)
	}
	if _, ok := formatPatchForToolbarAction(7, document.CellStyle{}); ok {
		t.Fatal("font color should open a picker instead of applying an implicit color")
	}
}

func TestToolbarPickerOptionsAndPatches(t *testing.T) {
	if got := toolbarPickerOptions(pickerAlignment); len(got) != 4 || got[2] != "居中" {
		t.Fatalf("alignment options=%v", got)
	}
	formatOptions := toolbarPickerOptions(pickerNumberFormat)
	if len(formatOptions) != 11 || formatOptions[3] != "千分位" || formatOptions[8] != "时间" || formatOptions[9] != "科学计数" {
		t.Fatalf("number format options=%v", formatOptions)
	}
	patch, ok := toolbarPickerPatch(pickerFillColor, 0)
	if !ok || patch.FillColor == nil || *patch.FillColor != "" {
		t.Fatalf("fill clear option=%+v, ok=%t", patch, ok)
	}
	patch, ok = toolbarPickerPatch(pickerNumberFormat, 6)
	if !ok || patch.NumberFormat == nil || *patch.NumberFormat != "yyyy-mm-dd" {
		t.Fatalf("date number format option=%+v, ok=%t", patch, ok)
	}
	for _, item := range []struct {
		index int
		want  string
	}{{3, "#,##0.00"}, {5, "0%"}, {7, "m/d/yy"}, {8, "h:mm AM/PM"}, {9, "0.00E+00"}} {
		patch, ok = toolbarPickerPatch(pickerNumberFormat, item.index)
		if !ok || patch.NumberFormat == nil || *patch.NumberFormat != item.want {
			t.Errorf("number format option %d = %+v, ok=%t; want %q", item.index, patch, ok, item.want)
		}
	}
}

func boolPointer(value bool) *bool { return &value }

func TestFormatPatchForExcelP0ToolbarActions(t *testing.T) {
	// Align Left
	patch, ok := formatPatchForToolbarAction(toolbarActionAlignLeft, document.CellStyle{HorizontalAlignment: ""})
	if !ok || patch.HorizontalAlignment == nil || *patch.HorizontalAlignment != "left" {
		t.Fatalf("align left empty: got %+v, ok=%t", patch, ok)
	}
	patch, ok = formatPatchForToolbarAction(toolbarActionAlignLeft, document.CellStyle{HorizontalAlignment: "left"})
	if !ok || patch.HorizontalAlignment == nil || *patch.HorizontalAlignment != "general" {
		t.Fatalf("align left toggle: got %+v, ok=%t", patch, ok)
	}

	// Align Right
	patch, ok = formatPatchForToolbarAction(toolbarActionAlignRight, document.CellStyle{HorizontalAlignment: ""})
	if !ok || patch.HorizontalAlignment == nil || *patch.HorizontalAlignment != "right" {
		t.Fatalf("align right empty: got %+v, ok=%t", patch, ok)
	}
	patch, ok = formatPatchForToolbarAction(toolbarActionAlignRight, document.CellStyle{HorizontalAlignment: "right"})
	if !ok || patch.HorizontalAlignment == nil || *patch.HorizontalAlignment != "general" {
		t.Fatalf("align right toggle: got %+v, ok=%t", patch, ok)
	}

	// Wrap Text
	patch, ok = formatPatchForToolbarAction(toolbarActionWrapText, document.CellStyle{WrapText: false})
	if !ok || patch.WrapText == nil || !*patch.WrapText {
		t.Fatalf("wrap text false: got %+v, ok=%t", patch, ok)
	}
	patch, ok = formatPatchForToolbarAction(toolbarActionWrapText, document.CellStyle{WrapText: true})
	if !ok || patch.WrapText == nil || *patch.WrapText {
		t.Fatalf("wrap text true: got %+v, ok=%t", patch, ok)
	}

	// Percent Format
	patch, ok = formatPatchForToolbarAction(toolbarActionPercentFormat, document.CellStyle{NumberFormat: ""})
	if !ok || patch.NumberFormat == nil || *patch.NumberFormat != "0.00%" {
		t.Fatalf("percent empty: got %+v, ok=%t", patch, ok)
	}
	patch, ok = formatPatchForToolbarAction(toolbarActionPercentFormat, document.CellStyle{NumberFormat: "0.00%"})
	if !ok || patch.NumberFormat == nil || *patch.NumberFormat != "General" {
		t.Fatalf("percent 0.00%% toggle: got %+v, ok=%t", patch, ok)
	}
	patch, ok = formatPatchForToolbarAction(toolbarActionPercentFormat, document.CellStyle{NumberFormat: "0%"})
	if !ok || patch.NumberFormat == nil || *patch.NumberFormat != "General" {
		t.Fatalf("percent 0%% toggle: got %+v, ok=%t", patch, ok)
	}

	// Check unimplemented returns false
	for _, slot := range []int{toolbarActionAlignLeft, toolbarActionAlignRight, toolbarActionWrapText, toolbarActionPercentFormat} {
		if unimplementedSpreadsheetToolbarAction(slot) {
			t.Errorf("slot %d should not be marked unimplemented", slot)
		}
		if !formatStyleSlot(slot) {
			t.Errorf("slot %d should be recognized as formatStyleSlot", slot)
		}
	}

	// Check formatStyleActive
	if !formatStyleActive(toolbarActionAlignLeft, document.CellStyle{HorizontalAlignment: "left"}) {
		t.Error("align left should be active")
	}
	if formatStyleActive(toolbarActionAlignLeft, document.CellStyle{HorizontalAlignment: "right"}) {
		t.Error("align left should not be active when right")
	}
	if !formatStyleActive(toolbarActionAlignRight, document.CellStyle{HorizontalAlignment: "right"}) {
		t.Error("align right should be active")
	}
	if !formatStyleActive(toolbarActionWrapText, document.CellStyle{WrapText: true}) {
		t.Error("wrap text should be active")
	}
	if formatStyleActive(toolbarActionWrapText, document.CellStyle{WrapText: false}) {
		t.Error("wrap text should not be active")
	}
	if !formatStyleActive(toolbarActionPercentFormat, document.CellStyle{NumberFormat: "0.00%"}) {
		t.Error("percent 0.00% should be active")
	}
	if !formatStyleActive(toolbarActionPercentFormat, document.CellStyle{NumberFormat: "0%"}) {
		t.Error("percent 0% should be active")
	}
	if formatStyleActive(toolbarActionPercentFormat, document.CellStyle{NumberFormat: "General"}) {
		t.Error("percent General should not be active")
	}
}

type mockRangeReader struct {
	cells map[string]string
}

func (m *mockRangeReader) ReadRange(sheet string, bounds document.CellRange) ([][]document.Cell, error) {
	rows := bounds.EndRow - bounds.StartRow + 1
	cols := bounds.EndColumn - bounds.StartColumn + 1
	res := make([][]document.Cell, rows)
	for r := 0; r < rows; r++ {
		res[r] = make([]document.Cell, cols)
		for c := 0; c < cols; c++ {
			ref := xlsx.CellName(bounds.StartColumn+c, bounds.StartRow+r)
			res[r][c] = document.Cell{Value: m.cells[ref]}
		}
	}
	return res, nil
}

func TestComputeAutoSumFormulaEdits(t *testing.T) {
	reader := &mockRangeReader{
		cells: map[string]string{
			"A1": "10",
			"A2": "20.5",
			"A3": "30",
			"B1": "100",
			"B2": "200",
			"B3": "300",
			"C1": "5",
			"D1": "15",
			"E1": "",
		},
	}

	// 1. Multi-cell multi-column selection (A1:B3 -> targets A4, B4)
	bounds := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 3, EndColumn: 2}
	edits := computeAutoSumFormulaEdits(reader, "Sheet1", bounds)
	if len(edits) != 2 {
		t.Fatalf("expected 2 edits, got %d: %+v", len(edits), edits)
	}
	if edits[0].Row != 4 || edits[0].Column != 1 || edits[0].Formula != "=SUM(A1:A3)" {
		t.Errorf("col A sum mismatch: %+v", edits[0])
	}
	if edits[1].Row != 4 || edits[1].Column != 2 || edits[1].Formula != "=SUM(B1:B3)" {
		t.Errorf("col B sum mismatch: %+v", edits[1])
	}

	// 2. Multi-cell with empty bottom row (A1:A4 where A4 is empty -> target is A4)
	bounds = document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 4, EndColumn: 1}
	edits = computeAutoSumFormulaEdits(reader, "Sheet1", bounds)
	if len(edits) != 1 || edits[0].Row != 4 || edits[0].Column != 1 || edits[0].Formula != "=SUM(A1:A3)" {
		t.Fatalf("empty bottom row sum mismatch: %+v", edits)
	}

	// 3. Single cell selection at A4 (empty, scans upwards to A1..A3)
	bounds = document.CellRange{StartRow: 4, StartColumn: 1, EndRow: 4, EndColumn: 1}
	edits = computeAutoSumFormulaEdits(reader, "Sheet1", bounds)
	if len(edits) != 1 || edits[0].Row != 4 || edits[0].Column != 1 || edits[0].Formula != "=SUM(A1:A3)" {
		t.Fatalf("single cell upward scan sum mismatch: %+v", edits)
	}

	// 4. Single-row multi-column selection (C1:D1 -> target is E1)
	bounds = document.CellRange{StartRow: 1, StartColumn: 3, EndRow: 1, EndColumn: 4}
	edits = computeAutoSumFormulaEdits(reader, "Sheet1", bounds)
	if len(edits) != 1 || edits[0].Row != 1 || edits[0].Column != 5 || edits[0].Formula != "=SUM(C1:D1)" {
		t.Fatalf("horizontal sum mismatch: %+v", edits)
	}

	// 5. Unimplemented actions check for Sum and Find
	if unimplementedSpreadsheetToolbarAction(toolbarActionSum) {
		t.Error("toolbarActionSum should not be unimplemented")
	}
	if unimplementedSpreadsheetToolbarAction(toolbarActionFind) {
		t.Error("toolbarActionFind should not be unimplemented")
	}
}

func TestComputeSpreadsheetAggregateFormulaEditsForAllMenuOperations(t *testing.T) {
	reader := &mockRangeReader{cells: map[string]string{
		"A1": "10", "A2": "20.5", "A3": "30",
		"B1": "100", "B2": "skip", "B3": "300",
		"C1": "5", "D1": "15",
	}}
	selection := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 3, EndColumn: 2}
	want := map[string][]string{
		"sum":     {"=SUM(A1:A3)", "=SUM(B1:B3)"},
		"average": {"=AVERAGE(A1:A3)", "=AVERAGE(B1:B3)"},
		"count":   {"=COUNT(A1:A3)", "=COUNT(B1:B3)"},
		"max":     {"=MAX(A1:A3)", "=MAX(B1:B3)"},
		"min":     {"=MIN(A1:A3)", "=MIN(B1:B3)"},
	}
	for operation, expected := range want {
		t.Run(operation, func(t *testing.T) {
			edits := computeSpreadsheetAggregateFormulaEdits(reader, "Sheet1", selection, operation)
			if len(edits) != len(expected) {
				t.Fatalf("aggregate edits = %+v; want %d output cells", edits, len(expected))
			}
			for index, value := range expected {
				if edits[index].Row != 4 || edits[index].Column != index+1 || edits[index].Formula != value {
					t.Errorf("edit %d = %+v; want row 4, column %d, formula %q", index, edits[index], index+1, value)
				}
			}
		})
	}

	horizontal := document.CellRange{StartRow: 1, StartColumn: 3, EndRow: 1, EndColumn: 4}
	for operation, expected := range map[string]string{
		"sum": "=SUM(C1:D1)", "average": "=AVERAGE(C1:D1)", "count": "=COUNT(C1:D1)", "max": "=MAX(C1:D1)", "min": "=MIN(C1:D1)",
	} {
		edits := computeSpreadsheetAggregateFormulaEdits(reader, "Sheet1", horizontal, operation)
		if len(edits) != 1 || edits[0].Row != 1 || edits[0].Column != 5 || edits[0].Formula != expected {
			t.Errorf("horizontal %s edits = %+v; want E1 = %q", operation, edits, expected)
		}
	}
	horizontalWithTrailingBlank := document.CellRange{StartRow: 1, StartColumn: 3, EndRow: 1, EndColumn: 5}
	trailingEdits := computeAutoSumFormulaEdits(reader, "Sheet1", horizontalWithTrailingBlank)
	if len(trailingEdits) != 1 || trailingEdits[0].Row != 1 || trailingEdits[0].Column != 5 || trailingEdits[0].Formula != "=SUM(C1:D1)" {
		t.Fatalf("horizontal trailing-blank AutoSum = %+v; want E1 = SUM(C1:D1)", trailingEdits)
	}
	for operation, expected := range map[string]string{
		"sum": "=SUM(A1:A3)", "average": "=AVERAGE(A1:A3)", "count": "=COUNT(A1:A3)", "max": "=MAX(A1:A3)", "min": "=MIN(A1:A3)",
	} {
		singleCell := document.CellRange{StartRow: 4, StartColumn: 1, EndRow: 4, EndColumn: 1}
		edits := computeSpreadsheetAggregateFormulaEdits(reader, "Sheet1", singleCell, operation)
		if len(edits) != 1 || edits[0].Row != 4 || edits[0].Column != 1 || edits[0].Formula != expected {
			t.Errorf("single-cell upward %s edits = %+v; want A4 = %q", operation, edits, expected)
		}
	}
}

func TestComputeSpreadsheetAggregateFormulaEditsUsesContiguousRunToTheLeft(t *testing.T) {
	reader := &mockRangeReader{cells: map[string]string{"A3": "12", "B3": "18", "A2": "header"}}
	selected := document.CellRange{StartRow: 3, StartColumn: 3, EndRow: 3, EndColumn: 3}
	edits := computeSpreadsheetAggregateFormulaEdits(reader, "Sheet1", selected, "sum")
	if len(edits) != 1 || edits[0].Row != 3 || edits[0].Column != 3 || edits[0].Formula != "=SUM(A3:B3)" {
		t.Fatalf("left-adjacent formula edits = %+v; want C3 = SUM(A3:B3)", edits)
	}
	if edits := computeSpreadsheetAggregateFormulaEdits(reader, "Sheet1", selected, "unknown"); len(edits) != 0 {
		t.Fatalf("unsupported aggregate produced formulas: %+v", edits)
	}
}

func TestSpreadsheetAutoSumMenuOffersImplementedOperationsOnly(t *testing.T) {
	menu := spreadsheetOfficeRibbonMenu(toolbarActionSum)
	want := []string{"excel-sum", "excel-average", "excel-count", "excel-max", "excel-min"}
	if len(menu.options) != len(want) {
		t.Fatalf("AutoSum menu has %d items; want exactly %d implemented operations: %+v", len(menu.options), len(want), menu.options)
	}
	for index, command := range want {
		if menu.options[index].command != command || !menu.options[index].enabled {
			t.Errorf("AutoSum menu item %d = %+v; want enabled command %q", index, menu.options[index], command)
		}
	}
}

func TestSpreadsheetAverageMenuCommandInsertsFormula(t *testing.T) {
	book := &mockSearcherBook{toolbarTestBook: toolbarTestBook{editable: true}}
	state := newDocumentSessionState("spreadsheet-average.xlsx")
	state.book = book
	state.sheet = "Sheet1"
	state.virtualGrid = newGridViewport()
	state.selectionSet = true
	state.selection = CellSelection{Anchor: CellPosition{Row: 0, Column: 0}, Active: CellPosition{Row: 1, Column: 0}}
	app := &App{documentSessionState: state, formatResults: make(chan spreadsheetFormatResult, 1)}

	app.executeOfficeRibbonMenuCommand(layout.Context{}, officeRibbonMenuSpreadsheet, "excel-average")
	select {
	case result := <-app.formatResults:
		if result.err != nil {
			t.Fatalf("Average command error: %v", result.err)
		}
		if !result.applied || result.noOp || result.details != "已插入平均值公式：A3" {
			t.Fatalf("Average command completion = applied %v noOp %v details %q", result.applied, result.noOp, result.details)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for average result")
	}
	if len(book.lastFormulas) != 1 || book.lastFormulas[0].Row != 3 || book.lastFormulas[0].Column != 1 || book.lastFormulas[0].Formula != "=AVERAGE(A1:A2)" {
		t.Fatalf("Average command batch = %+v; want A3 = AVERAGE(A1:A2)", book.lastFormulas)
	}
}

func TestParseNumericValueAcceptsCommonCurrencyAndAccountingDisplays(t *testing.T) {
	for _, item := range []struct {
		text string
		want float64
	}{
		{text: "¥1,234.50", want: 1234.5},
		{text: "￥1,234", want: 1234},
		{text: "$98.25", want: 98.25},
		{text: "(¥1,200.25)", want: -1200.25},
		{text: "(1,200.25)", want: -1200.25},
		{text: "12.5%", want: 0.125},
	} {
		got, ok := parseNumericValue(item.text)
		if !ok || math.Abs(got-item.want) > 1e-9 {
			t.Errorf("parseNumericValue(%q) = %v, %t; want %v, true", item.text, got, ok, item.want)
		}
	}
}

type mockSearcherBook struct {
	toolbarTestBook
	searchMatches []document.CellMatch
	lastSearch    string
	lastBatch     []document.CellEdit
	lastFormulas  []document.CellFormulaEdit
}

func (m *mockSearcherBook) SearchSheet(sheet, query string) ([]document.CellMatch, error) {
	m.lastSearch = query
	return m.searchMatches, nil
}

func (m *mockSearcherBook) ReadRange(sheet string, bounds document.CellRange) ([][]document.Cell, error) {
	return [][]document.Cell{
		{{Value: "10"}},
		{{Value: "20"}},
	}, nil
}

func (m *mockSearcherBook) SetCells(sheet string, edits []document.CellEdit) error {
	m.lastBatch = edits
	return nil
}

func (m *mockSearcherBook) SetCellFormulasChanged(sheet string, edits []document.CellFormulaEdit) (bool, error) {
	m.lastFormulas = append([]document.CellFormulaEdit(nil), edits...)
	return len(edits) > 0, nil
}

func TestSpreadsheetFindDialogAndAutoSumIntegration(t *testing.T) {
	book := &mockSearcherBook{
		toolbarTestBook: toolbarTestBook{editable: true},
		searchMatches: []document.CellMatch{
			{Row: 2, Column: 2, Value: "Apple"},
			{Row: 5, Column: 3, Value: "Apple Pie"},
		},
	}
	state := newDocumentSessionState("")
	state.book = book
	state.sheet = "Sheet1"
	state.virtualGrid = newGridViewport()
	state.selectionSet = true
	state.selection = CellSelection{Active: CellPosition{Row: 0, Column: 0}, Anchor: CellPosition{Row: 0, Column: 0}}
	app := &App{documentSessionState: state, formatResults: make(chan spreadsheetFormatResult, 10)}

	// Test Find enabled
	if !app.toolbarActionEnabled(toolbarActionFind) {
		t.Fatal("toolbarActionFind should be enabled")
	}
	if !app.toolbarActionEnabled(toolbarActionSum) {
		t.Fatal("toolbarActionSum should be enabled")
	}

	// Test Find execution
	app.showSpreadsheetFindDialog()
	if app.modal.Title != "查找和替换" {
		t.Fatalf("modal title = %q; want 查找和替换", app.modal.Title)
	}
	app.modal.Input.SetText("Apple")
	app.modal.OnAction("confirm")
	if book.lastSearch != "Apple" {
		t.Fatalf("search query = %q; want Apple", book.lastSearch)
	}
	if state.selection.Active.Row != 1 || state.selection.Active.Column != 1 {
		t.Fatalf("focused cell = (%d, %d); want (1, 1) for B2", state.selection.Active.Row, state.selection.Active.Column)
	}

	// Test AutoSum execution
	state.selection = CellSelection{Active: CellPosition{Row: 0, Column: 0}, Anchor: CellPosition{Row: 1, Column: 0}}
	app.applySpreadsheetAutoSum()
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("applyBatchEdits error: %v", res.err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for auto-sum result")
	}
	if len(book.lastFormulas) != 1 || book.lastFormulas[0].Formula != "=SUM(A1:A2)" {
		t.Fatalf("lastFormulas = %+v; want =SUM(A1:A2)", book.lastFormulas)
	}
}

func TestSpreadsheetFindPrefillsActiveCellAndSelectsQuery(t *testing.T) {
	state := newDocumentSessionState("spreadsheet-find-selected.xlsx")
	state.book = &mockSearcherBook{}
	state.sheet = "Sheet1"
	state.selectionSet = true
	state.selection = CellSelection{Active: CellPosition{Row: 1, Column: 1}, Anchor: CellPosition{Row: 1, Column: 1}}
	state.cellEdit.selectCell("B2", "活动内容")
	state.excelFindQuery = "上次查询"
	app := &App{documentSessionState: state}

	app.showSpreadsheetFindDialog()
	if got := app.modal.Input.Text(); got != "活动内容" {
		t.Fatalf("query=%q; want active-cell content", got)
	}
	if start, end := app.modal.Input.Selection(); start != 0 || end != len([]rune("活动内容")) {
		t.Fatalf("query selection=[%d,%d); want whole query selected", start, end)
	}
	if state.excelFindQuery != "上次查询" {
		t.Fatalf("opening Find changed the remembered query to %q", state.excelFindQuery)
	}
}

func TestSpreadsheetFindPrefillsFormulaResultInsteadOfFormula(t *testing.T) {
	state := newDocumentSessionState("spreadsheet-find-formula.xlsx")
	state.book = &mockSearcherBook{}
	state.sheet = "Sheet1"
	state.selectionSet = true
	state.selection = CellSelection{Active: CellPosition{Row: 1, Column: 1}, Anchor: CellPosition{Row: 1, Column: 1}}
	state.cellEdit.selectCell("B2", "=SUM(A1:A2)")
	state.virtualGrid = newGridViewport()
	state.virtualGrid.WindowSheet = "Sheet1"
	state.virtualGrid.WindowRange = document.CellRange{StartRow: 2, StartColumn: 2, EndRow: 2, EndColumn: 2}
	state.virtualGrid.Window = document.SheetWindow{
		StartRow: 2, StartColumn: 2,
		Cells: [][]document.Cell{{{Value: "结果", Formula: "SUM(A1:A2)"}}},
	}
	app := &App{documentSessionState: state}

	app.showSpreadsheetFindDialog()
	if got := app.modal.Input.Text(); got != "结果" {
		t.Fatalf("query=%q; want displayed formula result", got)
	}
}

func TestSpreadsheetFindKeepsRememberedQueryForEmptyActiveCell(t *testing.T) {
	state := newDocumentSessionState("spreadsheet-find-empty.xlsx")
	state.book = &mockSearcherBook{}
	state.sheet = "Sheet1"
	state.selectionSet = true
	state.selection = CellSelection{Active: CellPosition{Row: 0, Column: 0}, Anchor: CellPosition{Row: 0, Column: 0}}
	state.cellEdit.selectCell("A1", "")
	state.excelFindQuery = "上次查询"
	app := &App{documentSessionState: state}

	app.showSpreadsheetFindDialog()
	if got := app.modal.Input.Text(); got != "上次查询" {
		t.Fatalf("query=%q; want remembered query for an empty cell", got)
	}
	if start, end := app.modal.Input.Selection(); start != 0 || end != len([]rune("上次查询")) {
		t.Fatalf("query selection=[%d,%d); want whole query selected", start, end)
	}
}
