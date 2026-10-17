package desktopui

import (
	"image"
	"math"

	"glance/internal/document"
)

// gridPaneRanges identifies the four bounded windows needed to paint a frozen
// worksheet without loading the rectangle between the frozen area and a
// distant scrolled viewport.
type gridPaneRanges struct {
	Body, Top, Left, Corner document.CellRange
}

func (r gridPaneRanges) empty() bool {
	return r.Body.StartRow <= 0 && r.Top.StartRow <= 0 && r.Left.StartRow <= 0 && r.Corner.StartRow <= 0
}

func (v *gridViewport) paneSplits() (columns, rows int) {
	if v == nil || !v.PanesLoaded || !v.Panes.Frozen {
		return 0, 0
	}
	return max(0, min(v.Panes.XSplit, axisCount(v.Columns))), max(0, min(v.Panes.YSplit, axisCount(v.Rows)))
}

func (v *gridViewport) paneSplitsAtColumnBoundary(boundary int) bool {
	split, _ := v.paneSplits()
	return split > 0 && boundary <= split
}

func (v *gridViewport) paneSplitsAtRowBoundary(boundary int) bool {
	_, split := v.paneSplits()
	return split > 0 && boundary <= split
}

func (v *gridViewport) frozenColumnBoundary() float64 {
	columns, _ := v.paneSplits()
	boundary, _ := v.Columns.Boundary(columns)
	return boundary
}

func (v *gridViewport) frozenRowBoundary() float64 {
	_, rows := v.paneSplits()
	boundary, _ := v.Rows.Boundary(rows)
	return boundary
}

func (v *gridViewport) scrollWorldX() float64 { return v.frozenColumnBoundary() + v.X }
func (v *gridViewport) scrollWorldY() float64 { return v.frozenRowBoundary() + v.Y }

func (v *gridViewport) frozenViewportExtent(width, height float64) (frozenWidth, frozenHeight float64) {
	frozenWidth = math.Min(width, v.frozenColumnBoundary())
	frozenHeight = math.Min(height, v.frozenRowBoundary())
	return max(0, frozenWidth), max(0, frozenHeight)
}

func (v *gridViewport) visiblePaneRanges(width, height float64) gridPaneRanges {
	if v == nil || v.Rows == nil || v.Columns == nil {
		return gridPaneRanges{}
	}
	columns, rows := v.paneSplits()
	if columns == 0 && rows == 0 {
		return gridPaneRanges{Body: visibleCellWindow(v.Rows, v.Columns, v.X, v.Y, width, height)}
	}
	frozenWidth, frozenHeight := v.frozenViewportExtent(width, height)
	bodyWidth, bodyHeight := math.Max(0, width-frozenWidth), math.Max(0, height-frozenHeight)
	bodyColumnStart, bodyColumnEnd := v.Columns.VisibleRange(v.frozenColumnBoundary()+v.X, bodyWidth, 2)
	bodyRowStart, bodyRowEnd := v.Rows.VisibleRange(v.frozenRowBoundary()+v.Y, bodyHeight, 2)
	bodyColumnStart, bodyRowStart = max(columns, bodyColumnStart), max(rows, bodyRowStart)
	bodyColumnEnd, bodyRowEnd = max(bodyColumnStart, bodyColumnEnd), max(bodyRowStart, bodyRowEnd)
	freezeColumnStart, freezeColumnEnd := 0, 0
	if columns > 0 && frozenWidth > 0 {
		freezeColumnStart, freezeColumnEnd = v.Columns.VisibleRange(0, frozenWidth, 1)
		freezeColumnEnd = min(columns, freezeColumnEnd)
	}
	freezeRowStart, freezeRowEnd := 0, 0
	if rows > 0 && frozenHeight > 0 {
		freezeRowStart, freezeRowEnd = v.Rows.VisibleRange(0, frozenHeight, 1)
		freezeRowEnd = min(rows, freezeRowEnd)
	}
	return gridPaneRanges{
		Body:   gridCellRange(bodyRowStart, bodyRowEnd, bodyColumnStart, bodyColumnEnd),
		Top:    gridCellRange(freezeRowStart, freezeRowEnd, bodyColumnStart, bodyColumnEnd),
		Left:   gridCellRange(bodyRowStart, bodyRowEnd, freezeColumnStart, freezeColumnEnd),
		Corner: gridCellRange(freezeRowStart, freezeRowEnd, freezeColumnStart, freezeColumnEnd),
	}
}

