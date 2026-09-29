package desktopui

import (
	"fmt"
	"math"
	"time"

	"glance/internal/document"
)

const (
	xlsxRows          = 1048576
	xlsxColumns       = 16384
	gridDefaultRow    = 32.0
	gridDefaultColumn = 90.0
	gridRowHeader     = 42.0
	gridColumnHeader  = 32.0
)

type gridViewport struct {
	X, Y                        float64
	Scale                       float64
	Rows, Columns               *AxisGeometry
	Window                      document.SheetWindow
	WindowRange                 document.CellRange
	WindowSheet                 string
	SelectedRow, SelectedColumn int
	PointerTag                  struct{}
	WindowError                 error
	LastClickAddress            string
	LastClickTime               time.Duration
	PendingSelectionLoad        bool
	Resize                      *gridResizeSession
	ResizeCursor                resizeAxis
	GeometryChanged             bool
	Loading                     bool
	RequestedRange              document.CellRange
	RequestedSheet              string
	FailedRange                 document.CellRange
	FailedSheet                 string
	CellPaintCache              map[cellPaintKey]*cellPaintEntry
	CellPaintFIFO               []cellPaintKey
}

type gridLoadResult struct {
	generation uint64
	viewport   *gridViewport
	sheet      string
	rangeOf    document.CellRange
	window     document.SheetWindow
	err        error
}

func newGridViewport() *gridViewport {
	r, _ := NewAxisGeometry(xlsxRows, gridDefaultRow)
	c, _ := NewAxisGeometry(xlsxColumns, gridDefaultColumn)
	return &gridViewport{Rows: r, Columns: c, Scale: 1}
}

func (v *gridViewport) setScale(scale float64) {
	if !finite(scale) || scale <= 0 {
		scale = 1
	}
	if v.Scale == scale {
		return
	}
	factor := scale / v.Scale
	if v.Resize != nil {
		v.cancelResize()
		v.ResizeCursor = 0
	}
	_ = v.Rows.Scale(factor)
	_ = v.Columns.Scale(factor)
	v.X *= factor
	v.Y *= factor
	v.Scale = scale
	if v.WindowRange.StartRow > 0 {
		v.GeometryChanged = false
		_ = v.applyWindow(v.WindowSheet, v.WindowRange, v.Window)
	}
	v.GeometryChanged = true
}

func (v *gridViewport) invalidateWindow() {
	v.WindowRange = document.CellRange{}
	v.FailedRange = document.CellRange{}
	v.FailedSheet = ""
}

func (v *gridViewport) ensureWindow(reader document.GridReader, sheet string, visible document.CellRange) error {
	if visible.StartRow <= 0 || visible.StartColumn <= 0 {
		return nil
	}
	if v.WindowSheet == sheet && containsRange(v.WindowRange, visible) {
		return nil
	}
	rows, cols := visible.EndRow-visible.StartRow+1, visible.EndColumn-visible.StartColumn+1
	w, err := reader.Window(sheet, visible.StartRow, visible.StartColumn, rows, cols)
	if err != nil {
		v.WindowError = err
		return err
	}
	return v.applyWindow(sheet, visible, w)
}

func (v *gridViewport) applyWindow(sheet string, visible document.CellRange, w document.SheetWindow) error {
	anchorRow, anchorColumn := v.Rows.IndexAt(v.Y), v.Columns.IndexAt(v.X)
	rowWithin, columnWithin := 0.0, 0.0
	if anchorRow >= 0 {
		edge, _ := v.Rows.Boundary(anchorRow)
		rowWithin = v.Y - edge
	}
	if anchorColumn >= 0 {
		edge, _ := v.Columns.Boundary(anchorColumn)
		columnWithin = v.X - edge
	}
	v.Window, v.WindowRange, v.WindowSheet, v.WindowError = w, visible, sheet, nil
	for i, h := range w.RowHeightsPoints {
		if h > 0 {
			if v.Resize != nil && v.Resize.State.Axis == resizeRows && v.Resize.State.Index == w.StartRow-1+i {
				continue
			}
			old := v.Rows.sizeAt(w.StartRow - 1 + i)
			_ = v.Rows.SetSize(w.StartRow-1+i, pointsToGridPixels(h)*v.Scale)
			if old != v.Rows.sizeAt(w.StartRow-1+i) {
				v.GeometryChanged = true
			}
		}
	}
	for i, width := range w.ColumnWidthsChars {
		if width > 0 {
			if v.Resize != nil && v.Resize.State.Axis == resizeColumns && v.Resize.State.Index == w.StartColumn-1+i {
				continue
			}
			old := v.Columns.sizeAt(w.StartColumn - 1 + i)
			_ = v.Columns.SetSize(w.StartColumn-1+i, math.Max(minGridColumnPx*v.Scale, columnCharsToGridPixels(width)*v.Scale))
			if old != v.Columns.sizeAt(w.StartColumn-1+i) {
				v.GeometryChanged = true
			}
		}
	}
	if v.GeometryChanged {
		if anchorRow >= 0 {
			edge, _ := v.Rows.Boundary(anchorRow)
			v.Y = math.Max(0, edge+rowWithin)
		}
		if anchorColumn >= 0 {
			edge, _ := v.Columns.Boundary(anchorColumn)
			v.X = math.Max(0, edge+columnWithin)
		}
	}
	v.FailedRange, v.FailedSheet = document.CellRange{}, ""
	return nil
}

func containsRange(outer, inner document.CellRange) bool {
	return outer.StartRow > 0 && outer.StartRow <= inner.StartRow && outer.StartColumn <= inner.StartColumn && outer.EndRow >= inner.EndRow && outer.EndColumn >= inner.EndColumn
}

func (v *gridViewport) scroll(dx, dy, viewportWidth, viewportHeight float64) {
	v.X = clamp(v.X+dx, 0, math.Max(0, v.Columns.TotalSize()-viewportWidth))
	v.Y = clamp(v.Y+dy, 0, math.Max(0, v.Rows.TotalSize()-viewportHeight))
}

func gridScrollDelta(scrollX, scrollY float64, shift bool) (dx, dy float64) {
	if shift {
		return scrollX + scrollY, 0
	}
	return scrollX, scrollY
}

func clamp(x, min, max float64) float64 {
	if x < min {
		return min
	}
	if x > max {
		return max
	}
	return x
}

func gridWindowError(sheet string, err error) string {
	return fmt.Sprintf("工作表「%s」读取失败：%v", sheet, err)
}
