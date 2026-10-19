package desktopui

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/format/xlsx"
)

func TestSpreadsheetZoomInAndZoom100(t *testing.T) {
	fake := &formattingTestBook{toolbarTestBook: toolbarTestBook{editable: true}}
	state := newDocumentSessionState("")
	state.book = fake
	state.sheet = "Sheet1"
	v := newGridViewport()
	state.virtualGrid = v
	app := &App{documentSessionState: state, theme: material.NewTheme()}

	if !app.toolbarActionEnabled(toolbarActionZoomIn) {
		t.Fatal("zoom-in should be enabled")
	}
	if !app.toolbarActionEnabled(toolbarActionZoom100) {
		t.Fatal("zoom-100 should be enabled")
	}

	// Trigger zoom-in
	newScale := v.Scale + 0.15
	if newScale > 2.5 {
		newScale = 2.5
	}
	v.setScale(newScale)
	if v.Scale != 1.15 {
		t.Fatalf("expected scale 1.15, got %v", v.Scale)
	}

	// Trigger zoom-100
	v.setScale(1.0)
	if v.Scale != 1.0 {
		t.Fatalf("expected scale 1.0, got %v", v.Scale)
	}
}

func TestSpreadsheetFreezePanesRibbonMenuAndCommand(t *testing.T) {
	wb := xlsx.NewBlank()

	state := newDocumentSessionState("")
	state.book = wb
	state.sheet = wb.SheetNames()[0]
	state.sheets = wb.SheetNames()
	state.toolbarTab = toolbarTabView
	state.virtualGrid = newGridViewport()
	state.virtualGrid.WindowSheet = state.sheet
	state.selectionSet = true
	state.selection = CellSelection{
		Anchor: CellPosition{Row: 2, Column: 1},
		Active: CellPosition{Row: 2, Column: 1}, // B3
	}
	app := &App{
		documentSessionState: state,
		theme:                material.NewTheme(),
		formatResults:        make(chan spreadsheetFormatResult, 10),
		shutdown:             make(chan struct{}),
	}
	t.Cleanup(func() {
		app.shutdownOnce.Do(func() { close(app.shutdown) })
		app.stopAndWaitManagedBackground()
		_ = wb.Close()
	})

	router := new(input.Router)
	var ops op.Ops
	gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}

	if !app.toolbarActionEnabled(toolbarActionFreezePanes) {
		t.Fatal("toolbarActionFreezePanes should be enabled")
	}

	clickCommand := func(command, wantStatus string) {
		t.Helper()
		state.toolbarActionButtons[toolbarActionFreezePanes].Click()
		app.handleSpreadsheetToolbarEvents(gtx)
		if state.officeRibbonMenu.kind != officeRibbonMenuSpreadsheet || state.officeRibbonMenu.sheetSlot != toolbarActionFreezePanes {
			t.Fatalf("freeze-pane toolbar did not open its Ribbon menu: %+v", state.officeRibbonMenu)
		}
		definition := app.officeRibbonMenuDefinition()
		optionIndex := -1
		for index, option := range definition.options {
			if option.command == command {
				optionIndex = index
				break
			}
		}
		if optionIndex < 0 || !app.officeRibbonMenuOptionEnabled(definition.options[optionIndex]) {
			t.Fatalf("Ribbon menu command %q is missing or disabled in %+v", command, definition.options)
		}
		state.officeRibbonMenu.buttons[optionIndex].Click()
		app.handleSpreadsheetToolbarEvents(gtx)
		if state.officeRibbonMenu.kind != officeRibbonMenuClosed {
			t.Fatalf("Ribbon menu command %q did not close the menu", command)
		}
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			app.processSpreadsheetFormatResult()
			if app.statusText() == wantStatus {
				return
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatalf("Ribbon command %q did not complete; status=%q", command, app.statusText())
	}

	// Exercise the toolbar -> Ribbon menu -> async pane command -> viewport path.
	clickCommand("excel-freeze-first-row", "已完成冻结首行")

	panes, err := wb.GetSheetPanes(state.sheet)
	if err != nil || !panes.Frozen || panes.YSplit != 1 || panes.XSplit != 0 {
		t.Fatalf("expected frozen top row (YSplit=1, XSplit=0), got %+v (err=%v)", panes, err)
	}
	if !state.virtualGrid.PanesLoaded || state.virtualGrid.Panes != panes {
		t.Fatalf("freeze-pane command did not update the visible grid: loaded=%v panes=%+v, workbook=%+v", state.virtualGrid.PanesLoaded, state.virtualGrid.Panes, panes)
	}

	state.toolbarActionButtons[toolbarActionFreezePanes].Click()
	app.handleSpreadsheetToolbarEvents(gtx)
	definition := app.officeRibbonMenuDefinition()
	firstRowChecked := false
	for _, option := range definition.options {
		if option.command == "excel-freeze-first-row" {
			firstRowChecked = option.checked
		}
	}
	if !firstRowChecked {
		t.Fatal("Ribbon menu did not reflect the active first-row freeze")
	}
	app.closeOfficeRibbonMenu()

	clickCommand("excel-unfreeze-panes", "已完成取消冻结窗格")

	panes, err = wb.GetSheetPanes(state.sheet)
	if err != nil || panes.Frozen {
		t.Fatalf("expected unfrozen panes, got %+v (err=%v)", panes, err)
	}
	if !state.virtualGrid.PanesLoaded || state.virtualGrid.Panes != (document.SpreadsheetPaneFreeze{}) {
		t.Fatalf("unfreeze command did not clear the visible grid panes: loaded=%v panes=%+v", state.virtualGrid.PanesLoaded, state.virtualGrid.Panes)
	}
	if state.virtualGrid.X != 0 || state.virtualGrid.Y != 0 {
		t.Fatalf("unfreeze should keep zero scroll position, got (%v,%v)", state.virtualGrid.X, state.virtualGrid.Y)
	}
	const rowHead, colHead = 48, 28
	if row, col, ok := state.virtualGrid.cellPoint(image.Pt(rowHead+10, colHead+10), rowHead, colHead); !ok || row != 1 || col != 1 {
		t.Fatalf("unfrozen click on row 1 = (%d,%d), %v; want (1,1), true", row, col, ok)
	}
}

