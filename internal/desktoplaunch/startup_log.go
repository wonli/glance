package desktoplaunch

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"glance/internal/fileutil"
)

var startupLogFile *os.File

func initializeStartupLog() error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("locate the user config directory: %w", err)
	}
	logDir := filepath.Join(configDir, fileutil.AppDir)
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return fmt.Errorf("create startup log directory %q: %w", logDir, err)
	}
	file, err := os.OpenFile(filepath.Join(logDir, "startup.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open startup log: %w", err)
	}
	startupLogFile = file
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.SetOutput(file)
	return nil
}

// ReportStartupError records an error and presents it through the native
// platform dialog when one is available.
func ReportStartupError(err error) {
	if err == nil {
		return
	}
	log.Printf("startup error: %v", err)
	showStartupError(err)
}
