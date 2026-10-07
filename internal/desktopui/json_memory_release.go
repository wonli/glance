package desktopui

import (
	"runtime/debug"
	"sync/atomic"
	"time"

	"gioui.org/widget"
)

var jsonMemoryReleasePending atomic.Bool

// Delay collection until the closing frame and worker stacks have dropped
// their references. Coalesce closes and keep collection off the Gio loop.
func requestJSONMemoryRelease() {
	if !jsonMemoryReleasePending.CompareAndSwap(false, true) {
		return
	}
	time.AfterFunc(100*time.Millisecond, func() {
		jsonMemoryReleasePending.Store(false)
		debug.FreeOSMemory()
	})
}

// The signal is synchronized separately from view-local Gio state, since
// another window can close the shared document while this view is drawing.
func (state *documentSessionState) jsonWorkerSignal() chan struct{} {
	state.jsonWorkerMu.Lock()
	defer state.jsonWorkerMu.Unlock()
	if state.jsonWorkerDone == nil {
		state.jsonWorkerDone = make(chan struct{})
		if state.jsonWorkersClosed {
			close(state.jsonWorkerDone)
		}
	}
	return state.jsonWorkerDone
}

func (state *documentSessionState) cancelJSONWorkers() {
	state.jsonWorkerMu.Lock()
	if !state.jsonWorkersClosed {
		state.jsonWorkersClosed = true
		if state.jsonWorkerDone != nil {
			close(state.jsonWorkerDone)
		}
	}
	state.jsonWorkerMu.Unlock()
	if state.jsonLarge.Load() {
		requestJSONMemoryRelease()
	}
}

// Call only from the owning window's event loop, before dropping its view.
func (state *documentSessionState) releaseJSONViewResources() {
	large := state.jsonEditor.Len() > jsonSourceEditLimit
	if v := state.jsonView; v != nil {
		large = large || len(v.baseline) > jsonSourceEditLimit || len(v.requested) > jsonSourceEditLimit
		state.jsonView = nil
	}
	// SetText("") can retain backing buffers and undo state in the editor.
	state.jsonEditor = widget.Editor{}
	if large {
		state.jsonLarge.Store(true)
	}
	state.cancelJSONWorkers()
}

// A parse/transform already running at close can outlive the initial reclaim.
// Reclaim again after its last source/result reference leaves the worker.
func releaseClosedJSONWorkerMemory(done <-chan struct{}, large bool) {
	if large {
		select {
		case <-done:
			requestJSONMemoryRelease()
		default:
		}
	}
}
