//go:build windows

package desktopui

import (
	"sync"
	"time"

	"golang.org/x/sys/windows/registry"
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
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		systemThemeCached = false
		return false
	}
	defer k.Close()

	val, _, err := k.GetIntegerValue("AppsUseLightTheme")
	if err == nil && val == 0 {
		systemThemeCached = true
		return true
	}
	systemThemeCached = false
	return false
}
