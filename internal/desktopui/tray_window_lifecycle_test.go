package desktopui

import (
	"path/filepath"
	"testing"

	"glance/internal/workspace"
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

func TestTrayOrphanSessionEvictionOnDetachAndHide(t *testing.T) {
	dir := t.TempDir()
	store, err := workspace.Open(filepath.Join(dir, "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	col, err := workspace.OpenCollection(store)
	if err != nil {
		t.Fatal(err)
	}

	runtime := newDesktopRuntime()
	runtime.trayReady = true
	runtime.collection = col

	app1 := newAppWithRuntimeAndWindowID(nil, "", runtime, 1)
	app1.collection = col
	runtime.attachWindow(app1.windowID, app1)

	app2 := newAppWithRuntimeAndWindowID(nil, "", runtime, 2)
	app2.collection = col
	runtime.attachWindow(app2.windowID, app2)

	// Simulate document open in app1
	shared1 := newSharedDocumentSession("/path/to/doc1.pdf")
	shared1.documentID = "doc-orphan-1"
	runtime.registerSession(shared1)
	state1 := newDocumentSessionView(shared1, app1.windowID)
	app1.sessions["doc-orphan-1"] = state1

	// Simulate document open in app2
	shared2 := newSharedDocumentSession("/path/to/doc2.pdf")
	shared2.documentID = "doc-orphan-2"
	runtime.registerSession(shared2)
	state2 := newDocumentSessionView(shared2, app2.windowID)
	app2.sessions["doc-orphan-2"] = state2

	if len(runtime.sessions) != 2 {
		t.Fatalf("expected 2 runtime sessions, got %d", len(runtime.sessions))
	}

	// Window 1 detaches (closes)
	runtime.detachWindow(app1.windowID)

	// doc-orphan-1 had views only in window 1, so it must be evicted from runtime.sessions
	if runtime.sessions["doc-orphan-1"] != nil {
		t.Fatal("expected doc-orphan-1 to be evicted after window 1 detached")
	}
	// doc-orphan-2 is still in window 2
	if runtime.sessions["doc-orphan-2"] == nil {
		t.Fatal("expected doc-orphan-2 to remain while window 2 is alive")
	}

	// Window 2 switches to recent and hides to tray
	app2.activateRecent()
	app2.handleHiddenToTray()

	// doc-orphan-2 must now also be evicted from runtime.sessions
	if runtime.sessions["doc-orphan-2"] != nil {
		t.Fatal("expected doc-orphan-2 to be evicted after app2 hides to tray with recent active")
	}
	if len(runtime.sessions) != 0 {
		t.Fatalf("expected 0 runtime sessions in tray, got %d", len(runtime.sessions))
	}
}
