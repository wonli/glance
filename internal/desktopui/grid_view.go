package desktopui

import (
	"fmt"
	"math"
	"time"

	"glance/internal/document"
)

const (
	xlsxRows    = 1048576
	xlsxColumns = 16384
	// Excel's default row height is 15 pt (20 px at 96 DPI); its default
	// column width is 8.43 characters (about 64 px with the grid conversion).
	// Keep the pre-load viewport geometry aligned with the XLSX values that
	// replace it after the first asynchronous window read.
	gridDefaultRow    = 20.0
	gridDefaultColumn = 64.0
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
	Panes                       document.SpreadsheetPaneFreeze
	PanesLoaded                 bool
	TopWindow, LeftWindow       document.SheetWindow
	CornerWindow                document.SheetWindow
	TopRange, LeftRange         document.CellRange
	CornerRange                 document.CellRange
	LoadedWindows               gridPaneRanges
	RequestedWindows            gridPaneRanges
	FailedWindows               gridPaneRanges
	LoadedSheet                 string
	SelectedRow, SelectedColumn int
	PointerTag                  struct{}
	WindowError                 error
	LastClickAddress            string
	LastClickTime               time.Duration
	PendingSelectionLoad        bool
	KeyboardFocusRequested      bool
	Resize                      *gridResizeSession
	ResizeCursor                resizeAxis
	PictureResizeCursor         bool
	PictureMoveCursor           bool
	GeometryChanged             bool
	Loading                     bool
	RequestSequence             uint64
	RequestedID                 uint64
	RequestedRange              document.CellRange
	RequestedSheet              string
	FailedRange                 document.CellRange
	FailedSheet                 string
	CellPaintCache              map[cellPaintKey]*cellPaintEntry
	CellPaintFIFO               []cellPaintKey
	CellEditRowPreview          *gridCellRowPreview
}

type gridLoadResult struct {
	generation uint64
	requestID  uint64
	viewport   *gridViewport
	sheet      string
	rangeOf    document.CellRange
	windows    gridPaneRanges
	window     document.SheetWindow
	top        document.SheetWindow
	left       document.SheetWindow
	corner     document.SheetWindow
	panes      document.SpreadsheetPaneFreeze
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
	if preview := v.CellEditRowPreview; preview != nil {
		preview.OriginalPx *= factor
		preview.BasePx *= factor
		preview.PreviewPx *= factor
	}
	if v.Resize != nil {
		v.cancelResize()
		v.ResizeCursor = 0
	}
	_ = v.Rows.Scale(factor)
	_ = v.Columns.Scale(factor)
	v.X *= factor
	v.Y *= factor
	v.Scale = scale
	v.GeometryChanged = true
}

func (v *gridViewport) invalidateWindow() {
	v.RequestSequence++
	v.Loading = false
	v.WindowRange = document.CellRange{}
	v.TopWindow, v.LeftWindow, v.CornerWindow = document.SheetWindow{}, document.SheetWindow{}, document.SheetWindow{}
	v.TopRange, v.LeftRange, v.CornerRange = document.CellRange{}, document.CellRange{}, document.CellRange{}
	v.LoadedWindows = gridPaneRanges{}
	v.LoadedSheet = ""
	v.FailedWindows = gridPaneRanges{}
	v.FailedRange = document.CellRange{}
	v.FailedSheet = ""
}

func (v *gridViewport) applyLoadResult(result gridLoadResult) error {
	v.setPanes(result.panes)
	return v.applyPaneWindows(result.sheet, result.windows, result.window, result.top, result.left, result.corner)
}

func (v *gridViewport) resetSheetWindow() {
	worldX, worldY := 0.0, 0.0
	if v.X > 0 {
		worldX = v.scrollWorldX()
	}
	if v.Y > 0 {
		worldY = v.scrollWorldY()
	}
	v.invalidateWindow()
	v.Panes = document.SpreadsheetPaneFreeze{}
	v.PanesLoaded = false
	v.WindowSheet = ""
	v.X, v.Y = worldX, worldY
	v.GeometryChanged = true
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
	v.setPanes(w.Panes)
	if err := v.applyWindow(sheet, visible, w); err != nil {
		return err
	}
	v.LoadedWindows = gridPaneRanges{Body: visible}
	v.LoadedSheet = sheet
	return nil
}

func (v *gridViewport) applyWindow(sheet string, visible document.CellRange, w document.SheetWindow) error {
	v.Window, v.WindowRange, v.WindowSheet, v.WindowError = w, visible, sheet, nil
	if err := v.applyWindowGeometry(w); err != nil {
		return err
	}
	v.FailedRange, v.FailedSheet = document.CellRange{}, ""
	return nil
}

func containsRange(outer, inner document.CellRange) bool {
	return outer.StartRow > 0 && outer.StartRow <= inner.StartRow && outer.StartColumn <= inner.StartColumn && outer.EndRow >= inner.EndRow && outer.EndColumn >= inner.EndColumn
}

func (v *gridViewport) scroll(dx, dy, viewportWidth, viewportHeight float64) {
	width, height := v.frozenViewportExtent(viewportWidth, viewportHeight)
	bodyWidth, bodyHeight := math.Max(0, viewportWidth-width), math.Max(0, viewportHeight-height)
	maxX := math.Max(0, v.Columns.TotalSize()-v.frozenColumnBoundary()-bodyWidth)
	maxY := math.Max(0, v.Rows.TotalSize()-v.frozenRowBoundary()-bodyHeight)
	v.X = clamp(v.X+dx, 0, maxX)
	v.Y = clamp(v.Y+dy, 0, maxY)
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
