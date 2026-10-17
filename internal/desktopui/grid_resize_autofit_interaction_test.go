package desktopui

import (
	"image"
	"testing"
	"time"

	gioapp "gioui.org/app"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/widget/material"
	"glance/internal/document"
	"glance/internal/format/xlsx"
)

func TestGridResizeBoundaryDoubleClickAutoFitsColumn(t *testing.T) {
	h, book := newGridResizeAutoFitHarness(t)
	app := h.app
	state := app.documentSessionState
	const value = "a long value that must expand the first column when it is autofitted"
	if err := book.SetCellText("Sheet1", "A1", value); err != nil {
		t.Fatal(err)
	}
	if err := book.SetColumnWidth("Sheet1", 1, 5); err != nil {
		t.Fatal(err)
	}
	book.RebaseHistory()
	state.cellEdit.cancel()
	state.selection = CellSelection{Anchor: CellPosition{}, Active: CellPosition{}}
	state.selectionSet = true
	state.virtualGrid.Window.Cells[0][0].Value = value
	if err := state.virtualGrid.Columns.SetSize(0, columnCharsToGridPixels(5)); err != nil {
		t.Fatal(err)
	}

	const x, y = float32(gridRowHeader + 40), float32(15)
	h.point(pointer.Press, x, y)
	h.point(pointer.Release, x, y)
	if got := readGridColumnWidth(t, book, 1); got != 5 {
		t.Fatalf("single click changed column width to %v; want the existing 5 characters", got)
	}
	if book.Dirty() {
		t.Fatal("single click on a resize boundary dirtied the workbook")
	}

	h.point(pointer.Press, x, y)
	h.point(pointer.Release, x, y)
	select {
	case result := <-app.formatResults:
		if result.err != nil {
			t.Fatalf("double-click AutoFit failed: %v", result.err)
		}
		if !result.applied {
			t.Fatal("double-click AutoFit did not change the selected column")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("double-click did not start column AutoFit")
	}
	if got := readGridColumnWidth(t, book, 1); got <= 5 {
		t.Fatalf("double-click column width = %v; want wider than 5 characters", got)
	}
	if got := state.selection.Bounds(); got != (document.CellRange{StartRow: 1, EndRow: 1, StartColumn: 1, EndColumn: 1}) {
		t.Fatalf("AutoFit changed the active cell selection: %+v", got)
	}
	if applied, err := book.Undo(); err != nil || !applied {
		t.Fatalf("undo column AutoFit = %v, %v; want true, nil", applied, err)
	}
	if got := readGridColumnWidth(t, book, 1); got != 5 {
		t.Fatalf("column width after one undo = %v; want 5", got)
	}
	if applied, err := book.Undo(); err != nil || applied {
		t.Fatalf("second undo after column AutoFit = %v, %v; want false, nil", applied, err)
	}
}

func TestGridResizeBoundaryDoubleClickAutoFitsSelectedRows(t *testing.T) {
	h, book := newGridResizeAutoFitHarness(t)
	app := h.app
	state := app.documentSessionState
	const value = "first line\nsecond line\nthird line"
	if err := book.SetCellText("Sheet1", "A1", value); err != nil {
		t.Fatal(err)
	}
	wrap := true
	if err := book.SetCellFormats("Sheet1", document.CellRange{StartRow: 1, EndRow: 2, StartColumn: 1, EndColumn: 1}, document.CellFormatPatch{WrapText: &wrap}); err != nil {
		t.Fatal(err)
	}
	if err := book.SetRowHeight("Sheet1", 1, 15); err != nil {
		t.Fatal(err)
	}
	if err := book.SetRowHeight("Sheet1", 2, 15); err != nil {
		t.Fatal(err)
	}
	book.RebaseHistory()
	state.cellEdit.cancel()
	state.selection = CellSelection{Anchor: CellPosition{Row: 0}, Active: CellPosition{Row: 1}, Scope: selectionRows}
	state.selectionSet = true
	state.virtualGrid.Window.Cells[0][0].Value = value
	if err := state.virtualGrid.Rows.SetSize(0, pointsToGridPixels(15)); err != nil {
		t.Fatal(err)
	}
	if err := state.virtualGrid.Rows.SetSize(1, pointsToGridPixels(15)); err != nil {
		t.Fatal(err)
	}

	const x, y = float32(20), float32(gridColumnHeader + 20)
	h.point(pointer.Press, x, y)
	h.point(pointer.Release, x, y)
	h.point(pointer.Press, x, y)
	h.point(pointer.Release, x, y)
	select {
	case result := <-app.formatResults:
		if result.err != nil {
			t.Fatalf("double-click AutoFit failed: %v", result.err)
		}
		if !result.applied {
			t.Fatal("double-click AutoFit did not change the selected rows")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("double-click did not start row AutoFit")
	}
	if got := readGridRowHeight(t, book, 1); got <= 15 {
		t.Fatalf("row 1 AutoFit height = %v; want more than 15 points", got)
	}
	if got := readGridRowHeight(t, book, 2); got != 15 {
		t.Fatalf("blank selected row 2 height = %v; want unchanged 15 points", got)
	}
	if got := state.selection.Bounds(); got != (document.CellRange{StartRow: 1, EndRow: 2, StartColumn: 1, EndColumn: xlsxColumns}) {
		t.Fatalf("AutoFit changed the selected row range: %+v", got)
	}
}

func readGridColumnWidth(t *testing.T, book *xlsx.Workbook, column int) float64 {
	t.Helper()
	window, err := book.Window("Sheet1", 1, column, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	return window.ColumnWidthsChars[0]
}

func readGridRowHeight(t *testing.T, book *xlsx.Workbook, row int) float64 {
	t.Helper()
	window, err := book.Window("Sheet1", row, 1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	return window.RowHeightsPoints[0]
}

func newGridResizeAutoFitHarness(t *testing.T) (*gridTextHarness, *xlsx.Workbook) {
	t.Helper()
	book := xlsx.NewBlank()
	state := newDocumentSessionState("resize-autofit.xlsx")
	state.book = book
	state.sheet = book.SheetNames()[0]
	state.selectedCell = "A1"
	state.cellEdit.selectCell("A1", "value")
	state.selection = CellSelection{Anchor: CellPosition{}, Active: CellPosition{}}
	state.selectionSet = true
	viewport := newGridViewport()
	state.virtualGrid = viewport
	viewport.setScale(1)
	viewport.WindowSheet = state.sheet
	viewport.LoadedSheet = state.sheet
	viewport.LoadedWindows = viewport.visiblePaneRanges(552, 208)
	viewport.WindowRange = viewport.LoadedWindows.Body
	viewport.Window, _ = book.Window(state.sheet,
		viewport.WindowRange.StartRow, viewport.WindowRange.StartColumn,
		viewport.WindowRange.EndRow-viewport.WindowRange.StartRow+1,
		viewport.WindowRange.EndColumn-viewport.WindowRange.StartColumn+1,
	)
	app := &App{
		documentSessionState:   state,
		theme:                  material.NewTheme(),
		window:                 new(gioapp.Window),
		shutdown:               make(chan struct{}),
		gridLoadResults:        make(chan gridLoadResult, 8),
		gridNavigationResults:  make(chan spreadsheetGridNavigationResult, 8),
		gridPictureMetaResults: make(chan gridPictureMetadataResult, 8),
		gridPictureResults:     make(chan gridPicturePreviewResult, 8),
		gridPictureCommands:    make(chan gridPictureCommandResult, 8),
		formatResults:          make(chan spreadsheetFormatResult, 8),
	}
	h := &gridTextHarness{app: app, size: image.Pt(600, 240)}
	h.draw = func(gtx layout.Context) { app.layoutVirtualGrid(gtx) }
	t.Cleanup(func() {
		close(app.shutdown)
		app.stopAndWaitManagedBackground()
		_ = book.Close()
	})
	h.frame()
	return h, book
}
