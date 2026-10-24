package desktopui

import (
	"strings"

	"glance/internal/application"
	"glance/internal/diagramrender"
	"glance/internal/i18n"
	"glance/internal/workspace"
)

func (a *App) activateRecent() {
	if a.selectedProofreadingTabID != "" && a.proofreadingWorkbench != nil && !a.proofreadingWorkbench.settings {
		a.proofreadingWorkbench.loaded = false
	}
	a.selectedProofreadingTabID = ""
	a.invalidatePDFWorkspaceRestore()
	runtime := a.ensureDesktopRuntime()
	a.persistActiveSession()
	runtime.drainWorkspaceWriter()
	if a.activeTabID != "" && a.activeTabID != workspace.RecentTabID && a.documentSessionState != nil {
		if a.sessions == nil {
			a.sessions = make(map[string]*documentSessionState)
		}
		a.sessions[a.activeTabID] = a.documentSessionState
	}
	a.activeTabID = workspace.RecentTabID
	a.setActiveSessionID("")
	if a.collection != nil && runtime.isPrimaryWindow(a.windowID) {
		_ = a.collection.Activate(workspace.RecentTabID)
	}
	a.documentSessionState = newDocumentSessionState("")
	a.evictInactiveSessions()
	a.releaseIdleEngines()
	requestMemoryRelease()
	a.setStatusMessage(i18n.Message{ID: "workspace.status.recent_documents"}, feedbackInfo, notificationSuppressed)
}

func (a *App) evictInactiveSessionsExcept(keepTabID string, alsoKeep ...string) {
	if a == nil || a.collection == nil {
		return
	}
	runtime := a.ensureDesktopRuntime()
	runtime.drainWorkspaceWriter()
	keepSet := make(map[string]bool)
	if keepTabID != "" && keepTabID != workspace.RecentTabID {
		keepSet[keepTabID] = true
	}
	if a.activeTabID != "" && a.activeTabID != workspace.RecentTabID {
		keepSet[a.activeTabID] = true
	}
	for _, id := range alsoKeep {
		if id != "" && id != workspace.RecentTabID {
			keepSet[id] = true
		}
	}
	for id, state := range a.sessions {
		if id == "" || id == workspace.RecentTabID || state == nil {
			continue
		}
		if keepSet[id] {
			continue
		}
		if runtime.isSessionActiveInAnyWindow(state.documentID) {
			continue
		}
		state.operationMu.Lock()
		busy := state.operationActive
		state.operationMu.Unlock()
		if busy {
			continue
		}
		state.releaseViewResources()
		delete(a.sessions, id)
		runtime.removeSession(state.documentID)
	}
	if a.collection != nil {
		activeDocID := keepTabID
		if activeDocID == "" && a.activeTabID != "" && a.activeTabID != workspace.RecentTabID {
			activeDocID = a.activeTabID
		}
		a.collection.ReleaseInactivePayloads(activeDocID)
	}
	a.releaseIdleEngines()
	requestMemoryRelease()
}

func (a *App) evictInactiveSessions() {
	a.evictInactiveSessionsExcept("")
}

func (a *App) closeWorkspaceTab(id string) {
	if a.proofreadingTab(id) != nil {
		a.closeProofreadingTab(id)
		return
	}
	if a.pendingPDFRestoreTabID == id {
		a.invalidatePDFWorkspaceRestore()
	}
	if id == a.activeTabID {
		a.flushMarkdownDraft()
	}
	a.ensureDesktopRuntime()
	if a.modal.Visible {
		return
	}
	if a.collection != nil {
		state := a.sessions[id]
		if state == nil || state.closed.Load() {
			if record, ok := a.collection.GetMetadata(id); ok && strings.EqualFold(record.Format, "pdf") {
				if _, shared := a.ensureDesktopRuntime().session(record.Tab.DocumentID); !shared {
					if record.Dirty || record.Tab.SourcePath == "" {
						title := i18n.Message{ID: "workspace.close.confirm.title_changes"}
						message := i18n.Message{ID: "workspace.close.confirm.body_changes"}
						if record.Tab.SourcePath == "" {
							title = i18n.Message{ID: "workspace.close.confirm.title_new"}
							message = i18n.Message{ID: "workspace.close.confirm.body_new"}
						}
						a.showCloseConfirmationMessage(id, title, message)
						return
					}
					a.closeUnopenedPDFTabNow(id, *record)
					return
				}
			}
		}
	}
	state, err := a.viewForTabWithError(id)
	if err != nil {
		a.closeWorkspaceTabWithoutSession(id, err)
		return
	}
	if sessionPath(state) == "" || sessionDirty(state) || markdownHasPendingDraft(state) {
		title := i18n.Message{ID: "workspace.close.confirm.title_changes"}
		message := i18n.Message{ID: "workspace.close.confirm.body_changes"}
		if sessionPath(state) == "" {
			title = i18n.Message{ID: "workspace.close.confirm.title_new"}
			message = i18n.Message{ID: "workspace.close.confirm.body_new"}
		}
		a.showCloseConfirmationMessage(id, title, message)
		return
	}
	a.closeWorkspaceTabNow(id)
}

