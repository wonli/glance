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

func TestSpreadsheetFreezePanesDialogAndCommand(t *testing.T) {
	wb := xlsx.NewBlank()
	defer wb.Close()

	state := newDocumentSessionState("")
	state.book = wb
	state.sheet = wb.SheetNames()[0]
	state.selectionSet = true
	state.selection = CellSelection{
		Anchor: CellPosition{Row: 2, Column: 1},
		Active: CellPosition{Row: 2, Column: 1}, // B3
	}
	app := &App{documentSessionState: state, theme: material.NewTheme(), formatResults: make(chan spreadsheetFormatResult, 10)}

	if !app.toolbarActionEnabled(toolbarActionFreezePanes) {
		t.Fatal("toolbarActionFreezePanes should be enabled")
	}

	// 1. Freeze top row
	app.showSpreadsheetFreezePanesDialog()
	if !app.modal.Visible || app.modal.Title != "冻结窗格" {
		t.Fatalf("expected modal dialog for freeze panes, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("freeze_row")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("freeze top row failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for freeze top row formatResult")
	}

	panes, err := wb.GetSheetPanes(state.sheet)
	if err != nil || !panes.Frozen || panes.YSplit != 1 || panes.XSplit != 0 {
		t.Fatalf("expected frozen top row (YSplit=1, XSplit=0), got %+v (err=%v)", panes, err)
	}

	// 2. Unfreeze panes
	app.showSpreadsheetFreezePanesDialog()
	if !app.modal.Visible {
		t.Fatal("expected modal dialog for freeze panes")
	}
	app.modal.OnAction("unfreeze")
	select {
	case res := <-app.formatResults:
		if res.err != nil {
			t.Fatalf("unfreeze panes failed: %v", res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for unfreeze formatResult")
	}

	panes, err = wb.GetSheetPanes(state.sheet)
	if err != nil || panes.Frozen {
		t.Fatalf("expected unfrozen panes, got %+v (err=%v)", panes, err)
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
