package desktopui

import (
	"strings"
	"testing"
	"time"

	"glance/internal/document"
	"glance/internal/format/xlsx"
)

func TestSpreadsheetToolbar_SpellCheckAndTranslate(t *testing.T) {
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	app := newAppWithRuntime(nil, desktop)
	wb := xlsx.NewBlank()
	t.Cleanup(func() { _ = wb.Close() })

	sheet := wb.SheetNames()[0]
	_ = wb.SetCellText(sheet, "A1", "Hello  World")
	if err := app.installDocument(loadedDocument{book: wb, sheets: wb.SheetNames()}); err != nil {
		t.Fatalf("install workbook: %v", err)
	}
	state := app.documentSessionState
	state.virtualGrid = newGridViewport()
	state.selection = CellSelection{
		Anchor: CellPosition{Row: 0, Column: 0},
		Active: CellPosition{Row: 0, Column: 0},
	}
	state.selectionSet = true

	// 1. Spell Check
	if unimplementedSpreadsheetToolbarAction(toolbarActionSpellCheck) {
		t.Fatal("toolbarActionSpellCheck should not be unimplemented")
	}
	if !app.toolbarActionEnabled(toolbarActionSpellCheck) {
		t.Fatal("toolbarActionSpellCheck should be enabled")
	}
	app.showSpreadsheetSpellCheckDialog()
	deadline := time.Now().Add(2 * time.Second)
	for !app.modal.Visible && time.Now().Before(deadline) {
		app.processSpreadsheetFormatResult()
		time.Sleep(time.Millisecond)
	}
	if !app.modal.Visible || app.modal.Title != "拼写检查" {
		t.Fatalf("expected 拼写检查 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	if !strings.Contains(app.modal.Message, "连续 ASCII 空格") || !strings.Contains(app.modal.Message, "未提供词典") {
		t.Fatalf("spell-check dialog overstates results or omits the bounded findings: %q", app.modal.Message)
	}
	app.modal.OnAction("confirm")
	if !strings.Contains(app.status, "文本检查完成") {
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
