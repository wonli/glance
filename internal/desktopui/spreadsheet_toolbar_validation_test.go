package desktopui

import (
	"testing"
	"time"

	"gioui.org/widget/material"

	"glance/internal/format/xlsx"
)

func TestSpreadsheetToolbar_DataValidationAndComments(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	state := newDocumentSessionState("test.xlsx")
	state.book = wb
	state.sheets = wb.SheetNames()
	state.sheet = sheet
	state.virtualGrid = newGridViewport()
	state.selection = CellSelection{
		Anchor: CellPosition{Row: 0, Column: 0},
		Active: CellPosition{Row: 4, Column: 0},
	}
	state.selectionSet = true
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	// 1. Verify actions are implemented and enabled
	for _, slot := range []int{
		toolbarActionDataValidation,
		toolbarActionNewComment,
		toolbarActionDeleteComment,
	} {
		if unimplementedSpreadsheetToolbarAction(slot) {
			t.Fatalf("slot %d should not be reported as unimplemented", slot)
		}
		if !app.toolbarActionEnabled(slot) {
			t.Fatalf("slot %d should be enabled", slot)
		}
	}

	// 2. Data Validation Dialog & Execution
	app.showSpreadsheetDataValidationDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "数据验证" {
		t.Fatalf("expected 数据验证 modal, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.Input.SetText("选项A,选项B,选项C")
	app.modal.OnAction("confirm")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("apply data validation failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for data validation formatResult")
	}

	// 3. New Comment Dialog & Execution on A1
	state.selection = CellSelection{Active: CellPosition{Row: 0, Column: 0}}
	app.showSpreadsheetNewCommentDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "新建批注" {
		t.Fatalf("expected 新建批注 modal, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.Input.SetText("这是一条测试批注")
	app.modal.OnAction("confirm")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("apply comment failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for comment formatResult")
	}

	comments, err := wb.GetComments(sheet)
	if err != nil {
		t.Fatalf("GetComments failed: %v", err)
	}
	if len(comments) != 1 || comments[0].Cell != "A1" || comments[0].Text != "这是一条测试批注" {
		t.Fatalf("unexpected comments: %+v", comments)
	}

	// 4. Delete Comment Execution on A1
	app.deleteSpreadsheetComment()
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("delete comment failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for delete comment formatResult")
	}

	comments, err = wb.GetComments(sheet)
	if err != nil {
		t.Fatalf("GetComments after delete failed: %v", err)
	}
	if len(comments) != 0 {
		t.Fatalf("expected 0 comments after delete, got %d", len(comments))
	}
}

func TestSpreadsheetToolbar_ConditionalFormat(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	state := newDocumentSessionState("test.xlsx")
	state.book = wb
	state.sheets = wb.SheetNames()
	state.sheet = sheet
	state.virtualGrid = newGridViewport()
	state.selection = CellSelection{
		Anchor: CellPosition{Row: 0, Column: 0},
		Active: CellPosition{Row: 5, Column: 1},
	}
	state.selectionSet = true
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	// 1. Verify availability
	if unimplementedSpreadsheetToolbarAction(toolbarActionConditionalFormat) {
		t.Fatal("toolbarActionConditionalFormat should not be unimplemented")
	}
	if !app.toolbarActionEnabled(toolbarActionConditionalFormat) {
		t.Fatal("toolbarActionConditionalFormat should be enabled")
	}

	// 2. Open dialog and apply data bar
	app.showSpreadsheetConditionalFormatDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "条件格式" {
		t.Fatalf("expected 条件格式 modal, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	if app.modal.HasInput {
		t.Fatal("conditional format choices should not expose an editable internal rule ID")
	}
	if len(app.modal.Actions) != 6 || app.modal.Actions[0].ID != "data_bar" || app.modal.Actions[5].ID != "cancel" {
		t.Fatalf("conditional format actions = %+v; want five rules and a cancel action", app.modal.Actions)
	}
	app.modal.OnAction("data_bar")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("apply conditional format failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for conditional format formatResult")
	}

	// 3. Clear conditional format
	app.showSpreadsheetConditionalFormatDialog()
	app.modal.OnAction("clear")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("clear conditional format failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for clear conditional format formatResult")
	}
}
