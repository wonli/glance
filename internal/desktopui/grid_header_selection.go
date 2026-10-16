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
		row := v.headerRowAt(point.Y - columnHeader)
		return selectionRows, clampCellPosition(CellPosition{Row: row}), true
	}
	if point.Y < columnHeader {
		column := v.headerColumnAt(point.X - rowHeader)
		return selectionColumns, clampCellPosition(CellPosition{Column: column}), true
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
		position.Row = v.headerRowAt(int(point.Y) - columnHeader)
	case selectionColumns:
		position.Column = v.headerColumnAt(int(point.X) - rowHeader)
	default:
		return
	}
	a.updateGridHeaderSelection(v, a.selection.Scope, clampCellPosition(position), true)
}
