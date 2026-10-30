package desktoplaunch

import (
	"log"
	"os"

	"gioui.org/app"

	"glance/internal/desktopinstance"
	"glance/internal/desktopui"
	"glance/internal/diagnostics"
	"glance/internal/fileutil"
)

// RunDesktop starts the desktop runtime and enters Gio's platform event loop.
func RunDesktop(initialPath string, options ...desktopui.DesktopOptions) {
	desktopui.InitDesktopRuntimeMemory()
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
	if cleaned, err := fileutil.CleanOrphanedTempFiles(0); err != nil {
		log.Printf("clean orphaned temporary files: %v (cleaned %d)", err, cleaned)
	}
	profiling, err := diagnostics.StartProfiling(os.Getenv("GLANCE_PPROF_ADDR"))
	if err != nil {
		log.Printf("start pprof: %v", err)
	} else if profiling != nil {
		log.Printf("pprof profiling available at http://%s/debug/pprof/", profiling.Addr)
		defer profiling.Close()
	}

	done := desktopui.RunWithActivation(initialPath, ReportStartupError, lease.ActivationRequests(), options...)
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
