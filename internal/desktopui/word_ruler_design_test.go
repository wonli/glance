package desktopui

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func TestWordRulerDesignHasFourIndentTargets(t *testing.T) {
	state, closeDoc := newLongWordFlowState(t, 2)
	defer closeDoc()
	state.selectedWordParagraphID = state.wordBlocks[0].Paragraph.ID
	app := &App{documentSessionState: state, theme: material.NewTheme()}
	for _, density := range []float32{1, 2} {
		var ops op.Ops
		var router input.Router
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: density, PxPerSp: density}, Constraints: layout.Exact(image.Pt(int(1100*density), int(400*density)))}
		dims := app.layoutWordHorizontalRuler(gtx, state)
		router.Frame(&ops)
		if dims.Size.Y != int(28*density) {
			t.Fatalf("ruler height %d, want %v", dims.Size.Y, 28*density)
		}
		found := map[string]bool{}
		var visit func([]input.SemanticNode)
		visit = func(nodes []input.SemanticNode) {
			for _, n := range nodes {
				if !n.Desc.Bounds.Empty() {
					found[n.Desc.Description] = true
				}
				visit(n.Children)
			}
		}
		visit(router.AppendSemantics(nil))
		for _, name := range []string{"拖动调整段落首行缩进", "拖动调整段落悬挂缩进", "拖动调整段落左缩进", "拖动调整段落右缩进"} {
			if !found[name] {
				t.Errorf("missing %s", name)
			}
		}
	}
}
