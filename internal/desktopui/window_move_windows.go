package desktopui

import (
	"glance/internal/windowmove"
	"glance/internal/windowplacement"
	"log"

	"gioui.org/app"
	"gioui.org/io/event"
)

func handleWindowMoveViewEvent(window *app.Window, e event.Event) {
	view, ok := e.(app.Win32ViewEvent)
	if !ok || view.HWND == 0 {
		return
	}
	window.Run(func() {
		if err := windowplacement.FitInitialWindow(view.HWND); err != nil {
			log.Printf("fit initial window: %v", err)
		}
		if err := windowmove.Attach(view.HWND); err != nil {
			log.Printf("%v; using original Gio window movement", err)
		}
	})
}
