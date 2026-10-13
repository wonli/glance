package desktoplaunch

import (
	"os"

	"gioui.org/app"

	"glance/internal/desktopui"
)

// RunDesktop starts the desktop runtime and enters Gio's platform event loop.
func RunDesktop(initialPath string) {
	if err := initializeStartupLog(); err != nil {
		ReportStartupError(err)
	}
	done := desktopui.RunWithStartupError(initialPath, ReportStartupError)
	go func() {
		<-done
		os.Exit(0)
	}()
	app.Main()
}
