package desktopui

import (
	"fmt"
	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/document"
	"image"
	"image/color"
	"testing"
)

func TestGridSharedAndMergedBordersUseOnePixel(t *testing.T) {
	for _, scale := range []int{1, 2} {
		for _, merged := range []bool{false, true} {
			name := "adjacent"
			if merged {
				name = "merged"
			}
			t.Run(fmt.Sprintf("%s/%dx", name, scale), func(t *testing.T) {
				w, err := headless.NewWindow(250*scale, 100*scale)
				if err != nil {
					t.Skipf("GPU unavailable: %v", err)
				}
				defer w.Release()
				a := New(nil, "")
				defer close(a.shutdown)
				v := newGridViewport()
				v.setScale(float64(scale))
				b := document.CellBorder{Style: 1, Color: "#000000"}
				cell := document.Cell{Style: document.CellStyle{FillColor: "#FFFFFF", Borders: [4]document.CellBorder{b, b, b, b}}}
				v.Window = document.SheetWindow{StartRow: 1, StartColumn: 1, Cells: [][]document.Cell{{cell, cell}, {cell, cell}}}
				if merged {
					anchor := cell
					anchor.Style.Borders[1] = document.CellBorder{}
					v.Window.Cells[0][0] = anchor
					v.Window.Merges = []document.MergeRegion{{Range: document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 2}, Anchor: anchor}}
				}
				pane := gridPaintPane{window: v.Window, windowRange: document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 2, EndColumn: 2}, clip: image.Rect(42*scale, 22*scale, 250*scale, 100*scale), rowHeader: 42 * scale, columnHeader: 22 * scale}
				var ops op.Ops
				paintGridPaneCells(layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: float32(scale), PxPerSp: float32(scale)}, Constraints: layout.Exact(image.Pt(250*scale, 100*scale))}, a, v, pane, CellSelection{}, false, nil, false)
				if err := w.Frame(&ops); err != nil {
					t.Fatal(err)
				}
				shot := image.NewRGBA(image.Rect(0, 0, 250*scale, 100*scale))
				if err := w.Screenshot(shot); err != nil {
					t.Fatal(err)
				}
				black := func(x int) bool {
					return color.NRGBAModel.Convert(shot.At(x, 30*scale)).(color.NRGBA) == (color.NRGBA{A: 255})
				}
				edge := (42 + int(gridDefaultColumn)) * scale
				if !merged && (!black(edge-1) || black(edge)) {
					t.Fatalf("shared edge must be exactly one pixel: left=%v right=%v", black(edge-1), black(edge))
				}
				horizontal := (22 + int(gridDefaultRow)) * scale
				for dy := -1; dy <= 0; dy++ {
					got := color.NRGBAModel.Convert(shot.At(50*scale, horizontal+dy)).(color.NRGBA)
					if (got == (color.NRGBA{A: 255})) != (dy == -1) {
						t.Fatalf("horizontal shared edge: offset=%d color=%v", dy, got)
					}
				}
				if merged && !black((42+2*int(gridDefaultColumn))*scale-1) {
					t.Fatal("merged right border is missing")
				}
			})
		}
	}

}
