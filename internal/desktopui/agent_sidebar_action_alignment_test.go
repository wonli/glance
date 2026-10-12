package desktopui

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"image"
	"image/color"
	"runtime"
	"testing"
)

func TestAgentSidebarResponseActionsVisibleCentersAlign(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a := New(nil, "")
		var ops op.Ops
		size := image.Pt(int(180*scale), int(20*scale))
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
		paint.FillShape(&ops, color.NRGBA{R: 248, G: 249, B: 251, A: 255}, clip.Rect{Max: size}.Op())
		a.layoutAgentSidebarResponseActions(gtx)
		img := renderPresentationTestOps(t, &ops, size)
		centers := []int{}
		for _, region := range []image.Rectangle{image.Rect(0, 0, 13, 20), image.Rect(25, 0, 38, 20), image.Rect(50, 0, 120, 20)} {
			area := image.Rect(int(float32(region.Min.X)*scale), 0, int(float32(region.Max.X)*scale), size.Y)
			top, bottom, ok := darkInkVerticalBounds(img, area)
			if !ok {
				t.Fatal("response action has no visible content")
			}
			centers = append(centers, top+bottom)
		}
		low, high := centers[0], centers[0]
		for _, center := range centers {
			low = min(low, center)
			high = max(high, center)
		}
		if high-low > int(2*scale) {
			t.Fatalf("scale=%v visible doubled centers=%v; actions differ by more than 1dp", scale, centers)
		}
	}
}
