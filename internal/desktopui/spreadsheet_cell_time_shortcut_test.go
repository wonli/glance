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

func TestSpreadsheetTimeShortcutWritesStaticTimeToActiveCell(t *testing.T) {
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
	name, modifiers := key.Name(":"), key.ModShortcut|key.ModShift
	if runtime.GOOS == "darwin" {
		name, modifiers = key.Name(";"), key.ModShortcut
	}
	h.router.Queue(key.Event{Name: name, State: key.Press, Modifiers: modifiers})
	h.frame()
	if !state.formatRunning.Load() {
		t.Fatal("Excel's current-time shortcut did not start a time insertion operation")
	}
	select {
	case result := <-a.formatResults:
		if result.err != nil || !result.applied || result.operation != "插入时间" {
			t.Fatalf("time insertion result = %+v; want one successful time operation", result)
		}
		a.formatResults <- result
		a.processSpreadsheetFormatResult()
	case <-time.After(5 * time.Second):
		t.Fatal("current-time shortcut did not finish inserting a time")
	}
	if got, err := book.Value(state.sheet, "A1"); err != nil || got != "" {
		t.Fatalf("A1 changed to %q, %v; want untouched", got, err)
	}
	window, err := book.Window(state.sheet, 3, 2, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := window.Cells[0][0].Value; got == "" {
		t.Fatal("B3 value is empty; want a static current time")
	}
	if got := window.Cells[0][0].Style.NumberFormat; got != "h:mm" {
		t.Fatalf("B3 display format = %q; want the built-in 24-hour time format", got)
	}
	if !state.virtualGrid.PendingSelectionLoad {
		t.Fatal("time insertion did not schedule a refreshed grid window")
	}
	if got, want := state.selection.Bounds(), (document.CellRange{StartRow: 3, StartColumn: 2, EndRow: 3, EndColumn: 2}); got != want {
		t.Fatalf("active cell selection changed to %+v; want %+v", got, want)
	}
	if !a.applicationSessionRegistered(a.documentID) {
		t.Fatal("time shortcut did not route through the registered application session")
	}
}

func TestSpreadsheetDateAndTimeShortcutBindings(t *testing.T) {
	if !spreadsheetDateShortcut(key.Name(";"), key.ModCtrl) {
		t.Fatal("Control+; should insert the current date")
	}
	commandModifier := key.ModCommand
	if runtime.GOOS != "darwin" {
		commandModifier = key.ModCtrl | key.ModShift
	}
	if spreadsheetDateShortcut(key.Name(";"), commandModifier) {
		t.Fatal("time shortcut must not insert the date")
	}
	if !spreadsheetTimeShortcut(key.Name(";"), key.ModShortcut, "darwin") {
		t.Fatal("Command+; should insert the current time on macOS")
	}
	if !spreadsheetTimeShortcut(key.Name(":"), key.ModCtrl|key.ModShift, "windows") {
		t.Fatal("Ctrl+Shift+; should insert the current time on Windows")
	}
	if !spreadsheetTimeShortcut(key.Name(";"), key.ModCtrl|key.ModShift, "linux") {
		t.Fatal("Ctrl+Shift+; should insert the current time on Linux")
	}
}
