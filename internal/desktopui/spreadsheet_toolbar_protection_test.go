package desktopui

import (
	"testing"
	"time"

	"gioui.org/widget/material"

	"glance/internal/format/xlsx"
)

func TestSpreadsheetProtectSheetDialogAndCommand(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	state := newDocumentSessionState("")
	state.book = wb
	state.sheet = sheet
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	if !app.toolbarActionEnabled(toolbarActionProtectSheet) {
		t.Fatal("protect sheet should be enabled")
	}

	// 1. Show protect sheet dialog (currently unprotected)
	app.showSpreadsheetProtectSheetDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "保护工作表" {
		t.Fatalf("expected protect sheet modal, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.Input.SetText("testpass")
	app.modal.OnAction("confirm")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("protect sheet failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for protect sheet formatResult")
	}

	protected, err := wb.GetSheetProtection(sheet)
	if err != nil || !protected {
		t.Fatalf("expected sheet protected, got %v (err=%v)", protected, err)
	}

	// 2. Show unprotect sheet dialog (currently protected)
	app.showSpreadsheetProtectSheetDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "撤销保护工作表" {
		t.Fatalf("expected unprotect sheet modal, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.Input.SetText("testpass")
	app.modal.OnAction("confirm")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("unprotect sheet failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for unprotect sheet formatResult")
	}

	protected, err = wb.GetSheetProtection(sheet)
	if err != nil || protected {
		t.Fatalf("expected sheet unprotected, got %v (err=%v)", protected, err)
	}

	// 3. Test Undo restores protection
	applied, err := wb.Undo()
	if err != nil || !applied {
		t.Fatalf("undo failed: applied=%v, err=%v", applied, err)
	}
	protected, err = wb.GetSheetProtection(sheet)
	if err != nil || !protected {
		t.Fatalf("expected sheet protected after undo, got %v (err=%v)", protected, err)
	}
}

func TestSpreadsheetProtectWorkbookDialogAndCommand(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()

	state := newDocumentSessionState("")
	state.book = wb
	state.sheet = wb.SheetNames()[0]
	v := newGridViewport()
	state.virtualGrid = v
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	if !app.toolbarActionEnabled(toolbarActionProtectWorkbook) {
		t.Fatal("protect-workbook should be enabled")
	}

	// 1. Show protect workbook dialog (currently unprotected)
	app.showSpreadsheetProtectWorkbookDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "保护工作簿" {
		t.Fatalf("expected protect workbook modal, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.Input.SetText("bookpass")
	app.modal.OnAction("confirm")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("protect workbook failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for protect workbook formatResult")
	}

	protected, err := wb.GetWorkbookProtection()
	if err != nil || !protected {
		t.Fatalf("expected workbook protected, got %v (err=%v)", protected, err)
	}

	// 2. Show unprotect workbook dialog (currently protected)
	app.showSpreadsheetProtectWorkbookDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "撤销保护工作簿" {
		t.Fatalf("expected unprotect workbook modal, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.Input.SetText("bookpass")
	app.modal.OnAction("confirm")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("unprotect workbook failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for unprotect workbook formatResult")
	}

	protected, err = wb.GetWorkbookProtection()
	if err != nil || protected {
		t.Fatalf("expected workbook unprotected, got %v (err=%v)", protected, err)
	}

	// 3. Test Undo restores protection
	applied, err := wb.Undo()
	if err != nil || !applied {
		t.Fatalf("undo failed: applied=%v, err=%v", applied, err)
	}
	protected, err = wb.GetWorkbookProtection()
	if err != nil || !protected {
		t.Fatalf("expected workbook protected after undo, got %v (err=%v)", protected, err)
	}
}
