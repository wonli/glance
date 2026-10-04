package desktopui

import (
	"image"
	"image/color"
	"runtime"
	"sort"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/workspace"
)

const (
	workspaceTabTopGap        unit.Dp = 5
	workspaceTabHeight        unit.Dp = 37
	workspaceDocumentTabGap   unit.Dp = 6
	workspaceDocumentTabWidth unit.Dp = 190
	workspacePlusTabWidth     unit.Dp = 32
	workspaceTabStripSize     unit.Dp = workspaceTabTopGap + workspaceTabHeight
)

func (a *App) layoutWorkspaceTabs(gtx layout.Context) layout.Dimensions {
	ids := a.workspaceTabIDs()
	if a.clientTitlebar {
		a.processWindowControlClicks(gtx)
	}
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = gtx.Dp(workspaceTabStripSize), gtx.Dp(workspaceTabStripSize)
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Max
			paint.FillShape(gtx.Ops, workspaceTabStripSurface, clip.Rect{Max: size}.Op())
			return layout.Dimensions{Size: size}
		},
		func(gtx layout.Context) layout.Dimensions {
			if !a.clientTitlebar {
				return a.layoutWorkspaceTabList(gtx, ids)
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if runtime.GOOS == "darwin" {
						return a.layoutMacWindowControls(gtx)
					}
					return a.layoutStandardWindowControls(gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return a.layoutWorkspaceTabList(gtx, ids)
				}),
				layout.Rigid(a.layoutWindowDragHandle),
			)
		},
	)
}

func (a *App) layoutWorkspaceTabList(gtx layout.Context, ids []string) layout.Dimensions {
	return layout.Inset{Top: workspaceTabTopGap, Left: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// A horizontal list reports the width of its laid-out tabs. Keep its
		// viewport as wide as the tab strip so Background doesn't center that
		// shorter natural width (which shifts the selected tab away from the
		// ribbon seam).
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return a.workspaceTabsList.Layout(gtx, len(ids)+2, func(gtx layout.Context, index int) layout.Dimensions {
			return a.layoutWorkspaceTabListItem(gtx, ids, index)
		})
	})
}

func (a *App) layoutWorkspaceTabListItem(gtx layout.Context, ids []string, index int) layout.Dimensions {
	if index == 0 {
		return a.layoutRecentTab(gtx)
	}
	if index == len(ids)+1 {
		return a.layoutPlusTab(gtx)
	}
	if index < 1 || index > len(ids) {
		return layout.Dimensions{}
	}
	return layout.Inset{Left: workspaceDocumentTabGap}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return a.layoutDocumentTab(gtx, ids[index-1])
	})
}

func (a *App) processWindowControlClicks(gtx layout.Context) {
	if a.window == nil {
		return
	}
	if a.windowCloseButton.Clicked(gtx) {
		a.window.Perform(system.ActionClose)
	}
	if a.windowMinimizeButton.Clicked(gtx) {
		a.window.Perform(system.ActionMinimize)
	}
	if a.windowMaximizeButton.Clicked(gtx) {
		a.window.Perform(windowModeToggleAction(a.windowMode))
	}
}

func (a *App) layoutMacWindowControls(gtx layout.Context) layout.Dimensions {
	return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Right: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return a.layoutTrafficLight(gtx, &a.windowCloseButton, color.NRGBA{R: 255, G: 95, B: 87, A: 255})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Right: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return a.layoutTrafficLight(gtx, &a.windowMinimizeButton, color.NRGBA{R: 254, G: 188, B: 46, A: 255})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layoutTrafficLight(gtx, &a.windowMaximizeButton, color.NRGBA{R: 40, G: 201, B: 64, A: 255})
			}),
		)
	})
}

func (a *App) layoutTrafficLight(gtx layout.Context, click *widget.Clickable, fill color.NRGBA) layout.Dimensions {
	gtx.Constraints.Min = image.Pt(gtx.Dp(unit.Dp(18)), gtx.Dp(unit.Dp(28)))
	gtx.Constraints.Max = gtx.Constraints.Min
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			size := gtx.Dp(unit.Dp(12))
			shape := clip.Ellipse{Max: image.Pt(size, size)}
			paint.FillShape(gtx.Ops, fill, shape.Op(gtx.Ops))
			return layout.Dimensions{Size: image.Pt(size, size)}
		})
	})
}

func (a *App) layoutStandardWindowControls(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutStandardWindowButton(gtx, &a.windowMinimizeButton, "−")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			label := "□"
			if a.windowMode == app.Maximized {
				label = "❐"
			}
			return a.layoutStandardWindowButton(gtx, &a.windowMaximizeButton, label)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutStandardWindowButton(gtx, &a.windowCloseButton, "×")
		}),
	)
}

func (a *App) layoutStandardWindowButton(gtx layout.Context, click *widget.Clickable, label string) layout.Dimensions {
	gtx.Constraints.Min = image.Pt(gtx.Dp(unit.Dp(38)), gtx.Dp(unit.Dp(38)))
	gtx.Constraints.Max = gtx.Constraints.Min
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		style := material.Body1(a.theme, label)
		style.Color = color.NRGBA{R: 52, G: 57, B: 65, A: 255}
		return layout.Center.Layout(gtx, style.Layout)
	})
}

