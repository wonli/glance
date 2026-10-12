package desktopui

import (
	"bytes"
	"context"
	"image"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"glance/internal/agentsettings"
)

func TestSettingsEyeRevealsSavedKeyWithoutCreatingDraft(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	secret := "saved-test-key" + strings.Repeat("-long-test-key", 32)
	credentials := &desktopMemoryCredentials{items: make(map[string]string)}
	store, err := agentsettings.NewStore(filepath.Join(t.TempDir(), "settings.json"), credentials)
	if err != nil {
		t.Fatal(err)
	}
	settings := agentsettings.ModelSettings{ProviderID: "openai-compatible", BaseURL: "https://api.openai.com/v1", Model: "test-model"}
	if err := store.Save(context.Background(), settings, secret); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	a := New(nil, "")
	rt := a.ensureDesktopRuntime()
	rt.agentSettings = store
	rt.agentSettingsErr = nil
	t.Cleanup(func() { close(a.shutdown); a.stopAndWaitManagedBackground(); rt.shutdown() })
	a.homeSection = homeSectionSettings
	a.agentUI.settingsPageTab = "providers"
	a.agentUI.settingsTab = "openai"
	a.agentUI.settings = saved
	a.agentUI.providerInput.SetText(saved.ProviderID)
	a.agentUI.baseURLInput.SetText(saved.BaseURL)
	render := func() []byte {
		a.processAsyncResults()
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(1440, 900)}}
		a.layoutRecent(gtx)
		img := renderPresentationTestOps(t, &ops, image.Pt(1440, 900))
		var result []byte
		for y := 342; y < 378; y++ {
			start := img.PixOffset(322, y)
			result = append(result, img.Pix[start:start+300*4]...)
		}
		return result
	}
	a.agentUI.apiKeyInput.Mask = 0
	a.agentUI.apiKeyInput.SetText(secret)
	wantVisible := render()
	a.agentUI.apiKeyInput.SetText("")
	a.agentUI.apiKeyInput.Mask = '•'
	wantHidden := render()
	a.agentUI.settingsKeyToggle.Click()
	deadline := time.Now().Add(2 * time.Second)
	for !bytes.Equal(render(), wantVisible) {
		if time.Now().After(deadline) {
			t.Fatal("eye did not display the saved key")
		}
		time.Sleep(time.Millisecond)
	}
	if a.agentUI.settingsKeyPreview.editor.Text() != secret || !a.agentUI.settingsKeyPreview.editor.ReadOnly {
		t.Fatal("saved key does not have a complete read-only scrollable preview")
	}
	beforeScroll := render()
	end := len([]rune(secret))
	a.agentUI.settingsKeyPreview.editor.SetCaret(end, end)
	if bytes.Equal(beforeScroll, render()) {
		t.Fatal("long saved key cannot scroll to its suffix")
	}
	if a.agentUI.apiKeyInput.Text() != "" {
		t.Fatal("viewing saved key populated the editable draft")
	}
	a.agentUI.settingsKeyToggle.Click()
	if !bytes.Equal(render(), wantHidden) {
		t.Fatal("hiding saved key did not restore masked display")
	}
	unchanged, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if unchanged != saved || len(credentials.items) != 1 {
		t.Fatal("viewing changed saved settings or rotated credential")
	}
}
