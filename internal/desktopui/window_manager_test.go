package desktopui

import (
	"errors"
	"runtime"
	"testing"
)

func TestRuntimeRegistersAndRunsIndependentWindowApps(t *testing.T) {
	fixture := newWindowViewFixture(t)
	runtimeOwner := fixture.runtime
	source := fixture.apps[0]
	sourceState := fixture.states[0]
	runtimeOwner.detachWindow(2)
	var created *App
	runtimeOwner.windowLauncher = func(documentID string, snapshot windowViewSnapshot) error {
		created = newAppWithRuntime(nil, runtimeOwner)
		windowID := runtimeOwner.allocateWindowID()
		runtimeOwner.attachWindow(windowID, created)
		view := newDocumentViewFromSnapshot(fixture.shared[0], windowID, snapshot)
		view.documentID = documentID
		created.tabMeta[documentID] = source.tabMeta[documentID]
		created.sessions[documentID] = view
		created.activateSession(documentID, view)
		return nil
	}
	if err := runtimeOwner.openDocumentWindow(fixture.ids[0], captureWindowView(source)); err != nil {
		t.Fatal(err)
	}
	if created == nil || created == source {
		t.Fatal("runtime did not create a distinct window App")
	}
	if created.windowID == source.windowID || created.documentSessionState == sourceState || created.sharedDocumentSession != sourceState.sharedDocumentSession {
		t.Fatalf("new window ID/view/session = %d/%p/%p, source = %d/%p/%p", created.windowID, created.documentSessionState, created.sharedDocumentSession, source.windowID, sourceState, sourceState.sharedDocumentSession)
	}
	if len(runtimeOwner.collection.Tabs()) != 2 || len(runtimeOwner.sessions) != 2 {
		t.Fatalf("new window changed shared tab/session count: %d/%d", len(runtimeOwner.collection.Tabs()), len(runtimeOwner.sessions))
	}
}

func TestNewWindowFailureKeepsSourceWindow(t *testing.T) {
	fixture := newWindowViewFixture(t)
	source := fixture.apps[0]
	state, activeID, sessionID := source.documentSessionState, source.activeTabID, source.activeSessionID()
	fixture.runtime.windowLauncher = func(string, windowViewSnapshot) error { return errors.New("native window unavailable") }
	if err := fixture.runtime.openDocumentWindow(activeID, captureWindowView(source)); err == nil {
		t.Fatal("expected native window launch failure")
	}
	if source.documentSessionState != state || source.activeTabID != activeID || source.activeSessionID() != sessionID || state.closed.Load() {
		t.Fatal("failed window creation changed or closed the source view")
	}
}

func TestNewWindowCapabilityMatchesPlatform(t *testing.T) {
	want := runtime.GOOS == "darwin" || runtime.GOOS == "windows" || runtime.GOOS == "linux"
	if got := supportsMultipleDesktopWindows(); got != want {
		t.Fatalf("multi-window support on %s = %v, want %v", runtime.GOOS, got, want)
	}
}
