package desktopui

import (
	"image"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"glance/internal/document"
)

func TestPointerCoordinatesAreLocalToGridAfterTopContent(t *testing.T) {
	var router input.Router
	var ops op.Ops
	tag := new(struct{})
	var received pointer.Event
	draw := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      router.Source(),
			Constraints: layout.Constraints{Min: image.Pt(200, 300), Max: image.Pt(200, 300)},
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		}
		layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: image.Pt(200, 80)} }),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				area := clip.Rect(image.Rectangle{Max: gtx.Constraints.Max}).Push(gtx.Ops)
				event.Op(gtx.Ops, tag)
				area.Pop()
				for e, ok := gtx.Event(pointer.Filter{Target: tag, Kinds: pointer.Move}); ok; e, ok = gtx.Event(pointer.Filter{Target: tag, Kinds: pointer.Move}) {
					received = e.(pointer.Event)
				}
				return layout.Dimensions{Size: gtx.Constraints.Max}
			}),
		)
	}
	draw()
	router.Frame(&ops)
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(58, 133), PointerID: 1})
	draw()
	if received.Kind != pointer.Move {
		t.Fatal("expected pointer move on grid tag")
	}
	got := received.Position
	if got != f32.Pt(58, 53) {
		t.Fatalf("grid-local pointer=%v; want (58,53) after 80px of top content", got)
	}
	rows, _ := NewAxisGeometry(10, 20)
	columns, _ := NewAxisGeometry(10, 50)
	row, column, ok := cellAtPoint(rows, columns, 0, 0, image.Pt(int(got.X), int(got.Y)), 48, 28)
	if !ok || row != 2 || column != 1 {
		t.Fatalf("selected cell=(%d,%d,%v); want (2,1,true)", row, column, ok)
	}
}

func TestCellAtPointUsesVariableDimensions(t *testing.T) {
	r, _ := NewAxisGeometry(20, 20)
	c, _ := NewAxisGeometry(10, 50)
	_ = r.SetSize(0, 30)
	_ = c.SetSize(0, 90)
	row, col, ok := cellAtPoint(r, c, 40, 25, image.Pt(160, 70), 20, 20)
	if !ok || row != 4 || col != 3 {
		t.Fatalf("cell=(%d,%d,%v)", row, col, ok)
	}
}
func TestMergeAnchorAtInteriorAndEdge(t *testing.T) {
	ms := []document.MergeRegion{{Range: document.CellRange{StartRow: 2, StartColumn: 3, EndRow: 4, EndColumn: 5}}}
	for _, p := range [][2]int{{2, 3}, {3, 4}, {4, 5}} {
		r, c, ok := mergeAnchorAt(ms, p[0], p[1])
		if !ok || r != 2 || c != 3 {
			t.Fatalf("%v => %d,%d,%v", p, r, c, ok)
		}
	}
	if _, _, ok := mergeAnchorAt(ms, 1, 3); ok {
		t.Fatal("outside merge matched")
	}
}
func TestMergedCellRectSpansRowsAndColumns(t *testing.T) {
	r, _ := NewAxisGeometry(10, 20)
	c, _ := NewAxisGeometry(10, 50)
	_ = r.SetSize(1, 30)
	_ = c.SetSize(2, 70)
	got := cellRect(r, c, document.CellRange{StartRow: 2, StartColumn: 3, EndRow: 4, EndColumn: 5}, 10, 5)
	if got != image.Rect(90, 15, 260, 85) {
		t.Fatalf("rect=%v", got)
	}
}
func TestVisibleWindowAddsOverscanAndClamps(t *testing.T) {
	r, _ := NewAxisGeometry(10, 10)
	c, _ := NewAxisGeometry(8, 10)
	got := visibleCellWindow(r, c, 20, 30, 20, 20)
	if got != (document.CellRange{StartRow: 2, StartColumn: 1, EndRow: 7, EndColumn: 6}) {
		t.Fatalf("range=%+v", got)
	}
}
