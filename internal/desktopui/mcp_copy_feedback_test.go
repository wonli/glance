package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"runtime"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

func TestSettingsMCPCopyFeedbackVerticallyCentered(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		for _, copied := range []bool{false, true} {
			t.Run(fmt.Sprintf("scale=%g/copied=%t", scale, copied), func(t *testing.T) {
				desktop := newDesktopRuntime()
				t.Cleanup(desktop.shutdown)
				app := newAppWithRuntime(nil, desktop)
				var ops op.Ops
				paint.Fill(&ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
				size := image.Pt(int(120*scale), int(44*scale))
				gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
				layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return app.layoutMCPConnectionCopyButton(gtx, &app.mcpCopyConfigButton, copied)
				})
				img := renderPresentationTestOps(t, &ops, size)
				top, bottom, ok := darkInkVerticalBounds(img, image.Rect(0, 0, size.X, size.Y))
				if !ok {
					t.Fatal("copy control has no visible ink")
				}
				center := float64(top+bottom) / 2
				if math.Abs(center-float64(22*scale)) > float64(2*scale) {
					t.Fatalf("copy control center %.1f, want %.1f (ink %d..%d)", center, 22*scale, top, bottom)
				}
			})
		}
	}
}
