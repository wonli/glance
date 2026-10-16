package desktopui

import (
	"io"
	"strings"
	"testing"
	"time"

	"gioui.org/io/key"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"glance/internal/document"
	"glance/internal/format/xlsx"
)

func TestSpreadsheetClipboardKeyboardShortcutsCopyCutAndPasteSelection(t *testing.T) {
	h := newFullGridTextHarness(t, false, "")
	a := h.app
	state := a.documentSessionState
	book := a.book.(*xlsx.Workbook)
	if err := book.SetCellText(a.sheet, "A1", "甲"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellText(a.sheet, "B1", "乙"); err != nil {
		t.Fatal(err)
	}
	book.RebaseHistory()
	state.cellEdit.selectCell("A1", "甲")
	state.selectedCell = "A1"
	state.selection = CellSelection{Anchor: CellPosition{Row: 0, Column: 0}, Active: CellPosition{Row: 0, Column: 1}}
	state.selectionSet = true
	a.formatResults = make(chan spreadsheetFormatResult, 4)
	h.draw = func(gtx layout.Context) {
		a.handleOfficeShortcuts(gtx)
		a.layoutVirtualGrid(gtx)
	}
	h.frame()

	h.router.Queue(key.Event{Name: key.Name("C"), State: key.Press, Modifiers: key.ModShortcut})
	h.frame()
	_, copied, ok := h.router.WriteClipboard()
	if !ok || string(copied) != "甲\t乙\n" {
		t.Fatalf("Ctrl/Cmd+C clipboard=%q, available=%v; want TSV for both selected cells", copied, ok)
	}

	h.router.Queue(key.Event{Name: key.Name("X"), State: key.Press, Modifiers: key.ModShortcut})
	h.frame()
	_, cut, ok := h.router.WriteClipboard()
	if !ok || string(cut) != "甲\t乙\n" {
		t.Fatalf("Ctrl/Cmd+X clipboard=%q, available=%v; want the selected TSV before clearing", cut, ok)
	}
	waitForSpreadsheetClipboardMutation(t, a, "剪切")
	assertSpreadsheetCellValue(t, book, a.sheet, "A1", "")
	assertSpreadsheetCellValue(t, book, a.sheet, "B1", "")

	h.router.Queue(key.Event{Name: key.Name("V"), State: key.Press, Modifiers: key.ModShortcut})
	h.frame()
	if !h.router.ClipboardRequested() {
		t.Fatal("Ctrl/Cmd+V did not request clipboard data")
	}
	h.router.Queue(transfer.DataEvent{
		Type: "application/text",
		Open: func() io.ReadCloser { return io.NopCloser(strings.NewReader("新\t值")) },
	})
	h.frame()
	waitForSpreadsheetClipboardMutation(t, a, "粘贴")
	assertSpreadsheetCellValue(t, book, a.sheet, "A1", "新")
	assertSpreadsheetCellValue(t, book, a.sheet, "B1", "值")
}

func TestSpreadsheetClipboardSameWorkbookPastePreservesFormulaAndStyle(t *testing.T) {
	h := newFullGridTextHarness(t, false, "")
	a := h.app
	a.desktopRuntime = &desktopRuntime{}
	state := a.documentSessionState
	book := a.book.(*xlsx.Workbook)
	if err := book.SetCellFormula(a.sheet, "A1", "B1+$C$1"); err != nil {
		t.Fatal(err)
	}
	if _, err := book.SetCellValueChanged(a.sheet, "B1", document.CellValue{Kind: document.CellValueNumber, Number: 42.5}); err != nil {
		t.Fatal(err)
	}
	bold := true
	if _, err := book.SetCellFormatsChanged(a.sheet, document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 2}, document.CellFormatPatch{Bold: &bold}); err != nil {
		t.Fatal(err)
	}
	book.RebaseHistory()
	state.selection = CellSelection{Anchor: CellPosition{Row: 0, Column: 0}, Active: CellPosition{Row: 0, Column: 1}}
	state.selectionSet = true
	text, err := a.copySelection()
	if err != nil {
		t.Fatalf("copySelection: %v", err)
	}
	if text != "\t42.5\n" {
		t.Fatalf("system clipboard text=%q; want plain TSV", text)
	}
	a.formatResults = make(chan spreadsheetFormatResult, 1)
	if err := a.pasteClipboardAt(a.sheet, document.CellRange{StartRow: 3, StartColumn: 4, EndRow: 3, EndColumn: 4}, text); err != nil {
		t.Fatalf("pasteClipboardAt: %v", err)
	}
	waitForSpreadsheetClipboardMutation(t, a, "粘贴")
	pasted, err := book.ReadRange(a.sheet, document.CellRange{StartRow: 3, StartColumn: 4, EndRow: 3, EndColumn: 5})
	if err != nil {
		t.Fatal(err)
	}
	if pasted[0][0].Formula != "E3+$C$1" || !pasted[0][0].Style.Bold {
		t.Fatalf("pasted formula/style=%+v; want adjusted formula and bold style", pasted[0][0])
	}
	if pasted[0][1].TypedValue == nil || pasted[0][1].TypedValue.Kind != document.CellValueNumber || pasted[0][1].TypedValue.Number != 42.5 || !pasted[0][1].Style.Bold {
		t.Fatalf("pasted typed value/style=%+v; want numeric 42.5 and bold style", pasted[0][1])
	}
	if changed, err := book.Undo(); err != nil || !changed {
		t.Fatalf("single undo changed=%v err=%v", changed, err)
	}
	cleared, err := book.ReadRange(a.sheet, document.CellRange{StartRow: 3, StartColumn: 4, EndRow: 3, EndColumn: 5})
	if err != nil || cleared[0][0].Formula != "" || cleared[0][0].Value != "" || cleared[0][0].StyleID != 0 || cleared[0][1].Value != "" || cleared[0][1].StyleID != 0 {
		t.Fatalf("undo failed to restore both pasted cells: cells=%+v err=%v", cleared, err)
	}
}

