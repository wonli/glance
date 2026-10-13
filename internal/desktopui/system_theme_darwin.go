//go:build darwin

package desktopui

import (
	"os/exec"
	"strings"
	"sync"
	"time"
)

var (
	systemThemeMu       sync.Mutex
	systemThemeLastCheck time.Time
	systemThemeCached   bool
)

func detectSystemDarkMode() bool {
	systemThemeMu.Lock()
	defer systemThemeMu.Unlock()

	now := time.Now()
	if now.Sub(systemThemeLastCheck) < 2*time.Second {
		return systemThemeCached
	}

	systemThemeLastCheck = now
	out, err := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle").Output()
	if err == nil && strings.Contains(strings.TrimSpace(string(out)), "Dark") {
		systemThemeCached = true
		return true
	}
	systemThemeCached = false
	return false
}
