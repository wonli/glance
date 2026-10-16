package desktopui

import (
	"testing"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"glance/internal/format/xlsx"
)

func TestSpreadsheetHistoryKeyboardShortcuts(t *testing.T) {
	h := newFullGridTextHarness(t, false, "after")
	a := h.app
	state := a.documentSessionState
	book := a.book.(*xlsx.Workbook)
	if err := book.SetCellText(a.sheet, "A1", "before"); err != nil {
		t.Fatal(err)
	}
	book.RebaseHistory()
	if err := book.SetCellText(a.sheet, "A1", "after"); err != nil {
		t.Fatal(err)
	}
	state.cellEdit.selectCell("A1", "after")
	state.cellEditor.SetText("after")
	a.formatResults = make(chan spreadsheetFormatResult, 1)
	h.draw = func(gtx layout.Context) {
		a.handleOfficeShortcuts(gtx)
	}
	h.frame()

	press := func(name string, modifiers key.Modifiers, want string) {
		t.Helper()
		beforeGeneration := state.operationGeneration
		h.router.Queue(key.Event{Name: key.Name(name), State: key.Press, Modifiers: modifiers})
		h.frame()
		if state.operationGeneration == beforeGeneration {
			t.Fatalf("%s with modifiers %v did not start workbook history", name, modifiers)
		}
		waitForSpreadsheetHistoryShortcut(t, a)
		window, err := book.Window(a.sheet, 1, 1, 1, 1)
		if err != nil {
			t.Fatal(err)
		}
		if got := window.Cells[0][0].Value; got != want {
			t.Fatalf("A1 after %s with modifiers %v = %q; want %q", name, modifiers, got, want)
		}
	}

	press("Z", key.ModShortcut, "before")
	press("Y", key.ModShortcut, "after")
	press("Z", key.ModShortcut, "before")
	press("Z", key.ModShortcut|key.ModShift, "after")
}

func TestSpreadsheetHistoryShortcutsDeferToCellEditor(t *testing.T) {
	h := newFullGridTextHarness(t, false, "after")
	a := h.app
	state := a.documentSessionState
	book := a.book.(*xlsx.Workbook)
	if err := book.SetCellText(a.sheet, "A1", "before"); err != nil {
		t.Fatal(err)
	}
	book.RebaseHistory()
	if err := book.SetCellText(a.sheet, "A1", "after"); err != nil {
		t.Fatal(err)
	}
	state.cellEdit.selectCell("A1", "after")
	state.cellEdit.begin()
	state.cellEditor.SetText("after")
	a.formatResults = make(chan spreadsheetFormatResult, 1)
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &a.cellEditor})
		a.handleOfficeShortcuts(gtx)
		a.layoutVirtualGrid(gtx)
	}
	h.frame()
	h.router.Queue(key.Event{Name: key.Name("Z"), State: key.Press, Modifiers: key.ModShortcut})
	h.frame()
	if state.formatRunning.Load() {
		t.Fatal("undo shortcut started workbook history while the cell editor was active")
	}
	window, err := book.Window(a.sheet, 1, 1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := window.Cells[0][0].Value; got != "after" {
		t.Fatalf("A1 = %q; want workbook history left untouched while editing", got)
	}
}

func TestSpreadsheetHistoryShortcutsDeferToFormulaEditor(t *testing.T) {
	h := newFullGridTextHarness(t, true, "after")
	a := h.app
	state := a.documentSessionState
	book := a.book.(*xlsx.Workbook)
	if err := book.SetCellText(a.sheet, "A1", "before"); err != nil {
		t.Fatal(err)
	}
	book.RebaseHistory()
	if err := book.SetCellText(a.sheet, "A1", "after"); err != nil {
		t.Fatal(err)
	}
	state.cellEdit.selectCell("A1", "after")
	state.formulaEditor.SetText("after")
	a.formatResults = make(chan spreadsheetFormatResult, 1)
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &a.formulaEditor})
		a.handleOfficeShortcuts(gtx)
		a.layoutGridControls(gtx)
	}
	h.frame()
	h.router.Queue(key.Event{Name: key.Name("Z"), State: key.Press, Modifiers: key.ModShortcut})
	h.frame()
	if state.formatRunning.Load() {
		t.Fatal("undo shortcut started workbook history while the formula editor was active")
	}
	window, err := book.Window(a.sheet, 1, 1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := window.Cells[0][0].Value; got != "after" {
		t.Fatalf("A1 = %q; want workbook history left untouched in formula editor", got)
	}
}

func TestSpreadsheetHistoryShortcutsDeferToRibbonPopup(t *testing.T) {
	h := newFullGridTextHarness(t, false, "after")
	a := h.app
	state := a.documentSessionState
	book := a.book.(*xlsx.Workbook)
	if err := book.SetCellText(a.sheet, "A1", "before"); err != nil {
		t.Fatal(err)
	}
	book.RebaseHistory()
	if err := book.SetCellText(a.sheet, "A1", "after"); err != nil {
		t.Fatal(err)
	}
	state.cellEdit.selectCell("A1", "after")
	state.toolbarPicker = pickerAlignment
	beforeGeneration := state.operationGeneration
	h.draw = func(gtx layout.Context) { a.handleOfficeShortcuts(gtx) }
	h.frame()
	h.router.Queue(key.Event{Name: key.Name("Z"), State: key.Press, Modifiers: key.ModShortcut})
	h.frame()
	if state.operationGeneration != beforeGeneration {
		t.Fatal("undo shortcut took input while a Ribbon picker was open")
	}
	window, err := book.Window(a.sheet, 1, 1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := window.Cells[0][0].Value; got != "after" {
		t.Fatalf("A1 = %q; want history untouched while a Ribbon picker is open", got)
	}
}

func waitForSpreadsheetHistoryShortcut(t *testing.T, a *App) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case result := <-a.formatResults:
			if result.err != nil {
				t.Fatalf("spreadsheet history shortcut failed: %v", result.err)
			}
			if !result.history || !result.applied {
				t.Fatalf("history result = %+v; want an applied history operation", result)
			}
			a.formatResults <- result
			a.processSpreadsheetFormatResult()
			return
		case <-deadline.C:
			t.Fatal("spreadsheet history shortcut did not finish")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}
