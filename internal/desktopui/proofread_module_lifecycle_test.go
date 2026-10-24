package desktopui

import (
	"gioui.org/app"
	"strings"
	"testing"
)

func TestProofreadingModuleRetiresBeforeShutdownWorkCompletes(t *testing.T) {
	r := newDesktopRuntime()
	host := newAppWithRuntimeAndWindowID(nil, "", r, 1)
	host.window = new(app.Window)
	host.proofreadingModule = true
	r.attachWindow(1, host)
	r.proofreadingModuleWindow = host.window
	r.proofreadingModuleRequests = make(chan proofreadingWindowRequest, 16)
	r.proofTabOwners = map[string]*proofreadingTabOwner{"project": {host: host, id: "proofreading:1"}}
	r.markWindowClosing(host.windowID)
	if r.proofreadingModuleWindow != nil || r.proofreadingModuleRequests != nil {
		t.Fatal("closing document workbench remains available for new PDF requests")
	}
	if !r.proofTabOwners["project"].closing {
		t.Fatal("closing manuscript can still route focus to a destroyed window")
	}
}

type moduleLifecycleSnapshot struct {
	*strings.Reader
	closed bool
}

func (s *moduleLifecycleSnapshot) Close() error { s.closed = true; return nil }

func TestProofreadingModuleReopensAfterBothWindowsCloseAndTrayRestore(t *testing.T) {
	r := newDesktopRuntime()
	r.trayReady = true
	main := newAppWithRuntimeAndWindowID(nil, "", r, 1)
	r.attachWindow(1, main)
	oldSource := &moduleLifecycleSnapshot{Reader: strings.NewReader("old PDF")}
	oldWindow, oldQueue, start, err := r.enqueueProofreadingModuleRequest(proofreadingWindowRequest{sourceSnapshot: oldSource})
	if err != nil || !start {
		t.Fatalf("first module: start=%v err=%v", start, err)
	}
	oldHost := newAppWithRuntimeAndWindowID(nil, "", r, 2)
	oldHost.window, oldHost.proofreadingModule = oldWindow, true
	r.attachWindow(2, oldHost)
	r.markWindowClosing(1)
	r.detachWindow(1)
	if r.canHideGlanceWindowToTray(oldHost) {
		t.Fatal("last document module must close, not remain hidden in tray")
	}
	r.markWindowClosing(2)
	r.detachWindow(2)
	if r.hasWindows() || !r.shouldKeepAlive() {
		t.Fatal("tray runtime did not survive closing both windows")
	}
	// Simulate the ordinary window registration performed by tray restoration.
	restored := newAppWithRuntimeAndWindowID(nil, "", r, 3)
	r.attachWindow(3, restored)
	if !r.canHideGlanceWindowToTray(restored) {
		t.Fatal("restored Glance window lost normal tray behavior")
	}
	newSource := &moduleLifecycleSnapshot{Reader: strings.NewReader("new PDF")}
	window, queue, start, err := r.enqueueProofreadingModuleRequest(proofreadingWindowRequest{sourceSnapshot: newSource})
	if err != nil || !start || window == oldWindow || queue == oldQueue {
		t.Fatalf("reentry reused closed window: start=%v err=%v", start, err)
	}
	// Delayed cleanup must not clear the new module or consume its PDF.
	r.finishProofreadingModuleWindow(oldWindow, oldQueue)
	if !oldSource.closed || newSource.closed || r.proofreadingModuleWindow != window || r.proofreadingModuleRequests != queue {
		t.Fatal("old cleanup damaged replacement module or retained its old snapshot")
	}
	select {
	case request := <-queue:
		if request.sourceSnapshot != newSource {
			t.Fatal("wrong PDF delivered to replacement module")
		}
		request.sourceSnapshot.Close()
	default:
		t.Fatal("replacement module lost PDF request")
	}
	r.finishProofreadingModuleWindow(window, queue)
}
