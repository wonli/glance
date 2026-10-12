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
		app := New(nil, "")
		for _, copied := range []bool{false, true} {
			t.Run(fmt.Sprintf("scale=%g/copied=%t", scale, copied), func(t *testing.T) {
				var ops op.Ops
				paint.Fill(&ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
				size := image.Pt(int(400*scale), int(100*scale))
				gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Constraints{Min: image.Pt(size.X, 0), Max: size}}
				app.layoutMCPConnectionValueField(gtx, "服务地址", "127.0.0.1", &app.mcpCopyURLButton, copied)
				img := renderPresentationTestOps(t, &ops, size)
				top, bottom, ok := darkInkVerticalBounds(img, image.Rect(int(330*scale), int(28*scale), int(399*scale), int(70*scale)))
				if !ok {
					t.Fatal("copy control has no visible ink")
				}
				center := float64(top+bottom) / 2
				if math.Abs(center-float64(49*scale)) > float64(2*scale) {
					t.Fatalf("copy control center %.1f, want %.1f (ink %d..%d)", center, 49*scale, top, bottom)
				}
			})
		}
	}
}