func TestSpreadsheetClipboardPasteUsesSelectionCapturedAtRequest(t *testing.T) {
	h := newFullGridTextHarness(t, false, "")
	a := h.app
	state := a.documentSessionState
	book := a.book.(*xlsx.Workbook)
	if err := book.SetCellText(a.sheet, "A1", "原目标"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellText(a.sheet, "B1", "后来选中"); err != nil {
		t.Fatal(err)
	}
	book.RebaseHistory()
	state.cellEdit.selectCell("A1", "原目标")
	state.selectedCell = "A1"
	state.selection = CellSelection{Anchor: CellPosition{Row: 0, Column: 0}, Active: CellPosition{Row: 0, Column: 0}}
	state.selectionSet = true
	a.formatResults = make(chan spreadsheetFormatResult, 1)
	h.draw = func(gtx layout.Context) {
		a.handleOfficeShortcuts(gtx)
		a.layoutVirtualGrid(gtx)
	}
	h.frame()

	h.router.Queue(key.Event{Name: key.Name("V"), State: key.Press, Modifiers: key.ModShortcut})
	h.frame()
	if !h.router.ClipboardRequested() {
		t.Fatal("Ctrl/Cmd+V did not request clipboard data")
	}

	state.cellEdit.selectCell("B1", "后来选中")
	state.selectedCell = "B1"
	state.selection = CellSelection{Anchor: CellPosition{Row: 0, Column: 1}, Active: CellPosition{Row: 0, Column: 1}}
	h.router.Queue(transfer.DataEvent{
		Type: "application/text",
		Open: func() io.ReadCloser { return io.NopCloser(strings.NewReader("剪贴板内容")) },
	})
	h.frame()
	waitForSpreadsheetClipboardMutation(t, a, "粘贴")

	assertSpreadsheetCellValue(t, book, a.sheet, "A1", "剪贴板内容")
	assertSpreadsheetCellValue(t, book, a.sheet, "B1", "后来选中")
}

func TestSpreadsheetClipboardPasteUsesWorksheetCapturedAtRequest(t *testing.T) {
	h := newFullGridTextHarness(t, false, "")
	a := h.app
	state := a.documentSessionState
	book := a.book.(*xlsx.Workbook)
	initialSheet := state.sheet
	if err := book.SetCellText(initialSheet, "A1", "原目标"); err != nil {
		t.Fatal(err)
	}
	if err := book.EditStructure(document.StructureEdit{Kind: document.StructureCreateSheet}); err != nil {
		t.Fatal(err)
	}
	otherSheet := book.SheetNames()[1]
	if err := book.SetCellText(otherSheet, "B1", "后来选中"); err != nil {
		t.Fatal(err)
	}
	book.RebaseHistory()
	state.cellEdit.selectCell("A1", "原目标")
	state.selectedCell = "A1"
	state.selection = CellSelection{Anchor: CellPosition{Row: 0, Column: 0}, Active: CellPosition{Row: 0, Column: 0}}
	state.selectionSet = true
	a.formatResults = make(chan spreadsheetFormatResult, 1)
	h.draw = func(gtx layout.Context) {
		a.handleOfficeShortcuts(gtx)
		a.layoutVirtualGrid(gtx)
	}
	h.frame()

	h.router.Queue(key.Event{Name: key.Name("V"), State: key.Press, Modifiers: key.ModShortcut})
	h.frame()
	if !h.router.ClipboardRequested() {
		t.Fatal("Ctrl/Cmd+V did not request clipboard data")
	}

	state.sheet = otherSheet
	state.sheets = book.SheetNames()
	state.cellEdit.selectCell("B1", "后来选中")
	state.selectedCell = "B1"
	state.selection = CellSelection{Anchor: CellPosition{Row: 0, Column: 1}, Active: CellPosition{Row: 0, Column: 1}}
	h.router.Queue(transfer.DataEvent{
		Type: "application/text",
		Open: func() io.ReadCloser { return io.NopCloser(strings.NewReader("剪贴板内容")) },
	})
	h.frame()
	waitForSpreadsheetClipboardMutation(t, a, "粘贴")

	assertSpreadsheetCellValue(t, book, initialSheet, "A1", "剪贴板内容")
	assertSpreadsheetCellValue(t, book, otherSheet, "B1", "后来选中")
}

func TestSpreadsheetRibbonPasteTextUsesSelectionCapturedAtRequest(t *testing.T) {
	h := newFullGridTextHarness(t, false, "")
	a := h.app
	state := a.documentSessionState
	book := a.book.(*xlsx.Workbook)
	if err := book.SetCellText(a.sheet, "A1", "原目标"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellText(a.sheet, "B1", "后来选中"); err != nil {
		t.Fatal(err)
	}
	book.RebaseHistory()
	state.cellEdit.selectCell("A1", "原目标")
	state.selectedCell = "A1"
	state.selection = CellSelection{Anchor: CellPosition{Row: 0, Column: 0}, Active: CellPosition{Row: 0, Column: 0}}
	state.selectionSet = true
	a.formatResults = make(chan spreadsheetFormatResult, 1)
	requested := false
	h.draw = func(gtx layout.Context) {
		if !requested {
			a.executeOfficeRibbonMenuCommand(gtx, officeRibbonMenuSpreadsheet, "excel-paste-text")
			requested = true
		}
		a.layoutVirtualGrid(gtx)
	}
	h.frame()
	if !h.router.ClipboardRequested() {
		t.Fatal("Ribbon Paste Text did not request clipboard data")
	}

	state.cellEdit.selectCell("B1", "后来选中")
	state.selectedCell = "B1"
	state.selection = CellSelection{Anchor: CellPosition{Row: 0, Column: 1}, Active: CellPosition{Row: 0, Column: 1}}
	h.router.Queue(transfer.DataEvent{
		Type: "application/text",
		Open: func() io.ReadCloser { return io.NopCloser(strings.NewReader("剪贴板内容")) },
	})
	h.frame()
	waitForSpreadsheetClipboardMutation(t, a, "粘贴")

	assertSpreadsheetCellValue(t, book, a.sheet, "A1", "剪贴板内容")
	assertSpreadsheetCellValue(t, book, a.sheet, "B1", "后来选中")
}

func TestSpreadsheetClipboardShortcutsDeferToCellEditor(t *testing.T) {
	h := newFullGridTextHarness(t, false, "单元格文字")
	a := h.app
	state := a.documentSessionState
	book := a.book.(*xlsx.Workbook)
	if err := book.SetCellText(a.sheet, "A1", "单元格文字"); err != nil {
		t.Fatal(err)
	}
	state.cellEdit.selectCell("A1", "单元格文字")
	state.cellEdit.begin()
	state.cellEditor.SetText("单元格文字")
	state.selectedCell = "A1"
	state.selection = CellSelection{Anchor: CellPosition{Row: 0, Column: 0}, Active: CellPosition{Row: 0, Column: 0}}
	state.selectionSet = true
	a.formatResults = make(chan spreadsheetFormatResult, 1)
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &a.cellEditor})
		a.handleOfficeShortcuts(gtx)
		a.layoutVirtualGrid(gtx)
	}
	h.frame()
	h.router.Queue(key.Event{Name: key.Name("X"), State: key.Press, Modifiers: key.ModShortcut})
	h.frame()
	if state.formatRunning.Load() {
		t.Fatal("Ctrl/Cmd+X started a worksheet cut while the cell editor was active")
	}
	assertSpreadsheetCellValue(t, book, a.sheet, "A1", "单元格文字")
}

