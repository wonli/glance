package desktopui

import (
	"image"

	"glance/internal/document"
)

func visibleCellWindow(rowAxis, columnAxis *AxisGeometry, scrollX, scrollY, viewportWidth, viewportHeight float64) document.CellRange {
	rs, re := rowAxis.VisibleRange(scrollY, viewportHeight, 2)
	cs, ce := columnAxis.VisibleRange(scrollX, viewportWidth, 2)
	if re <= rs || ce <= cs {
		return document.CellRange{}
	}
	return document.CellRange{StartRow: rs + 1, StartColumn: cs + 1, EndRow: re, EndColumn: ce}
}

func cellAtPoint(rowAxis, columnAxis *AxisGeometry, scrollX, scrollY float64, point image.Point, rowHeader, columnHeader int) (row, column int, ok bool) {
	x, y := float64(point.X-rowHeader)+scrollX, float64(point.Y-columnHeader)+scrollY
	if x < 0 || y < 0 {
		return 0, 0, false
	}
	c, r := columnAxis.IndexAt(x), rowAxis.IndexAt(y)
	if c < 0 || r < 0 {
		return 0, 0, false
	}
	return r + 1, c + 1, true
}

func mergeAnchorAt(merges []document.MergeRegion, row, column int) (int, int, bool) {
	for _, m := range merges {
		r := m.Range
		if row >= r.StartRow && row <= r.EndRow && column >= r.StartColumn && column <= r.EndColumn {
			return r.StartRow, r.StartColumn, true
		}
	}
	return row, column, false
}

func cellRect(rowAxis, columnAxis *AxisGeometry, r document.CellRange, scrollX, scrollY float64) image.Rectangle {
	x0, _ := columnAxis.Boundary(r.StartColumn - 1)
	x1, _ := columnAxis.Boundary(r.EndColumn)
	y0, _ := rowAxis.Boundary(r.StartRow - 1)
	y1, _ := rowAxis.Boundary(r.EndRow)
	return image.Rect(int(x0-scrollX), int(y0-scrollY), int(x1-scrollX), int(y1-scrollY))
}

func editingCellRect(v *gridViewport, address string, rowHeader, columnHeader int) image.Rectangle {
	if v == nil || v.Rows == nil || v.Columns == nil {
		return image.Rectangle{}
	}
	row, column, ok := parseCellAddress(address)
	if !ok {
		return image.Rectangle{}
	}
	rng := document.CellRange{StartRow: row, StartColumn: column, EndRow: row, EndColumn: column}
	for _, merge := range v.Window.Merges {
		if row >= merge.Range.StartRow && row <= merge.Range.EndRow && column >= merge.Range.StartColumn && column <= merge.Range.EndColumn {
			rng = merge.Range
			break
		}
	}
	return cellRect(v.Rows, v.Columns, rng, v.X, v.Y).Add(image.Pt(rowHeader, columnHeader))
}
