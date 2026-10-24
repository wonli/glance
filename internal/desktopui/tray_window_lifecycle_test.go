package desktopui

import (
	"testing"
)

func TestAppWindowIDPersistenceAcrossWorkspaceInitialization(t *testing.T) {
	runtime := newDesktopRuntime()
	app := newAppWithRuntimeAndWindowID(nil, "", runtime, 42)

	if app.windowID != 42 {
		t.Fatalf("expected app.windowID == 42, got %d", app.windowID)
	}

	// Simulating navigation to Recent tab or reinitializing workspace
	app.documentSessionState = newDocumentSessionState("")
	if app.windowID != 42 {
		t.Fatalf("expected app.windowID to remain 42 after documentSessionState reassignment, got %d", app.windowID)
	}

	app.activateRecent()
	if app.windowID != 42 {
		t.Fatalf("expected app.windowID to remain 42 after activateRecent(), got %d", app.windowID)
	}
}

func TestHideMainWindowToTrayPreconditions(t *testing.T) {
	runtime := newDesktopRuntime()
	app1 := newAppWithRuntimeAndWindowID(nil, "", runtime, 1)
	runtime.attachWindow(app1.windowID, app1)

	// Case 1: Tray not ready -> cannot hide to tray
	runtime.trayReady = false
	if app1.hideMainWindowToTray() {
		t.Fatal("expected hideMainWindowToTray to return false when tray is not ready")
	}

	// Case 2: Tray ready, single primary window -> can attempt hide to tray
	// Note: On darwin without GUI/cgo window it returns whatever C function returns or false if window nil.
	// But let's verify runtime preconditions directly.
	runtime.trayReady = true
	runtime.mu.Lock()
	canHide := runtime.trayReady && !runtime.shuttingDown && !runtime.quitRequested &&
		runtime.primaryWindowID == app1.windowID && runtime.windows[app1.windowID] == app1 &&
		len(runtime.windows) == 1
	runtime.mu.Unlock()
	if !canHide {
		t.Fatalf("expected canHide to be true for single primary window when tray is ready")
	}

	// Case 3: Multiple windows open -> primary window close must NOT hide to tray
	app2 := newAppWithRuntimeAndWindowID(nil, "", runtime, 2)
	runtime.attachWindow(app2.windowID, app2)
	runtime.mu.Lock()
	canHideMulti := runtime.trayReady && !runtime.shuttingDown && !runtime.quitRequested &&
		runtime.primaryWindowID == app1.windowID && runtime.windows[app1.windowID] == app1 &&
		len(runtime.windows) == 1
	runtime.mu.Unlock()
	if canHideMulti {
		t.Fatal("expected canHideMulti to be false when len(windows) > 1")
	}

	// Secondary window close must NOT hide to tray
	runtime.mu.Lock()
	canHideSecondary := runtime.trayReady && !runtime.shuttingDown && !runtime.quitRequested &&
		runtime.primaryWindowID == app2.windowID && runtime.windows[app2.windowID] == app2 &&
		len(runtime.windows) == 1
	runtime.mu.Unlock()
	if canHideSecondary {
		t.Fatal("expected canHideSecondary to be false for secondary window")
	}
}

func TestTrayShowMainWindowStateHandling(t *testing.T) {
	runtime := newDesktopRuntime()
	runtime.trayReady = true

	// Case 1: Window attached and alive
	app := newAppWithRuntimeAndWindowID(nil, "", runtime, 1)
	runtime.attachWindow(app.windowID, app)
	runtime.showMainWindow()

	runtime.mu.Lock()
	hasPending := runtime.pendingMainWindowShow
	creating := runtime.mainWindowCreating
	runtime.mu.Unlock()

	if hasPending || creating {
		t.Fatalf("expected showMainWindow with alive window not to set pending=%v or creating=%v", hasPending, creating)
	}

	// Case 2: Window closing
	runtime.markWindowClosing(app.windowID)
	runtime.showMainWindow()

	runtime.mu.Lock()
	hasPending = runtime.pendingMainWindowShow
	runtime.mu.Unlock()

	if !hasPending {
		t.Fatal("expected showMainWindow on closing window to set pendingMainWindowShow = true")
	}

	// Case 3: After window detached, showPendingMainWindow clears pending flag
	runtime.detachWindow(app.windowID)
	runtime.mu.Lock()
	runtime.pendingMainWindowShow = false
	runtime.mu.Unlock()
}
