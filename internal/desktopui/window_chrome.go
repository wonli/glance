package desktopui

import (
	runtimelib "runtime"
	"runtime/debug"
	"time"

	"gioui.org/app"
	"gioui.org/io/system"
	"glance/internal/diagramrender"
)

func (a *App) applyWindowConfig(config app.Config) {
	a.windowMode = config.Mode
	a.clientTitlebar = !config.Decorated
	if config.Mode == app.Minimized {
		requestMemoryRelease()
	}
	a.handleWindowFocusConfig(config.Focused)
	if a.themeMode == ThemeModeSystem {
		a.designTokens = ResolveDesignTokens(a.themeMode, a.designTokens.Typography)
	}
}

var backgroundIdleReclaimDelay = 20 * time.Second

func (a *App) handleWindowFocusConfig(focused bool) {
	a.backgroundReclaimMu.Lock()
	defer a.backgroundReclaimMu.Unlock()
	wasFocused := a.windowFocused
	a.windowFocused = focused
	if focused {
		if a.backgroundReclaimTimer != nil {
			a.backgroundReclaimTimer.Stop()
			a.backgroundReclaimTimer = nil
		}
		return
	}
	// Window transitioned from focused to unfocused, or started unfocused.
	// Schedule idle background memory reclamation after a grace period.
	if wasFocused || a.backgroundReclaimTimer == nil {
		if a.backgroundReclaimTimer != nil {
			a.backgroundReclaimTimer.Stop()
		}
		a.backgroundReclaimTimer = time.AfterFunc(backgroundIdleReclaimDelay, func() {
			a.backgroundReclaimMu.Lock()
			a.backgroundReclaimTimer = nil
			stillUnfocused := !a.windowFocused
			a.backgroundReclaimMu.Unlock()
			if stillUnfocused {
				a.reclaimBackgroundMemory()
			}
		})
	}
}

func (a *App) reclaimBackgroundMemory() {
	if a == nil {
		return
	}
	// 1. Evict inactive document sessions (retains only the active document)
	a.evictInactiveSessions()
	// 2. Evict any orphan sessions in runtime
	runtime := a.ensureDesktopRuntime()
	runtime.evictOrphanSessions()
	// 3. Release Word layout engine and font source while idle in the background.
	// They will be lazily re-instantiated on demand when the user returns to edit or repaginate.
	a.releaseIdleEnginesForBackground()
	// 4. Request debounced memory release (calls SQLite PRAGMA shrink_memory, runtime.GC, debug.FreeOSMemory)
	requestMemoryRelease()
}

func (a *App) handleHiddenToTray() {
	if a == nil {
		return
	}
	a.windowFocused = false
	a.backgroundReclaimMu.Lock()
	if a.backgroundReclaimTimer != nil {
		a.backgroundReclaimTimer.Stop()
		a.backgroundReclaimTimer = nil
	}
	a.backgroundReclaimMu.Unlock()

	// When hidden to tray, window is not visible to the user.
	// 1. Evict inactive document sessions
	a.evictInactiveSessions()
	// 2. Evict any orphan sessions across all windows
	runtime := a.ensureDesktopRuntime()
	runtime.evictOrphanSessions()
	// 3. Release idle engines and font sources
	a.releaseIdleEnginesForBackground()
	// 4. Force immediate SQLite shrink and OS memory reclamation off the event loop
	go func() {
		_ = runtime.shrinkWorkspace()
		runtimelib.GC()
		debug.FreeOSMemory()
	}()
}

func (a *App) releaseIdleEnginesForBackground() {
	if a == nil {
		return
	}
	if !a.hasActiveWordDocument() {
		a.wordLayoutEngine = nil
		a.wordLayoutEngineFontVersion = 0
		a.wordPDFFontSource = nil
	}
	if !a.hasActiveMarkdownDocument() {
		a.launchManagedBackground(diagramrender.IdleCleanup())
	}
}

func windowModeToggleAction(mode app.WindowMode) system.Action {
	if mode == app.Maximized {
		return system.ActionUnmaximize
	}
	return system.ActionMaximize
}
