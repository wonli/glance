//go:build !windows

package desktopui

import (
	"gioui.org/app"
	"gioui.org/io/event"
)

func handleWindowMoveViewEvent(window *app.Window, e event.Event) {}
