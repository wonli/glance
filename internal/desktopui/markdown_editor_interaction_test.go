package desktopui

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestMarkdownEmptyEditorStartsAtTopAndFillsWritingArea(t *testing.T) {
	for _, scale := range []float32{1, 1.5, 2} {
		a := markdownTestApp(t)
		var router input.Router
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(int(1000*scale), int(700*scale)))}
		a.layoutMarkdownEditor(gtx)
		router.Frame(&ops)
		var bounds image.Rectangle
		var visit func([]input.SemanticNode)
		visit = func(nodes []input.SemanticNode) {
			for _, n := range nodes {
				if n.Desc.Class == semantic.Editor {
					bounds = n.Desc.Bounds
				}
				visit(n.Children)
			}
		}
		visit(router.AppendSemantics(nil))
		if bounds.Min.Y != gtx.Dp(48) || bounds.Dy() != gtx.Dp(620) {
			t.Fatalf("empty editor vertically centered or has tiny hit area: %v", bounds)
		}
	}
}

func TestMarkdownPageBlankClicksFocusEditor(t *testing.T) {
	for _, point := range []image.Point{image.Pt(700, 500), image.Pt(260, 70), image.Pt(1190, 660)} {
		t.Run(point.String(), func(t *testing.T) {
			a := markdownTestApp(t)
			defer close(a.shutdown)
			v := a.markdownView
			v.focus = false
			var router input.Router
			var ops op.Ops
			frame := func() {
				ops.Reset()
				gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1200, 700))}
				a.layoutMarkdownDocument(gtx)
				router.Frame(&ops)
			}
			frame()
			position := f32.Pt(float32(point.X), float32(point.Y))
			router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: position})
			frame()
			router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: position})
			frame()
			frame()
			if !router.Source().Focused(&v.editor) {
				t.Fatalf("blank page click %v did not focus editor", point)
			}
			router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 0}, Text: "中文"})
			frame()
			if a.markdownDoc.Text() != "中文" {
				t.Fatalf("typing after blank click lost: %q", a.markdownDoc.Text())
			}
		})
	}
}

func TestMarkdownNewEditorReceivesKeyboardFocus(t *testing.T) {
	a := markdownTestApp(t)
	defer close(a.shutdown)
	var router input.Router
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1200, 700))}
	a.layoutMarkdownDocument(gtx)
	router.Frame(&ops)
	if !router.Source().Focused(&a.markdownView.editor) {
		t.Fatal("new document requires clicking hint before typing")
	}
}

func TestMarkdownPageFocusKeepsTextDragSelection(t *testing.T) {
	a := markdownTestApp(t)
	defer close(a.shutdown)
	v := a.markdownView
	v.editor.SetText("中文正文测试")
	a.flushMarkdownDraft()
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1200, 700))}
		a.layoutMarkdownDocument(gtx)
		router.Frame(&ops)
	}
	frame()
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: f32.Pt(342, 112)})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: f32.Pt(399, 112)})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(399, 112)})
	frame()
	frame()
	start, end := v.editor.Selection()
	if start == end {
		t.Fatal("page focus layer intercepted text drag selection")
	}
	if v.text() != "中文正文测试" {
		t.Fatal("selection altered source")
	}
}
