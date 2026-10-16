package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"runtime"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/document"
)

func TestFrozenGridRowsAndColumnsRenderInFixedPanesAtOneAndTwoX(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	for _, scale := range []int{1, 2} {
		t.Run(fmt.Sprintf("%dx", scale), func(t *testing.T) {
			const viewportWidthDP, viewportHeightDP = 400, 240
			size := image.Pt((int(gridRowHeader)+viewportWidthDP)*scale, (int(gridColumnHeader)+viewportHeightDP)*scale)
			renderer, err := headless.NewWindow(size.X, size.Y)
			if err != nil {
				t.Skipf("headless GPU is unavailable: %v", err)
			}
			defer renderer.Release()

			app := New(nil, "")
			defer close(app.shutdown)
			app.sheet = "Sheet1"
			app.documentSessionState.sheet = "Sheet1"
			viewport := newGridViewport()
			viewport.setScale(float64(scale))
			viewport.setPanes(document.SpreadsheetPaneFreeze{Frozen: true, XSplit: 1, YSplit: 1})
			app.documentSessionState.virtualGrid = viewport

			var ops op.Ops
			metric := unit.Metric{PxPerDp: float32(scale), PxPerSp: float32(scale)}
			render := func(scrollXDP, scrollYDP float64) *image.RGBA {
				t.Helper()
				viewport.X, viewport.Y = scrollXDP*float64(scale), scrollYDP*float64(scale)
				ranges := viewport.visiblePaneRanges(float64(size.X-gridRowHeader*scale), float64(size.Y-gridColumnHeader*scale))
				viewport.WindowRange, viewport.TopRange = ranges.Body, ranges.Top
				viewport.LeftRange, viewport.CornerRange = ranges.Left, ranges.Corner
				viewport.Window = freezeVisualWindow(ranges.Body)
				viewport.TopWindow = freezeVisualWindow(ranges.Top)
				viewport.LeftWindow = freezeVisualWindow(ranges.Left)
				viewport.CornerWindow = freezeVisualWindow(ranges.Corner)

				ops.Reset()
				gtx := layout.Context{Ops: &ops, Metric: metric, Constraints: layout.Exact(size)}
				paintVirtualGrid(gtx, app, viewport)
				if err := renderer.Frame(&ops); err != nil {
					t.Fatalf("render frozen worksheet at scroll (%g,%g): %v", scrollXDP, scrollYDP, err)
				}
				shot := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
				if err := renderer.Screenshot(shot); err != nil {
					t.Fatalf("capture frozen worksheet at scroll (%g,%g): %v", scrollXDP, scrollYDP, err)
				}
				return shot
			}

			base := render(0, 0)
			verticalScroll := render(0, gridDefaultRow)
			horizontalScroll := render(gridDefaultColumn, 0)

			pixel := func(img *image.RGBA, xDP, yDP int) color.NRGBA {
				return color.NRGBAModel.Convert(img.At(xDP*scale, yDP*scale)).(color.NRGBA)
			}
			assertPixel := func(img *image.RGBA, xDP, yDP, row, column int, label string) {
				t.Helper()
				if got, want := pixel(img, xDP, yDP), freezeVisualColor(row, column); got != want {
					t.Fatalf("%s pixel at (%d,%d) = %v; want row %d column %d fill %v", label, xDP, yDP, got, row, column, want)
				}
			}

			// The corner cell stays fixed in both directions; row 1 stays put
			// during vertical scrolling while ordinary body cells move down.
			assertPixel(base, int(gridRowHeader)+5, int(gridColumnHeader)+5, 1, 1, "base frozen corner")
			assertPixel(verticalScroll, int(gridRowHeader)+5, int(gridColumnHeader)+5, 1, 1, "vertically scrolled frozen corner")
			assertPixel(horizontalScroll, int(gridRowHeader)+5, int(gridColumnHeader)+5, 1, 1, "horizontally scrolled frozen corner")
			assertPixel(base, int(gridRowHeader)+int(gridDefaultColumn)+5, int(gridColumnHeader)+5, 1, 2, "base frozen row")
			assertPixel(verticalScroll, int(gridRowHeader)+int(gridDefaultColumn)+5, int(gridColumnHeader)+5, 1, 2, "vertically scrolled frozen row")
			assertPixel(base, int(gridRowHeader)+int(gridDefaultColumn)+5, int(gridColumnHeader)+int(gridDefaultRow)+5, 2, 2, "base body")
			assertPixel(verticalScroll, int(gridRowHeader)+int(gridDefaultColumn)+5, int(gridColumnHeader)+int(gridDefaultRow)+5, 3, 2, "vertically scrolled body")

			// Column A stays fixed during horizontal scrolling while body cells
			// advance into the same viewport location.
			assertPixel(base, int(gridRowHeader)+5, int(gridColumnHeader)+int(gridDefaultRow)+5, 2, 1, "base frozen column")
			assertPixel(horizontalScroll, int(gridRowHeader)+5, int(gridColumnHeader)+int(gridDefaultRow)+5, 2, 1, "horizontally scrolled frozen column")
			assertPixel(horizontalScroll, int(gridRowHeader)+int(gridDefaultColumn)+5, int(gridColumnHeader)+int(gridDefaultRow)+5, 2, 3, "horizontally scrolled body")
		})
	}
}

func freezeVisualWindow(bounds document.CellRange) document.SheetWindow {
	if bounds.StartRow <= 0 || bounds.StartColumn <= 0 {
		return document.SheetWindow{}
	}
	window := document.SheetWindow{StartRow: bounds.StartRow, StartColumn: bounds.StartColumn}
	window.Cells = make([][]document.Cell, bounds.EndRow-bounds.StartRow+1)
	for rowIndex := range window.Cells {
		row := bounds.StartRow + rowIndex
		window.Cells[rowIndex] = make([]document.Cell, bounds.EndColumn-bounds.StartColumn+1)
		for columnIndex := range window.Cells[rowIndex] {
			column := bounds.StartColumn + columnIndex
			window.Cells[rowIndex][columnIndex].Style.FillColor = freezeVisualColorHex(row, column)
		}
	}
	return window
}

func freezeVisualColor(row, column int) color.NRGBA {
	return color.NRGBA{
		R: uint8((row*41+column*17)%180 + 30),
		G: uint8((row*13+column*53)%180 + 30),
		B: uint8((row*29+column*31)%180 + 30),
		A: 255,
	}
}

func freezeVisualColorHex(row, column int) string {
	c := freezeVisualColor(row, column)
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}
