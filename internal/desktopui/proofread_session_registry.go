package desktopui

import (
	"gioui.org/app"
	"gioui.org/io/system"
)

// claimProofreadingWindow prevents independent writable models for one task.
func (r *desktopRuntime) claimProofreadingWindow(id string, window *app.Window) *app.Window {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.proofWindows == nil {
		r.proofWindows = map[string]*app.Window{}
	}
	existing := r.proofWindows[id]
	if existing == nil {
		r.proofWindows[id] = window
	}
	return existing
}

func (r *desktopRuntime) focusProofreadingWindow(window *app.Window) {
	r.mu.Lock()
	for _, a := range r.windows {
		if a.window == window {
			a.proofreadingResumeRequested.Store(true)
		}
	}
	r.mu.Unlock()
	window.Perform(system.ActionRaise)
	window.Invalidate()
}