func (a *App) layoutWindowDragHandle(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min = image.Pt(gtx.Dp(unit.Dp(48)), gtx.Dp(unit.Dp(40)))
	gtx.Constraints.Max = gtx.Constraints.Min
	return a.windowDecorations.LayoutMove(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	})
}

func (a *App) layoutRecentTab(gtx layout.Context) layout.Dimensions {
	if a.recentTabButton.Clicked(gtx) {
		a.persistActiveSession()
		a.activateRecent()
	}
	return a.paintTab(gtx, &a.recentTabButton, "⌂  Glance", a.activeTabID == workspace.RecentTabID)
}

func (a *App) layoutPlusTab(gtx layout.Context) layout.Dimensions {
	if a.newButton.Clicked(gtx) {
		a.showNewMenu = !a.showNewMenu
	}
	return layout.Inset{Left: workspaceDocumentTabGap}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return a.paintTab(gtx, &a.newButton, "+", a.showNewMenu)
	})
}

func (a *App) workspaceTabIDs() []string {
	a.ensureDesktopRuntime()
	a.reconcileWorkspaceTabs()
	ids := make([]string, 0, len(a.tabMeta))
	for id := range a.tabMeta {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		left, right := a.tabMeta[ids[i]].Position, a.tabMeta[ids[j]].Position
		if left == right {
			return ids[i] < ids[j]
		}
		return left < right
	})
	return ids
}

func (a *App) layoutDocumentTab(gtx layout.Context, id string) layout.Dimensions {
	meta := a.tabMeta[id]
	tabButton, closeButton := a.documentTabWidgets(id)
	if tabButton.Clicked(gtx) {
		a.persistActiveSession()
		if state := a.viewForTab(id); state != nil {
			a.activateSession(id, state)
		}
	}
	if closeButton.Clicked(gtx) {
		a.closeWorkspaceTab(id)
	}
	label := meta.Title
	if state := a.sessions[id]; state != nil && sessionDirty(state) {
		label += " •"
	}
	// Keep this frame's widgets alive even if the close click removed the
	// tab and its widget map entries above.
	return a.paintDocumentTab(gtx, tabButton, closeButton, label, id == a.activeTabID)
}

func (a *App) documentTabWidgets(id string) (*widget.Clickable, *widget.Clickable) {
	if a.tabButtons[id] == nil {
		a.tabButtons[id] = new(widget.Clickable)
	}
	if a.tabCloseButtons[id] == nil {
		a.tabCloseButtons[id] = new(widget.Clickable)
	}
	return a.tabButtons[id], a.tabCloseButtons[id]
}

func (a *App) paintDocumentTab(gtx layout.Context, tab, close *widget.Clickable, label string, active bool) layout.Dimensions {
	width := gtx.Dp(workspaceDocumentTabWidth)
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = gtx.Dp(workspaceTabHeight), gtx.Dp(workspaceTabHeight)
	child := op.Record(gtx.Ops)
	dims := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return tab.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					style := material.Body2(a.theme, label)
					style.TextSize = unit.Sp(12)
					style.MaxLines = 1
					style.Color = color.NRGBA{R: 93, G: 102, B: 113, A: 255}
					if active {
						style.Color = color.NRGBA{R: 52, G: 95, B: 150, A: 255}
					}
					return layout.Center.Layout(gtx, style.Layout)
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return close.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(5), Right: unit.Dp(9)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					style := material.Body2(a.theme, "×")
					style.TextSize = unit.Sp(18)
					style.Color = color.NRGBA{R: 104, G: 113, B: 123, A: 255}
					return layout.Center.Layout(gtx, style.Layout)
				})
			})
		}),
	)
	dims.Size.X = width
	dims.Size.Y = gtx.Constraints.Max.Y
	call := child.Stop()
	bg := workspaceTabStripSurface
	if active {
		bg = contentSurface
	}
	paint.FillShape(gtx.Ops, bg, clip.RRect{Rect: image.Rect(0, 0, dims.Size.X, dims.Size.Y), NW: gtx.Dp(unit.Dp(6)), NE: gtx.Dp(unit.Dp(6))}.Op(gtx.Ops))
	call.Add(gtx.Ops)
	return dims
}

