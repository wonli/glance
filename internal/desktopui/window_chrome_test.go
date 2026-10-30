package desktopui

import (
	"image"
	"runtime"
	"testing"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestWindowModeToggleAction(t *testing.T) {
	tests := []struct {
		name string
		mode app.WindowMode
		want system.Action
	}{
		{name: "windowed maximizes", mode: app.Windowed, want: system.ActionMaximize},
		{name: "maximized restores", mode: app.Maximized, want: system.ActionUnmaximize},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := windowModeToggleAction(tt.mode); got != tt.want {
				t.Fatalf("windowModeToggleAction(%v) = %v, want %v", tt.mode, got, tt.want)
			}
		})
	}
}

func TestMacWindowControlsKeepIntrinsicWidth(t *testing.T) {
	a := New(nil, "")
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(500, 40)},
	}
	dims := a.layoutMacWindowControls(gtx)
	if dims.Size.X >= 120 {
		t.Fatalf("macOS window controls width = %d px; expected a compact traffic-light cluster", dims.Size.X)
	}
}

func TestWorkspaceTabStripAddsTopGapAndMeetsContentEdge(t *testing.T) {
	a := New(nil, "")
	a.clientTitlebar = true
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(800, 80)},
	}
	dims := a.layoutWorkspaceTabs(gtx)
	if dims.Size.Y != 42 {
		t.Fatalf("workspace tab strip height = %d px, want 42 px including the top gap", dims.Size.Y)
	}
}

func TestWindowConfigSelectsClientTitlebarOnlyWhenPlatformAllowsIt(t *testing.T) {
	a := &App{}
	a.applyWindowConfig(app.Config{Decorated: false, Mode: app.Maximized})
	if !a.clientTitlebar {
		t.Fatal("client titlebar disabled for an undecorated platform window")
	}
	if a.windowMode != app.Maximized {
		t.Fatalf("window mode = %v, want maximized", a.windowMode)
	}

	a.applyWindowConfig(app.Config{Decorated: true, Mode: app.Windowed})
	if a.clientTitlebar {
		t.Fatal("client titlebar enabled even though platform retained native decorations")
	}
	if a.windowMode != app.Windowed {
		t.Fatalf("window mode = %v, want windowed", a.windowMode)
	}
}

func TestWindowsChromePreviewOverridesOnlyChromePlatform(t *testing.T) {
	r := newDesktopRuntime()
	a := newAppWithRuntime(nil, r)
	if got := a.windowChromePlatform(); got != runtime.GOOS {
		t.Fatalf("default chrome=%q want %q", got, runtime.GOOS)
	}
	r.simulateWindowsChrome = true
	if got := a.windowChromePlatform(); got != "windows" {
		t.Fatalf("preview chrome=%q", got)
	}
	b := newAppWithRuntime(nil, r)
	if b.windowChromePlatform() != "windows" {
		t.Fatal("preview not inherited by another window")
	}
}
