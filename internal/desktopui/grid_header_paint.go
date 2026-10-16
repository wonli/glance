package desktopui

import (
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"glance/internal/document"
	"image"
	"math"
	"strconv"
)

// Header geometry comes from the viewport, including blank rows outside the
// current data window. Only visible headers are laid out.
func paintGridHeaders(gtx layout.Context, a *App, v *gridViewport) {
	size := gtx.Constraints.Max
	rowHead, colHead := gtx.Dp(unit.Dp(gridRowHeader)), gtx.Dp(unit.Dp(gridColumnHeader))
	bounds := a.selection.Bounds()
	viewportWidth, viewportHeight := float64(size.X-rowHead), float64(size.Y-colHead)
	frozenWidth, frozenHeight := v.frozenViewportExtent(viewportWidth, viewportHeight)
	columnSplit, rowSplit := v.paneSplits()
	frozenRowsStart, frozenRowsEnd := 0, 0
	if rowSplit > 0 && frozenHeight > 0 {
		frozenRowsStart, frozenRowsEnd = v.Rows.VisibleRange(0, frozenHeight, 2)
		frozenRowsEnd = min(rowSplit, frozenRowsEnd)
	}
	frozenColumnsStart, frozenColumnsEnd := 0, 0
	if columnSplit > 0 && frozenWidth > 0 {
		frozenColumnsStart, frozenColumnsEnd = v.Columns.VisibleRange(0, frozenWidth, 2)
		frozenColumnsEnd = min(columnSplit, frozenColumnsEnd)
	}
	rowArea := clip.Rect(image.Rect(0, colHead, rowHead, size.Y)).Push(gtx.Ops)
	paintGridRowHeaders(gtx, a, v, bounds, a.selectionSet, frozenRowsStart, frozenRowsEnd, true, colHead)
	bodyRowsStart, bodyRowsEnd := v.Rows.VisibleRange(v.frozenRowBoundary()+v.Y, math.Max(0, viewportHeight-frozenHeight), 2)
	bodyRowsStart = max(rowSplit, bodyRowsStart)
	paintGridRowHeaders(gtx, a, v, bounds, a.selectionSet, bodyRowsStart, bodyRowsEnd, false, colHead)
	rowArea.Pop()
	colArea := clip.Rect(image.Rect(rowHead, 0, size.X, colHead)).Push(gtx.Ops)
	paintGridColumnHeaders(gtx, a, v, bounds, a.selectionSet, frozenColumnsStart, frozenColumnsEnd, true, rowHead)
	bodyColumnsStart, bodyColumnsEnd := v.Columns.VisibleRange(v.frozenColumnBoundary()+v.X, math.Max(0, viewportWidth-frozenWidth), 2)
	bodyColumnsStart = max(columnSplit, bodyColumnsStart)
	paintGridColumnHeaders(gtx, a, v, bounds, a.selectionSet, bodyColumnsStart, bodyColumnsEnd, false, rowHead)
	colArea.Pop()
	if a.selectionSet && a.selection.Scope == selectionSheet {
		paint.FillShape(gtx.Ops, selectedFill, clip.Rect(image.Rect(0, 0, rowHead, colHead)).Op())
	}
}

func paintGridRowHeaders(gtx layout.Context, a *App, v *gridViewport, selection document.CellRange, hasSelection bool, start, end int, frozen bool, columnHeader int) {
	for row := start; row < end && row >= 0; row++ {
		y0, y1 := v.screenRowBoundary(row, frozen, columnHeader), v.screenRowBoundary(row+1, frozen, columnHeader)
		fill := gridHeaderFill
		if hasSelection && row+1 >= selection.StartRow && row+1 <= selection.EndRow {
			fill = selectedFill
		}
		paintTextBox(gtx, a.theme, strconv.Itoa(row+1), image.Rect(0, int(y0), gtx.Dp(unit.Dp(gridRowHeader)), int(y1)), mutedText, fill, false)
	}
}

func paintGridColumnHeaders(gtx layout.Context, a *App, v *gridViewport, selection document.CellRange, hasSelection bool, start, end int, frozen bool, rowHeader int) {
	for column := start; column < end && column >= 0; column++ {
		x0, x1 := v.screenColumnBoundary(column, frozen, rowHeader), v.screenColumnBoundary(column+1, frozen, rowHeader)
		fill := gridHeaderFill
		if hasSelection && column+1 >= selection.StartColumn && column+1 <= selection.EndColumn {
			fill = selectedFill
		}
		paintTextBox(gtx, a.theme, columnName(column+1), image.Rect(int(x0), 0, int(x1), gtx.Dp(unit.Dp(gridColumnHeader))), mutedText, fill, false)
	}
}
