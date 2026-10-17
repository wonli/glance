package desktopui

import (
	"runtime"
	"testing"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"glance/internal/document"
	"glance/internal/format/xlsx"
)

func TestSpreadsheetDateShortcutWritesStaticDateToActiveCell(t *testing.T) {
	h := newFullGridTextHarness(t, false, "")
	a, state := h.app, h.app.documentSessionState
	book := state.book.(*xlsx.Workbook)
	state.selection = CellSelection{Anchor: CellPosition{Row: 2, Column: 1}, Active: CellPosition{Row: 2, Column: 1}}
	state.selectionSet = true
	state.selection.Scope = selectionCells
	state.selectedCell = "B3"
	a.selectedCell = "B3"
	state.cellEdit.cancel()
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	a.desktopRuntime = desktop
	desktop.registerSession(state.sharedDocumentSession)
	a.documentID = state.documentID
	a.formatResults = make(chan spreadsheetFormatResult, 1)
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &state.virtualGrid.PointerTag})
		a.layoutVirtualGrid(gtx)
	}
	h.frame()
	h.router.Queue(key.Event{Name: key.Name(";"), State: key.Press, Modifiers: key.ModCtrl})
	h.frame()
	if !state.formatRunning.Load() {
		t.Fatal("Control+; did not start a date insertion operation")
	}
	select {
	case result := <-a.formatResults:
		if result.err != nil || !result.applied || result.operation != "插入日期" {
			t.Fatalf("date insertion result = %+v; want one successful date operation", result)
		}
		a.formatResults <- result
		a.processSpreadsheetFormatResult()
	case <-time.After(5 * time.Second):
		t.Fatal("Control+; did not finish inserting the date")
	}
	if got, err := book.Value(state.sheet, "A1"); err != nil || got != "" {
		t.Fatalf("A1 changed to %q, %v; want untouched", got, err)
	}
	window, err := book.Window(state.sheet, 3, 2, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := window.Cells[0][0].Value; got == "" {
		t.Fatalf("B3 value is empty; want today's date")
	}
	if got := window.Cells[0][0].Style.NumberFormat; got != "m/d/yy" {
		t.Fatalf("B3 display format = %q; want default date format", got)
	}
	if !state.virtualGrid.PendingSelectionLoad {
		t.Fatal("date insertion did not schedule a refreshed grid window")
	}
	if got, want := state.selection.Bounds(), (document.CellRange{StartRow: 3, StartColumn: 2, EndRow: 3, EndColumn: 2}); got != want {
		t.Fatalf("active cell selection changed to %+v; want %+v", got, want)
	}
	if !a.applicationSessionRegistered(a.documentID) {
		t.Fatal("date shortcut did not route through the registered application session")
	}
}

func TestSpreadsheetDateShortcutUsesControlOnMacOS(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Excel for macOS also supports Control+;")
	}
	h := newFullGridTextHarness(t, false, "")
	a, state := h.app, h.app.documentSessionState
	state.cellEdit.cancel()
	a.formatResults = make(chan spreadsheetFormatResult, 1)
	if !a.handleSpreadsheetGridNavigationKey(layout.Context{}, state.virtualGrid, key.Event{
		Name: key.Name(";"), State: key.Press, Modifiers: key.ModCtrl,
	}) {
		t.Fatal("Control+; was not consumed on macOS")
	}
	select {
	case result := <-a.formatResults:
		if result.err != nil || result.operation != "插入日期" {
			t.Fatalf("Control+; result = %+v; want a successful date operation", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Control+; did not finish inserting the date")
	}
}

func TestSpreadsheetDateShortcutDefersToCellEditor(t *testing.T) {
	h := newFullGridTextHarness(t, false, "")
	a, state := h.app, h.app.documentSessionState
	state.cellEdit.Editing = true
	if a.handleSpreadsheetGridNavigationKey(layout.Context{}, state.virtualGrid, key.Event{
		Name: key.Name(";"), State: key.Press, Modifiers: key.ModShortcut,
	}) {
		t.Fatal("date shortcut was consumed while the cell editor was active")
	}
	if state.formatRunning.Load() {
		t.Fatal("date shortcut started a workbook operation while the cell editor was active")
	}
}
