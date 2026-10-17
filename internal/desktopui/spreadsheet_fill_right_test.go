package desktopui

import (
	"testing"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"glance/internal/document"
	"glance/internal/format/xlsx"
)

func TestSpreadsheetFillRightShortcutCopiesLeftColumnAndKeepsSelection(t *testing.T) {
	h := newFullGridTextHarness(t, false, "10")
	a, state := h.app, h.app.documentSessionState
	book := state.book.(*xlsx.Workbook)
	for address, value := range map[string]string{"A1": "10", "A2": "label", "B1": "old-b1", "C1": "old-c1", "B2": "old-b2", "C2": "old-c2"} {
		if err := book.SetCellText(state.sheet, address, value); err != nil {
			t.Fatalf("SetCellText(%s): %v", address, err)
		}
	}
	if err := book.SetCellFormula(state.sheet, "A1", "=B1*2+$D$1"); err != nil {
		t.Fatalf("SetCellFormula(A1): %v", err)
	}
	state.selection = CellSelection{Anchor: CellPosition{Row: 1, Column: 2}, Active: CellPosition{Row: 0, Column: 0}}
	state.selectionSet = true
	state.selectedCell = "A1"
	a.selectedCell = "A1"
	state.cellEdit.cancel()
	state.cellEditor.SetText("")
	a.formatResults = make(chan spreadsheetFormatResult, 1)
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &state.virtualGrid.PointerTag})
		a.layoutVirtualGrid(gtx)
	}
	h.frame()
	h.router.Queue(key.Event{Name: key.Name("R"), State: key.Press, Modifiers: key.ModShortcut})
	h.frame()
	if !state.formatRunning.Load() {
		t.Fatal("Ctrl/Cmd+R did not start a fill-right operation")
	}
	select {
	case result := <-a.formatResults:
		if result.err != nil || !result.applied || result.operation != "向右填充" {
			t.Fatalf("fill-right result = %+v; want one successful fill operation", result)
		}
		a.formatResults <- result
		a.processSpreadsheetFormatResult()
	case <-time.After(5 * time.Second):
		t.Fatal("Ctrl/Cmd+R did not finish filling right")
	}
	for address, want := range map[string]string{"A2": "label", "B2": "label", "C2": "label"} {
		if got, err := book.Value(state.sheet, address); err != nil || got != want {
			t.Errorf("%s value after fill right = %q, %v; want %q", address, got, err, want)
		}
	}
	for address, want := range map[string]string{"A1": "B1*2+$D$1", "B1": "C1*2+$D$1", "C1": "D1*2+$D$1"} {
		if got, err := book.GetCellFormula(state.sheet, address); err != nil || got != want {
			t.Errorf("%s formula after fill right = %q, %v; want %q", address, got, err, want)
		}
	}
	if got, want := state.selection.Bounds(), (document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 2, EndColumn: 3}); got != want {
		t.Fatalf("selection after fill right = %+v; want %+v", got, want)
	}
}
