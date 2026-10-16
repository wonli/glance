package desktopui

import (
	"image"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/document"
	"glance/internal/format/xlsx"
)

func TestSpreadsheetAutoFitAppliesSelectedRowsAsOneUndoableBatch(t *testing.T) {
	book := newAutoFitFixture(t, func(book *xlsx.Workbook) error {
		sheet := book.SheetNames()[0]
		if err := book.SetCellText(sheet, "A1", "first line\nsecond line\nthird line\nfourth line"); err != nil {
			return err
		}
		if err := book.SetCellText(sheet, "A3", "another\nwrapped\nrow"); err != nil {
			return err
		}
		if err := book.SetColumnWidth(sheet, 1, 5); err != nil {
			return err
		}
		wrap := true
		return document.SpreadsheetFormatter(book).SetCellFormats(sheet, document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 3, EndColumn: 1}, document.CellFormatPatch{WrapText: &wrap})
	})
	defer book.Close()
	sheet := book.SheetNames()[0]

	state := newDocumentSessionState("selected-row-autofit.xlsx")
	state.book = book
	state.sheet = sheet
	state.sheets = book.SheetNames()
	state.virtualGrid = newGridViewport()
	state.selectionSet = true
	state.selection = CellSelection{Anchor: CellPosition{Row: 2, Column: 0}, Active: CellPosition{Row: 0, Column: 0}}
	app := &App{documentSessionState: state, documentGeneration: 7, formatResults: make(chan spreadsheetFormatResult, 1)}
	result := runSpreadsheetAutoFitForTest(t, app, true)
	if len(result.dimensions) != 2 || result.dimensions[0].index != 1 || result.dimensions[1].index != 3 {
		t.Fatalf("row AutoFit changes = %+v; want only populated selected rows 1 and 3", result.dimensions)
	}
	reader := document.SpreadsheetDimensionRangeCellReader(book)
	rowHeights := make(map[int]float64)
	readRowHeights := func() {
		t.Helper()
		clear(rowHeights)
		if err := reader.VisitDimensionRangeCells(sheet, document.SpreadsheetDimensionRow, 1, 3, func(entry document.SpreadsheetDimensionCell) error {
			rowHeights[entry.DimensionIndex] = entry.DimensionSize
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	readRowHeights()
	if got := rowHeights[1]; got <= 15 {
		t.Errorf("row 1 height = %v; want it to grow for wrapped text", got)
	}
	if _, ok := rowHeights[2]; ok {
		t.Errorf("empty row 2 unexpectedly got an AutoFit size: %v", rowHeights[2])
	}
	if got := rowHeights[3]; got <= 15 {
		t.Errorf("row 3 height = %v; want it to grow for wrapped text", got)
	}
	for _, change := range result.dimensions {
		if got, want := state.virtualGrid.Rows.sizeAt(change.index-1), pointsToDevicePixels(change.value, state.virtualGrid.Scale); got != want {
			t.Errorf("row %d grid height = %v; want %v", change.index, got, want)
		}
	}
	if applied, err := book.Undo(); err != nil || !applied {
		t.Fatalf("undo selected row AutoFit = %v, %v; want true, nil", applied, err)
	}
	readRowHeights()
	for _, row := range []int{1, 3} {
		if got := rowHeights[row]; got != 15 {
			t.Errorf("row %d after one undo = %v; want 15", row, got)
		}
	}
	if applied, err := book.Undo(); err != nil || applied {
		t.Fatalf("second undo after selected row AutoFit = %v, %v; want false, nil", applied, err)
	}
}

func TestSpreadsheetAutoFitAppliesSelectedColumnsAsOneUndoableBatch(t *testing.T) {
	book := newAutoFitFixture(t, func(book *xlsx.Workbook) error {
		sheet := book.SheetNames()[0]
		if err := book.SetCellText(sheet, "B1", "a column that needs more width"); err != nil {
			return err
		}
		if err := book.SetCellText(sheet, "D1", "another wider column"); err != nil {
			return err
		}
		if err := book.SetColumnWidth(sheet, 2, 8); err != nil {
			return err
		}
		return book.SetColumnWidth(sheet, 4, 8)
	})
	defer book.Close()
	sheet := book.SheetNames()[0]

	state := newDocumentSessionState("selected-column-autofit.xlsx")
	state.book = book
	state.sheet = sheet
	state.sheets = book.SheetNames()
	state.virtualGrid = newGridViewport()
	state.selectionSet = true
	state.selection = CellSelection{Anchor: CellPosition{Row: 0, Column: 3}, Active: CellPosition{Row: 0, Column: 1}}
	app := &App{documentSessionState: state, documentGeneration: 9, formatResults: make(chan spreadsheetFormatResult, 1)}
	result := runSpreadsheetAutoFitForTest(t, app, false)
	if len(result.dimensions) != 2 || result.dimensions[0].index != 2 || result.dimensions[1].index != 4 {
		t.Fatalf("column AutoFit changes = %+v; want only populated selected columns 2 and 4", result.dimensions)
	}
	reader := document.SpreadsheetDimensionRangeCellReader(book)
	columnWidths := make(map[int]float64)
	if err := reader.VisitDimensionRangeCells(sheet, document.SpreadsheetDimensionColumn, 2, 4, func(entry document.SpreadsheetDimensionCell) error {
		columnWidths[entry.DimensionIndex] = entry.DimensionSize
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, column := range []int{2, 4} {
		if got := columnWidths[column]; got <= 8 {
			t.Errorf("column %d width = %v; want it to grow", column, got)
		}
	}
	for _, change := range result.dimensions {
		if got, want := state.virtualGrid.Columns.sizeAt(change.index-1), columnCharsToGridPixels(change.value)*state.virtualGrid.Scale; got != want {
			t.Errorf("column %d grid width = %v; want %v", change.index, got, want)
		}
	}
	if applied, err := book.Undo(); err != nil || !applied {
		t.Fatalf("undo selected column AutoFit = %v, %v; want true, nil", applied, err)
	}
	columnWidths = make(map[int]float64)
	if err := reader.VisitDimensionRangeCells(sheet, document.SpreadsheetDimensionColumn, 2, 4, func(entry document.SpreadsheetDimensionCell) error {
		columnWidths[entry.DimensionIndex] = entry.DimensionSize
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, column := range []int{2, 4} {
		if got := columnWidths[column]; got != 8 {
			t.Errorf("column %d after one undo = %v; want 8", column, got)
		}
	}
	if applied, err := book.Undo(); err != nil || applied {
		t.Fatalf("second undo after selected column AutoFit = %v, %v; want false, nil", applied, err)
	}
}

func TestSpreadsheetAutoFitSingleCellSelectionTargetsActiveRowAndColumn(t *testing.T) {
	book := newAutoFitFixture(t, func(book *xlsx.Workbook) error {
		sheet := book.SheetNames()[0]
		for cell, value := range map[string]string{
			"A1": "first\nsecond\nthird",
			"A3": "selected\nrow\ncontent",
			"C1": "a column that needs much more width",
			"C3": "another long column value",
		} {
			if err := book.SetCellText(sheet, cell, value); err != nil {
				return err
			}
		}
		if err := book.SetColumnWidth(sheet, 1, 5); err != nil {
			return err
		}
		return book.SetColumnWidth(sheet, 3, 5)
	})
	defer book.Close()
	sheet := book.SheetNames()[0]

	state := newDocumentSessionState("single-cell-autofit.xlsx")
	state.book = book
	state.sheet = sheet
	state.sheets = book.SheetNames()
	state.virtualGrid = newGridViewport()
	state.selectionSet = true
	state.selection = CellSelection{Anchor: CellPosition{Row: 2, Column: 2}, Active: CellPosition{Row: 2, Column: 2}}
	app := &App{documentSessionState: state, documentGeneration: 11, formatResults: make(chan spreadsheetFormatResult, 1)}

	rowResult := runSpreadsheetAutoFitForTest(t, app, true)
	if len(rowResult.dimensions) != 1 || rowResult.dimensions[0].index != 3 || !rowResult.dimensions[0].row {
		t.Fatalf("single-cell row AutoFit changes = %+v; want only row 3", rowResult.dimensions)
	}
	columnResult := runSpreadsheetAutoFitForTest(t, app, false)
	if len(columnResult.dimensions) != 1 || columnResult.dimensions[0].index != 3 || columnResult.dimensions[0].row {
		t.Fatalf("single-cell column AutoFit changes = %+v; want only column 3", columnResult.dimensions)
	}
}

func newAutoFitFixture(t *testing.T, configure func(*xlsx.Workbook) error) *xlsx.Workbook {
	t.Helper()
	seed := xlsx.NewBlank()
	if err := configure(seed); err != nil {
		_ = seed.Close()
		t.Fatal(err)
	}
	contents, err := seed.Snapshot()
	if closeErr := seed.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	book, err := xlsx.OpenSnapshot(contents)
	if err != nil {
		t.Fatal(err)
	}
	return book
}

func runSpreadsheetAutoFitForTest(t *testing.T, app *App, row bool) spreadsheetFormatResult {
	t.Helper()
	var ops op.Ops
	app.startSpreadsheetAutoFitDimension(layout.Context{
		Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(800, 600)},
	}, row)
	select {
	case result := <-app.formatResults:
		if result.err != nil {
			t.Fatalf("start selected AutoFit: %v", result.err)
		}
		app.formatResults <- result
		app.processSpreadsheetFormatResult()
		return result
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for selected AutoFit result")
		return spreadsheetFormatResult{}
	}
}
