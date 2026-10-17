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
	state.selection = CellSelection{Anchor: CellPosition{}, Active: CellPosition{}}
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
	// Column A's right edge used to get an extra thin selection outline even
	// though the active-cell outline already covered the full A1:B1 merge.
	internalEdge := image.Pt(int(gridRowHeader+gridDefaultColumn)-1, int(gridColumnHeader)+10)
	got := color.NRGBAModel.Convert(screenshot.At(internalEdge.X, internalEdge.Y)).(color.NRGBA)
	if got != (color.NRGBA{R: 255, G: 255, B: 255, A: 255}) {
		t.Fatalf("pixel at the merged cell's internal divider = %v; want white fill without a selection line", got)
	}
}