func (a *App) paintTab(gtx layout.Context, button *widget.Clickable, label string, active bool) layout.Dimensions {
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		width := workspacePlusTabWidth
		if label != "+" {
			width = unit.Dp(112)
		}
		fixedWidth := gtx.Dp(width)
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = fixedWidth, fixedWidth
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = gtx.Dp(workspaceTabHeight), gtx.Dp(workspaceTabHeight)
		child := op.Record(gtx.Ops)
		var dims layout.Dimensions
		if label == "+" {
			style := material.Body2(a.theme, label)
			style.TextSize = unit.Sp(18)
			style.Color = color.NRGBA{R: 88, G: 97, B: 108, A: 255}
			dims = layout.Center.Layout(gtx, style.Layout)
		} else {
			dims = layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				gtx.Constraints.Min.Y = 0
				return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							iconColor := color.NRGBA{R: 105, G: 114, B: 125, A: 255}
							if active {
								iconColor = color.NRGBA{R: 52, G: 95, B: 150, A: 255}
							}
							icon, _ := tintedToolbarIcon("house", iconColor)
							if icon == nil {
								return layout.Dimensions{}
							}
							iconSize := gtx.Dp(unit.Dp(18))
							gtx.Constraints.Min = image.Pt(iconSize, iconSize)
							gtx.Constraints.Max = gtx.Constraints.Min
							shift := op.Offset(image.Pt(0, -gtx.Dp(unit.Dp(3)))).Push(gtx.Ops)
							dims := widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: 18.0 / 96.0}.Layout(gtx)
							shift.Pop()
							return dims
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(8)), 0)}
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							style := material.Body2(a.theme, "Glance")
							style.TextSize = unit.Sp(13)
							style.Color = color.NRGBA{R: 104, G: 113, B: 123, A: 255}
							if active {
								style.Color = color.NRGBA{R: 52, G: 95, B: 150, A: 255}
							}
							return style.Layout(gtx)
						}),
					)
				})
			})
		}
		dims.Size.Y = gtx.Constraints.Max.Y
		dims.Size.X = fixedWidth
		call := child.Stop()
		bg := tabSurface
		if active {
			bg = contentSurface
		}
		if label != "+" || active {
			paint.FillShape(gtx.Ops, bg, clip.RRect{Rect: image.Rect(0, 0, dims.Size.X, dims.Size.Y), NW: gtx.Dp(unit.Dp(6)), NE: gtx.Dp(unit.Dp(6))}.Op(gtx.Ops))
		}
		call.Add(gtx.Ops)
		return dims
	})
}

func (a *App) activateRecent() {
	a.ensureDesktopRuntime()
	a.persistActiveSession()
	if a.activeTabID != "" && a.activeTabID != workspace.RecentTabID && a.documentSessionState != nil {
		if a.sessions == nil {
			a.sessions = make(map[string]*documentSessionState)
		}
		a.sessions[a.activeTabID] = a.documentSessionState
	}
	a.activeTabID = workspace.RecentTabID
	a.setActiveSessionID("")
	if a.collection != nil && a.ensureDesktopRuntime().isPrimaryWindow(a.windowID) {
		_ = a.collection.Activate(workspace.RecentTabID)
	}
	a.documentSessionState = newDocumentSessionState("")
	a.status = "最近打开的本地文档"
}

func (a *App) closeWorkspaceTab(id string) {
	a.ensureDesktopRuntime()
	if a.modal.Visible {
		return
	}
	state, err := a.viewForTabWithError(id)
	if err != nil {
		a.closeWorkspaceTabWithoutSession(id, err)
		return
	}
	if sessionPath(state) == "" || sessionDirty(state) {
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
	a.showModalDialog(title, message, a.handleCloseConfirmation,
		modalDialogActionSpec{ID: "save", Label: "保存"},
		modalDialogActionSpec{ID: "discard", Label: "不保存"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) closeWorkspaceTabNow(id string) {
	runtime := a.ensureDesktopRuntime()
	state, err := a.viewForTabWithError(id)
	if err != nil {
		a.closeWorkspaceTabWithoutSession(id, err)
		return
	}
	if !state.finalizeIfIdle() {
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
	delete(a.sessions, id)
	delete(a.tabMeta, id)
	delete(a.tabButtons, id)
	delete(a.tabCloseButtons, id)
	a.reconcileWorkspaceTabs()
	// The current frame's tab list was built before this close removed its ID.
	// Schedule a fresh frame so the stale list snapshot is replaced immediately.
	if a.window != nil {
		a.window.Invalidate()
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
	a.status = "已关闭无法恢复的文档标签：" + cause.Error()
	if a.window != nil {
		a.window.Invalidate()
	}
	if snapshotCleanupErr != nil {
		message := "标签已从工作区删除，但损坏快照未能清理。\n\n" + snapshotCleanupErr.Error()
		a.showModalDialog("损坏快照未能清理", message, func(string) {
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
	a.status = message
	a.showModalDialog("无法关闭文档标签", message, func(string) {
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
			} else if a.jsonDoc != nil {
				ext = "json"
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
		for _, session := range a.collection.Tabs() {
			next[session.Tab.ID] = session.Tab
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
		s.closed.Store(true)
	}
}

func (a *App) closeWorkspace() {
	runtime := a.ensureDesktopRuntime()
	// A promoted window can be destroyed before its next frame. Apply the
	// promotion on this App's own event loop before persisting its final state.
	a.processPrimaryPromotion()
	a.persistActiveSession()
	runtime.drainWorkspaceWriter()
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
	if !runtime.hasWindows() {
		runtime.shutdown()
	}
}

func (a *App) hasActiveDocument() bool {
	return a.book != nil || a.word != nil || a.jsonDoc != nil
}
