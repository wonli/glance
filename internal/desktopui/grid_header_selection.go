package desktopui

import (
	"gioui.org/f32"
	"image"
)

func gridHeaderAtPoint(v *gridViewport, point image.Point, rowHeader, columnHeader int) (cellSelectionScope, CellPosition, bool) {
	if point.X < 0 || point.Y < 0 {
		return selectionCells, CellPosition{}, false
	}
	if point.X < rowHeader && point.Y < columnHeader {
		return selectionSheet, CellPosition{}, true
	}
	if point.X < rowHeader {
		return selectionRows, clampCellPosition(CellPosition{Row: v.Rows.IndexAt(float64(point.Y-columnHeader) + v.Y)}), true
	}
	if point.Y < columnHeader {
		return selectionColumns, clampCellPosition(CellPosition{Column: v.Columns.IndexAt(float64(point.X-rowHeader) + v.X)}), true
	}
	return selectionCells, CellPosition{}, false
}

func (a *App) updateGridHeaderSelection(v *gridViewport, scope cellSelectionScope, position CellPosition, extend bool) {
	state := a.documentSessionState
	selection := CellSelection{Scope: scope, Anchor: position, Active: position}
	if extend && state.selectionSet && state.selection.Scope == scope {
		selection.Anchor = state.selection.Anchor
	}
	a.updateGridSelection(v, position, false, false)
	state.selection = selection
}
func (a *App) extendGridHeaderSelection(v *gridViewport, point f32.Point, rowHeader, columnHeader int) {
	position := a.selection.Active
	switch a.selection.Scope {
	case selectionRows:
		position.Row = v.Rows.IndexAt(float64(point.Y) - float64(columnHeader) + v.Y)
	case selectionColumns:
		position.Column = v.Columns.IndexAt(float64(point.X) - float64(rowHeader) + v.X)
	default:
		return
	}
	a.updateGridHeaderSelection(v, a.selection.Scope, clampCellPosition(position), true)
}
