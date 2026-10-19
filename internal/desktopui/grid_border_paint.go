package desktopui

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"glance/internal/document"
)

type gridBorderPaint struct {
	rect   image.Rectangle
	border document.CellBorder
}

// Both owners of a shared edge use the same pixel, just as gridlines do.
// Draw after all cell backgrounds so a neighbor cannot erase that pixel.
func appendGridBorders(edges []gridBorderPaint, rect, viewport image.Rectangle, borders [4]document.CellBorder) []gridBorderPaint {
	left, top := rect.Min.X-1, rect.Min.Y-1
	if rect.Min.X == viewport.Min.X {
		left = rect.Min.X
	}
	if rect.Min.Y == viewport.Min.Y {
		top = rect.Min.Y
	}
	rects := [4]image.Rectangle{
		image.Rect(left, top, rect.Max.X, top+1),
		image.Rect(rect.Max.X-1, top, rect.Max.X, rect.Max.Y),
		image.Rect(left, rect.Max.Y-1, rect.Max.X, rect.Max.Y),
		image.Rect(left, top, left+1, rect.Max.Y),
	}
	for i, b := range borders {
		if b.Style > 0 {
			r := rects[i].Intersect(viewport)
			if !r.Empty() {
				edges = append(edges, gridBorderPaint{rect: r, border: b})
			}
		}
	}
	return edges
}

// The adapter can store a merge's perimeter styles on its covered cells.
// Preserve those edges without introducing any internal cell dividers.
func appendMergedGridBorders(edges []gridBorderPaint, v *gridViewport, pane gridPaintPane, r document.CellRange) []gridBorderPaint {
	for ri, row := range pane.window.Cells {
		rn := pane.window.StartRow + ri
		if rn < r.StartRow || rn > r.EndRow {
			continue
		}
		for ci, cell := range row {
			cn := pane.window.StartColumn + ci
			if cn < r.StartColumn || cn > r.EndColumn {
				continue
			}
			borders := cell.Style.Borders
			if rn != r.StartRow {
				borders[0] = document.CellBorder{}
			}
			if cn != r.EndColumn {
				borders[1] = document.CellBorder{}
			}
			if rn != r.EndRow {
				borders[2] = document.CellBorder{}
			}
			if cn != r.StartColumn {
				borders[3] = document.CellBorder{}
			}
			rect := v.screenCellRect(document.CellRange{StartRow: rn, StartColumn: cn, EndRow: rn, EndColumn: cn}, pane.frozenRows, pane.frozenColumns, pane.rowHeader, pane.columnHeader)
			edges = appendGridBorders(edges, rect, pane.clip, borders)
		}
	}
	return edges
}

func paintGridBorders(gtx layout.Context, edges []gridBorderPaint) {
	for _, edge := range edges {
		paint.FillShape(gtx.Ops, parseGridColor(edge.border.Color, gridLine), clip.Rect(edge.rect).Op())
	}
}
