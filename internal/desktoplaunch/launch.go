package desktoplaunch

import (
	"os"

	"gioui.org/app"

	"glance/internal/desktopui"
)

// RunDesktop starts the desktop runtime and enters Gio's platform event loop.
func RunDesktop(initialPath string) {
	done := desktopui.Run(initialPath)
	go func() {
		<-done
		os.Exit(0)
	}()
	app.Main()
}
