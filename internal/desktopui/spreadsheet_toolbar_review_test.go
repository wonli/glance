package desktopui

import (
	"strings"
	"testing"
	"time"

	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/format/xlsx"
)

func TestSpreadsheetToolbar_SpellCheckAndTranslate(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	_ = wb.SetCellText(sheet, "A1", "Hello World")

	state := newDocumentSessionState("test_review.xlsx")
	state.book = wb
	state.sheets = wb.SheetNames()
	state.sheet = sheet
	state.virtualGrid = newGridViewport()
	state.selection = CellSelection{
		Anchor: CellPosition{Row: 0, Column: 0},
		Active: CellPosition{Row: 0, Column: 0},
	}
	state.selectionSet = true
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	// 1. Spell Check
	if unimplementedSpreadsheetToolbarAction(toolbarActionSpellCheck) {
		t.Fatal("toolbarActionSpellCheck should not be unimplemented")
	}
	if !app.toolbarActionEnabled(toolbarActionSpellCheck) {
		t.Fatal("toolbarActionSpellCheck should be enabled")
	}
	app.showSpreadsheetSpellCheckDialog()
	if !app.modal.Visible || app.modal.Title != "拼写检查" {
		t.Fatalf("expected 拼写检查 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("confirm")
	if !strings.Contains(app.status, "拼写检查完成") {
		t.Fatalf("unexpected status after spell check: %q", app.status)
	}

	// 2. Translate
	if unimplementedSpreadsheetToolbarAction(toolbarActionTranslate) {
		t.Fatal("toolbarActionTranslate should not be unimplemented")
	}
	if !app.toolbarActionEnabled(toolbarActionTranslate) {
		t.Fatal("toolbarActionTranslate should be enabled")
	}
	app.showSpreadsheetTranslateDialog()
	if !app.modal.Visible || app.modal.Title != "智能翻译" {
		t.Fatalf("expected 智能翻译 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.SecondInput.SetText("你好世界")
	app.modal.OnAction("confirm")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("translate failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for translate formatResult")
	}

	rows, err := wb.ReadRange(sheet, document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1})
	if err != nil || len(rows) == 0 || len(rows[0]) == 0 || rows[0][0].Value != "你好世界" {
		t.Fatalf("expected translated text '你好世界' in cell A1, got %v (err: %v)", rows, err)
	}
}
