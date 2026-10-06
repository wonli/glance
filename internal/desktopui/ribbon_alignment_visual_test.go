package desktopui

import (
	"image"
	"image/color"
	"math"
	"runtime"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

func TestRibbonIconAndCaptionShareCenterAxis(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	theme := New(nil, "").theme
	for _, scale := range []float32{1, 1.5, 2} {
		for _, label := range []string{"保存", "格式化", "全部展开"} {
			metric := unit.Metric{PxPerDp: scale, PxPerSp: scale}
			size := image.Pt(metric.Dp(48), metric.Dp(48))
			var ops op.Ops
			gtx := layout.Context{Ops: &ops, Metric: metric, Constraints: layout.Exact(size)}
			paint.FillShape(&ops, contentSurface, clip.Rect{Max: size}.Op())
			icon := image.NewNRGBA(image.Rect(0, 0, 18, 18))
			for y := 0; y < 18; y++ {
				for x := 0; x < 18; x++ {
					icon.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
				}
			}
			toolbarIconButton(gtx, theme, nil, icon, toolbarIconSize, label, label, true, false, contentSurface)
			window, err := headless.NewWindow(size.X, size.Y)
			if err != nil {
				t.Skipf("headless unavailable: %v", err)
			}
			if err = window.Frame(&ops); err != nil {
				window.Release()
				t.Fatal(err)
			}
			shot := image.NewRGBA(image.Rectangle{Max: size})
			err = window.Screenshot(shot)
			window.Release()
			if err != nil {
				t.Fatal(err)
			}
			for _, glyph := range []string{"icon", "caption"} {
				min, max := size.X, -1
				for y := 0; y < size.Y; y++ {
					for x := 0; x < size.X; x++ {
						c := color.NRGBAModel.Convert(shot.At(x, y)).(color.NRGBA)
						matches := glyph == "icon" && c.R > 200 && c.G < 80 && c.B < 80 || glyph == "caption" && c.R < 205 && c.G < 205 && c.B < 205
						if matches {
							if x < min {
								min = x
							}
							if x > max {
								max = x
							}
						}
					}
				}
				if max < min || math.Abs(float64(min+max)/2-float64(size.X-1)/2) > 2*float64(scale) {
					t.Errorf("%s %q scale %v ink x=%d..%d; want axis %v", glyph, label, scale, min, max, float64(size.X-1)/2)
				}
			}
		}
	}
}
