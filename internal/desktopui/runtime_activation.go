package desktopui

import (
	"gioui.org/io/system"
)

func (r *desktopRuntime) consumeActivationRequests(app *App, requests <-chan struct{}) {
	if app == nil {
		return
	}
	for {
		select {
		case _, ok := <-requests:
			if !ok {
				return
			}
			r.activatePrimaryWindow()
		case <-app.shutdown:
			return
		}
	}
}

func (r *desktopRuntime) activatePrimaryWindow() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.shuttingDown || r.quitRequested {
		r.mu.Unlock()
		return
	}
	app := r.windows[r.primaryWindowID]
	if app != nil && r.closingWindows[r.primaryWindowID] {
		app = nil
	}
	r.mu.Unlock()
	if app == nil || app.window == nil {
		// In tray mode the last window may already be gone while the runtime
		// remains alive. Reuse the runtime's existing restoration path.
		r.showMainWindow()
		return
	}
	raiseActiveWindow()
	app.window.Perform(system.ActionRaise)
	app.window.Invalidate()
}
