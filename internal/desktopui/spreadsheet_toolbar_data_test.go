package desktopui

import (
	"testing"
	"time"

	"gioui.org/widget/material"

	"glance/internal/format/xlsx"
)

func TestSpreadsheetLinkDialogAndCommand(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()

	state := newDocumentSessionState("")
	state.book = wb
	state.sheet = wb.SheetNames()[0]
	state.selectionSet = true
	state.selection = CellSelection{
		Anchor: CellPosition{Row: 1, Column: 1},
		Active: CellPosition{Row: 1, Column: 1}, // B2
	}
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	if !app.toolbarActionEnabled(toolbarActionInsertLink) {
		t.Fatal("toolbarActionInsertLink should be enabled")
	}

	// 1. Show link dialog on empty cell B2
	app.showSpreadsheetLinkDialog()
	if !app.modal.Visible || app.modal.Title != "插入链接" {
		t.Fatalf("expected modal dialog for link, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("https://example.com")
	app.modal.SecondInput.SetText("Example")
	app.modal.OnAction("confirm")

	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("insert link failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for insert link formatResult")
	}

	link, ok, err := wb.GetCellHyperlink(state.sheet, "B2")
	if err != nil || !ok || link.Target != "https://example.com" {
		t.Fatalf("expected hyperlink https://example.com on B2, got ok=%v target=%q (err=%v)", ok, link.Target, err)
	}

	// 2. Reopen link dialog on B2 (now has link) and remove link
	app.showSpreadsheetLinkDialog()
	if !app.modal.Visible {
		t.Fatal("expected modal dialog for link")
	}
	app.modal.OnAction("remove")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("remove link failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for remove link formatResult")
	}

	link, ok, err = wb.GetCellHyperlink(state.sheet, "B2")
	if err != nil || ok {
		t.Fatalf("expected no hyperlink after remove, got ok=%v (err=%v)", ok, err)
	}
}