func (a *App) showCloseConfirmationMessage(id string, title, message i18n.Message) {
	a.pendingCloseTabID = id
	a.modal.ShowConfirmMessage("workspace.close.confirm", title, message, modalDialogSaveConfirmStyle, a.handleCloseConfirmation,
		modalDialogActionSpec{ID: "save", LabelRef: i18n.Message{ID: "common.action.save"}, Style: modalDialogActionPrimary},
		modalDialogActionSpec{ID: "discard", LabelRef: i18n.Message{ID: "workspace.close.action.discard"}, Style: modalDialogActionDanger},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}, Style: modalDialogActionSecondary},
	)
}

func (a *App) closeWorkspaceTabNow(id string) {
	runtime := a.ensureDesktopRuntime()
	if state := a.sessions[id]; state == nil || state.closed.Load() {
		if a.collection != nil {
			if record, ok := a.collection.GetMetadata(id); ok && strings.EqualFold(record.Format, "pdf") {
				if _, shared := runtime.session(record.Tab.DocumentID); !shared {
					a.closeUnopenedPDFTabNow(id, *record)
					return
				}
			}
		}
	}
	state, err := a.viewForTabWithError(id)
	if err != nil {
		a.closeWorkspaceTabWithoutSession(id, err)
		return
	}
	if !state.finalizeIfIdleOrSnapshot() {
		state.operationMu.Lock()
		operationActive := state.operationActive
		operationKind := state.activeOperation.kind
		finalized := state.finalized.Load()
		state.operationMu.Unlock()
		if operationActive && !finalized {
			a.showWorkspaceTabCloseFailure(i18n.Message{ID: "workspace.close.error.operation_in_progress", Args: i18n.Args{
				"Operation": a.text(workspaceSessionOperationMessage(operationKind)),
			}}, nil)
			return
		}
		a.showWorkspaceTabCloseFailure(i18n.Message{ID: "workspace.close.error.session_ended"}, nil)
		return
	}
	state.wordExportCommitMu.Lock()
	state.closed.Store(true)
	state.wordExportCommitMu.Unlock()
	if a.collection != nil {
		if _, persisted := a.collection.GetMetadata(id); persisted {
			a.workspaceDBMu.Lock()
			if err := a.collection.CloseTab(id); err != nil {
				a.workspaceDBMu.Unlock()
				state.reopenAfterFailedClose()
				state.wordExportCommitMu.Lock()
				state.closed.Store(false)
				state.wordExportCommitMu.Unlock()
				a.showWorkspaceTabCloseFailure(i18n.Message{ID: "workspace.close.error.remove_failed"}, err)
				return
			}
			a.workspaceDBMu.Unlock()
		}
	}
	runtime.removeSession(state.documentID)
	// Unregistered/headless views are not in the runtime's window map.
	// The closing tab must release its own resources in either case.
	state.releaseViewResources()
	delete(a.sessions, id)
	delete(a.tabMeta, id)
	delete(a.tabButtons, id)
	delete(a.tabCloseButtons, id)
	a.workspaceTabsListPendingReset = true
	a.drainGridLoadResults()
	if a.activeTabID == id {
		a.activeTabID = ""
		a.setActiveSessionID("")
		a.activateRecent()
	}
	a.reconcileWorkspaceTabs()
	a.releaseIdleEngines()
	requestMemoryRelease()
	// The current frame's tab list was built before this close removed its ID.
	// Schedule a fresh frame so the stale list snapshot is replaced immediately.
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) closeUnopenedPDFTabNow(id string, record workspace.Session) {
	runtime := a.ensureDesktopRuntime()
	if a.pendingPDFRestoreTabID == id {
		a.invalidatePDFWorkspaceRestore()
	}
	documentID := record.Tab.DocumentID
	var closeErr error
	var snapshotCleanupErr error
	if a.collection != nil {
		if latest, exists := a.collection.GetMetadata(id); exists {
			if latest.Tab.DocumentID != "" {
				documentID = latest.Tab.DocumentID
			}
			a.workspaceDBMu.Lock()
			closeErr = a.collection.CloseTab(id)
			a.workspaceDBMu.Unlock()
			if _, stillOpen := a.collection.GetMetadata(id); !stillOpen {
				snapshotCleanupErr = closeErr
				closeErr = nil
			}
		}
	}
	if closeErr != nil {
		a.showWorkspaceTabCloseFailure(i18n.Message{ID: "workspace.close.error.remove_failed"}, closeErr)
		return
	}
	runtime.removeSession(documentID)
	delete(a.sessions, id)
	delete(a.tabMeta, id)
	delete(a.tabButtons, id)
	delete(a.tabCloseButtons, id)
	a.workspaceTabsListPendingReset = true
	if a.activeTabID == id {
		a.activeTabID = ""
		a.setActiveSessionID("")
		a.activateRecent()
	}
	a.reconcileWorkspaceTabs()
	a.releaseIdleEngines()
	requestMemoryRelease()
	if a.window != nil {
		a.window.Invalidate()
	}
	if snapshotCleanupErr != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "workspace.close.error.snapshot_cleanup"}, snapshotCleanupErr, notificationToast)
		a.modal.ShowMessage("workspace.close.snapshot_cleanup",
			i18n.Message{ID: "workspace.close.error.title"},
			i18n.Message{ID: "workspace.close.error.snapshot_cleanup"}, func(string) {
				a.modal.Hide()
			}, modalDialogActionSpec{ID: "ok", LabelRef: i18n.Message{ID: "common.action.ok"}})
	}
}

