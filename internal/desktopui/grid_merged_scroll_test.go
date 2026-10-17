package desktopui

import (
	"image"
	"runtime"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"glance/internal/document"
)

func TestMergedCellGeometryPreservesFullBoundsWhenScrolled(t *testing.T) {
	v := newGridViewport()
	const rowHeader, columnHeader = 40, 30
	mergeRange := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 4}

	// Initial position: v.X = 0
	rect0 := v.screenCellRect(mergeRange, false, false, rowHeader, columnHeader)
	wantWidth := int(gridDefaultColumn * 4)
	if rect0.Dx() != wantWidth {
		t.Fatalf("rect0.Dx() = %d; want %d", rect0.Dx(), wantWidth)
	}
	if rect0.Min.X != rowHeader {
		t.Fatalf("rect0.Min.X = %d; want %d", rect0.Min.X, rowHeader)
	}

	// Drag horizontal scrollbar by 80px (scrolled past column 1)
	v.X = 80
	rectScrolled := v.screenCellRect(mergeRange, false, false, rowHeader, columnHeader)
	if rectScrolled.Dx() != wantWidth {
		t.Fatalf("rectScrolled.Dx() = %d; want %d (full merged width must be preserved)", rectScrolled.Dx(), wantWidth)
	}
	wantScrolledMinX := rowHeader - 80
	if rectScrolled.Min.X != wantScrolledMinX {
		t.Fatalf("rectScrolled.Min.X = %d; want %d", rectScrolled.Min.X, wantScrolledMinX)
	}

	// Verify clipping: visibleRect must clip to viewport, but fullRect remains at world offset
	clip := image.Rect(rowHeader, columnHeader, 500, 300)
	visible := rectScrolled.Intersect(clip)
	if visible.Min.X != rowHeader {
		t.Fatalf("visible.Min.X = %d; want %d", visible.Min.X, rowHeader)
	}
	if visible.Max.X != rectScrolled.Max.X {
		t.Fatalf("visible.Max.X = %d; want %d", visible.Max.X, rectScrolled.Max.X)
	}
}

func TestMergedCellTextStaysFixedInWorldCoordinatesDuringHorizontalScroll(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	const width, height = 480, 160
	renderer, err := headless.NewWindow(width, height)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer renderer.Release()

	app := New(nil, "")
	defer close(app.shutdown)
	app.sheet = "Sheet1"
	state := app.documentSessionState
	state.sheet = "Sheet1"

	viewport := newGridViewport()
	viewport.WindowSheet = "Sheet1"
	state.virtualGrid = viewport

	cellText := "MERGED"
	mergedCell := document.Cell{
		Value: cellText,
		Style: document.CellStyle{
			HorizontalAlignment: "center",
			FillColor:           "#FFFFFF",
		},
	}
	mergeRange := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 4}

	findTextInkCenter := func(vX float64, windowStartCol int) int {
		viewport.X = vX
		viewport.WindowRange = document.CellRange{StartRow: 1, StartColumn: windowStartCol, EndRow: 5, EndColumn: windowStartCol + 5}
		viewport.Window = document.SheetWindow{
			StartRow:    1,
			StartColumn: windowStartCol,
			Cells: [][]document.Cell{
				{mergedCell, {}, {}, {}},
			},
			Merges: []document.MergeRegion{
				{Range: mergeRange, Anchor: mergedCell},
			},
		}

		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(width, height))}
		paintVirtualGrid(gtx, app, viewport)
		if err := renderer.Frame(&ops); err != nil {
			t.Fatalf("render frame at scrollX=%v: %v", vX, err)
		}
		screenshot := image.NewRGBA(image.Rect(0, 0, width, height))
		if err := renderer.Screenshot(screenshot); err != nil {
			t.Fatalf("capture screenshot at scrollX=%v: %v", vX, err)
		}

		// Look for dark pixels (text ink) in row 1
		rowTop := int(gridColumnHeader)
		rowBottom := int(gridColumnHeader + gridDefaultRow)
		minX, maxX := width, -1
		for y := rowTop; y < rowBottom; y++ {
			for x := int(gridRowHeader); x < width; x++ {
				c := screenshot.RGBAAt(x, y)
				// Check for dark text ink (NRGBA with low RGB components and non-zero alpha)
				if c.A > 200 && c.R < 80 && c.G < 80 && c.B < 80 {
					if x < minX {
						minX = x
					}
					if x > maxX {
						maxX = x
					}
				}
			}
		}
		if minX > maxX {
			t.Fatalf("no text ink found in row 1 at scrollX=%v", vX)
		}
		return (minX + maxX) / 2
	}

	center0 := findTextInkCenter(0, 1)

	// Scroll by 30 pixels (column 1 still partially visible)
	center30 := findTextInkCenter(30, 1)
	diff30 := center0 - center30
	if diff30 != 30 {
		t.Fatalf("text center shifted by %d px after 30px scroll; want exactly 30 px shift", diff30)
	}

	// Scroll by 64 pixels (scrolled past column 1; window starts at column 2)
	center64 := findTextInkCenter(64, 2)
	diff64 := center0 - center64
	if diff64 != 64 {
		t.Fatalf("text center shifted by %d px after 64px scroll (windowStart=2); want exactly 64 px shift (text was following scroll position!)", diff64)
	}
}
