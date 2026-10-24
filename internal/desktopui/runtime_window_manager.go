package desktopui

func (r *desktopRuntime) attachWindow(windowID uint64, app *App) {
	if r == nil || app == nil {
		return
	}
	r.mu.Lock()
	if r.windows == nil {
		r.windows = make(map[uint64]*App)
	}
	if r.windowAttachmentOrder == nil {
		r.windowAttachmentOrder = make(map[uint64]uint64)
	}
	if r.closingWindows == nil {
		r.closingWindows = make(map[uint64]bool)
	}
	if windowID == 0 {
		r.nextWindowID++
		windowID = r.nextWindowID
	} else if windowID > r.nextWindowID {
		r.nextWindowID = windowID
	}
	r.windows[windowID] = app
	if _, attached := r.windowAttachmentOrder[windowID]; !attached {
		r.nextWindowAttachment++
		r.windowAttachmentOrder[windowID] = r.nextWindowAttachment
	}
	if r.primaryWindowID == 0 {
		r.primaryWindowID = windowID
	}
	r.mu.Unlock()
	app.desktopRuntime = r
	app.windowID = windowID
	if app.documentSessionState != nil {
		app.documentSessionState.windowID = windowID
		app.setActiveSessionID(app.documentSessionState.documentID)
		app.documentSessionState.sharedDocumentSession.attachView(windowID, app.documentSessionState)
	}
}

func (r *desktopRuntime) detachWindow(windowID uint64) {
	if r == nil || windowID == 0 {
		return
	}
	r.mu.Lock()
	app := r.windows[windowID]
	if app == nil {
		r.mu.Unlock()
		return
	}
	delete(r.windows, windowID)
	delete(r.windowAttachmentOrder, windowID)
	delete(r.closingWindows, windowID)
	wasPrimary := r.primaryWindowID == windowID
	if wasPrimary {
		r.primaryWindowID = 0
	}
	sessions := make([]*sharedDocumentSession, 0, len(r.sessions))
	for _, session := range r.sessions {
		sessions = append(sessions, session)
	}
	r.mu.Unlock()
	for _, session := range sessions {
		session.clearPresentationDrafts(windowID)
		session.viewsMu.Lock()
		view := session.views[windowID]
		delete(session.views, windowID)
		session.viewsMu.Unlock()
		if view != nil {
			releaseMarkdownDraftState(view)
			view.releaseJSONViewResources()
			view.releaseWordViewResources()
			view.releasePDFViewResources()
			view.closed.Store(true)
		}
	}
	for _, view := range app.sessions {
		if view != nil {
			view.releaseJSONViewResources()
			view.releaseWordViewResources()
			view.releasePDFViewResources()
			view.closed.Store(true)
		}
	}
	if app.documentSessionState != nil {
		releaseMarkdownDraftState(app.documentSessionState)
		app.documentSessionState.releaseJSONViewResources()
		app.documentSessionState.releaseWordViewResources()
		app.documentSessionState.releasePDFViewResources()
		app.documentSessionState.closed.Store(true)
	}
	if wasPrimary {
		r.promotePrimaryWindow()
	}
}

func (r *desktopRuntime) markWindowClosing(windowID uint64) {
	if r == nil || windowID == 0 {
		return
	}
	r.mu.Lock()
	if r.windows[windowID] != nil {
		if r.closingWindows == nil {
			r.closingWindows = make(map[uint64]bool)
		}
		r.closingWindows[windowID] = true
		host := r.windows[windowID]
		if host.proofreadingModule && r.proofreadingModuleWindow == host.window {
			r.proofreadingModuleWindow = nil
			r.proofreadingModuleRequests = nil
		}
		for _, owner := range r.proofTabOwners {
			if owner.host == host {
				owner.closing = true
			}
		}
	}
	r.mu.Unlock()
}

// promotePrimaryWindow assigns the oldest attached window and asks that
// window's own Gio loop to persist its independent workspace view state.
func (r *desktopRuntime) promotePrimaryWindow() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.primaryWindowID != 0 && r.windows[r.primaryWindowID] != nil {
		r.mu.Unlock()
		return
	}
	var promotedID, oldestOrder uint64
	var promoted *App
	for windowID, app := range r.windows {
		order := r.windowAttachmentOrder[windowID]
		if app == nil || (promotedID != 0 && order >= oldestOrder) {
			continue
		}
		promotedID, promoted, oldestOrder = windowID, app, order
	}
	r.primaryWindowID = promotedID
	r.mu.Unlock()
	if promoted != nil {
		promoted.primaryPromotionPending.Store(true)
		if promoted.window != nil {
			promoted.window.Invalidate()
		}
	}
}

func (r *desktopRuntime) isPrimaryWindow(windowID uint64) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	primary := r.primaryWindowID == windowID && (windowID != 0 || len(r.windows) == 0)
	r.mu.Unlock()
	return primary
}

func (r *desktopRuntime) hasWindows() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	hasWindows := len(r.windows) > 0
	r.mu.Unlock()
	return hasWindows
}

func (r *desktopRuntime) viewsForSession(documentID string) []*App {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	apps := make([]*App, 0, len(r.windows))
	for _, app := range r.windows {
		if app != nil {
			apps = append(apps, app)
		}
	}
	r.mu.Unlock()
	views := make([]*App, 0, len(apps))
	for _, app := range apps {
		if app.activeSessionID() == documentID {
			views = append(views, app)
		}
	}
	return views
}

func (r *desktopRuntime) windowCount() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	count := len(r.windows)
	r.mu.Unlock()
	return count
}

func (r *desktopRuntime) hasWindow(windowID uint64) bool {
	if r == nil || windowID == 0 {
		return false
	}
	r.mu.Lock()
	_, exists := r.windows[windowID]
	r.mu.Unlock()
	return exists
}

// glanceWindowLocked selects a regular Glance window, never the independent
// proofreading module. The primary window may belong to either kind after a
// close, so it cannot alone identify the Glance workbench. Caller holds r.mu.
func (r *desktopRuntime) glanceWindowLocked() *App {
	if primary := r.windows[r.primaryWindowID]; primary != nil && !primary.proofreadingModule && !r.closingWindows[primary.windowID] {
		return primary
	}
	var candidate *App
	var oldest uint64
	for id, window := range r.windows {
		if window == nil || window.proofreadingModule {
			continue
		}
		order := r.windowAttachmentOrder[id]
		if candidate == nil || (r.closingWindows[candidate.windowID] && !r.closingWindows[id]) ||
			(r.closingWindows[candidate.windowID] == r.closingWindows[id] && order < oldest) {
			candidate, oldest = window, order
		}
	}
	return candidate
}

// Only a regular Glance workbench may remain hidden in the tray. A module
// close must retire its native loop and release manuscript resources.
func (r *desktopRuntime) canHideGlanceWindowToTray(a *App) bool {
	if a == nil || a.proofreadingModule {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.trayReady && !r.shuttingDown && !r.quitRequested &&
		r.primaryWindowID == a.windowID && r.windows[a.windowID] == a &&
		!r.closingWindows[a.windowID] && len(r.windows) == 1
}
