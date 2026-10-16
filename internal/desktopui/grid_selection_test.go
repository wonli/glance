package desktopui

import (
	"image"
	"testing"

	"gioui.org/layout"
	"glance/internal/document"
)

func TestCellSelectionBoundsNormalizesAndConvertsCoordinates(t *testing.T) {
	selection := CellSelection{
		Anchor: CellPosition{Row: 8, Column: 5},
		Active: CellPosition{Row: 2, Column: 1},
	}
	want := document.CellRange{StartRow: 3, StartColumn: 2, EndRow: 9, EndColumn: 6}
	if got := selection.Bounds(); got != want {
		t.Fatalf("Bounds()=%+v, want %+v", got, want)
	}
}

func TestCellSelectionBoundsClampsToXLSXLimits(t *testing.T) {
	selection := CellSelection{
		Anchor: CellPosition{Row: -4, Column: -2},
		Active: CellPosition{Row: xlsxRows + 40, Column: xlsxColumns + 5},
	}
	want := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: xlsxRows, EndColumn: xlsxColumns}
	if got := selection.Bounds(); got != want {
		t.Fatalf("Bounds()=%+v, want clamped %+v", got, want)
	}
}

func TestSelectionAtPointMapsViewportLocalPointToCell(t *testing.T) {
	rows, err := NewAxisGeometry(3, gridDefaultRow)
	if err != nil {
		t.Fatal(err)
	}
	columns, err := NewAxisGeometry(4, gridDefaultColumn)
	if err != nil {
		t.Fatal(err)
	}
	viewport := &gridViewport{Rows: rows, Columns: columns, Scale: 1}
	got, ok := selectionAtPoint(viewport, image.Pt(int(gridRowHeader+gridDefaultColumn+2), int(gridColumnHeader+gridDefaultRow+2)))
	if !ok {
		t.Fatal("selectionAtPoint() did not hit a visible cell")
	}
	want := CellPosition{Row: 1, Column: 1}
	if got != want {
		t.Fatalf("selectionAtPoint()=%+v, want %+v", got, want)
	}
	if _, ok := selectionAtPoint(viewport, image.Pt(0, 0)); ok {
		t.Fatal("selectionAtPoint() hit a cell inside the header area")
	}
}

func TestUpdatedCellSelectionSupportsClickShiftAndDragExtension(t *testing.T) {
	first := CellPosition{Row: 3, Column: 2}
	second := CellPosition{Row: 5, Column: 4}
	third := CellPosition{Row: 1, Column: 6}
	selection := updatedSelection(CellSelection{}, false, first, false)
	if selection != (CellSelection{Anchor: first, Active: first}) {
		t.Fatalf("click selection=%+v", selection)
	}
	selection = updatedSelection(selection, true, second, true)
	if selection != (CellSelection{Anchor: first, Active: second}) {
		t.Fatalf("shift extension=%+v", selection)
	}
	selection = updatedSelection(selection, true, third, true)
	if selection != (CellSelection{Anchor: first, Active: third}) {
		t.Fatalf("drag extension=%+v", selection)
	}
}

func TestSelectionRectUsesVisibleGridGeometry(t *testing.T) {
	rows, _ := NewAxisGeometry(10, gridDefaultRow)
	columns, _ := NewAxisGeometry(10, gridDefaultColumn)
	viewport := &gridViewport{Rows: rows, Columns: columns, X: 32, Y: 10, Scale: 1}
	selection := CellSelection{Anchor: CellPosition{Row: 1, Column: 2}, Active: CellPosition{Row: 2, Column: 3}}
	want := image.Rect(138, 42, 266, 82)
	if got := selectionRect(viewport, selection); got != want {
		t.Fatalf("selectionRect()=%v, want %v", got, want)
	}
}

func TestSelectionBorderRectsMakeFourEdges(t *testing.T) {
	rect := image.Rect(10, 20, 30, 40)
	want := [4]image.Rectangle{
		image.Rect(10, 20, 30, 22),
		image.Rect(10, 38, 30, 40),
		image.Rect(10, 22, 12, 38),
		image.Rect(28, 22, 30, 38),
	}
	if got := selectionBorderRects(rect, 2); got != want {
		t.Fatalf("selectionBorderRects()=%v, want %v", got, want)
	}
}

func TestUpdateGridSelectionPersistsActiveCellAndAnchor(t *testing.T) {
	state := newDocumentSessionState("")
	app := &App{documentSessionState: state}
	viewport := newGridViewport()
	app.updateGridSelection(viewport, CellPosition{Row: 3, Column: 2}, false, false)
	app.updateGridSelection(viewport, CellPosition{Row: 5, Column: 4}, true, false)
	want := CellSelection{Anchor: CellPosition{Row: 3, Column: 2}, Active: CellPosition{Row: 5, Column: 4}}
	if !state.selectionSet || state.selection != want {
		t.Fatalf("session selection=%+v, set=%t, want %+v", state.selection, state.selectionSet, want)
	}
	if app.selectedCell != "E6" || viewport.SelectedRow != 6 || viewport.SelectedColumn != 5 {
		t.Fatalf("active address=%q viewport=(%d,%d)", app.selectedCell, viewport.SelectedRow, viewport.SelectedColumn)
	}
}

func TestNewSessionInitializesPersistentToolbarWidgets(t *testing.T) {
	state := newDocumentSessionState("")
	if state.toolbarTab != toolbarTabHome {
		t.Fatalf("toolbar tab=%v; want Home", state.toolbarTab)
	}
	if state.toolbarActionList.List.Axis != layout.Horizontal {
		t.Fatalf("toolbar action list axis=%v; want horizontal", state.toolbarActionList.List.Axis)
	}
}

func TestSelectionPositionMapsMergedCellToAnchor(t *testing.T) {
	merges := []document.MergeRegion{{Range: document.CellRange{StartRow: 2, StartColumn: 3, EndRow: 4, EndColumn: 5}}}
	got := selectionAnchor(merges, CellPosition{Row: 3, Column: 4})
	want := CellPosition{Row: 1, Column: 2}
	if got != want {
		t.Fatalf("selectionAnchor()=%+v, want merge anchor %+v", got, want)
	}
}
