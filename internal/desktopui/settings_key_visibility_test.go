package desktopui

import (
	"fmt"
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestSettingsAPIKeyEyeClickTogglesVisibility(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		for _, y := range []float32{340, 360, 374} {
			t.Run(fmt.Sprintf("scale=%g/y=%g", scale, y), func(t *testing.T) {
				a := New(nil, "")
				a.homeSection = homeSectionSettings
				a.agentUI.settingsPageTab = "providers"
				a.agentUI.settingsTab = "openai"
				a.agentUI.apiKeyInput.SetText("sk-test-visible")
				var router input.Router
				var ops op.Ops
				frame := func() {
					ops.Reset()
					gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Constraints{Max: image.Pt(int(1440*scale), int(900*scale))}}
					a.layoutRecent(gtx)
					router.Frame(&ops)
				}
				frame()
				click := func() {
					position := f32.Pt(1069*scale, y*scale)
					router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Position: position, Buttons: pointer.ButtonPrimary})
					frame()
					router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: position})
					frame()
					frame()
				}
				click()
				if a.agentUI.apiKeyInput.Mask != 0 {
					t.Fatal("eye click did not reveal the entered key")
				}
				click()
				if a.agentUI.apiKeyInput.Mask != '•' {
					t.Fatal("eye click did not mask the entered key again")
				}
				if a.agentUI.apiKeyInput.Text() != "sk-test-visible" {
					t.Fatal("visibility toggle changed editor contents")
				}
			})
		}
	}
}
