//go:build !darwin || ios || !cgo

package desktopui

type nativePinchMonitor struct{}

func (a *App) handleNativePinchViewEvent(event any) {}
func (a *App) closeNativePinch()                    {}
