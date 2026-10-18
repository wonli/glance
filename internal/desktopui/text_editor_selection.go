package desktopui

import (
	"image"

	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget"
)

// textEditorParagraphSelection observes native editor clicks without taking
// their pointer grab. Keep it in stable state; Word shares a handler across
// the formatted runs in one paragraph so double-click detection spans them.
type textEditorParagraphSelection struct {
	click             gesture.Click
	paragraphSelected bool
}

func (s *textEditorParagraphSelection) layout(gtx layout.Context, editor *widget.Editor, content layout.Widget) layout.Dimensions {
	return s.layoutWithDoubleClick(gtx, editor, content, nil)
}

func (s *textEditorParagraphSelection) layoutWithDoubleClick(gtx layout.Context, editor *widget.Editor, content layout.Widget, onDoubleClick func(int) bool) layout.Dimensions {
	s.paragraphSelected = false
	pass := pointer.PassOp{}.Push(gtx.Ops)
	dims := content(gtx)
	area := clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops)
	s.click.Add(gtx.Ops)
	area.Pop()
	pass.Pop()
	for ev, ok := s.click.Update(gtx.Source); ok; ev, ok = s.click.Update(gtx.Source) {
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
