package desktopui

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
)

func TestSettingsFieldBlankEdgesFocusEditor(t *testing.T) {
	for _, isKey := range []bool{false, true} {
		for _, y := range []float32{29, 49, 68} {
			t.Run(image.Pt(int(y), map[bool]int{false: 0, true: 1}[isKey]).String(), func(t *testing.T) {
				app := New(nil, "")
				editor := &widget.Editor{SingleLine: true}
				if isKey {
					editor = &app.agentUI.apiKeyInput
				}
				var router input.Router
				var ops op.Ops
				frame := func() {
					ops.Reset()
					gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Min: image.Pt(400, 0), Max: image.Pt(400, 150)}}
					if isKey {
						app.layoutAgentSettingsKeyField(gtx)
					} else {
						agentSettingsPageField(gtx, app.theme, "默认模型", editor, "输入模型")
					}
					router.Frame(&ops)
				}
				frame()
				router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(150, y), Buttons: pointer.ButtonPrimary})
				frame()
				router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(150, y)})
				frame()
				frame()
				if !router.Source().Focused(editor) {
					t.Fatal("click inside field did not focus editor")
				}
				router.Queue(key.EditEvent{Text: "中"})
				frame()
				if editor.Text() != "中" {
					t.Fatalf("typing after click was lost: %q", editor.Text())
				}
			})
		}
	}
}
