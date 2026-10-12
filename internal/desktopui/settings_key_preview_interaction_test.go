package desktopui

import (
	"image"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"glance/internal/agentsettings"
)

func TestSettingsSavedKeyEdgeClickFocusesVisiblePreview(t *testing.T) {
	for _, y := range []float32{29, 68} {
		t.Run(image.Pt(150, int(y)).String(), func(t *testing.T) {
			a := New(nil, "")
			a.agentUI.settingsTab = "openai"
			a.agentUI.settings = agentsettings.ModelSettings{ProviderID: "openai-compatible", BaseURL: "https://api.openai.com/v1", CredentialRef: "test-ref"}
			a.agentUI.providerInput.SetText("openai-compatible")
			a.agentUI.baseURLInput.SetText("https://api.openai.com/v1")
			a.agentUI.apiKeyInput.Mask = 0
			secret := strings.Repeat("saved-test-key", 30)
			preview := &a.agentUI.settingsKeyPreview
			preview.secret = secret
			preview.editor.SingleLine = true
			preview.editor.ReadOnly = true
			preview.editor.SetText(secret)
			var router input.Router
			var ops op.Ops
			frame := func() {
				ops.Reset()
				gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Min: image.Pt(400, 0), Max: image.Pt(400, 150)}}
				a.layoutAgentSettingsKeyField(gtx)
				router.Frame(&ops)
			}
			frame()
			router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(150, y), Buttons: pointer.ButtonPrimary})
			frame()
			router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(150, y)})
			frame()
			frame()
			if !router.Source().Focused(&preview.editor) {
				t.Fatal("blank edge focused invisible draft instead of saved-key preview")
			}
			router.Queue(key.Event{Name: key.NameEnd, State: key.Press})
			frame()
			start, end := preview.editor.Selection()
			if start != len(secret) || end != len(secret) {
				t.Fatal("End did not navigate to saved key suffix")
			}
			router.Queue(key.EditEvent{Text: "replacement"})
			frame()
			if preview.editor.Text() != secret || a.agentUI.apiKeyInput.Text() != "" {
				t.Fatal("read-only viewing modified saved key or draft")
			}
		})
	}
}
