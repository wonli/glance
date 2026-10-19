//go:build !windows

package desktopui

import "gioui.org/app"

func (a *App) handleWordPointerViewEvent(event any)         {}
func (a *App) prepareWordPointerFrame(frame app.FrameEvent) {}
