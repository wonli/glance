package desktopui

import (
	"image"
	"image/color"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/gpu/headless"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestWordPointerDragHighlightMatchesLiveRange(t *testing.T) {
	for _, tc := range []struct {
		name    string
		anchor  int
		offsets []int
		native  bool
	}{
		{"forward", 3, []int{4, 6, 8, 5}, false}, {"backward", 8, []int{7, 5, 2, 6}, false},
		{"windows-forward", 3, []int{4, 6, 8, 5}, true}, {"windows-backward", 8, []int{7, 5, 2, 6}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, state, router, _ := newWordSelectionPageTest(t, `<w:p><w:r><w:t>中文abcdef尾部</w:t></w:r></w:p>`)
			window, err := headless.NewWindow(600, 500)
			if err != nil {
				t.Skipf("headless GPU unavailable: %v", err)
			}
			defer window.Release()
			var ops op.Ops
			frame := func() *image.RGBA {
				t.Helper()
				ops.Reset()
				gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(600, 500)}}
				app.layoutWordFlow(gtx)
				router.Frame(&ops)
				if err := window.Frame(&ops); err != nil {
					t.Fatal(err)
				}
				shot := image.NewRGBA(image.Rect(0, 0, 600, 500))
				if err := window.Screenshot(shot); err != nil {
					t.Fatal(err)
				}
				return shot
			}
			frame()
			transform := state.wordPageVisibleInteractions[0].Transform
			viewport, ok := findNativePinchSurface(router.AppendSemantics(nil), image.Pt(300, 300))
			if !ok {
				t.Fatal("missing viewport")
			}
			transform.Origin = transform.Origin.Add(viewport.Min)
			point := func(offset int) f32.Point { return transform.PageToScreen(f32.Pt(float32(10+offset*10), 30)) }
			router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: point(tc.anchor), Time: time.Second})
			frame()
			for _, focus := range tc.offsets {
				movePoint := point(focus)
				if tc.native {
					app.wordPointer.sample = wordPointerSample{valid: true, client: movePoint, origin: viewport.Min}
					// Replay the up/left displacement observed in the Windows trace.
					movePoint = movePoint.Sub(f32.Pt(233, 256))
				}
				router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: movePoint, Time: 2 * time.Second})
				shot := frame()
				for i := 0; i < 10; i++ {
					p := transform.PageToScreen(f32.Pt(float32(15+i*10), 25))
					ink := color.NRGBAModel.Convert(shot.At(int(p.X), int(p.Y))).(color.NRGBA)
					selected := ink.R < 230 && ink.B > ink.R
					want := i >= min(tc.anchor, focus) && i < max(tc.anchor, focus)
					if selected != want {
						t.Fatalf("during drag %d:%d glyph %d color=%v selected=%v want=%v", tc.anchor, focus, i, ink, selected, want)
					}
				}
			}
		})
	}
}