func TestSpreadsheetClipboardShortcutsDeferToFormulaEditor(t *testing.T) {
	h := newFullGridTextHarness(t, true, "单元格文字")
	a := h.app
	state := a.documentSessionState
	book := a.book.(*xlsx.Workbook)
	if err := book.SetCellText(a.sheet, "A1", "单元格文字"); err != nil {
		t.Fatal(err)
	}
	state.cellEdit.selectCell("A1", "单元格文字")
	state.selectedCell = "A1"
	state.selection = CellSelection{Anchor: CellPosition{Row: 0, Column: 0}, Active: CellPosition{Row: 0, Column: 0}}
	state.selectionSet = true
	state.formulaEditor.SetText("单元格文字")
	a.formatResults = make(chan spreadsheetFormatResult, 1)
	h.draw = func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: &a.formulaEditor})
		a.handleOfficeShortcuts(gtx)
		a.layoutGridControls(gtx)
	}
	h.frame()
	h.router.Queue(key.Event{Name: key.Name("X"), State: key.Press, Modifiers: key.ModShortcut})
	h.frame()
	if state.formatRunning.Load() {
		t.Fatal("Ctrl/Cmd+X started a worksheet cut while the formula editor was active")
	}
	assertSpreadsheetCellValue(t, book, a.sheet, "A1", "单元格文字")
}

