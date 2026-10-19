package desktopui

import (
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

func TestMergedCellSelectionDoesNotDrawAnchorCellDivider(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	const width, height = 280, 120
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
	state.selection = CellSelection{Anchor: CellPosition{Row: 0, Column: 0}, Active: CellPosition{Row: 0, Column: 1}}
	state.selectionSet = true
	viewport := newGridViewport()
	viewport.WindowSheet = "Sheet1"
	viewport.WindowRange = document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 2}
	merged := document.Cell{Style: document.CellStyle{FillColor: "#FFFFFF"}}
	viewport.Window = document.SheetWindow{
		StartRow: 1, StartColumn: 1,
		Cells:  [][]document.Cell{{merged, {}}},
		Merges: []document.MergeRegion{{Range: document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 2}, Anchor: merged}},
	}
	state.virtualGrid = viewport

	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(width, height))}
	paintVirtualGrid(gtx, app, viewport)
	if err := renderer.Frame(&ops); err != nil {
		t.Fatalf("render merged cell selection: %v", err)
	}
	screenshot := image.NewRGBA(image.Rect(0, 0, width, height))
	if err := renderer.Screenshot(screenshot); err != nil {
		t.Fatalf("capture merged cell selection: %v", err)
	}
	// The selection covers both original cells. Its fill should stay continuous
	// across their former boundary; only the outside of the merged range gets
	// a selection outline.
	internalEdge := int(gridRowHeader + gridDefaultColumn)
	for x := internalEdge - 2; x <= internalEdge+2; x++ {
		got := color.NRGBAModel.Convert(screenshot.At(x, int(gridColumnHeader)+10)).(color.NRGBA)
		if got != selectedFill {
			t.Fatalf("pixel at x=%d on the merged cell's internal divider = %v; want continuous selected fill %v", x, got, selectedFill)
		}
	}
}

func TestRangeSelectionDoesNotOutlineLastCell(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	const width, height = 280, 120
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
	state.selection = CellSelection{Anchor: CellPosition{Row: 0, Column: 0}, Active: CellPosition{Row: 2, Column: 2}}
	state.selectionSet = true
	viewport := newGridViewport()
	viewport.WindowSheet = "Sheet1"
	viewport.WindowRange = document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 3, EndColumn: 3}
	merged := document.Cell{Style: document.CellStyle{FillColor: "#FFFFFF"}}
	viewport.Window = document.SheetWindow{
		StartRow: 1, StartColumn: 1,
		Cells:  [][]document.Cell{{merged, {}, {}}, {{}, {}, {}}, {{}, {}, {}}},
		Merges: []document.MergeRegion{{Range: document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 2}, Anchor: merged}},
	}
	state.virtualGrid = viewport

	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(width, height))}
	paintVirtualGrid(gtx, app, viewport)
	if err := renderer.Frame(&ops); err != nil {
		t.Fatalf("render merged cell selection: %v", err)
	}
	screenshot := image.NewRGBA(image.Rect(0, 0, width, height))
	if err := renderer.Screenshot(screenshot); err != nil {
		t.Fatalf("capture merged cell selection: %v", err)
	}
	// C3 is the last cell. Its left edge lies inside A1:C3 and must not
	// receive a second selection outline.
	x := int(gridRowHeader + 2*gridDefaultColumn)
	y := int(gridColumnHeader + 2*gridDefaultRow + 10)
	outline := app.tokens().Colors.Grid.SelectionBorder
	for dx := 0; dx < 2; dx++ {
		got := color.NRGBAModel.Convert(screenshot.At(x+dx, y)).(color.NRGBA)
		if got == outline {
			t.Fatalf("duplicate active-cell border at %d,%d", x+dx, y)
		}
	}
	if got := color.NRGBAModel.Convert(screenshot.At(x+10, y)).(color.NRGBA); got != selectedFill {
		t.Fatalf("last cell leaves a fill gap: %v want %v", got, selectedFill)
	}
}