func TestSpreadsheetRemoveDuplicatesDialogAndCommand(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	_ = wb.SetCellText(sheet, "A1", "Alice")
	_ = wb.SetCellText(sheet, "B1", "100")
	_ = wb.SetCellText(sheet, "A2", "Bob")
	_ = wb.SetCellText(sheet, "B2", "200")
	_ = wb.SetCellText(sheet, "A3", "Alice")
	_ = wb.SetCellText(sheet, "B3", "100") // exact duplicate of row 1

	state := newDocumentSessionState("")
	state.book = wb
	state.sheet = sheet
	state.selectionSet = true
	state.selection = CellSelection{
		Anchor: CellPosition{Row: 0, Column: 0},
		Active: CellPosition{Row: 2, Column: 1}, // A1:B3 (3 rows, 2 columns)
	}
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	if !app.toolbarActionEnabled(toolbarActionRemoveDuplicates) {
		t.Fatal("toolbarActionRemoveDuplicates should be enabled")
	}

	app.showSpreadsheetRemoveDuplicatesDialog()
	if !app.modal.Visible || app.modal.Title != "删除重复项" {
		t.Fatalf("expected modal dialog for remove duplicates, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}

	// Deduplicate across all columns
	app.modal.OnAction("all")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("remove duplicates failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for remove duplicates formatResult")
	}

	// Verify row 3 was removed, leaving 2 rows
	cells, err := wb.Range(sheet, 10, 10)
	if err != nil {
		t.Fatalf("failed to read range: %v", err)
	}
	var rowCount int
	for _, row := range cells {
		if len(row) > 0 && row[0].Value != "" {
			rowCount++
		}
	}
	if rowCount != 2 {
		t.Fatalf("expected 2 remaining rows, got %d", rowCount)
	}

	// Verify Undo restores row 3
	if applied, err := wb.Undo(); err != nil || !applied {
		t.Fatalf("Undo failed: applied=%v, err=%v", applied, err)
	}
	cells, err = wb.Range(sheet, 10, 10)
	if err != nil {
		t.Fatalf("failed to read range: %v", err)
	}
	rowCount = 0
	for _, row := range cells {
		if len(row) > 0 && row[0].Value != "" {
			rowCount++
		}
	}
	if rowCount != 3 {
		t.Fatalf("expected 3 rows restored after Undo, got %d", rowCount)
	}
}

func TestSpreadsheetTextToColumnsDialogAndCommand(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()
	sheet := wb.SheetNames()[0]

	_ = wb.SetCellText(sheet, "A1", "Alpha,Beta,Gamma")
	_ = wb.SetCellText(sheet, "A2", "One,Two,Three")

	state := newDocumentSessionState("test.xlsx")
	state.book = wb
	state.sheets = wb.SheetNames()
	state.sheet = sheet
	state.selectionSet = true
	state.selection = CellSelection{
		Anchor: CellPosition{Row: 0, Column: 0},
		Active: CellPosition{Row: 1, Column: 0}, // A1:A2
	}
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	if !app.toolbarActionEnabled(toolbarActionTextToColumns) {
		t.Fatal("toolbarActionTextToColumns should be enabled")
	}

	app.showSpreadsheetTextToColumnsDialog()
	if !app.modal.Visible || app.modal.Title != "文本分列" {
		t.Fatalf("expected modal dialog for text to columns, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}

	// Split by comma
	app.modal.OnAction("comma")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("split text to columns failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for text to columns formatResult")
	}

	// Verify values in A1..C2
	vA1, _ := wb.Value(sheet, "A1")
	vB1, _ := wb.Value(sheet, "B1")
	vC1, _ := wb.Value(sheet, "C1")
	if vA1 != "Alpha" || vB1 != "Beta" || vC1 != "Gamma" {
		t.Fatalf("unexpected row 1 split: %q, %q, %q", vA1, vB1, vC1)
	}
	vA2, _ := wb.Value(sheet, "A2")
	vB2, _ := wb.Value(sheet, "B2")
	vC2, _ := wb.Value(sheet, "C2")
	if vA2 != "One" || vB2 != "Two" || vC2 != "Three" {
		t.Fatalf("unexpected row 2 split: %q, %q, %q", vA2, vB2, vC2)
	}
}

func TestSpreadsheetInsertTableDialogAndCommand(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()
	sheet := wb.SheetNames()[0]

	_ = wb.SetCellText(sheet, "A1", "Name")
	_ = wb.SetCellText(sheet, "B1", "Score")
	_ = wb.SetCellText(sheet, "A2", "Alice")
	_ = wb.SetCellText(sheet, "B2", "100")

	state := newDocumentSessionState("test.xlsx")
	state.book = wb
	state.sheets = wb.SheetNames()
	state.sheet = sheet
	state.selectionSet = true
	state.selection = CellSelection{
		Anchor: CellPosition{Row: 0, Column: 0},
		Active: CellPosition{Row: 1, Column: 1}, // A1:B2
	}
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	if !app.toolbarActionEnabled(toolbarActionInsertTable) {
		t.Fatal("toolbarActionInsertTable should be enabled")
	}
	if !app.toolbarActionEnabled(toolbarActionTableFormat) {
		t.Fatal("toolbarActionTableFormat should be enabled")
	}

	app.showSpreadsheetInsertTableDialog("")
	if !app.modal.Visible || app.modal.Title != "创建表格" {
		t.Fatalf("expected modal dialog for insert table, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}

	app.modal.OnAction("medium")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("create table failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for table formatResult")
	}

	tables, err := wb.Range(sheet, 5, 5)
	if err != nil {
		t.Fatalf("failed to read range: %v", err)
	}
	if len(tables) < 2 {
		t.Fatalf("expected at least 2 rows in range, got %d", len(tables))
	}
}

func TestSpreadsheetRecalculateAndRefreshAll(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()
	sheet := wb.SheetNames()[0]

	state := newDocumentSessionState("test.xlsx")
	state.book = wb
	state.sheets = wb.SheetNames()
	state.sheet = sheet
	state.virtualGrid = newGridViewport()
	app := &App{
		documentSessionState: state,
		theme:                material.NewTheme(),
		formatResults:        make(chan spreadsheetFormatResult, 2),
		shutdown:             make(chan struct{}),
	}
	app.sheet = state.sheet

	if !app.toolbarActionEnabled(toolbarActionRecalculate) {
		t.Fatal("toolbarActionRecalculate should be enabled")
	}
	if !app.toolbarActionEnabled(toolbarActionRefreshAll) {
		t.Fatal("toolbarActionRefreshAll should be enabled")
	}

	apply := func(run func(), operation, wantStatus string) {
		t.Helper()
		run()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			app.processSpreadsheetFormatResult()
			if app.status == wantStatus {
				return
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatalf("%s did not publish status %q; last status %q", operation, wantStatus, app.status)
	}
	apply(app.recalculateSpreadsheet, "重新计算公式", "已刷新公式结果（不包含外部数据连接）")
	apply(app.refreshAllSpreadsheet, "刷新全部工作表", "已刷新全部工作表公式结果（不包含外部数据连接）")
}

func TestSpreadsheetFormulaAndNameManagerAndFlashFill(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()
	sheet := wb.SheetNames()[0]

	_ = wb.SetCellText(sheet, "A1", "10")
	_ = wb.SetCellText(sheet, "A2", "20")

	state := newDocumentSessionState("test.xlsx")
	state.book = wb
	state.sheets = wb.SheetNames()
	state.sheet = sheet
	state.virtualGrid = newGridViewport()
	state.selection = CellSelection{Active: CellPosition{Row: 2, Column: 0}}
	state.selectionSet = true
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	// 1. Verify toolbar actions are implemented and enabled
	for _, slot := range []int{
		toolbarActionInsertFunction, toolbarActionRecentFunctions,
		toolbarActionLogicalFunctions, toolbarActionTextFunctions,
		toolbarActionDateTimeFunctions, toolbarActionNameManager,
		toolbarActionFlashFill,
	} {
		if unimplementedSpreadsheetToolbarAction(slot) {
			t.Fatalf("slot %d should not be reported as unimplemented", slot)
		}
		if !app.toolbarActionEnabled(slot) {
			t.Fatalf("slot %d should be enabled", slot)
		}
	}

	// 2. Insert Formula Dialog and execution
	app.showSpreadsheetInsertFunctionDialog("")
	if !app.modal.Visible || app.modal.Title != "插入函数" {
		t.Fatalf("expected 插入函数 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("=SUM(A1:A2)")
	app.modal.OnAction("confirm")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("insert formula failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for insert formula formatResult")
	}

	formula, err := wb.GetCellFormula(sheet, "A3")
	if err != nil || formula != "SUM(A1:A2)" {
		t.Fatalf("expected formula SUM(A1:A2), got %q (err=%v)", formula, err)
	}

	// 3. Name Manager Dialog and execution
	app.showSpreadsheetNameManagerDialog()
	if !app.modal.Visible || app.modal.Title != "名称管理器" {
		t.Fatalf("expected 名称管理器 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("TestTotal")
	app.modal.OnAction("confirm")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("create defined name failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for defined-name formatResult")
	}

	names, err := wb.GetDefinedNames()
	if err != nil {
		t.Fatalf("GetDefinedNames failed: %v", err)
	}
	found := false
	for _, n := range names {
		if n.Name == "TestTotal" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected defined name TestTotal in %v", names)
	}

	// 4. Flash Fill execution
	_ = wb.SetCellText(sheet, "B1", "1")
	_ = wb.SetCellText(sheet, "B2", "2")
	state.selection = CellSelection{
		Anchor: CellPosition{Row: 0, Column: 1},
		Active: CellPosition{Row: 4, Column: 1},
	}
	app.applySpreadsheetFlashFill()
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("flash fill failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for flash fill formatResult")
	}

	vB3, _ := wb.Value(sheet, "B3")
	vB4, _ := wb.Value(sheet, "B4")
	vB5, _ := wb.Value(sheet, "B5")
	if vB3 != "3" || vB4 != "4" || vB5 != "5" {
		t.Fatalf("unexpected flash fill values: B3=%s, B4=%s, B5=%s", vB3, vB4, vB5)
	}
}
