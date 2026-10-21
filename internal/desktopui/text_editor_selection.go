package desktopui

import (
	"image"

	"gioui.org/gesture"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
)

// textEditorParagraphSelection observes native editor clicks without taking
// their pointer grab. Keep it in stable state; Word shares a handler across
// the formatted runs in one paragraph so double-click detection spans them.
type textEditorParagraphSelection struct {
	click             gesture.Click
	paragraphSelected bool
	clicked           bool
	clickedOnText     bool
}

func (s *textEditorParagraphSelection) layout(gtx layout.Context, editor *widget.Editor, content layout.Widget) layout.Dimensions {
	return s.layoutWithDoubleClick(gtx, editor, content, nil)
}

func (s *textEditorParagraphSelection) layoutWithDoubleClick(gtx layout.Context, editor *widget.Editor, content layout.Widget, onDoubleClick func(int) bool) layout.Dimensions {
	return s.layoutWithDoubleClickAtTextSize(gtx, editor, content, onDoubleClick, 0)
}

func (s *textEditorParagraphSelection) layoutWithDoubleClickAtTextSize(gtx layout.Context, editor *widget.Editor, content layout.Widget, onDoubleClick func(int) bool, textSize unit.Sp) layout.Dimensions {
	return s.layoutWithDoubleClickAtTextSizeAndPointer(gtx, editor, content, onDoubleClick, textSize, nil, nil)
}

func (s *textEditorParagraphSelection) layoutWithDoubleClickAtTextSizeAndPointer(gtx layout.Context, editor *widget.Editor, content layout.Widget, onDoubleClick func(int) bool, textSize unit.Sp, pointerTag event.Tag, onPointerEvent func(layout.Context, pointer.Event)) layout.Dimensions {
	s.paragraphSelected = false
	s.clicked = false
	s.clickedOnText = false
	pass := pointer.PassOp{}.Push(gtx.Ops)
	dims := content(gtx)
	area := clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops)
	s.click.Add(gtx.Ops)
	if pointerTag != nil && onPointerEvent != nil {
		event.Op(gtx.Ops, pointerTag)
		kinds := pointer.Press | pointer.Enter | pointer.Leave | pointer.Move
		for value, ok := gtx.Event(pointer.Filter{Target: pointerTag, Kinds: kinds}); ok; value, ok = gtx.Event(pointer.Filter{Target: pointerTag, Kinds: kinds}) {
			if press, ok := value.(pointer.Event); ok {
				onPointerEvent(gtx, press)
			}
		}
	}
	area.Pop()
	pass.Pop()
	for ev, ok := s.click.Update(gtx.Source); ok; ev, ok = s.click.Update(gtx.Source) {
		if ev.Kind == gesture.KindPress || ev.Kind == gesture.KindClick {
			s.clicked = true
			s.clickedOnText = clickNearEditorText(gtx, editor, ev.Position, textSize)
		}
		if ev.Source != pointer.Mouse || ev.Kind != gesture.KindPress || ev.NumClicks != 2 {
			continue
		}
		// The native editor has already resolved the click to a rune position.
		caret, _ := editor.Selection()
		text := []rune(editor.Text())
		caret = max(0, min(caret, len(text)))
		if onDoubleClick != nil && onDoubleClick(caret) {
			s.paragraphSelected = true
			gtx.Execute(op.InvalidateCmd{})
			continue
		}
		start, end := caret, caret
		for start > 0 && text[start-1] != '\n' {
			start--
		}
		for end < len(text) && text[end] != '\n' {
			end++
		}
		editor.SetCaret(end, start)
		s.paragraphSelected = true
		gtx.Execute(op.InvalidateCmd{})
	}
	return dims
}

func clickNearEditorText(gtx layout.Context, editor *widget.Editor, position image.Point, textSize unit.Sp) bool {
	if editor == nil || editor.Len() == 0 {
		return false
	}
	caret := editor.CaretCoords()
	dx := float32(position.X) - caret.X
	if dx < 0 {
		dx = -dx
	}
	dy := float32(position.Y) - caret.Y
	if dy < 0 {
		dy = -dy
	}
	fontPixels := gtx.Sp(textSize)
	toleranceX := max(gtx.Dp(unit.Dp(8)), int(float32(fontPixels)*0.55))
	toleranceY := max(gtx.Dp(unit.Dp(8)), fontPixels)
	return dx <= float32(toleranceX) && dy <= float32(toleranceY)
}
