package desktopui

import (
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"image"
	"strconv"
)

// Header geometry comes from the viewport, including blank rows outside the
// current data window. Only visible headers are laid out.
func paintGridHeaders(gtx layout.Context, a *App, v *gridViewport) {
	size := gtx.Constraints.Max
	rowHead, colHead := gtx.Dp(unit.Dp(gridRowHeader)), gtx.Dp(unit.Dp(gridColumnHeader))
	bounds := a.selection.Bounds()
	rowArea := clip.Rect(image.Rect(0, colHead, rowHead, size.Y)).Push(gtx.Ops)
	first, last := v.Rows.IndexAt(v.Y), v.Rows.IndexAt(v.Y+float64(size.Y-colHead))
	for row := first; row <= last && row >= 0; row++ {
		y0, _ := v.Rows.Boundary(row)
		y1, _ := v.Rows.Boundary(row + 1)
		fill := gridHeaderFill
		if a.selectionSet && row+1 >= bounds.StartRow && row+1 <= bounds.EndRow {
			fill = selectedFill
		}
		paintTextBox(gtx, a.theme, strconv.Itoa(row+1), image.Rect(0, int(y0-v.Y)+colHead, rowHead, int(y1-v.Y)+colHead), mutedText, fill, false)
	}
	rowArea.Pop()
	colArea := clip.Rect(image.Rect(rowHead, 0, size.X, colHead)).Push(gtx.Ops)
	first, last = v.Columns.IndexAt(v.X), v.Columns.IndexAt(v.X+float64(size.X-rowHead))
	for column := first; column <= last && column >= 0; column++ {
		x0, _ := v.Columns.Boundary(column)
		x1, _ := v.Columns.Boundary(column + 1)
		fill := gridHeaderFill
		if a.selectionSet && column+1 >= bounds.StartColumn && column+1 <= bounds.EndColumn {
			fill = selectedFill
		}
		paintTextBox(gtx, a.theme, columnName(column+1), image.Rect(int(x0-v.X)+rowHead, 0, int(x1-v.X)+rowHead, colHead), mutedText, fill, false)
	}
	colArea.Pop()
	if a.selectionSet && a.selection.Scope == selectionSheet {
		paint.FillShape(gtx.Ops, selectedFill, clip.Rect(image.Rect(0, 0, rowHead, colHead)).Op())
	}
}
