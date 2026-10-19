package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"runtime"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func TestDropdownIndicatorVisibleAtNormalAndRetinaScale(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			metric := unit.Metric{PxPerDp: scale, PxPerSp: scale}
			size := image.Pt(metric.Dp(140), metric.Dp(120))
			var ops op.Ops
			paint.Fill(&ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			theme := material.NewTheme()
			theme.Face = preferredSystemTypeface
			var arrows []image.Rectangle
			for i, label := range []string{"Paste ▾", "Format as Table ▾", "设置打印区域 ▾"} {
				offset := image.Pt(metric.Dp(8), metric.Dp(unit.Dp(4+i*36)))
				shift := op.Offset(offset).Push(&ops)
				style := material.Caption(theme, label)
				style.Color = color.NRGBA{R: 20, G: 130, B: 50, A: 255}
				gtx := layout.Context{Ops: &ops, Metric: metric, Constraints: layout.Constraints{Max: image.Pt(metric.Dp(80), metric.Dp(34))}}
				dims := layoutDropdownLabel(gtx, style)
				width, height := metric.Dp(6), metric.Dp(4)
				arrows = append(arrows, image.Rect(dims.Size.X-width, dims.Size.Y-height-gtx.Sp(style.TextSize)*3/4, dims.Size.X, dims.Size.Y-gtx.Sp(style.TextSize)*3/4).Add(offset))
				shift.Pop()
			}
			window, err := headless.NewWindow(size.X, size.Y)
			if err != nil {
				t.Skipf("headless GPU unavailable: %v", err)
			}
			defer window.Release()
			if err := window.Frame(&ops); err != nil {
				t.Fatal(err)
			}
			shot := image.NewRGBA(image.Rectangle{Max: size})
			if err := window.Screenshot(shot); err != nil {
				t.Fatal(err)
			}
			for _, bounds := range arrows {
				ink := 0
				for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
					for x := bounds.Min.X; x < bounds.Max.X; x++ {
						c := shot.RGBAAt(x, y)
						if c.G > c.R+20 && c.G > c.B+20 {
							ink++
						}
					}
				}
				if ink < metric.Dp(3) {
					t.Fatalf("missing tinted triangle at %v: pixels=%d", bounds, ink)
				}
			}
			path := fmt.Sprintf("/tmp/glance-dropdown-%gx.png", scale)
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(file, shot); err != nil {
				file.Close()
				t.Fatal(err)
			}
			file.Close()
		})
	}
}