func gridCellRange(rowStart, rowEnd, columnStart, columnEnd int) document.CellRange {
	if rowEnd <= rowStart || columnEnd <= columnStart {
		return document.CellRange{}
	}
	return document.CellRange{StartRow: rowStart + 1, EndRow: rowEnd, StartColumn: columnStart + 1, EndColumn: columnEnd}
}

func (v *gridViewport) windowSetLoaded(sheet string, ranges gridPaneRanges) bool {
	return v != nil && v.LoadedSheet == sheet && v.LoadedWindows == ranges
}

func (v *gridViewport) setPanes(panes document.SpreadsheetPaneFreeze) {
	if v == nil {
		return
	}
	if panes.XSplit < 0 {
		panes.XSplit = 0
	}
	if panes.YSplit < 0 {
		panes.YSplit = 0
	}
	if !panes.Frozen || (panes.XSplit == 0 && panes.YSplit == 0) {
		panes = document.SpreadsheetPaneFreeze{}
	}
	if v.PanesLoaded && v.Panes == panes {
		return
	}
	oldX, oldY := v.scrollWorldX(), v.scrollWorldY()
	wasScrolledX := v.X > 0
	wasScrolledY := v.Y > 0
	v.Panes, v.PanesLoaded = panes, true
	if wasScrolledX {
		v.X = math.Max(0, oldX-v.frozenColumnBoundary())
	} else {
		v.X = 0
	}
	if wasScrolledY {
		v.Y = math.Max(0, oldY-v.frozenRowBoundary())
	} else {
		v.Y = 0
	}
	v.invalidateWindow()
	v.WindowError = nil
	v.GeometryChanged = true
}

func (v *gridViewport) applyPaneWindows(sheet string, ranges gridPaneRanges, body, top, left, corner document.SheetWindow) error {
	if v == nil {
		return nil
	}
	if body.StartRow > 0 {
		v.Window, v.WindowRange, v.WindowSheet = body, ranges.Body, sheet
		if err := v.applyWindowGeometry(body); err != nil {
			return err
		}
	} else {
		v.Window, v.WindowRange, v.WindowSheet = document.SheetWindow{}, document.CellRange{}, sheet
	}
	v.TopWindow, v.TopRange = top, ranges.Top
	v.LeftWindow, v.LeftRange = left, ranges.Left
	v.CornerWindow, v.CornerRange = corner, ranges.Corner
	for _, window := range []document.SheetWindow{top, left, corner} {
		if err := v.applyWindowGeometry(window); err != nil {
			return err
		}
	}
	v.WindowError = nil
	v.LoadedWindows, v.LoadedSheet = ranges, sheet
	v.FailedRange, v.FailedSheet = document.CellRange{}, ""
	v.FailedWindows = gridPaneRanges{}
	return nil
}

func (v *gridViewport) cell(row, column int) document.Cell {
	if v == nil {
		return document.Cell{}
	}
	for _, item := range []struct {
		window  document.SheetWindow
		rangeOf document.CellRange
	}{{v.Window, v.WindowRange}, {v.TopWindow, v.TopRange}, {v.LeftWindow, v.LeftRange}, {v.CornerWindow, v.CornerRange}} {
		if rangeContainsCell(item.rangeOf, row, column) {
			return windowCell(item.window, row, column)
		}
	}
	return document.Cell{}
}

func (v *gridViewport) cellLoaded(row, column int) bool {
	if v == nil {
		return false
	}
	for _, rangeOf := range []document.CellRange{v.WindowRange, v.TopRange, v.LeftRange, v.CornerRange} {
		if rangeContainsCell(rangeOf, row, column) {
			return true
		}
	}
	return false
}

