package desktopui

import (
	"gioui.org/app"
	"gioui.org/io/system"
)

func (a *App) applyWindowConfig(config app.Config) {
	a.windowMode = config.Mode
	a.clientTitlebar = !config.Decorated
	if config.Mode == app.Minimized {
		requestMemoryRelease()
	}
	if a.themeMode == ThemeModeSystem {
		a.designTokens = ResolveDesignTokens(a.themeMode, a.designTokens.Typography)
	}
}

func windowModeToggleAction(mode app.WindowMode) system.Action {
	if mode == app.Maximized {
		return system.ActionUnmaximize
	}
	return system.ActionMaximize
}
