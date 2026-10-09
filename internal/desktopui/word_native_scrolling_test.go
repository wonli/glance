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
)

func TestWordFlowCanReverseScrollImmediatelyFromBottom(t *testing.T) {
	state, closeDocument := newLongWordFlowState(t, 160)
	defer closeDocument()
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: metric, Constraints: layout.Constraints{Max: image.Pt(1100, 700)}}
		app.layoutWordFlow(gtx)
		router.Frame(&ops)
	}
	frame()

	router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(540, 320), Scroll: f32.Pt(0, 100000)})
	frame()
	if state.wordListState.Position.BeforeEnd {
		t.Fatalf("large downward scroll did not reach the end: %#v", state.wordListState.Position)
	}
	bottom := state.wordListState.Position

	router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(540, 320), Scroll: f32.Pt(0, -80)})
	frame()
	upward := state.wordListState.Position
	if upward.First == bottom.First && upward.Offset == bottom.Offset {
		t.Fatalf("one reverse wheel event did not move up from bottom: bottom=%#v after=%#v", bottom, upward)
	}
}

func TestWordFlowScrollsDownImmediatelyFromTop(t *testing.T) {
	state, closeDocument := newLongWordFlowState(t, 160)
	defer closeDocument()
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: metric, Constraints: layout.Constraints{Max: image.Pt(1100, 700)}}
		app.layoutWordFlow(gtx)
		router.Frame(&ops)
	}
	frame()
	state.wordListState.ScrollTo(0)
	frame()
	top := state.wordListState.Position

	router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(540, 320), Scroll: f32.Pt(0, 80)})
	frame()
	down := state.wordListState.Position
	if down.First == top.First && down.Offset == top.Offset {
		t.Fatalf("one downward wheel event did not move down from top: top=%#v after=%#v", top, down)
	}
}

func TestWordScrollbarTracksAtWorkspaceRightEdge(t *testing.T) {
	state, closeDocument := newLongWordFlowState(t, 160)
	defer closeDocument()
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	router := new(input.Router)
	var ops op.Ops
	gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(1200, 700)}}
	app.layoutWordFlow(gtx)
	router.Frame(&ops)
	var bounds, pageBounds image.Rectangle
	var visit func([]input.SemanticNode)
	visit = func(nodes []input.SemanticNode) {
		for _, node := range nodes {
			if node.Desc.Description == "Word 文档滚动条" {
				bounds = node.Desc.Bounds
			}
			if node.Desc.Description == "Word 文档画布" {
				pageBounds = node.Desc.Bounds
			}
			visit(node.Children)
		}
	}
	visit(router.AppendSemantics(nil))
	if bounds.Empty() {
		t.Fatal("Word document scrollbar has no accessible bounds")
	}
	if bounds.Max.X < 1180 || bounds.Max.X > 1200 {
		t.Fatalf("Word scrollbar bounds = %v; want the right edge of the 1200px workspace", bounds)
	}
	if pageBounds.Empty() {
		t.Fatal("Word page canvas has no accessible bounds")
	}
	if center := (pageBounds.Min.X + pageBounds.Max.X) / 2; center < 590 || center > 610 {
		t.Fatalf("Word page canvas bounds = %v; want it centered in the 1200px workspace", pageBounds)
	}
}
