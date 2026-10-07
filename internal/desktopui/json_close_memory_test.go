package desktopui

import (
	"image"
	"os"
	"runtime"
	"runtime/debug"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/format/jsondoc"
)

func TestClosedJSONViewDropsDocumentResources(t *testing.T) {
	a := newTestAppWithJSON(t, `{"value":"x"}`)
	v := a.ensureJSONView()
	tree, err := jsondoc.ParseTree(a.jsonDoc.Text())
	if err != nil {
		t.Fatal(err)
	}
	v.accept(jsonTreeResult{text: tree.Source, tree: tree})
	state := a.documentSessionState
	// A queued snapshot or late completion may retain the closed view itself.
	closeSessionState(state)
	if state.jsonView != nil || state.jsonEditor.Len() != 0 {
		t.Fatal("closed view still owns JSON tree/source/editor")
	}
	select {
	case <-v.done:
	default:
		t.Fatal("closed view did not stop worker publication")
	}
	closeSessionState(state) // Repeated teardown must be harmless.
}

func TestJSONWorkerCancellationDuringViewCreation(t *testing.T) {
	a := newTestAppWithJSON(t, `{"value":"x"}`)
	shared := a.sharedDocumentSession
	for i := 0; i < 100; i++ {
		state := newDocumentSessionView(shared, 0)
		a.documentSessionState = state
		cancelled := make(chan struct{})
		go func() {
			// Shared-session close may run on another window's UI loop.
			state.cancelJSONWorkers()
			close(cancelled)
		}()
		v := a.ensureJSONView()
		<-cancelled
		select {
		case <-v.done:
		default:
			t.Fatal("a view created during close can still publish results")
		}
		closeSessionState(state)
	}
}

func TestRealJSONCloseReclaimsMemory(t *testing.T) {
	path := os.Getenv("GLANCE_JSON_LARGE_FILE")
	if path == "" {
		t.Skip("set GLANCE_JSON_LARGE_FILE to measure close memory")
	}
	a := New(nil, "")
	defer close(a.shutdown)
	warm := jsondoc.NewBlank()
	renderAndCloseJSON(t, a, loadedDocument{path: "warm.json", jsonDoc: warm})
	debug.FreeOSMemory()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for _, pending := range []bool{false, true} {
		openRenderAndCloseJSON(t, a, path, pending)
		deadline := time.Now().Add(3 * time.Second)
		// The application must reclaim on its own; no test-triggered GC here.
		for {
			runtime.ReadMemStats(&after)
			retained := int64(after.HeapAlloc) - int64(before.HeapAlloc)
			if retained <= 32*1024*1024 {
				t.Logf("pending=%v baseline=%.1f MiB closed=%.1f MiB retained=%.1f MiB committedHeap=%.1f MiB", pending, float64(before.HeapAlloc)/1048576, float64(after.HeapAlloc)/1048576, float64(retained)/1048576, float64(after.HeapSys-after.HeapReleased)/1048576)
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("pending=%v: closed document retained %.1f MiB", pending, float64(retained)/1048576)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	runtime.KeepAlive(a)
}

func openRenderAndCloseJSON(t *testing.T, a *App, path string, pending bool) {
	t.Helper()
	doc, err := jsondoc.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if pending {
		if err := a.installDocument(loadedDocument{path: path, jsonDoc: doc}); err != nil {
			t.Fatal(err)
		}
		a.requestJSONTree()
		a.closeWorkspaceTabNow(a.activeTabID)
	} else {
		renderAndCloseJSON(t, a, loadedDocument{path: path, jsonDoc: doc})
	}
}

func renderAndCloseJSON(t *testing.T, a *App, loaded loadedDocument) {
	t.Helper()
	if err := a.installDocument(loaded); err != nil {
		t.Fatal(err)
	}
	v := a.ensureJSONView()
	tree, err := jsondoc.ParseTree(a.jsonDoc.Text())
	if err != nil {
		t.Fatal(err)
	}
	segments := jsonSourceSegments(tree.Source)
	v.accept(jsonTreeResult{text: tree.Source, tree: tree, segments: segments, baselineSegments: segments})
	for _, mode := range []jsonViewMode{jsonModeTree, jsonModeSource, jsonModeCompare} {
		v.mode = mode
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1440, 858))}
		a.layoutJSON(gtx)
	}
	a.closeWorkspaceTabNow(a.activeTabID)
}
