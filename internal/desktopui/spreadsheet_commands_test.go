package desktopui

import (
	"testing"
	"time"

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
	patch, ok := toolbarPickerPatch(pickerFillColor, 0)
	if !ok || patch.FillColor == nil || *patch.FillColor != "" {
		t.Fatalf("fill clear option=%+v, ok=%t", patch, ok)
	}
	patch, ok = toolbarPickerPatch(pickerNumberFormat, 3)
	if !ok || patch.NumberFormat == nil || *patch.NumberFormat != "yyyy-mm-dd" {
		t.Fatalf("date number format option=%+v, ok=%t", patch, ok)
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

func TestComputeAutoSumEdits(t *testing.T) {
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
	edits := computeAutoSumEdits(reader, "Sheet1", bounds)
	if len(edits) != 2 {
		t.Fatalf("expected 2 edits, got %d: %+v", len(edits), edits)
	}
	if edits[0].Row != 4 || edits[0].Column != 1 || edits[0].Value != "60.5" {
		t.Errorf("col A sum mismatch: %+v", edits[0])
	}
	if edits[1].Row != 4 || edits[1].Column != 2 || edits[1].Value != "600" {
		t.Errorf("col B sum mismatch: %+v", edits[1])
	}

	// 2. Multi-cell with empty bottom row (A1:A4 where A4 is empty -> target is A4)
	bounds = document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 4, EndColumn: 1}
	edits = computeAutoSumEdits(reader, "Sheet1", bounds)
	if len(edits) != 1 || edits[0].Row != 4 || edits[0].Column != 1 || edits[0].Value != "60.5" {
		t.Fatalf("empty bottom row sum mismatch: %+v", edits)
	}

	// 3. Single cell selection at A4 (empty, scans upwards to A1..A3)
	bounds = document.CellRange{StartRow: 4, StartColumn: 1, EndRow: 4, EndColumn: 1}
	edits = computeAutoSumEdits(reader, "Sheet1", bounds)
	if len(edits) != 1 || edits[0].Row != 4 || edits[0].Column != 1 || edits[0].Value != "60.5" {
		t.Fatalf("single cell upward scan sum mismatch: %+v", edits)
	}

	// 4. Single-row multi-column selection (C1:D1 -> target is E1)
	bounds = document.CellRange{StartRow: 1, StartColumn: 3, EndRow: 1, EndColumn: 4}
	edits = computeAutoSumEdits(reader, "Sheet1", bounds)
	if len(edits) != 1 || edits[0].Row != 1 || edits[0].Column != 5 || edits[0].Value != "20" {
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

type mockSearcherBook struct {
	toolbarTestBook
	searchMatches []document.CellMatch
	lastSearch    string
	lastBatch     []document.CellEdit
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
	if app.modal.Title != "查找" {
		t.Fatalf("modal title = %q; want 查找", app.modal.Title)
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
	if len(book.lastBatch) != 1 || book.lastBatch[0].Value != "30" {
		t.Fatalf("lastBatch = %+v; want sum of 30", book.lastBatch)
	}
}
