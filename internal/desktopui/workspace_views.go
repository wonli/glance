package desktopui

import (
	"path/filepath"
	"strings"

	"glance/internal/workspace"
)

// reconcileWorkspaceTabs refreshes this window's lightweight catalog from the
// process-wide collection and drops views for tabs closed in another window.
func (a *App) reconcileWorkspaceTabs() {
	if a == nil || a.collection == nil {
		return
	}
	records := a.collection.Tabs()
	current := make(map[string]workspace.Tab, len(records))
	for _, record := range records {
		current[record.Tab.ID] = record.Tab
	}
	for id, meta := range a.tabMeta {
		if _, exists := current[id]; exists {
			continue
		}
		// Legacy XLS remains an explicitly local, read-only preview and has no
		// durable workspace record.
		if strings.EqualFold(filepath.Ext(meta.SourcePath), ".xls") {
			current[id] = meta
			continue
		}
		if state := a.sessions[id]; state != nil {
			state.closed.Store(true)
		}
		delete(a.sessions, id)
		delete(a.tabButtons, id)
		delete(a.tabCloseButtons, id)
	}
	a.tabMeta = current
	if a.activeTabID != "" && a.activeTabID != workspace.RecentTabID {
		if _, exists := current[a.activeTabID]; !exists {
			a.activeTabID = ""
			a.setActiveSessionID("")
			a.activateRecent()
		}
	}
}

// viewForTab attaches this window to the shared document session and creates
// its own persistent Gio widgets on first use.
func (a *App) viewForTab(id string) *documentSessionState {
	if a == nil {
		return nil
	}
	if state := a.sessions[id]; state != nil && !state.closed.Load() {
		return state
	}
	if a.collection == nil {
		return nil
	}
	record, ok := a.collection.Get(id)
	if !ok {
		return nil
	}
	runtime := a.ensureDesktopRuntime()
	shared, found := runtime.session(record.Tab.DocumentID)
	if !found {
		state, err := a.restoreSessionState(*record)
		if err != nil {
			a.status = "恢复 " + record.Tab.Title + " 失败：" + err.Error()
			return nil
		}
		state.documentID = record.Tab.DocumentID
		registered := runtime.registerSession(state.sharedDocumentSession)
		if registered == state.sharedDocumentSession {
			state.windowID = a.windowID
			state.sharedDocumentSession.attachView(a.windowID, state)
			a.sessions[id] = state
			return state
		}
		closeUnregisteredSessionAdapters(state.sharedDocumentSession, registered)
		shared = registered
	}
	state := newDocumentSessionView(shared, a.windowID)
	state.documentID = record.Tab.DocumentID
	a.sessions[id] = state
	return state
}
