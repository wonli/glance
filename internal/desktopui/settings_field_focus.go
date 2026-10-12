package desktopui

import (
	"image"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/widget"
)

// Centered editor text occupies only a line. Keep the surrounding field
// available for focus without enlarging the editor's text viewport.
func settingsFieldFocus(gtx layout.Context, editor *widget.Editor) {
	for _, ok := gtx.Event(pointer.Filter{Target: editor, Kinds: pointer.Press}); ok; _, ok = gtx.Event(pointer.Filter{Target: editor, Kinds: pointer.Press}) {
		gtx.Execute(key.FocusCmd{Tag: editor})
	}
	defer clip.Rect(image.Rectangle{Max: gtx.Constraints.Max}).Push(gtx.Ops).Pop()
	pointer.CursorText.Add(gtx.Ops)
	event.Op(gtx.Ops, editor)
}