func (v *gridViewport) containsLoadedRange(r document.CellRange) bool {
	if v == nil || r.StartRow <= 0 || r.StartColumn <= 0 || r.EndRow < r.StartRow || r.EndColumn < r.StartColumn {
		return false
	}
	columnSplit, rowSplit := v.paneSplits()
	type axisRange struct{ start, end int }
	rows := []axisRange{{r.StartRow, r.EndRow}}
	columns := []axisRange{{r.StartColumn, r.EndColumn}}
	if rowSplit > 0 {
		rows = rows[:0]
		if end := min(rowSplit, r.EndRow); end >= r.StartRow {
			rows = append(rows, axisRange{r.StartRow, end})
		}
		if start := max(rowSplit+1, r.StartRow); start <= r.EndRow {
			rows = append(rows, axisRange{start, r.EndRow})
		}
	}
	if columnSplit > 0 {
		columns = columns[:0]
		if end := min(columnSplit, r.EndColumn); end >= r.StartColumn {
			columns = append(columns, axisRange{r.StartColumn, end})
		}
		if start := max(columnSplit+1, r.StartColumn); start <= r.EndColumn {
			columns = append(columns, axisRange{start, r.EndColumn})
		}
	}
	for _, rowRange := range rows {
		for _, columnRange := range columns {
			loaded := v.WindowRange
			switch {
			case rowSplit > 0 && rowRange.end <= rowSplit && columnSplit > 0 && columnRange.end <= columnSplit:
				loaded = v.CornerRange
			case rowSplit > 0 && rowRange.end <= rowSplit:
				loaded = v.TopRange
			case columnSplit > 0 && columnRange.end <= columnSplit:
				loaded = v.LeftRange
			}
			if !containsRange(loaded, document.CellRange{
				StartRow: rowRange.start, EndRow: rowRange.end,
				StartColumn: columnRange.start, EndColumn: columnRange.end,
			}) {
				return false
			}
		}
	}
	return true
}

func (v *gridViewport) headerRowAt(position int) int {
	if v == nil || position < 0 {
		return -1
	}
	_, split := v.paneSplits()
	boundary := v.frozenRowBoundary()
	world := float64(position)
	if split == 0 || world >= boundary {
		world += v.Y
	}
	return v.Rows.IndexAt(world)
}

func (v *gridViewport) headerColumnAt(position int) int {
	if v == nil || position < 0 {
		return -1
	}
	split, _ := v.paneSplits()
	boundary := v.frozenColumnBoundary()
	world := float64(position)
	if split == 0 || world >= boundary {
		world += v.X
	}
	return v.Columns.IndexAt(world)
}

func (v *gridViewport) merges() []document.MergeRegion {
	if v == nil {
		return nil
	}
	result := make([]document.MergeRegion, 0, len(v.Window.Merges)+len(v.TopWindow.Merges)+len(v.LeftWindow.Merges)+len(v.CornerWindow.Merges))
	seen := make(map[document.CellRange]struct{})
	for _, window := range []document.SheetWindow{v.Window, v.TopWindow, v.LeftWindow, v.CornerWindow} {
		for _, merge := range window.Merges {
			if _, ok := seen[merge.Range]; ok {
				continue
			}
			seen[merge.Range] = struct{}{}
			result = append(result, merge)
		}
	}
	return result
}

func rangeContainsCell(r document.CellRange, row, column int) bool {
	return r.StartRow > 0 && row >= r.StartRow && row <= r.EndRow && column >= r.StartColumn && column <= r.EndColumn
}

func (v *gridViewport) cellPoint(point image.Point, rowHeader, columnHeader int) (row, column int, ok bool) {
	if v == nil || point.X < rowHeader || point.Y < columnHeader {
		return 0, 0, false
	}
	x, y := float64(point.X-rowHeader), float64(point.Y-columnHeader)
	columnSplit, rowSplit := v.paneSplits()
	frozenWidth, frozenHeight := v.frozenViewportExtent(math.Inf(1), math.Inf(1))
	if columnSplit == 0 || x >= frozenWidth {
		x = v.frozenColumnBoundary() + v.X + x - frozenWidth
	}
	if rowSplit == 0 || y >= frozenHeight {
		y = v.frozenRowBoundary() + v.Y + y - frozenHeight
	}
	column, row = v.Columns.IndexAt(x), v.Rows.IndexAt(y)
	if column < 0 || row < 0 {
		return 0, 0, false
	}
	return row + 1, column + 1, true
}

