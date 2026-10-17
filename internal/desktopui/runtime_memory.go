package desktopui

import (
	"os"
	"runtime/debug"
)

// InitDesktopRuntimeMemory tunes Go's runtime GC and memory limits for a responsive desktop application.
func InitDesktopRuntimeMemory() {
	if os.Getenv("GOMEMLIMIT") == "" {
		// Soft limit 512MB to keep desktop footprint well below WPS while giving ample room for large docs.
		debug.SetMemoryLimit(512 * 1024 * 1024)
	}
	if os.Getenv("GOGC") == "" {
		// Set GC target to 50% growth (default is 100%), so memory reclamation happens much sooner.
		debug.SetGCPercent(50)
	}
}