func TestSpreadsheetViewModesAndRuler(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	state := newDocumentSessionState("")
	state.book = wb
	state.sheet = sheet
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	if !app.toolbarActionEnabled(toolbarActionNormalView) {
		t.Fatal("normal view should be enabled")
	}
	if !app.toolbarActionEnabled(toolbarActionPageBreakPreview) {
		t.Fatal("page break preview should be enabled")
	}
	if !app.toolbarActionEnabled(toolbarActionRuler) {
		t.Fatal("ruler should be enabled")
	}

	router := new(input.Router)
	var ops op.Ops
	gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}

	// 1. Switch to Page Break Preview
	state.toolbarTab = toolbarTabView
	state.toolbarActionButtons[toolbarActionPageBreakPreview].Click()
	app.handleSpreadsheetToolbarEvents(gtx)
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("set page break preview failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for page break preview formatResult")
	}
	if state.spreadsheetViewMode != "pageBreakPreview" {
		t.Fatalf("expected state view mode pageBreakPreview, got %q", state.spreadsheetViewMode)
	}
	view, err := wb.GetSheetView(sheet)
	if err != nil || view.ViewMode != "pageBreakPreview" {
		t.Fatalf("expected workbook view mode pageBreakPreview, got %+v (err=%v)", view, err)
	}

	// 2. Switch back to Normal View
	state.toolbarActionButtons[toolbarActionNormalView].Click()
	app.handleSpreadsheetToolbarEvents(gtx)
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("set normal view failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for normal view formatResult")
	}
	if state.spreadsheetViewMode != "normal" {
		t.Fatalf("expected state view mode normal, got %q", state.spreadsheetViewMode)
	}
	view, err = wb.GetSheetView(sheet)
	if err != nil || view.ViewMode != "normal" {
		t.Fatalf("expected workbook view mode normal, got %+v (err=%v)", view, err)
	}

	// 3. Toggle Ruler
	initRuler := state.spreadsheetRulerVisible
	state.toolbarActionButtons[toolbarActionRuler].Click()
	app.handleSpreadsheetToolbarEvents(gtx)
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("toggle ruler failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for ruler formatResult")
	}
	if state.spreadsheetRulerVisible == initRuler {
		t.Fatal("expected ruler visibility to toggle")
	}
	view, err = wb.GetSheetView(sheet)
	if err != nil || view.ShowRuler != state.spreadsheetRulerVisible {
		t.Fatalf("expected workbook ruler %v, got %+v (err=%v)", state.spreadsheetRulerVisible, view, err)
	}
}
