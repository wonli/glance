package desktopui

import (
	"image"
	"image/color"
	"runtime"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestWordRulerRightHandleStaysVisibleWithAgentSidebar(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a := New(nil, "")
		state, closeDoc := newLongWordFlowState(t, 2)
		defer closeDoc()
		state.documentID = "ruler-sidebar-test"
		state.wordCanvasWidthDP = 900
		a.documentSessionState = state
		a.agentUI.sidebarVisible = true
		a.agentUI.conversationDocumentID = state.documentID
		var ops op.Ops
		var router input.Router
		size := image.Pt(int(1100*scale), int(700*scale))
		frame := func() {
			ops.Reset()
			gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
			a.layoutAgentSidebarWorkspace(gtx, func(gtx layout.Context) layout.Dimensions { return a.layoutWordHorizontalRuler(gtx, state) })
			router.Frame(&ops)
		}
		frame()
		img := renderPresentationTestOps(t, &ops, size)
		pixels := 0
		for y := 0; y < int(33*scale); y++ {
			for x := int(1080 * scale); x < int(1100*scale); x++ {
				got := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
				if got == rulerResizeHandleColor {
					pixels++
				}
			}
		}
		if pixels < int(90*scale*scale) {
			t.Fatalf("right handle clipped at scale %v: %d visible pixels", scale, pixels)
		}
		start := f32.Pt(1095*scale, 16*scale)
		end := start.Add(f32.Pt(-40*scale, 0))
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 9, Buttons: pointer.ButtonPrimary, Position: start})
		frame()
		router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 9, Buttons: pointer.ButtonPrimary, Position: end})
		frame()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 9, Position: end})
		frame()
		if state.wordCanvasWidthDP != 770 {
			t.Fatalf("right grip drag width at scale %v = %d, want 770dp", scale, state.wordCanvasWidthDP)
		}
	}
}
