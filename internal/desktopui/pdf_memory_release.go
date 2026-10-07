package desktopui

import (
	"runtime/debug"
	"sync/atomic"
	"time"
)

var pdfMemoryReleasePending atomic.Bool

// Delay collection until the closing frame and worker stacks have dropped
// their references. Coalesce closes and keep collection off the Gio loop.
func requestPDFMemoryRelease() {
	if !pdfMemoryReleasePending.CompareAndSwap(false, true) {
		return
	}
	time.AfterFunc(100*time.Millisecond, func() {
		pdfMemoryReleasePending.Store(false)
		debug.FreeOSMemory()
	})
}

// releasePDFViewResources drops view-local raster caches, thumbnails,
// widgets and cancels background workers.
func (state *documentSessionState) releasePDFViewResources() {
	if state == nil {
		return
	}
	if v := state.pdfView; v != nil {
		v.cancelWorkers()
		v.cache = nil
		v.thumbCache = nil
		v.buttons = nil
		state.pdfView = nil
	}
	requestPDFMemoryRelease()
}
