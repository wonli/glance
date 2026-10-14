//go:build !darwin

package desktopui

func (a *App) hideMainWindowToTray() bool { return false }
