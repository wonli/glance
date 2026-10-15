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
	"gioui.org/widget"
	"gioui.org/widget/material"
	"glance/internal/document"
)

func TestClickInsideEditingCellPlacesCaretWithoutChangingCell(t *testing.T) {
	var router input.Router
	var ops op.Ops
	var parentTag struct{}
	var gridPresses int
	editor := &widget.Editor{}
	editor.SetText("click to place caret")
	editor.SetCaret(0, 0)
	theme := material.NewTheme()
	cellStyle := document.CellStyle{}
	style := paintStyle(cellStyle)
	draw := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      router.Source(),
			Constraints: layout.Constraints{Min: image.Pt(220, 40), Max: image.Pt(220, 40)},
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		}
		area := clip.Rect(image.Rectangle{Max: gtx.Constraints.Max}).Push(gtx.Ops)
		defer area.Pop()
		event.Op(gtx.Ops, &parentTag)
		pass := pointer.PassOp{}.Push(gtx.Ops)
		for _, ok := gtx.Event(pointer.Filter{Target: &parentTag, Kinds: pointer.Press}); ok; _, ok = gtx.Event(pointer.Filter{Target: &parentTag, Kinds: pointer.Press}) {
			gridPresses++
		}
		paintCellEditor(gtx, theme, editor, image.Rect(0, 0, 220, 40), image.Rect(0, 0, 220, 40), cellStyle, style, LightColorTokens.Grid.SelectionBorder)
		pass.Pop()
	}
	draw()
	router.Frame(&ops)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(105, 20), Buttons: pointer.ButtonPrimary})
	draw()
	start, end := editor.Selection()
	if start != end || start < 5 {
		t.Fatalf("caret after in-cell click=(%d,%d); want a collapsed caret at the clicked text position", start, end)
	}
	if gridPresses != 1 {
		t.Fatalf("parent grid presses=%d; want pass-through event to remain observable", gridPresses)
	}
}
