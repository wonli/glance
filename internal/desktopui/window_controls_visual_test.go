package desktopui

import (
	"image"
	"image/color"
	"runtime"
	"testing"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/gpu/headless"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

func TestStandardWindowControlsHoverAndPressPixels(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a := New(nil, "")
		size := image.Pt(int(138*scale), int(42*scale))
		var ops op.Ops
		var router input.Router
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
		win, err := headless.NewWindow(size.X, size.Y)
		if err != nil {
			t.Skipf("headless GPU unavailable: %v", err)
		}
		clicked := false
		draw := func() *image.RGBA {
			t.Helper()
			ops.Reset()
			if a.windowCloseButton.Clicked(gtx) {
				clicked = true
			}
			paint.Fill(&ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			dims := a.layoutStandardWindowControls(gtx)
			if dims.Size != size {
				t.Fatalf("%gx: dimensions %v want %v", scale, dims.Size, size)
			}
			router.Frame(&ops)
			if err := win.Frame(&ops); err != nil {
				t.Fatal(err)
			}
			shot := image.NewRGBA(image.Rectangle{Max: size})
			if err := win.Screenshot(shot); err != nil {
				t.Fatal(err)
			}
			return shot
		}
		sample := func(shot *image.RGBA, x, y float32) color.RGBA { return shot.RGBAAt(int(x*scale), int(y*scale)) }
		normal := draw()
		for button := 0; button < 3; button++ {
			ink := 0
			for y := int(14 * scale); y < int(28*scale); y++ {
				for x := int(float32(46*button+15) * scale); x < int(float32(46*button+31)*scale); x++ {
					p := normal.RGBAAt(x, y)
					if p.R < 200 && p.G < 200 && p.B < 200 {
						ink++
					}
				}
			}
			if ink == 0 {
				t.Fatalf("%gx: button %d has no visible vector glyph", scale, button)
			}
		}
		a.windowMode = app.Maximized
		restored := draw()
		different := false
		for y := int(14 * scale); y < int(28*scale); y++ {
			for x := int(61 * scale); x < int(77*scale); x++ {
				if normal.RGBAAt(x, y) != restored.RGBAAt(x, y) {
					different = true
				}
			}
		}
		if !different {
			t.Fatalf("%gx: restore glyph did not change", scale)
		}
		a.windowMode = app.Windowed
		draw()
		router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(10*scale, 10*scale)})
		p := sample(draw(), 5, 5)
		if p.R >= 250 || p.R != p.G || p.G != p.B {
			t.Fatalf("%gx: minimize hover = %v, want grey", scale, p)
		}
		router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(110*scale, 10*scale)})
		p = sample(draw(), 100, 5)
		if p.R < 180 || int(p.R) < int(p.G)+80 {
			t.Fatalf("%gx: close hover = %v, want red", scale, p)
		}
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: f32.Pt(110*scale, 10*scale)})
		pressed := sample(draw(), 100, 5)
		if pressed.R >= p.R {
			t.Fatalf("%gx: close press = %v, hover = %v", scale, pressed, p)
		}
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(110*scale, 10*scale)})
		draw()
		if !clicked {
			t.Fatal("close click did not reach its button")
		}
		router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(1, 100*scale)})
		p = sample(draw(), 100, 5)
		if p.R != 255 || p.G != 255 || p.B != 255 {
			t.Fatalf("%gx: hover remains after exit: %v", scale, p)
		}
		win.Release()
	}
}

func TestWindowsWorkspaceCloseControlReachesTopEdge(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a := New(nil, "")
		a.clientTitlebar = true
		a.desktopRuntime.simulateWindowsChrome = true
		size := image.Pt(int(800*scale), int(42*scale))
		var ops op.Ops
		var router input.Router
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
		win, err := headless.NewWindow(size.X, size.Y)
		if err != nil {
			t.Skipf("headless GPU unavailable: %v", err)
		}
		draw := func() *image.RGBA {
			t.Helper()
			ops.Reset()
			paint.Fill(&ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			a.layoutWorkspaceTabs(gtx)
			router.Frame(&ops)
			if err := win.Frame(&ops); err != nil {
				t.Fatal(err)
			}
			shot := image.NewRGBA(image.Rectangle{Max: size})
			if err := win.Screenshot(shot); err != nil {
				t.Fatal(err)
			}
			return shot
		}
		draw()
		router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(790*scale, 1*scale)})
		shot := draw()
		p := shot.RGBAAt(int(790*scale), int(1*scale))
		if p.R < 180 || int(p.R) < int(p.G)+80 {
			t.Fatalf("%gx: top-right hover = %v, want red close control", scale, p)
		}
		win.Release()
	}
}

func TestStandardWindowControlHoverRetainsTitlebarColor(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a := New(nil, "")
		size := image.Pt(int(138*scale), int(42*scale))
		var ops op.Ops
		var router input.Router
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
		win, err := headless.NewWindow(size.X, size.Y)
		if err != nil {
			t.Skipf("headless GPU unavailable: %v", err)
		}
		draw := func() *image.RGBA {
			t.Helper()
			ops.Reset()
			paint.Fill(&ops, workspaceTabStripSurface)
			a.layoutStandardWindowControls(gtx)
			router.Frame(&ops)
			if err := win.Frame(&ops); err != nil {
				t.Fatal(err)
			}
			shot := image.NewRGBA(image.Rectangle{Max: size})
			if err := win.Screenshot(shot); err != nil {
				t.Fatal(err)
			}
			return shot
		}
		draw()
		router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(10*scale, 10*scale)})
		p := draw().RGBAAt(int(5*scale), int(5*scale))
		if !(p.R < p.G && p.G < p.B && p.R < workspaceTabStripSurface.R) {
			t.Fatalf("%gx hover = %v: want darkened titlebar color", scale, p)
		}
		win.Release()
	}
}