func TestSpreadsheetClipboardShortcutsDeferToRibbonPopup(t *testing.T) {
	h := newFullGridTextHarness(t, false, "")
	a := h.app
	state := a.documentSessionState
	book := a.book.(*xlsx.Workbook)
	if err := book.SetCellText(a.sheet, "A1", "甲"); err != nil {
		t.Fatal(err)
	}
	state.cellEdit.selectCell("A1", "甲")
	state.selectedCell = "A1"
	state.selection = CellSelection{Anchor: CellPosition{Row: 0, Column: 0}, Active: CellPosition{Row: 0, Column: 0}}
	state.selectionSet = true
	state.toolbarPicker = pickerAlignment
	beforeGeneration := state.operationGeneration
	h.draw = func(gtx layout.Context) { a.handleOfficeShortcuts(gtx) }
	h.frame()
	h.router.Queue(key.Event{Name: key.Name("X"), State: key.Press, Modifiers: key.ModShortcut})
	h.frame()
	if state.operationGeneration != beforeGeneration {
		t.Fatal("Ctrl/Cmd+X started a worksheet cut while a Ribbon picker was open")
	}
	assertSpreadsheetCellValue(t, book, a.sheet, "A1", "甲")
}

func waitForSpreadsheetClipboardMutation(t *testing.T, a *App, operation string) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case result := <-a.formatResults:
			if result.err != nil {
				t.Fatalf("%s shortcut failed: %v", operation, result.err)
			}
			if result.operation != operation || !result.applied {
				t.Fatalf("clipboard result=%+v; want applied %q", result, operation)
			}
			return
		case <-deadline.C:
			t.Fatalf("%s shortcut did not finish", operation)
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func assertSpreadsheetCellValue(t *testing.T, book *xlsx.Workbook, sheet, address, want string) {
	t.Helper()
	window, err := book.Window(sheet, 1, 1, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	column := 0
	if address == "B1" {
		column = 1
	}
	if got := window.Cells[0][column].Value; got != want {
		t.Fatalf("%s=%q; want %q", address, got, want)
	}
}
