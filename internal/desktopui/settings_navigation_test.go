package desktopui

import (
	"bytes"
	"glance/internal/agentsettings"
	"image"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestSettingsReturnDrawsHomeOnClickFrame(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	app := New(nil, "")
	size := image.Pt(1440, 900)
	now := time.Date(2026, time.October, 12, 12, 0, 0, 0, time.UTC)
	render := func() *image.RGBA {
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Now: now, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: size}}
		start := time.Now()
		app.layoutRecent(gtx)
		t.Logf("layout section=%d took %s", app.homeSection, time.Since(start))
		return renderPresentationTestOps(t, &ops, size)
	}
	render() // Home was already displayed before entering settings.
	app.homeSection = homeSectionSettings
	app.agentUI.settingsPageTab = "providers"
	app.agentUI.settingsTab = "openai"
	render()
	app.homeNavButton("settings-back").Click()
	onClick := render()
	if app.homeSection != homeSectionRecent {
		t.Fatal("back click did not select home")
	}
	nextFrame := render()
	if !bytes.Equal(onClick.Pix, nextFrame.Pix) {
		t.Fatal("back click selected home but drew a stale settings frame before home")
	}
}

func TestSettingsReentryDrawsSettingsOnClickFrame(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := New(nil, "")
	rt := a.ensureDesktopRuntime()
	store, err := agentsettings.NewStore(filepath.Join(t.TempDir(), "settings.json"), &desktopMemoryCredentials{items: make(map[string]string)})
	if err != nil {
		t.Fatal(err)
	}
	rt.agentSettings = store
	rt.agentSettingsErr = nil
	t.Cleanup(rt.shutdown)
	size := image.Pt(1440, 900)
	now := time.Date(2026, time.October, 12, 12, 0, 0, 0, time.UTC)
	render := func() *image.RGBA {
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Now: now, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: size}}
		start := time.Now()
		a.layoutRecent(gtx)
		t.Logf("section=%d layout=%s", a.homeSection, time.Since(start))
		return renderPresentationTestOps(t, &ops, size)
	}
	render()
	a.homeNavButton("agent-settings").Click()
	render()
	render()
	a.homeNavButton("settings-back").Click()
	render()
	a.homeNavButton("agent-settings").Click()
	onClick := render()
	if a.homeSection != homeSectionSettings {
		t.Fatal("reentry did not select settings")
	}
	stable := render()
	if !bytes.Equal(onClick.Pix, stable.Pix) {
		t.Fatal("reentry drew home sidebar and settings content before the real settings page")
	}
}