func (v *gridViewport) cellRangeRect(r document.CellRange, rowHeader, columnHeader int) image.Rectangle {
	if v == nil || r.StartRow <= 0 || r.StartColumn <= 0 {
		return image.Rectangle{}
	}
	columnSplit, rowSplit := v.paneSplits()
	return v.screenCellRect(r, rowSplit > 0 && r.EndRow <= rowSplit, columnSplit > 0 && r.EndColumn <= columnSplit, rowHeader, columnHeader)
}

func (v *gridViewport) screenCellRect(r document.CellRange, frozenRow, frozenColumn bool, rowHeader, columnHeader int) image.Rectangle {
	if v == nil || v.Columns == nil || v.Rows == nil || r.StartRow <= 0 || r.StartColumn <= 0 {
		return image.Rectangle{}
	}
	if r.EndRow < r.StartRow || r.EndColumn < r.StartColumn {
		return image.Rectangle{}
	}
	startCol := min(max(0, r.StartColumn-1), v.Columns.Count())
	endCol := min(max(0, r.EndColumn), v.Columns.Count())
	startRow := min(max(0, r.StartRow-1), v.Rows.Count())
	endRow := min(max(0, r.EndRow), v.Rows.Count())
	x0, _ := v.Columns.Boundary(startCol)
	x1, _ := v.Columns.Boundary(endCol)
	y0, _ := v.Rows.Boundary(startRow)
	y1, _ := v.Rows.Boundary(endRow)
	if !frozenColumn {
		x0 -= v.X
		x1 -= v.X
	}
	if !frozenRow {
		y0 -= v.Y
		y1 -= v.Y
	}
	return image.Rect(int(x0)+rowHeader, int(y0)+columnHeader, int(x1)+rowHeader, int(y1)+columnHeader)
}

func (v *gridViewport) paneClipForCell(row, column int, size image.Point, rowHeader, columnHeader int) image.Rectangle {
	viewport := image.Rect(rowHeader, columnHeader, size.X, size.Y)
	columnSplit, rowSplit := v.paneSplits()
	frozenWidth, frozenHeight := v.frozenViewportExtent(float64(viewport.Dx()), float64(viewport.Dy()))
	if columnSplit > 0 {
		if column <= columnSplit {
			viewport.Max.X = rowHeader + int(frozenWidth)
		} else {
			viewport.Min.X = rowHeader + int(frozenWidth)
		}
	}
	if rowSplit > 0 {
		if row <= rowSplit {
			viewport.Max.Y = columnHeader + int(frozenHeight)
		} else {
			viewport.Min.Y = columnHeader + int(frozenHeight)
		}
	}
	return viewport
}

func (v *gridViewport) screenColumnBoundary(index int, frozen bool, rowHeader int) float64 {
	boundary, _ := v.Columns.Boundary(index)
	if !frozen {
		boundary -= v.X
	}
	return float64(rowHeader) + boundary
}

func (v *gridViewport) screenRowBoundary(index int, frozen bool, columnHeader int) float64 {
	boundary, _ := v.Rows.Boundary(index)
	if !frozen {
		boundary -= v.Y
	}
	return float64(columnHeader) + boundary
}

