package desktopui

import (
	"image"
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

func TestAgentSidebarRightEdgeDragsWithoutCoordinateJump(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a := New(nil, "")
		a.documentSessionState = &documentSessionState{sharedDocumentSession: &sharedDocumentSession{}}
		a.documentID = "sidebar-resize-test"
		a.agentUI.conversationDocumentID = a.documentID
		a.agentUI.sidebarVisible = true
		var router input.Router
		var ops op.Ops
		contentWidth := 0
		frame := func() {
			ops.Reset()
			gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(int(1000*scale), int(700*scale)))}
			a.layoutAgentSidebarWorkspace(gtx, func(gtx layout.Context) layout.Dimensions {
				contentWidth = gtx.Constraints.Max.X
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
			router.Frame(&ops)
		}
		frame()
		img := renderPresentationTestOps(t, &ops, image.Pt(int(1000*scale), int(700*scale)))
		r, g, b, _ := img.At(int(249*scale), int(350*scale)).RGBA()
		if r != 201*257 || g != 208*257 || b != 217*257 {
			t.Fatal("sidebar divider differs from design #C9D0D9")
		}
		var handle image.Rectangle
		var visit func([]input.SemanticNode)
		visit = func(nodes []input.SemanticNode) {
			for _, n := range nodes {
				if n.Desc.Description == "拖动调整 AI 侧栏宽度" {
					handle = n.Desc.Bounds
				}
				visit(n.Children)
			}
		}
		visit(router.AppendSemantics(nil))
		if handle.Empty() {
			t.Error("AI sidebar has no resize grip")
		}
		start := f32.Pt(249*scale, 350*scale)
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 8, Buttons: pointer.ButtonPrimary, Position: start})
		frame()
		for _, delta := range []float32{30, 60} {
			router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 8, Buttons: pointer.ButtonPrimary, Position: start.Add(f32.Pt(delta*scale, 0))})
			frame()
			if contentWidth != int((750-delta)*scale) {
				t.Fatalf("resize delta %v scale %v: content width %d", delta, scale, contentWidth)
			}
		}
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 8, Position: start.Add(f32.Pt(60*scale, 0))})
		frame()
		frame()
		if contentWidth != int(690*scale) {
			t.Fatal("released sidebar width did not persist")
		}
		a.agentUI.sidebarVisible = false
		frame()
		a.agentUI.sidebarVisible = true
		frame()
		if contentWidth != int(690*scale) {
			t.Fatal("reopening reset sidebar width")
		}
		start = f32.Pt(309*scale, 350*scale)
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 8, Buttons: pointer.ButtonPrimary, Position: start})
		frame()
		router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 8, Buttons: pointer.ButtonPrimary, Position: start.Add(f32.Pt(900*scale, 0))})
		frame()
		if contentWidth != int(400*scale) {
			t.Fatal("sidebar resize exceeded maximum and squeezed document")
		}
		router.Queue(pointer.Event{Kind: pointer.Cancel, Source: pointer.Mouse, PointerID: 8})
		frame()
		if contentWidth != int(690*scale) {
			t.Fatal("cancelled resize did not restore committed width")
		}
	}
}
