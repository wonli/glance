package desktopui

import (
	"testing"
	"time"

	"gioui.org/layout"
	"glance/internal/document"
	"glance/internal/format/xlsx"
)

func TestSpreadsheetMergeAndCenterRibbonConfirmsBeforeDiscardAndUnmerges(t *testing.T) {
	h := newFullGridTextHarness(t, false, "标题")
	app, state := h.app, h.app.documentSessionState
	book := state.book.(*xlsx.Workbook)
	if err := book.SetCellText(state.sheet, "A1", "标题"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellText(state.sheet, "B1", "副内容"); err != nil {
		t.Fatal(err)
	}
	state.cellEdit.cancel()
	state.selection = CellSelection{Anchor: CellPosition{Row: 0, Column: 0}, Active: CellPosition{Row: 0, Column: 1}}
	state.selectionSet = true
	state.selectedCell = "A1"
	app.selectedCell = "A1"
	app.formatResults = make(chan spreadsheetFormatResult, 4)

	var action *spreadsheetToolbarAction
	for index := range homeRibbonActions {
		if homeRibbonActions[index].label == "合并后居中" {
			action = &homeRibbonActions[index]
			break
		}
	}
	if action == nil {
		t.Fatal("Home ribbon must expose a functional Merge & Center action")
	}
	if unimplementedSpreadsheetToolbarAction(action.slot) || !app.toolbarActionEnabled(action.slot) {
		t.Fatal("Merge & Center should be implemented and enabled for a multi-cell selection")
	}
	state.toolbarTab = toolbarTabHome
	state.toolbarActionButtons[action.slot].Click()
	app.handleSpreadsheetToolbarEvents(layout.Context{})
	deadline := time.Now().Add(3 * time.Second)
	for !app.modal.Visible && time.Now().Before(deadline) {
		app.processSpreadsheetFormatResult()
		time.Sleep(time.Millisecond)
	}
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "合并单元格" {
		t.Fatalf("non-anchor content should open a merge confirmation, visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	if app.formatRunning.Load() {
		t.Fatal("showing merge confirmation must not mutate the workbook")
	}
	app.modal.OnAction("confirm")
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		app.processSpreadsheetFormatResult()
		if !app.formatRunning.Load() && len(app.formatResults) == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if app.formatRunning.Load() {
		t.Fatal("confirmed Merge & Center did not finish")
	}
	window, err := book.Window(state.sheet, 1, 1, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(window.Merges) != 1 || window.Merges[0].Range != (document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 2}) || window.Merges[0].Anchor.Value != "标题" {
		t.Fatalf("Merge & Center result = %#v; want merged A1:B1 with top-left content", window.Merges)
	}
	if got := window.Merges[0].Anchor.Style.HorizontalAlignment; got != "center" {
		t.Fatalf("merged anchor horizontal alignment = %q; want center", got)
	}

	state.toolbarActionButtons[action.slot].Click()
	app.handleSpreadsheetToolbarEvents(layout.Context{})
	waitForSpreadsheetMerge(t, app)
	window, err = book.Window(state.sheet, 1, 1, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(window.Merges) != 0 {
		t.Fatalf("clicking Merge & Center on the existing merged selection did not unmerge: %#v", window.Merges)
	}
}

func waitForSpreadsheetMerge(t *testing.T, app *App) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		app.processSpreadsheetFormatResult()
		if !app.formatRunning.Load() && len(app.formatResults) == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("spreadsheet merge operation timed out; status=%q", app.statusText())
}
