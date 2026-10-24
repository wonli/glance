package desktopui

import (
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"image"
	"testing"
	"time"
)

func TestProofreadingContextMenuUsesRightClickAndKeepsLeftClickEditing(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		a, s := proofreadingModelFixture(t)
		view := &pdfViewState{}
		var router input.Router
		var ops op.Ops
		frame := func() {
			ops.Reset()
			size := image.Pt(int(400*scale), int(300*scale))
			g := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
			a.processProofreadingEditorEvents(g)
			pass := pointer.PassOp{}.Push(&ops)
			a.handleProofreadingXPagePointer(g, view, 0, s.xPages[0])
			a.registerProofreadingXPagePointer(g, 0, size)
			material.Editor(a.theme, &s.xPageEditors[0], "").Layout(g)
			pass.Pop()
			router.Frame(&ops)
		}
		frame()
		click := func(button pointer.Buttons) {
			point := f32.Pt(20*scale, 10*scale)
			router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: button, Position: point}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: point})
			frame()
			frame()
		}
		click(pointer.ButtonPrimary)
		if s.xPageMenuOpen {
			t.Fatal("left click unexpectedly opened context menu")
		}
		if !router.Source().Focused(&s.xPageEditors[0]) {
			t.Fatal("left click lost editor focus")
		}
		router.Queue(key.EditEvent{Text: "中文"})
		frame()
		if s.controller.Model.Text(1) == "abcd" {
			t.Fatal("left click editing stopped working")
		}
		click(pointer.ButtonSecondary)
		if !s.xPageMenuOpen || s.xPageMenuPage != 0 {
			t.Fatal("right click did not open page context menu")
		}
	}
}
