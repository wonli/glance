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
	gridDefaultRow    = 24.0
	gridDefaultColumn = 96.0
	gridRowHeader     = 48.0
	gridColumnHeader  = 28.0
)

type gridViewport struct {
	X, Y                        float64
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
}

func newGridViewport() *gridViewport {
	r, _ := NewAxisGeometry(xlsxRows, gridDefaultRow)
	c, _ := NewAxisGeometry(xlsxColumns, gridDefaultColumn)
	return &gridViewport{Rows: r, Columns: c}
}

func (v *gridViewport) invalidateWindow() { v.WindowRange = document.CellRange{} }

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
	v.Window, v.WindowRange, v.WindowSheet, v.WindowError = w, visible, sheet, nil
	for i, h := range w.RowHeightsPoints {
		if h > 0 {
			_ = v.Rows.SetSize(w.StartRow-1+i, h*96.0/72.0)
		}
	}
	for i, width := range w.ColumnWidthsChars {
		if width > 0 {
			_ = v.Columns.SetSize(w.StartColumn-1+i, math.Max(24, width*7.0+12))
		}
	}
	return nil
}

func containsRange(outer, inner document.CellRange) bool {
	return outer.StartRow > 0 && outer.StartRow <= inner.StartRow && outer.StartColumn <= inner.StartColumn && outer.EndRow >= inner.EndRow && outer.EndColumn >= inner.EndColumn
}

func (v *gridViewport) scroll(dx, dy, viewportWidth, viewportHeight float64) {
	v.X = clamp(v.X+dx, 0, math.Max(0, v.Columns.TotalSize()-viewportWidth))
	v.Y = clamp(v.Y+dy, 0, math.Max(0, v.Rows.TotalSize()-viewportHeight))
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
