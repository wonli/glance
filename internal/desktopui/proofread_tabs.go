package desktopui

import (
	"fmt"
	"gioui.org/io/system"
	"gioui.org/layout"
	"glance/internal/workspace"
)

// A proofreading tab owns its editors, PDF view and asynchronous jobs. The host
// owns the window and the common tab strip, just as it does for document tabs.
type proofreadingTab struct {
	id        string
	editor    *App
	closing   bool
	duplicate bool
}

type proofreadingTabFocusRequest struct {
	id      string
	history *workspace.ProofreadingProject
}

func (a *App) proofreadingTab(id string) *proofreadingTab {
	for _, tab := range a.proofreadingTabs {
		if tab.id == id {
			return tab
		}
	}
	return nil
}

func (a *App) openProofreadingTab(request proofreadingWindowRequest) error {
	if request.sourceSnapshot == nil {
		return fmt.Errorf("无法读取当前 PDF 快照")
	}
	host := a
	if a.proofreadingTabHost != nil {
		host = a.proofreadingTabHost
	}
	r := host.ensureDesktopRuntime()
	if !host.proofreadingModule {
		return r.openProofreadingModuleWindow(request)
	}
	if request.resume != nil {
		r.mu.Lock()
		existing := r.proofTabOwners[request.resume.ID]
		closing := existing != nil && existing.closing
		r.mu.Unlock()
		if existing != nil {
			request.sourceSnapshot.Close()
			if closing {
				return fmt.Errorf("正在保存并关闭校对稿，请稍后重试")
			}
			var history *workspace.ProofreadingProject
			if request.showHistory {
				project := *request.resume
				history = &project
			}
			existing.host.requestProofreadingTabFocus(existing.id, history)
			return nil
		}
	}
	// Construct without a native window: a tab must not register another window,
	// initialize another workspace, or run another native event loop.
	editor := newAppWithRuntimeAndWindowID(nil, "", r, host.windowID)
	editor.window = host.window
	editor.theme = host.theme
	editor.designTokens = host.designTokens
	editor.proofreadingTabHost = host
	editor.proofreading = newProofreadingWorkspace(request)
	host.nextProofreadingTabID++
	id := fmt.Sprintf("proofreading:%d", host.nextProofreadingTabID)
	tab := &proofreadingTab{id: id, editor: editor}
	host.proofreadingTabs = append(host.proofreadingTabs, tab)
	if request.resume != nil {
		owner := host.registerProofreadingTab(tab, request.resume.ID)
		if owner.host != host || owner.id != id {
			host.proofreadingTabs = host.proofreadingTabs[:len(host.proofreadingTabs)-1]
			request.sourceSnapshot.Close()
			var history *workspace.ProofreadingProject
			if request.showHistory {
				project := *request.resume
				history = &project
			}
			owner.host.requestProofreadingTabFocus(owner.id, history)
			return nil
		}
	}
	host.selectedProofreadingTabID = id
	host.showNewMenu = false
	if request.showHistory && request.resume != nil {
		editor.openProofreadingHistory(*request.resume, 0)
	}
	editor.startProofreadingSourceSnapshot(request.sourceSnapshot)
	if host.window != nil {
		host.window.Invalidate()
	}
	return nil
}

func (a *App) requestProofreadingTabFocus(id string, history *workspace.ProofreadingProject) {
	// Called on other windows' UI threads as well. Only the owning event loop
	// changes selection; the request channel crosses the window boundary.
	select {
	case a.proofreadingTabFocus <- proofreadingTabFocusRequest{id: id, history: history}:
	default:
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) processProofreadingTabs() {
	if a.proofreadingModule {
		select {
		case request := <-a.proofreadingModuleRequests:
			if a.window != nil {
				a.window.Perform(system.ActionRaise)
			}
			if err := a.openProofreadingTab(request); err != nil {
				a.ensureProofreadingWorkbench().message = err.Error()
			}
		default:
		}
	}
	select {
	case request := <-a.proofreadingTabFocus:
		if tab := a.proofreadingTab(request.id); tab != nil {
			if a.window != nil {
				a.window.Perform(system.ActionRaise)
			}
			a.selectedProofreadingTabID = request.id
			w := tab.editor.ensureProofreadingWorkbench()
			w.active = false
			w.settings = false
			w.historyRequest++
			w.historyOpen = false
			w.historyLoading = false
			if request.history != nil {
				tab.editor.openProofreadingHistory(*request.history, 0)
			}
		}
	default:
	}
	for _, tab := range a.proofreadingTabs {
		editor := tab.editor
		editor.processAsyncResults()
		editor.processProofreadingWorkbench()
		editor.processProofreadingResults()
		editor.acceptProofreadingConfirmation()
		editor.acceptProofreadingModelLayout()
	}
	for _, tab := range append([]*proofreadingTab(nil), a.proofreadingTabs...) {
		if tab.duplicate {
			a.closeProofreadingTab(tab.id)
		}
	}
}

func (a *App) layoutProofreadingTab(g layout.Context, tab *proofreadingTab) layout.Dimensions {
	editor := tab.editor
	editor.theme = a.theme
	editor.designTokens = a.designTokens
	editor.handleOverlayEscape(g)
	if editor.ensureProofreadingWorkbench().active {
		return editor.layoutProofreadingWorkbench(g)
	}
	return editor.layoutProofreadingWorkspace(g)
}

func (a *App) closeProofreadingTab(id string) {
	tab := a.proofreadingTab(id)
	if tab == nil || tab.closing {
		return
	}
	tab.closing = true
	r := a.ensureDesktopRuntime()
	r.mu.Lock()
	for _, owner := range r.proofTabOwners {
		if owner.host == a && owner.id == id {
			owner.closing = true
		}
	}
	r.mu.Unlock()
	editor := tab.editor
	// Capture the final draft on the UI thread. Joining workers and closing PDF
	// adapters happen in the background, outside layout/frame processing.
	editor.queueProofreadingDraftPersistence()
	if editor.proofreading.modelLayoutCancel != nil {
		editor.proofreading.modelLayoutCancel()
	}
	if editor.proofreading.ocrScan != nil {
		editor.proofreading.ocrScan.cancelTask()
	}
	editor.shutdownOnce.Do(func() { close(editor.shutdown) })
	a.launchManagedBackground(func() {
		editor.stopAndWaitManagedBackground()
		editor.closeProofreadingDraft()
		r := a.ensureDesktopRuntime()
		r.mu.Lock()
		for project, owner := range r.proofTabOwners {
			if owner.host == a && owner.id == id {
				delete(r.proofTabOwners, project)
			}
		}
		r.mu.Unlock()
	})
	for i, candidate := range a.proofreadingTabs {
		if candidate == tab {
			a.proofreadingTabs = append(a.proofreadingTabs[:i], a.proofreadingTabs[i+1:]...)
			break
		}
	}
	delete(a.tabButtons, id)
	delete(a.tabCloseButtons, id)
	if a.selectedProofreadingTabID == id {
		a.selectedProofreadingTabID = ""
		// Closing the selected manuscript returns to the fixed workbench tab.
		a.activateRecent()
		a.ensureProofreadingWorkbench().loaded = false
	}
}

func (a *App) closeAllProofreadingTabs() {
	a.selectedProofreadingTabID = ""
	for len(a.proofreadingTabs) > 0 {
		a.closeProofreadingTab(a.proofreadingTabs[0].id)
	}
}

func (a *App) layoutProofreadingTabChrome(g layout.Context) layout.Dimensions {
	if a.proofreadingTabHost != nil {
		return layout.Dimensions{}
	}
	return a.layoutProofreadingChrome(g)
}
