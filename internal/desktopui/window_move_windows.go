package desktopui

import (
	"glance/internal/windowmove"
	"log"

	"gioui.org/app"
	"gioui.org/io/event"
)

func handleWindowMoveViewEvent(window *app.Window, e event.Event) {
	view, ok := e.(app.Win32ViewEvent)
	if !ok || view.HWND == 0 || !windowmove.Enabled() {
		return
	}
	window.Run(func() {
		if err := windowmove.Attach(view.HWND); err != nil {
			log.Printf("%v; using original Gio window movement", err)
		}
	})
}