func (a *App) hasActiveWordDocument() bool {
	return a.activeTabID != "" && a.activeTabID != workspace.RecentTabID && a.word != nil
}

func (a *App) hasActiveMarkdownDocument() bool {
	if a.markdownDoc != nil {
		return true
	}
	for _, state := range a.sessions {
		if state != nil && !state.closed.Load() && state.markdownDoc != nil {
			return true
		}
	}
	return false
}

func (a *App) hasActiveDocument() bool {
	if a.activeTabID != "" && a.activeTabID != workspace.RecentTabID {
		return true
	}
	for id, s := range a.sessions {
		if id != "" && id != workspace.RecentTabID && s != nil && !s.closed.Load() {
			return true
		}
	}
	return false
}

func (a *App) releaseIdleEngines() {
	if !a.hasActiveWordDocument() {
		a.wordLayoutEngine = nil
		a.wordLayoutEngineFontVersion = 0
		a.wordPDFFontSource = nil
	}
	if !a.hasActiveMarkdownDocument() {
		a.launchManagedBackground(diagramrender.IdleCleanup())
	}
}

// closeWorkspaceTabWithoutSession removes an unrestorable tab from the durable
// workspace directly. A document view is not required to discard its corrupt
// snapshot and close the tab.
func (a *App) closeWorkspaceTabWithoutSession(id string, cause error) {
	documentID := id
	if meta, ok := a.tabMeta[id]; ok && meta.DocumentID != "" {
		documentID = meta.DocumentID
	}
	var closeErr error
	var snapshotCleanupErr error
	if a.collection != nil {
		if record, exists := a.collection.GetMetadata(id); exists {
			if record.Tab.DocumentID != "" {
				documentID = record.Tab.DocumentID
			}
			a.workspaceDBMu.Lock()
			closeErr = a.collection.CloseTab(id)
			a.workspaceDBMu.Unlock()
			// CloseTab persists tab removal before deleting its snapshot. If the
			// tab is gone, close succeeded even if snapshot cleanup needs attention.
			if _, stillOpen := a.collection.GetMetadata(id); !stillOpen {
				snapshotCleanupErr = closeErr
				closeErr = nil
			}
		}
	}
	if closeErr != nil {
		a.showWorkspaceTabCloseFailure(i18n.Message{ID: "workspace.close.error.corrupt_record"}, closeErr)
		return
	}
	a.ensureDesktopRuntime().removeSession(documentID)
	delete(a.sessions, id)
	delete(a.tabMeta, id)
	delete(a.tabButtons, id)
	delete(a.tabCloseButtons, id)
	if a.activeTabID == id {
		a.activeTabID = ""
		a.setActiveSessionID("")
	}
	a.reconcileWorkspaceTabs()
	if a.activeTabID == "" {
		a.activateRecent()
	}
	a.setStatusErrorMessage(i18n.Message{ID: "workspace.close.error.unrecoverable"}, cause, notificationToast)
	if a.window != nil {
		a.window.Invalidate()
	}
	if snapshotCleanupErr != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "workspace.close.error.snapshot_cleanup"}, snapshotCleanupErr, notificationToast)
		a.modal.ShowMessage("workspace.close.snapshot_cleanup",
			i18n.Message{ID: "workspace.close.error.title"},
			i18n.Message{ID: "workspace.close.error.snapshot_cleanup"}, func(string) {
				a.modal.Hide()
			}, modalDialogActionSpec{ID: "ok", LabelRef: i18n.Message{ID: "common.action.ok"}})
	}
}

