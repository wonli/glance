package desktopui

import (
	"testing"
	"time"
)

func TestTrayShowWaitsForClosingGlanceWindowInsteadOfModule(t *testing.T) {
	r := newDesktopRuntime()
	r.trayReady = true
	module := newAppWithRuntimeAndWindowID(nil, "", r, 1)
	module.proofreadingModule = true
	r.attachWindow(module.windowID, module)
	main := newAppWithRuntimeAndWindowID(nil, "", r, 2)
	r.attachWindow(main.windowID, main)
	r.markWindowClosing(main.windowID)
	r.showMainWindow()
	if !r.pendingMainWindowShow {
		t.Fatal("show request targeted document module instead of waiting to restore Glance window")
	}
}

func TestActivationListenerSurvivesInitialWindowClose(t *testing.T) {
	r := newDesktopRuntime()
	r.completion = make(chan struct{})
	// Avoid native window operations; this test exercises listener lifetime.
	r.quitRequested = true
	a := newAppWithRuntimeAndWindowID(nil, "", r, 1)
	close(a.shutdown)
	requests := make(chan struct{})
	done := make(chan struct{})
	go func() { r.consumeActivationRequests(requests); close(done) }()
	defer close(r.completion)
	select {
	case <-done:
		t.Fatal("activation listener stopped when initial window closed")
	case requests <- struct{}{}:
	case <-time.After(time.Second):
		t.Fatal("activation listener did not accept request")
	}
	close(requests)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("listener did not stop after activation source closed")
	}
}

func TestGlanceWindowSelectionExcludesPromotedDocumentModule(t *testing.T) {
	r := newDesktopRuntime()
	main := newAppWithRuntimeAndWindowID(nil, "", r, 1)
	r.attachWindow(main.windowID, main)
	module := newAppWithRuntimeAndWindowID(nil, "", r, 2)
	module.proofreadingModule = true
	r.attachWindow(module.windowID, module)
	r.detachWindow(main.windowID)
	if r.primaryWindowID != module.windowID {
		t.Fatal("fixture module was not promoted")
	}
	if got := r.glanceWindowLocked(); got != nil {
		t.Fatal("document module mistaken for Glance workbench")
	}
	reopened := newAppWithRuntimeAndWindowID(nil, "", r, 3)
	r.attachWindow(reopened.windowID, reopened)
	if got := r.glanceWindowLocked(); got != reopened {
		t.Fatal("reopened Glance window not found")
	}
	r.markWindowClosing(reopened.windowID)
	another := newAppWithRuntimeAndWindowID(nil, "", r, 4)
	r.attachWindow(another.windowID, another)
	if got := r.glanceWindowLocked(); got != another {
		t.Fatal("closing Glance window selected over live Glance window")
	}
}
