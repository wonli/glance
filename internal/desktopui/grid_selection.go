package desktopui

import (
	"image"

	"glance/internal/document"
)

// CellPosition is a zero-based cell coordinate used by transient UI state.
type CellPosition struct {
	Row, Column int
}

// CellSelection stores a rectangular selection. Most ranges use the fixed
// anchor and active cell; current-region selection can supply exact bounds
// while keeping the active cell inside the selected region.
type CellSelection struct {
	Anchor, Active CellPosition
	Scope          cellSelectionScope
	boundsOverride *document.CellRange
}

type cellSelectionScope uint8

const (
	selectionCells cellSelectionScope = iota
	selectionRows
	selectionColumns
	selectionSheet
)

func updatedSelection(current CellSelection, hasSelection bool, active CellPosition, extend bool) CellSelection {
	active = clampCellPosition(active)
	if !hasSelection || !extend {
		return CellSelection{Anchor: active, Active: active}
	}
	current.Active = active
	current.Scope = selectionCells
	current.boundsOverride = nil
	return current
}

func selectionAnchor(merges []document.MergeRegion, position CellPosition) CellPosition {
	row, column, _ := mergeAnchorAt(merges, position.Row+1, position.Column+1)
	return clampCellPosition(CellPosition{Row: row - 1, Column: column - 1})
}

func (s CellSelection) Bounds() document.CellRange {
	if s.boundsOverride != nil {
		return *s.boundsOverride
	}
	anchor := clampCellPosition(s.Anchor)
	active := clampCellPosition(s.Active)
	startRow, endRow := min(anchor.Row, active.Row), max(anchor.Row, active.Row)
	startColumn, endColumn := min(anchor.Column, active.Column), max(anchor.Column, active.Column)
	if s.Scope == selectionRows || s.Scope == selectionSheet {
		startColumn, endColumn = 0, xlsxColumns-1
	}
	if s.Scope == selectionColumns || s.Scope == selectionSheet {
		startRow, endRow = 0, xlsxRows-1
		if anchor.Row >= xlsxRows || active.Row >= xlsxRows {
			endRow = max(anchor.Row, active.Row)
		}
	}
	return document.CellRange{
		StartRow: startRow + 1, StartColumn: startColumn + 1,
		EndRow: endRow + 1, EndColumn: endColumn + 1,
	}
}

func (s CellSelection) Contains(position CellPosition) bool {
	position = clampCellPosition(position)
	bounds := s.Bounds()
	return position.Row+1 >= bounds.StartRow && position.Row+1 <= bounds.EndRow &&
		position.Column+1 >= bounds.StartColumn && position.Column+1 <= bounds.EndColumn
}

func clampCellPosition(position CellPosition) CellPosition {
	maxRows := xlsxRows
	if position.Row >= maxRows {
		maxRows = position.Row + 1
	}
	return CellPosition{
		Row:    max(0, min(position.Row, maxRows-1)),
		Column: max(0, min(position.Column, xlsxColumns-1)),
	}
}

// selectionAtPoint maps a pointer position local to the full grid widget to a
// zero-based cell coordinate. The row and column headers are part of that widget.
func selectionAtPoint(v *gridViewport, point image.Point) (CellPosition, bool) {
	if v == nil {
		return CellPosition{}, false
	}
	return selectionAtPointWithHeaders(v, point, int(gridRowHeader*v.Scale), int(gridColumnHeader*v.Scale))
}

func selectionAtPointWithHeaders(v *gridViewport, point image.Point, rowHeader, columnHeader int) (CellPosition, bool) {
	if v == nil || v.Rows == nil || v.Columns == nil || !finite(v.Scale) || v.Scale <= 0 {
		return CellPosition{}, false
	}
	row, column, ok := v.cellPoint(point, rowHeader, columnHeader)
	if !ok {
		return CellPosition{}, false
	}
	maxRow := v.Rows.Count()
	maxCol := v.Columns.Count()
	return CellPosition{
		Row:    max(0, min(row-1, maxRow-1)),
		Column: max(0, min(column-1, maxCol-1)),
	}, true
}

func selectionRect(v *gridViewport, selection CellSelection) image.Rectangle {
	if v == nil {
		return image.Rectangle{}
	}
	return selectionRectWithHeaders(v, selection, int(gridRowHeader*v.Scale), int(gridColumnHeader*v.Scale))
}

func selectionRectWithHeaders(v *gridViewport, selection CellSelection, rowHeader, columnHeader int) image.Rectangle {
	if v == nil || v.Rows == nil || v.Columns == nil {
		return image.Rectangle{}
	}
	return v.screenCellRect(selection.Bounds(), false, false, rowHeader, columnHeader)
}

func selectionBorderRects(rect image.Rectangle, width int) [4]image.Rectangle {
	if width <= 0 || rect.Empty() {
		return [4]image.Rectangle{}
	}
	width = min(width, min(rect.Dx(), rect.Dy()))
	return [4]image.Rectangle{
		image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y+width),
		image.Rect(rect.Min.X, rect.Max.Y-width, rect.Max.X, rect.Max.Y),
		image.Rect(rect.Min.X, rect.Min.Y+width, rect.Min.X+width, rect.Max.Y-width),
		image.Rect(rect.Max.X-width, rect.Min.Y+width, rect.Max.X, rect.Max.Y-width),
	}
}

// Expand until every intersecting merge is wholly inside the rectangle. One
// expansion may meet another merge, so a single pass is insufficient.
func selectionIncludingMerges(selection CellSelection, merges []document.MergeRegion) CellSelection {
	bounds := selection.Bounds()
	original := bounds
	for {
		previous := bounds
		for _, merge := range merges {
			r := merge.Range
			if r.StartRow < 1 || r.StartColumn < 1 || r.EndRow < r.StartRow || r.EndColumn < r.StartColumn {
				continue
			}
			if r.StartRow > bounds.EndRow || r.EndRow < bounds.StartRow || r.StartColumn > bounds.EndColumn || r.EndColumn < bounds.StartColumn {
				continue
			}
			bounds.StartRow = min(bounds.StartRow, r.StartRow)
			bounds.StartColumn = min(bounds.StartColumn, r.StartColumn)
			bounds.EndRow = max(bounds.EndRow, r.EndRow)
			bounds.EndColumn = max(bounds.EndColumn, r.EndColumn)
		}
		if bounds == previous {
			break
		}
	}
	if bounds != original {
		selection.boundsOverride = &bounds
	}
	return selection
}
