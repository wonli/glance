package desktoplaunch

import (
	"log"
	"os"

	"gioui.org/app"

	"glance/internal/desktopinstance"
	"glance/internal/desktopui"
)

// RunDesktop starts the desktop runtime and enters Gio's platform event loop.
func RunDesktop(initialPath string) {
	if err := initializeStartupLog(); err != nil {
		ReportStartupError(err)
	}
	lease, primary, err := desktopinstance.AcquireOrActivate()
	if err != nil {
		ReportStartupError(err)
		return
	}
	if !primary {
		return
	}
	done := desktopui.RunWithActivation(initialPath, ReportStartupError, lease.ActivationRequests())
	go func() {
		<-done
		if err := lease.Close(); err != nil {
			log.Printf("release desktop instance lease: %v", err)
		}
		os.Exit(0)
	}()
	app.Main()
	if err := lease.Close(); err != nil {
		log.Printf("release desktop instance lease: %v", err)
	}
}
