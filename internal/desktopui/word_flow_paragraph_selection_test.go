package desktopui

import (
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"image"
	"testing"
	"time"
)

func TestWordFlowDoubleClickSelectsParagraphAcrossFormattedRuns(t *testing.T) {
	app, state, router, _ := newWordSelectionPageTest(t, `<w:p><w:r><w:t>alpha beta</w:t></w:r><w:r><w:rPr><w:b/></w:rPr><w:t> 中文尾部</w:t></w:r></w:p><w:p><w:r><w:t>other paragraph</w:t></w:r></w:p>`)
	state.wordViewMode = wordViewWebLayout
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(1100, 700)}}
		app.layoutWord(gtx)
		router.Frame(&ops)
	}
	frame()
	runID := state.wordBlocks[0].Paragraph.Inlines[0].Text.ID
	var bounds image.Rectangle
	var find func([]input.SemanticNode)
	find = func(nodes []input.SemanticNode) {
		for _, n := range nodes {
			if n.Desc.Description == "正文文字 "+runID {
				bounds = n.Desc.Bounds
			}
			find(n.Children)
		}
	}
	find(router.AppendSemantics(nil))
	if bounds.Empty() {
		t.Fatal("no run hit bounds")
	}
	point := f32.Pt(float32(bounds.Min.X+10), float32(bounds.Min.Y+bounds.Dy()/2))
	for i := 0; i < 2; i++ {
		when := time.Second + time.Duration(i)*100*time.Millisecond
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: point, Time: when})
		frame()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point, Time: when + 20*time.Millisecond})
		frame()
	}
	if text, ok := wordPageSelectedText(state); !ok || text != "alpha beta 中文尾部" {
		t.Fatalf("double click selected %q,%v; want entire formatted paragraph", text, ok)
	}
}
