package desktopui

import (
	"glance/internal/application"
	"glance/internal/workspace"
)
func (a *App) activateRecent() {
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
	a.setStatus("最近打开的本地文档")
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
	if id == a.activeTabID {
		a.flushMarkdownDraft()
	}
	a.ensureDesktopRuntime()
	if a.modal.Visible {
		return
	}
	state, err := a.viewForTabWithError(id)
	if err != nil {
		a.closeWorkspaceTabWithoutSession(id, err)
		return
	}
	if sessionPath(state) == "" || sessionDirty(state) || markdownHasPendingDraft(state) {
		title, message := "保存更改？", "此文档有尚未保存的修改。关闭前要保存吗？"
		if sessionPath(state) == "" {
			title, message = "保存新文档？", "此新建文档尚未保存到本地目录。关闭后将丢弃草稿；要先保存吗？"
		}
		a.showCloseConfirmation(id, title, message)
		return
	}
	a.closeWorkspaceTabNow(id)
}

func (a *App) showCloseConfirmation(id, title, message string) {
	a.pendingCloseTabID = id
	a.showConfirmDialog(title, message, modalDialogSaveConfirmStyle, a.handleCloseConfirmation,
		modalDialogActionSpec{ID: "save", Label: "保存", Style: modalDialogActionPrimary},
		modalDialogActionSpec{ID: "discard", Label: "不保存", Style: modalDialogActionDanger},
		modalDialogActionSpec{ID: "cancel", Label: "取消", Style: modalDialogActionSecondary},
	)
}

func (a *App) closeWorkspaceTabNow(id string) {
	runtime := a.ensureDesktopRuntime()
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
			a.showWorkspaceTabCloseFailure("当前" + workspaceSessionOperationName(operationKind) + "操作尚未完成，关闭没有执行。请等待操作结束后重试。")
			return
		}
		a.showWorkspaceTabCloseFailure("文档会话正在关闭或已结束，当前标签未关闭。请重试；如果问题持续，请重新打开文档。")
		return
	}
	state.wordExportCommitMu.Lock()
	state.closed.Store(true)
	state.wordExportCommitMu.Unlock()
	if a.collection != nil {
		if _, persisted := a.collection.Get(id); persisted {
			a.workspaceDBMu.Lock()
			if err := a.collection.CloseTab(id); err != nil {
				a.workspaceDBMu.Unlock()
				state.reopenAfterFailedClose()
				state.wordExportCommitMu.Lock()
				state.closed.Store(false)
				state.wordExportCommitMu.Unlock()
				a.showWorkspaceTabCloseFailure("工作区未能移除此标签，文档仍保持打开。\n\n" + err.Error())
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

func (a *App) hasActiveWordDocument() bool {
	return a.activeTabID != "" && a.activeTabID != workspace.RecentTabID && a.word != nil
}

func (a *App) releaseIdleEngines() {
	if !a.hasActiveWordDocument() {
		a.wordLayoutEngine = nil
		a.wordPDFFontSource = nil
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
		if record, exists := a.collection.Get(id); exists {
			if record.Tab.DocumentID != "" {
				documentID = record.Tab.DocumentID
			}
			a.workspaceDBMu.Lock()
			closeErr = a.collection.CloseTab(id)
			a.workspaceDBMu.Unlock()
			// CloseTab persists tab removal before deleting its snapshot. If the
			// tab is gone, close succeeded even if snapshot cleanup needs attention.
			if _, stillOpen := a.collection.Get(id); !stillOpen {
				snapshotCleanupErr = closeErr
				closeErr = nil
			}
		}
	}
	if closeErr != nil {
		a.showWorkspaceTabCloseFailure("删除损坏文档的工作区记录失败，标签仍保持打开。\n\n" + closeErr.Error())
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
	a.setStatus("已关闭无法恢复的文档标签：" + cause.Error())
	if a.window != nil {
		a.window.Invalidate()
	}
	if snapshotCleanupErr != nil {
		message := "标签已从工作区删除，但损坏快照未能清理。\n\n" + snapshotCleanupErr.Error()
		a.showConfirmDialog("损坏快照未能清理", message, modalDialogWarningConfirmStyle, func(string) {
			a.modal.Hide()
		}, modalDialogActionSpec{ID: "ok", Label: "知道了"})
	}
}

func workspaceSessionOperationName(kind sessionOperationKind) string {
	switch kind {
	case sessionOperationRead:
		return "读取"
	case sessionOperationMutation:
		return "编辑"
	case sessionOperationSave:
		return "保存"
	case sessionOperationSnapshot:
		return "版本快照"
	default:
		return "文档处理"
	}
}

func (a *App) showWorkspaceTabCloseFailure(message string) {
	a.setStatus(message)
	a.showConfirmDialog("无法关闭文档标签", message, modalDialogWarningConfirmStyle, func(string) {
		a.modal.Hide()
	}, modalDialogActionSpec{ID: "ok", Label: "知道了"})
}

func (a *App) handleCloseConfirmation(action string) {
	id := a.pendingCloseTabID
	if a.isSaving() && action != "cancel" {
		a.showWorkspaceTabCloseFailure("关闭没有执行：工作区当前有文档正在保存。请等待保存结束后重新关闭。")
		return
	}
	switch action {
	case "cancel":
		a.modal.Hide()
		a.pendingCloseTabID = ""
	case "discard":
		a.modal.Hide()
		a.pendingCloseTabID = ""
		a.closeWorkspaceTabNow(id)
	case "save":
		state := a.sessions[id]
		if state == nil {
			a.modal.Hide()
			a.pendingCloseTabID = ""
			a.showWorkspaceTabCloseFailure("无法找到此文档的会话，保存和关闭均未执行。请重新打开文档。")
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

func (a *App) hasActiveDocument() bool {
	return a.book != nil || a.word != nil || a.jsonDoc != nil || a.markdownDoc != nil || a.pdfDoc != nil
}
