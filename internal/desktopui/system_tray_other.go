//go:build !linux && !darwin

package desktopui

func trayHostAvailable() error { return nil }
