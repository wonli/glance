package desktopui

import (
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/format/xlsx"
)

func TestSpreadsheetPageLayoutDialogsAndCommands(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()

	state := newDocumentSessionState("")
	state.book = wb
	state.sheet = wb.SheetNames()[0]
	v := newGridViewport()
	state.virtualGrid = v
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	if !app.toolbarActionEnabled(toolbarActionLayoutMargins) {
		t.Fatal("layout-margins should be enabled")
	}
	if !app.toolbarActionEnabled(toolbarActionLayoutOrientation) {
		t.Fatal("layout-orientation should be enabled")
	}
	if !app.toolbarActionEnabled(toolbarActionLayoutSize) {
		t.Fatal("layout-size should be enabled")
	}

	// 1. Orientation Dialog
	app.showSpreadsheetOrientationDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "纸张方向" {
		t.Fatalf("expected modal dialog for orientation, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.OnAction("landscape")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("orientation command failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for orientation formatResult")
	}
	pageLayout, err := wb.GetSheetPageLayout(state.sheet)
	if err != nil || pageLayout.Orientation != "landscape" {
		t.Fatalf("expected landscape orientation, got %+v (err=%v)", pageLayout, err)
	}

	// 2. Margins Dialog
	app.showSpreadsheetMarginsDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "页边距设置" {
		t.Fatalf("expected modal dialog for margins, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.OnAction("narrow")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("margins command failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for margins formatResult")
	}
	pageLayout, err = wb.GetSheetPageLayout(state.sheet)
	if err != nil || pageLayout.MarginLeft != 0.25 {
		t.Fatalf("expected narrow margin 0.25, got %+v (err=%v)", pageLayout, err)
	}

	// 3. Page Size Dialog
	app.showSpreadsheetPageSizeDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "纸张大小设置" {
		t.Fatalf("expected modal dialog for page size, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.OnAction("letter")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("page size command failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for page size formatResult")
	}
	pageLayout, err = wb.GetSheetPageLayout(state.sheet)
	if err != nil || pageLayout.PaperSize != 1 {
		t.Fatalf("expected paper size 1 (Letter), got %+v (err=%v)", pageLayout, err)
	}

	// 4. Print Area Dialog and Commands
	if !app.toolbarActionEnabled(toolbarActionPrintArea) {
		t.Fatal("print-area should be enabled")
	}
	state.selectionSet = true
	state.selection = CellSelection{
		Anchor: CellPosition{Row: 0, Column: 0},
		Active: CellPosition{Row: 9, Column: 3},
	}
	app.showSpreadsheetPrintAreaDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "打印区域设置" {
		t.Fatalf("expected modal dialog for print area, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.OnAction("set")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("set print area failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for print area formatResult")
	}
	pageLayout, err = wb.GetSheetPageLayout(state.sheet)
	if err != nil || pageLayout.PrintArea != "$A$1:$D$10" {
		t.Fatalf("expected print area $A$1:$D$10, got %+v (err=%v)", pageLayout, err)
	}

	// Clear print area
	app.showSpreadsheetPrintAreaDialog()
	if !app.modal.Visible {
		t.Fatal("expected modal dialog for print area")
	}
	app.modal.OnAction("clear")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("clear print area failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for clear print area formatResult")
	}
	pageLayout, err = wb.GetSheetPageLayout(state.sheet)
	if err != nil || pageLayout.PrintArea != "" {
		t.Fatalf("expected empty print area after clear, got %+v (err=%v)", pageLayout, err)
	}

	// 5. Print Titles / Header Row Dialog
	if !app.toolbarActionEnabled(toolbarActionHeaderRow) {
		t.Fatal("header-row / print-titles should be enabled")
	}
	app.showSpreadsheetPrintTitlesDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "打印标题设置" {
		t.Fatalf("expected modal dialog for print titles, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.OnAction("set")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("set print titles failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for print titles formatResult")
	}
	pageLayout, err = wb.GetSheetPageLayout(state.sheet)
	if err != nil || pageLayout.PrintTitles != "$1:$10" {
		t.Fatalf("expected print titles $1:$10, got %+v (err=%v)", pageLayout, err)
	}

	// Clear print titles
	app.showSpreadsheetPrintTitlesDialog()
	if !app.modal.Visible {
		t.Fatal("expected modal dialog for print titles")
	}
	app.modal.OnAction("clear")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("clear print titles failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for clear print titles formatResult")
	}
	pageLayout, err = wb.GetSheetPageLayout(state.sheet)
	if err != nil || pageLayout.PrintTitles != "" {
		t.Fatalf("expected empty print titles after clear, got %+v (err=%v)", pageLayout, err)
	}

	// 6. View Gridlines Toggle
	if !app.toolbarActionEnabled(toolbarActionViewGridlines) {
		t.Fatal("view-gridlines should be enabled")
	}
	if state.spreadsheetGridlinesHidden {
		t.Fatal("expected gridlines initially visible")
	}
	state.toolbarTab = toolbarTabView
	state.toolbarActionButtons[toolbarActionViewGridlines].Click()
	router := new(input.Router)
	var ops op.Ops
	gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	app.handleSpreadsheetToolbarEvents(gtx)
	if !state.spreadsheetGridlinesHidden {
		t.Fatal("expected gridlines to be hidden after toggle")
	}
	if app.statusText() != "已隐藏网格线" {
		t.Fatalf("expected status '已隐藏网格线', got %q", app.statusText())
	}
	select {
	case result := <-app.formatResults:
		if result.err != nil {
			t.Fatalf("hide gridlines failed: %v", result.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for hide-gridlines formatResult")
	}
	state.toolbarActionButtons[toolbarActionViewGridlines].Click()
	app.handleSpreadsheetToolbarEvents(gtx)
	if state.spreadsheetGridlinesHidden {
		t.Fatal("expected gridlines to be visible after second toggle")
	}
	if app.statusText() != "已显示网格线" {
		t.Fatalf("expected status '已显示网格线', got %q", app.statusText())
	}
	select {
	case result := <-app.formatResults:
		if result.err != nil {
			t.Fatalf("show gridlines failed: %v", result.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for show-gridlines formatResult")
	}

	// 7. Layout Gridlines Toggle
	if !app.toolbarActionEnabled(toolbarActionLayoutGridlines) {
		t.Fatal("layout-gridlines should be enabled")
	}
	state.toolbarTab = toolbarTabPageLayout
	state.toolbarActionButtons[toolbarActionLayoutGridlines].Click()
	app.handleSpreadsheetToolbarEvents(gtx)
	if !state.spreadsheetGridlinesHidden {
		t.Fatal("expected gridlines to be hidden after layout gridlines toggle")
	}
	select {
	case result := <-app.formatResults:
		if result.err != nil {
			t.Fatalf("hide layout gridlines failed: %v", result.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for hide-layout-gridlines formatResult")
	}
	gridlines, err := wb.GetSheetGridlines(state.sheet)
	if err != nil || gridlines {
		t.Fatalf("expected workbook gridlines to be false, got %v (err=%v)", gridlines, err)
	}
	state.toolbarActionButtons[toolbarActionLayoutGridlines].Click()
	app.handleSpreadsheetToolbarEvents(gtx)
	if state.spreadsheetGridlinesHidden {
		t.Fatal("expected gridlines to be visible after second layout toggle")
	}
	select {
	case result := <-app.formatResults:
		if result.err != nil {
			t.Fatalf("show layout gridlines failed: %v", result.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for show-layout-gridlines formatResult")
	}
	gridlines, err = wb.GetSheetGridlines(state.sheet)
	if err != nil || !gridlines {
		t.Fatalf("expected workbook gridlines to be true, got %v (err=%v)", gridlines, err)
	}
}
