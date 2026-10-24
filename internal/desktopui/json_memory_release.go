package desktopui

import (
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"gioui.org/widget"
)

var (
	memoryReleaseMu     sync.Mutex
	memoryReleaseTimer  *time.Timer
	memoryShrinkHooks   []func() error
	memoryShrinkHooksMu sync.Mutex
)

// registerMemoryShrinkHook attaches engine-level memory release routines (such as SQLite PRAGMA shrink_memory).
func registerMemoryShrinkHook(fn func() error) {
	if fn == nil {
		return
	}
	memoryShrinkHooksMu.Lock()
	defer memoryShrinkHooksMu.Unlock()
	memoryShrinkHooks = append(memoryShrinkHooks, fn)
}

func invokeMemoryShrinkHooks() {
	memoryShrinkHooksMu.Lock()
	hooks := append([]func() error(nil), memoryShrinkHooks...)
	memoryShrinkHooksMu.Unlock()
	for _, fn := range hooks {
		if fn != nil {
			_ = fn()
		}
	}
}

// requestMemoryRelease schedules OS memory reclamation off the Gio event loop.
// Calls are debounced so rapid tab closes or background worker terminations coalesce
// into a single collection after all references have been dropped.
func requestMemoryRelease() {
	memoryReleaseMu.Lock()
	defer memoryReleaseMu.Unlock()
	if memoryReleaseTimer != nil {
		memoryReleaseTimer.Stop()
	}
	memoryReleaseTimer = time.AfterFunc(150*time.Millisecond, func() {
		memoryReleaseMu.Lock()
		memoryReleaseTimer = nil
		memoryReleaseMu.Unlock()
		invokeMemoryShrinkHooks()
		runtime.GC()
		debug.FreeOSMemory()
	})
}

// Delay collection until the closing frame and worker stacks have dropped
// their references. Coalesce closes and keep collection off the Gio loop.
func requestJSONMemoryRelease() {
	requestMemoryRelease()
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
		requestMemoryRelease()
	}
}

// Call only from the owning window's event loop, before dropping its view.
func (state *documentSessionState) releaseJSONViewResources() {
	large := state.jsonEditor.Len() > jsonSourceEditLimit
	if v := state.jsonView; v != nil {
		large = large || len(v.baseline) > jsonSourceEditLimit || len(v.requested) > jsonSourceEditLimit
		v.tree = nil
		v.rows = nil
		v.outline = nil
		v.sourceSegments = nil
		v.baselineSegments = nil
		v.rowButtons = nil
		v.toggleButtons = nil
		v.outlineButtons = nil
		v.closingButtons = nil
		v.baseline = ""
		v.requested = ""
		v.parsed = ""
		v.sourceSynced = ""
		v.treeList = widget.List{}
		v.outlineList = widget.List{}
		v.inspectorList = widget.List{}
		v.sourceList = widget.List{}
		v.baselineList = widget.List{}
		v.search = widget.Editor{}
		v.value = widget.Editor{}
		v.original = widget.Editor{}
		select {
		case <-v.results:
		default:
		}
		select {
		case <-v.toolResults:
		default:
		}
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
			requestMemoryRelease()
		default:
		}
	}
}
