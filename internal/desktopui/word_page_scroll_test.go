package desktopui

import (
	"image"

	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"gioui.org/widget/material"

	"glance/internal/wordlayout"
)

func TestWordPageScrollbarCanBeDraggedToLaterPages(t *testing.T) {
	const pageCount = 20
	pages := make([]wordlayout.PageScene, pageCount)
	for index := range pages {
		pages[index] = wordlayout.PageScene{Index: index, Clip: wordlayout.Rect{WidthPT: 120, HeightPT: 100}}
	}
	state := newDocumentSessionState("page-scrollbar.docx")
	state.wordLayoutZoom = 0.375
	state.wordLayoutResult = &wordlayout.Result{CanPreview: true, Pages: pages}
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{
			Source: router.Source(), Ops: &ops, Now: time.Now(),
			Metric: unit.Metric{PxPerDp: 2, PxPerSp: 2}, Constraints: layout.Constraints{Max: image.Pt(600, 250)},
		}
		app.layoutWordPages(gtx, state, wordPagePrint)
		router.Frame(&ops)
	}
	frame()
	var scrollbarBounds image.Rectangle
	var visit func([]input.SemanticNode)
	visit = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if node.Desc.Description == "Word 页面滚动条" {
				scrollbarBounds = node.Desc.Bounds
			}
			visit(node.Children)
		}
	}
	visit(router.AppendSemantics(nil))
	if scrollbarBounds.Empty() {
		t.Fatal("paginated Word view has no visible, accessible scrollbar")
	}
	start := f32.Pt(float32((scrollbarBounds.Min.X+scrollbarBounds.Max.X)/2), float32(scrollbarBounds.Min.Y+12))
	end := f32.Pt(start.X, float32(scrollbarBounds.Max.Y-12))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 106, Buttons: pointer.ButtonPrimary, Position: start})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 106, Buttons: pointer.ButtonPrimary, Position: end})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 106, Buttons: pointer.ButtonPrimary, Position: f32.Pt(end.X, end.Y-8)})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 106, Position: f32.Pt(end.X, end.Y-8)})
	frame()
	if state.wordPageListState.Position.First == 0 && state.wordPageListState.Position.Offset == 0 {
		t.Fatal("dragging the page scrollbar did not advance the visible page window")
	}
}
