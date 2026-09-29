package desktopui

import (
	"testing"

	"glance/internal/document"
)

type toolbarTestBook struct{ editable bool }

func (*toolbarTestBook) SheetNames() []string                              { return []string{"Sheet1"} }
func (*toolbarTestBook) Range(string, int, int) ([][]document.Cell, error) { return nil, nil }
func (*toolbarTestBook) SetCellText(string, string, string) error          { return nil }
func (*toolbarTestBook) Save() error                                       { return nil }
func (*toolbarTestBook) Dirty() bool                                       { return false }
func (b *toolbarTestBook) Editable() bool                                  { return b.editable }
func (*toolbarTestBook) Close() error                                      { return nil }
func (*toolbarTestBook) Window(string, int, int, int, int) (document.SheetWindow, error) {
	return document.SheetWindow{}, nil
}
func (*toolbarTestBook) SetRowHeight(string, int, float64) error   { return nil }
func (*toolbarTestBook) SetColumnWidth(string, int, float64) error { return nil }

func TestSpreadsheetToolbarVisibilityRequiresEditableGridReader(t *testing.T) {
	state := newDocumentSessionState("")
	app := &App{documentSessionState: state}
	app.book = &toolbarTestBook{editable: true}
	if !app.spreadsheetToolbarVisible() {
		t.Fatal("editable grid reader should show the toolbar")
	}
	app.book = &toolbarTestBook{editable: false}
	if app.spreadsheetToolbarVisible() {
		t.Fatal("read-only workbook should not show editing controls")
	}
}

func TestSpreadsheetToolbarActionsCoverHomeAndDataGroups(t *testing.T) {
	if len(homeToolbarActions) != 22 || len(dataToolbarActions) != 3 {
		t.Fatalf("toolbar actions home=%d data=%d; want 22 and 3", len(homeToolbarActions), len(dataToolbarActions))
	}
	seen := map[int]bool{}
	for _, action := range append(append([]spreadsheetToolbarAction(nil), homeToolbarActions...), dataToolbarActions...) {
		if action.slot < 0 || action.slot >= 25 || seen[action.slot] {
			t.Fatalf("invalid or duplicate toolbar action slot %d", action.slot)
		}
		seen[action.slot] = true
	}
	for slot := 0; slot < 25; slot++ {
		if !seen[slot] {
			t.Fatalf("toolbar action slot %d is missing", slot)
		}
	}
}

func TestSpreadsheetToolbarActionsHaveIconGlyphsAndAccessibleLabels(t *testing.T) {
	actions := append(append([]spreadsheetToolbarAction(nil), homeToolbarActions...), dataToolbarActions...)
	for _, action := range actions {
		if action.icon == "" {
			t.Errorf("toolbar action %q has no icon", action.label)
		}
		if action.label == "" {
			t.Errorf("toolbar action slot %d has no accessible label", action.slot)
		}
		if _, ok := toolbarIconData(action.icon); !ok {
			t.Errorf("toolbar action %q references missing icon %q", action.label, action.icon)
		}
	}
}

func TestToolbarStyleUsesActiveCellAndMarksMixedSelection(t *testing.T) {
	state := newDocumentSessionState("")
	state.sheet = "Sheet1"
	state.selection = CellSelection{Anchor: CellPosition{Row: 0, Column: 0}, Active: CellPosition{Row: 0, Column: 1}}
	state.selectionSet = true
	state.virtualGrid = newGridViewport()
	state.virtualGrid.WindowSheet = "Sheet1"
	state.virtualGrid.WindowRange = document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 2}
	state.virtualGrid.Window = document.SheetWindow{StartRow: 1, StartColumn: 1, Cells: [][]document.Cell{{{Style: document.CellStyle{Bold: false, Italic: true}}, {Style: document.CellStyle{Bold: true, Italic: true}}}}}
	app := &App{documentSessionState: state}
	if got := app.selectedCellStyle(); !got.Bold {
		t.Fatal("toolbar should reflect the active cell's bold style")
	}
	if !app.selectionFormatMixed(4) {
		t.Fatal("bold should be marked mixed across the selection")
	}
	if app.selectionFormatMixed(5) {
		t.Fatal("uniform italic should not be marked mixed")
	}
}
