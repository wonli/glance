//go:build !darwin && !windows

package desktopui

import (
	"os/exec"
	"strings"
	"sync"
	"time"
)

var (
	systemThemeMu        sync.Mutex
	systemThemeLastCheck time.Time
	systemThemeCached    bool
)

func detectSystemDarkMode() bool {
	systemThemeMu.Lock()
	defer systemThemeMu.Unlock()

	now := time.Now()
	if now.Sub(systemThemeLastCheck) < 2*time.Second {
		return systemThemeCached
	}

	systemThemeLastCheck = now
	out, err := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme").Output()
	if err == nil && strings.Contains(strings.ToLower(string(out)), "dark") {
		systemThemeCached = true
		return true
	}
	systemThemeCached = false
	return false
}
