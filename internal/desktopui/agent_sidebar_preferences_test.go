package desktopui

import (
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/workspace"
	"image"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestAgentSidebarWidthSurvivesRuntimeRestart(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		path := filepath.Join(t.TempDir(), "workspace.db")
		store, err := workspace.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		r := newDesktopRuntime()
		r.workspaceStore = store
		a := newAppWithRuntime(nil, r)
		a.initializeWorkspace()
		a.documentID = "width-test"
		a.agentUI.conversationDocumentID = a.documentID
		a.agentUI.sidebarVisible = true
		var ops op.Ops
		var router input.Router
		frame := func() {
			ops.Reset()
			gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(int(1000*scale), int(701*scale)))}
			a.layoutAgentSidebarWorkspace(gtx, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} })
			router.Frame(&ops)
		}
		frame()
		start := f32.Pt(249*scale, 350*scale)
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 3, Buttons: pointer.ButtonPrimary, Position: start})
		frame()
		end := start.Add(f32.Pt(70*scale, 0))
		router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 3, Buttons: pointer.ButtonPrimary, Position: end})
		frame()
		r.drainWorkspaceWriter()
		if _, found, err := store.Preference(agentSidebarWidthPreference); err != nil || found {
			t.Fatal("drag preview persisted before release")
		}
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 3, Position: end})
		frame()
		r.shutdown()
		reopened, err := workspace.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		next := newDesktopRuntime()
		next.workspaceStore = reopened
		b := newAppWithRuntime(nil, next)
		b.initializeWorkspace()
		if b.agentUI.sidebarWidthDP != 320 {
			t.Fatalf("scale %v restored width=%v want 320dp", scale, b.agentUI.sidebarWidthDP)
		}
		next.shutdown()
	}
}

func TestAgentSidebarPreferenceWritesKeepLatestValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.db")
	store, err := workspace.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	r := newDesktopRuntime()
	r.workspaceStore = store
	for i := 0; i < 500; i++ {
		r.queueWorkspacePreference(agentSidebarWidthPreference, "380.5")
	}
	r.queueWorkspacePreference(agentSidebarWidthPreference, "410")
	r.shutdown()
	store, err = workspace.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	value, found, err := store.Preference(agentSidebarWidthPreference)
	if err != nil || !found || value != "410" {
		t.Fatalf("latest width=%q found=%v err=%v", value, found, err)
	}
}

func TestAgentSidebarIgnoresInvalidSavedWidths(t *testing.T) {
	store, err := workspace.Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	r := newDesktopRuntime()
	r.workspaceStore = store
	defer r.shutdown()
	a := newAppWithRuntime(nil, r)
	for _, value := range []string{"NaN", "+Inf", "-1", "0", "601", "bad"} {
		if err := store.SetPreference(agentSidebarWidthPreference, value); err != nil {
			t.Fatal(err)
		}
		a.agentUI.sidebarWidthDP = 0
		a.loadAgentSidebarPreference()
		if a.agentUI.sidebarWidthDP != 0 {
			t.Fatalf("invalid width %q restored", value)
		}
	}
}

func TestPreferenceFlushDoesNotDeadlockFullWorkspaceQueueDrain(t *testing.T) {
	store, err := workspace.Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	r := newDesktopRuntime()
	r.workspaceStore = store
	r.startWorkspaceWriter()
	// Hold preference flushing so the writer stops after receiving this job.
	r.workspacePreferenceMu.Lock()
	r.workspaceWriteJobs <- workspaceWriteJob{savePreferences: true}
	for i := 0; i < cap(r.workspaceWriteJobs); i++ {
		r.workspaceWriteJobs <- workspaceWriteJob{savePreferences: true}
	}
	drained := make(chan struct{})
	go func() { r.drainWorkspaceWriter(); close(drained) }()
	// Confirm drain has locked the full queue before letting the worker flush.
	deadline := time.Now().Add(time.Second)
	locked := false
	for time.Now().Before(deadline) {
		if !r.workspaceQueueMu.TryLock() {
			locked = true
			break
		}
		r.workspaceQueueMu.Unlock()
		runtime.Gosched()
	}
	r.workspacePreferenceMu.Unlock()
	if !locked {
		t.Fatal("drain never reached the full queue")
	}
	select {
	case <-drained:
	case <-time.After(2 * time.Second):
		t.Fatal("full queue drain deadlocked with preference flush")
	}
	r.shutdown()
}