func workspaceSessionOperationMessage(kind sessionOperationKind) i18n.Message {
	switch kind {
	case sessionOperationRead:
		return i18n.Message{ID: "workspace.operation.read"}
	case sessionOperationMutation:
		return i18n.Message{ID: "workspace.operation.edit"}
	case sessionOperationSave:
		return i18n.Message{ID: "workspace.operation.save"}
	case sessionOperationSnapshot:
		return i18n.Message{ID: "workspace.operation.snapshot"}
	default:
		return i18n.Message{ID: "workspace.operation.document"}
	}
}

func (a *App) showWorkspaceTabCloseFailure(message i18n.Message, source error) {
	a.setStatusErrorMessage(message, source, notificationToast)
	a.modal.ShowMessage("workspace.close.error",
		i18n.Message{ID: "workspace.close.error.title"}, message, func(string) {
			a.modal.Hide()
		}, modalDialogActionSpec{ID: "ok", LabelRef: i18n.Message{ID: "common.action.ok"}})
}

func (a *App) handleCloseConfirmation(action string) {
	id := a.pendingCloseTabID
	if a.isSaving() && action != "cancel" {
		a.showWorkspaceTabCloseFailure(i18n.Message{ID: "workspace.close.error.saving"}, nil)
		return
	}
	switch action {
	case "cancel":
		a.modal.Hide()
		a.pendingCloseTabID = ""
		a.pendingPDFRestoreSaveOnReady = false
	case "discard":
		a.modal.Hide()
		a.pendingCloseTabID = ""
		a.closeWorkspaceTabNow(id)
	case "save":
		state := a.sessions[id]
		if state == nil {
			if a.collection != nil {
				if record, exists := a.collection.GetMetadata(id); exists && strings.EqualFold(record.Format, "pdf") {
					a.modal.Hide()
					if a.startWorkspacePDFRestore(*record) {
						a.pendingCloseTabID = id
						a.pendingPDFRestoreSaveOnReady = true
						return
					}
					a.pendingCloseTabID = ""
				}
			}
			a.modal.Hide()
			a.pendingCloseTabID = ""
			a.showWorkspaceTabCloseFailure(i18n.Message{ID: "workspace.close.error.session_missing"}, nil)
			return
		}
		if a.activeTabID != id {
			a.activateSession(id, state)
		}
		a.modal.Hide()
		if a.path == "" {
			ext := "xlsx"
			if a.word != nil {
				ext = "docx"
			} else if a.pdfDoc != nil {
				a.startFileCommand(commandSaveAs, "pdf")
			} else if a.markdownDoc != nil {
				ext = "md"
			} else if a.jsonDoc != nil {
				ext = "json"
			} else if a.book != nil && state.sourceFormat == application.FormatCSV {
				ext = "csv"
			}
			a.startSaveAsPicker(ext)
			return
		}
		a.startSave()
	}
}

func (a *App) reindexTabMetadata() {
	a.ensureDesktopRuntime()
	if a.collection != nil {
		next := make(map[string]workspace.Tab)
		for _, tab := range a.collection.TabMetadata() {
			next[tab.ID] = tab
		}
		a.tabMeta = next
		return
	}
	for position, id := range a.workspaceTabIDs() {
		if a.proofreadingTab(id) != nil {
			continue
		}
		meta := a.tabMeta[id]
		meta.Position = position
		a.tabMeta[id] = meta
	}
}

func closeSessionState(s *documentSessionState) {
	if s != nil {
		s.releaseViewResources()
		s.closed.Store(true)
		requestMemoryRelease()
	}
}

func (a *App) closeWorkspace() {
	a.invalidatePDFWorkspaceRestore()
	a.discardPDFWorkspaceRestoreResults()
	a.completeQueuedWordSplitsOnClose()
	runtime := a.ensureDesktopRuntime()
	// A promoted window can be destroyed before its next frame. Apply the
	// promotion on this App's own event loop before persisting its final state.
	a.processPrimaryPromotion()
	a.persistActiveSession()
	runtime.drainWorkspaceWriter()
	if runtime.shouldKeepAlive() && runtime.windowCount() == 1 && runtime.isPrimaryWindow(a.windowID) {
		runtime.recordResumeActiveTab(a.activeTabID)
	}
	if a.windowID != 0 {
		runtime.detachWindow(a.windowID)
	} else {
		seen := map[*documentSessionState]bool{}
		for _, state := range a.sessions {
			if state != nil && !seen[state] {
				closeSessionState(state)
				seen[state] = true
			}
		}
		if a.documentSessionState != nil && !seen[a.documentSessionState] {
			closeSessionState(a.documentSessionState)
		}
	}
	runtime.showPendingMainWindow()
	if !runtime.hasWindows() && !runtime.shouldKeepAlive() {
		runtime.shutdown()
	}
}
