package desktopui

import (
	"strings"
	"testing"
	"time"

	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/format/xlsx"
)

func TestSpreadsheetToolbar_ShapesAndCharts(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	_ = wb.SetCellText(sheet, "A1", "季度")
	_ = wb.SetCellText(sheet, "B1", "营收")
	_ = wb.SetCellText(sheet, "A2", "Q1")
	_ = wb.SetCellText(sheet, "B2", "100")
	_ = wb.SetCellText(sheet, "A3", "Q2")
	_ = wb.SetCellText(sheet, "B3", "120")

	state := newDocumentSessionState("test.xlsx")
	state.book = wb
	state.sheets = wb.SheetNames()
	state.sheet = sheet
	state.virtualGrid = newGridViewport()
	state.selection = CellSelection{
		Anchor: CellPosition{Row: 0, Column: 0},
		Active: CellPosition{Row: 2, Column: 1},
	}
	state.selectionSet = true
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	// 1. Shapes
	if unimplementedSpreadsheetToolbarAction(toolbarActionInsertShapes) {
		t.Fatal("toolbarActionInsertShapes should not be unimplemented")
	}
	if !app.toolbarActionEnabled(toolbarActionInsertShapes) {
		t.Fatal("toolbarActionInsertShapes should be enabled")
	}
	app.showSpreadsheetInsertShapeDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "插入形状" {
		t.Fatalf("expected 插入形状 modal, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.OnAction("roundRect")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("insert shape failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for insert shape formatResult")
	}

	// 2. Text Box
	if unimplementedSpreadsheetToolbarAction(toolbarActionInsertTextBox) {
		t.Fatal("toolbarActionInsertTextBox should not be unimplemented")
	}
	if !app.toolbarActionEnabled(toolbarActionInsertTextBox) {
		t.Fatal("toolbarActionInsertTextBox should be enabled")
	}
	app.showSpreadsheetInsertTextBoxDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "插入文本框" {
		t.Fatalf("expected 插入文本框 modal, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.Input.SetText("关键指标备注")
	app.modal.OnAction("confirm")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("insert text box failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for insert text box formatResult")
	}

	// 3. Column Chart
	if unimplementedSpreadsheetToolbarAction(toolbarActionInsertColumnChart) {
		t.Fatal("toolbarActionInsertColumnChart should not be unimplemented")
	}
	if !app.toolbarActionEnabled(toolbarActionInsertColumnChart) {
		t.Fatal("toolbarActionInsertColumnChart should be enabled")
	}
	app.showSpreadsheetInsertChartDialog(document.SpreadsheetChartColumn)
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "插入柱状图" {
		t.Fatalf("expected 插入柱状图 modal, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.OnAction("confirm")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("insert column chart failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for insert column chart formatResult")
	}

	// 4. Line Chart
	if unimplementedSpreadsheetToolbarAction(toolbarActionInsertLineChart) {
		t.Fatal("toolbarActionInsertLineChart should not be unimplemented")
	}
	if !app.toolbarActionEnabled(toolbarActionInsertLineChart) {
		t.Fatal("toolbarActionInsertLineChart should be enabled")
	}
	app.showSpreadsheetInsertChartDialog(document.SpreadsheetChartLine)
	app.modal.OnAction("confirm")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("insert line chart failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for insert line chart formatResult")
	}

	// 5. Pie Chart
	if unimplementedSpreadsheetToolbarAction(toolbarActionInsertPieChart) {
		t.Fatal("toolbarActionInsertPieChart should not be unimplemented")
	}
	if !app.toolbarActionEnabled(toolbarActionInsertPieChart) {
		t.Fatal("toolbarActionInsertPieChart should be enabled")
	}
	app.showSpreadsheetInsertChartDialog(document.SpreadsheetChartPie)
	app.modal.OnAction("confirm")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("insert pie chart failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for insert pie chart formatResult")
	}
}

func TestSpreadsheetToolbar_PivotTableAndPrint(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	_ = wb.SetCellText(sheet, "A1", "Item")
	_ = wb.SetCellText(sheet, "B1", "Dept")
	_ = wb.SetCellText(sheet, "C1", "100")

	desktop := newDesktopRuntimeWithPrinter(&darwinTestPrinter{})
	t.Cleanup(desktop.shutdown)
	app := newAppWithRuntime(nil, desktop)
	app.theme = material.NewTheme()
	app.formatResults = make(chan spreadsheetFormatResult, 10)
	if err := app.installDocument(loadedDocument{book: wb, sheets: wb.SheetNames()}); err != nil {
		t.Fatalf("install workbook: %v", err)
	}
	state := app.documentSessionState
	state.sheet = sheet
	state.virtualGrid = newGridViewport()
	state.selection = CellSelection{
		Anchor: CellPosition{Row: 0, Column: 0},
		Active: CellPosition{Row: 2, Column: 2},
	}
	state.selectionSet = true

	// 1. Pivot Table
	if unimplementedSpreadsheetToolbarAction(toolbarActionPivotTable) {
		t.Fatal("toolbarActionPivotTable should not be unimplemented")
	}
	if !app.toolbarActionEnabled(toolbarActionPivotTable) {
		t.Fatal("toolbarActionPivotTable should be enabled")
	}
	app.showSpreadsheetPivotTableDialog()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "插入数据透视表" {
		t.Fatalf("expected 插入数据透视表 modal, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.OnAction("confirm")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("insert pivot table failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for pivot table formatResult")
	}

	// 2. Print
	if unimplementedSpreadsheetToolbarAction(toolbarActionPrint) {
		t.Fatal("toolbarActionPrint should not be unimplemented")
	}
	if !app.toolbarActionEnabled(toolbarActionPrint) {
		t.Fatal("toolbarActionPrint should be enabled")
	}
	app.requestSpreadsheetPrint()
	if !app.modal.Visible || app.messageText(app.modal.TitleRef, app.modal.Title) != "打印工作表" {
		t.Fatalf("expected 打印工作表 modal, got visible=%v title=%q", app.modal.Visible, app.messageText(app.modal.TitleRef, app.modal.Title))
	}
	app.modal.OnAction("print")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		app.processAsyncResults()
		if strings.Contains(app.statusText(), "系统打印服务") || strings.Contains(app.statusText(), "打印失败") {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !strings.Contains(app.statusText(), "系统打印服务") {
		t.Fatalf("unexpected status after print: %q", app.statusText())
	}
}