func (v *gridViewport) selectionRects(r document.CellRange, rowHeader, columnHeader int, viewport image.Rectangle) []image.Rectangle {
	if v == nil || r.StartRow <= 0 || r.StartColumn <= 0 {
		return nil
	}
	columnSplit, rowSplit := v.paneSplits()
	type axisRange struct {
		start, end int
		frozen     bool
	}
	rows := []axisRange{{start: r.StartRow - 1, end: r.EndRow, frozen: false}}
	columns := []axisRange{{start: r.StartColumn - 1, end: r.EndColumn, frozen: false}}
	if rowSplit > 0 {
		rows = rows[:0]
		if start, end := max(0, r.StartRow-1), min(rowSplit, r.EndRow); end > start {
			rows = append(rows, axisRange{start: start, end: end, frozen: true})
		}
		if start, end := max(rowSplit, r.StartRow-1), r.EndRow; end > start {
			rows = append(rows, axisRange{start: start, end: end})
		}
	}
	if columnSplit > 0 {
		columns = columns[:0]
		if start, end := max(0, r.StartColumn-1), min(columnSplit, r.EndColumn); end > start {
			columns = append(columns, axisRange{start: start, end: end, frozen: true})
		}
		if start, end := max(columnSplit, r.StartColumn-1), r.EndColumn; end > start {
			columns = append(columns, axisRange{start: start, end: end})
		}
	}
	frozenWidth, frozenHeight := v.frozenViewportExtent(float64(viewport.Dx()), float64(viewport.Dy()))
	result := make([]image.Rectangle, 0, len(rows)*len(columns))
	for _, rowRange := range rows {
		for _, columnRange := range columns {
			bounds := document.CellRange{StartRow: rowRange.start + 1, EndRow: rowRange.end, StartColumn: columnRange.start + 1, EndColumn: columnRange.end}
			rect := v.screenCellRect(bounds, rowRange.frozen, columnRange.frozen, rowHeader, columnHeader)
			clipRect := viewport
			if columnSplit > 0 {
				if columnRange.frozen {
					clipRect.Max.X = min(clipRect.Max.X, rowHeader+int(frozenWidth))
				} else {
					clipRect.Min.X = max(clipRect.Min.X, rowHeader+int(frozenWidth))
				}
			}
			if rowSplit > 0 {
				if rowRange.frozen {
					clipRect.Max.Y = min(clipRect.Max.Y, columnHeader+int(frozenHeight))
				} else {
					clipRect.Min.Y = max(clipRect.Min.Y, columnHeader+int(frozenHeight))
				}
			}
			rect = rect.Intersect(clipRect)
			if !rect.Empty() {
				result = append(result, rect)
			}
		}
	}
	return result
}

func (v *gridViewport) applyWindowGeometry(w document.SheetWindow) error {
	anchorRow, anchorColumn := v.Rows.IndexAt(v.scrollWorldY()), v.Columns.IndexAt(v.scrollWorldX())
	rowWithin, columnWithin := 0.0, 0.0
	if anchorRow >= 0 {
		edge, _ := v.Rows.Boundary(anchorRow)
		rowWithin = v.scrollWorldY() - edge
	}
	if anchorColumn >= 0 {
		edge, _ := v.Columns.Boundary(anchorColumn)
		columnWithin = v.scrollWorldX() - edge
	}
	for i, h := range w.RowHeightsPoints {
		if h <= 0 {
			continue
		}
		index := w.StartRow - 1 + i
		if v.Resize != nil && v.Resize.State.Axis == resizeRows && v.Resize.State.Index == index {
			continue
		}
		old := v.Rows.sizeAt(index)
		if err := v.Rows.SetSize(index, pointsToGridPixels(h)*v.Scale); err != nil {
			return err
		}
		if old != v.Rows.sizeAt(index) {
			v.GeometryChanged = true
		}
	}
	for i, width := range w.ColumnWidthsChars {
		if width <= 0 {
			continue
		}
		index := w.StartColumn - 1 + i
		if v.Resize != nil && v.Resize.State.Axis == resizeColumns && v.Resize.State.Index == index {
			continue
		}
		old := v.Columns.sizeAt(index)
		if err := v.Columns.SetSize(index, math.Max(minGridColumnPx*v.Scale, columnCharsToGridPixels(width)*v.Scale)); err != nil {
			return err
		}
		if old != v.Columns.sizeAt(index) {
			v.GeometryChanged = true
		}
	}
	if v.GeometryChanged {
		if anchorRow >= 0 {
			edge, _ := v.Rows.Boundary(anchorRow)
			v.Y = math.Max(0, edge+rowWithin-v.frozenRowBoundary())
		}
		if anchorColumn >= 0 {
			edge, _ := v.Columns.Boundary(anchorColumn)
			v.X = math.Max(0, edge+columnWithin-v.frozenColumnBoundary())
		}
	}
	return nil
}
