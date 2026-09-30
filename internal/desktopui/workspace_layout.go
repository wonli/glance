package desktopui

import (
	"image"
	"image/color"
	"runtime"
	"sort"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/format/docx"
	"glance/internal/format/jsondoc"
	"glance/internal/format/xlsx"
	"glance/internal/workspace"
)

const (
	workspaceTabTopGap        unit.Dp = 5
	workspaceTabHeight        unit.Dp = 37
	workspaceDocumentTabWidth unit.Dp = 224
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
			if index == 0 {
				return a.layoutRecentTab(gtx)
			}
			if index == len(ids)+1 {
				return a.layoutPlusTab(gtx)
			}
			return a.layoutDocumentTab(gtx, ids[index-1])
		})
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
	return layout.Inset{Left: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return a.paintTab(gtx, &a.newButton, "+", false)
	})
}

func (a *App) layoutCreateMenuPopover(gtx layout.Context) layout.Dimensions {
	left := a.plusMenuOffset(gtx)
	shift := op.Offset(image.Pt(left, gtx.Dp(unit.Dp(7)))).Push(gtx.Ops)
	dims := a.layoutCreateActions(gtx)
	shift.Pop()
	return dims
}

func (a *App) plusMenuOffset(gtx layout.Context) int {
	ids := a.workspaceTabIDs()
	plusIndex := len(ids) + 1
	x := -a.workspaceTabsList.Position.Offset
	for index := a.workspaceTabsList.Position.First; index < plusIndex; index++ {
		if index == 0 {
			x += gtx.Dp(unit.Dp(112))
		} else {
			x += gtx.Dp(workspaceDocumentTabWidth)
		}
	}
	x += gtx.Dp(unit.Dp(28))
	if a.clientTitlebar {
		if runtime.GOOS == "darwin" {
			x += gtx.Dp(unit.Dp(84))
		} else {
			x += gtx.Dp(unit.Dp(114))
		}
	}
	maxX := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(260)) - gtx.Dp(unit.Dp(8))
	if x > maxX {
		x = maxX
	}
	if x < gtx.Dp(unit.Dp(8)) {
		x = gtx.Dp(unit.Dp(8))
	}
	return x
}

func (a *App) workspaceTabIDs() []string {
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
		if state := a.sessions[id]; state != nil {
			a.activateSession(id, state)
		}
	}
	if closeButton.Clicked(gtx) {
		a.closeWorkspaceTab(id)
	}
	label := meta.Title
	if state := a.sessions[id]; state != nil && state.book != nil && state.book.Dirty() || state != nil && state.word != nil && state.word.Dirty() || state != nil && state.jsonDoc != nil && state.jsonDoc.Dirty() {
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
		width := unit.Dp(32)
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
		} else if label == "+" {
			bg = color.NRGBA{R: 231, G: 234, B: 237, A: 255}
		}
		paint.FillShape(gtx.Ops, bg, clip.RRect{Rect: image.Rect(0, 0, dims.Size.X, dims.Size.Y), NW: gtx.Dp(unit.Dp(6)), NE: gtx.Dp(unit.Dp(6))}.Op(gtx.Ops))
		call.Add(gtx.Ops)
		return dims
	})
}

func (a *App) activateRecent() {
	a.persistActiveSession()
	a.activeTabID = workspace.RecentTabID
	if a.collection != nil {
		_ = a.collection.Activate(workspace.RecentTabID)
	}
	a.documentSessionState = newDocumentSessionState("")
	a.status = "最近打开的本地文档"
}

func (a *App) closeWorkspaceTab(id string) {
	if a.modal.Visible {
		return
	}
	state := a.sessions[id]
	if state == nil {
		return
	}
	if state.path == "" || (state.book != nil && state.book.Dirty()) || (state.word != nil && state.word.Dirty()) || (state.jsonDoc != nil && state.jsonDoc.Dirty()) {
		title, message := "保存更改？", "此文档有尚未保存的修改。关闭前要保存吗？"
		if state.path == "" {
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
	state := a.sessions[id]
	if state == nil {
		return
	}
	wasActive := a.activeTabID == id
	state.closed.Store(true)
	if a.collection != nil {
		if _, persisted := a.collection.Get(id); persisted {
			a.workspaceDBMu.Lock()
			if err := a.collection.CloseTab(id); err != nil {
				a.workspaceDBMu.Unlock()
				state.closed.Store(false)
				a.status = err.Error()
				return
			}
			a.workspaceDBMu.Unlock()
		}
	}
	closeSessionState(state)
	delete(a.sessions, id)
	delete(a.tabMeta, id)
	delete(a.tabButtons, id)
	delete(a.tabCloseButtons, id)
	a.reindexTabMetadata()
	if wasActive {
		next := workspace.RecentTabID
		if a.collection != nil {
			next = a.collection.ActiveTabID()
		}
		// The active session has already been removed above. Clear its identity
		// before activation so activateSession cannot reinsert the closed state.
		a.activeTabID = ""
		if s := a.sessions[next]; s != nil {
			a.activateSession(next, s)
		} else {
			a.activateRecent()
		}
	}
	// The current frame's tab list was built before this close removed its ID.
	// Schedule a fresh frame so the stale list snapshot is replaced immediately.
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) handleCloseConfirmation(action string) {
	id := a.pendingCloseTabID
	if a.saving && action != "cancel" {
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
	if a.collection != nil {
		for _, session := range a.collection.Tabs() {
			a.tabMeta[session.Tab.ID] = session.Tab
		}
		return
	}
	for position, id := range a.workspaceTabIDs() {
		meta := a.tabMeta[id]
		meta.Position = position
		a.tabMeta[id] = meta
	}
}

func closeSessionState(s *documentSessionState) {
	if s.book != nil {
		_ = s.book.Close()
	}
	if s.word != nil {
		_ = s.word.Close()
	}
	if s.jsonDoc != nil {
		_ = s.jsonDoc.Close()
	}
}

func (a *App) closeWorkspace() {
	a.persistActiveSession()
	if a.workspaceWriteJobs != nil {
		close(a.workspaceWriteJobs)
		<-a.workspaceWriterDone
	}
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
	if a.workspaceStore != nil {
		_ = a.workspaceStore.Close()
	}
}

func (a *App) layoutCreateActions(gtx layout.Context) layout.Dimensions {
	if a.actionOpenFile.Clicked(gtx) {
		a.showNewMenu = false
		a.startOpenFilePicker()
	}
	if a.actionOpenFolder.Clicked(gtx) {
		a.showNewMenu = false
		a.startFolderPicker()
	}
	if a.actionSave.Clicked(gtx) && !a.saving && a.isDirty() {
		a.showNewMenu = false
		if a.path != "" {
			a.startSave()
		} else {
			a.saveAsFromMenu()
		}
	}
	if a.actionSaveAs.Clicked(gtx) && !a.saving && a.hasActiveDocument() {
		a.showNewMenu = false
		a.saveAsFromMenu()
	}
	if a.actionNewXLSX.Clicked(gtx) {
		a.showNewMenu = false
		book := xlsx.NewBlank()
		_ = a.installDocument(loadedDocument{book: book, sheets: book.SheetNames()})
	}
	if a.actionNewDOCX.Clicked(gtx) {
		a.showNewMenu = false
		word, err := docx.NewBlank()
		if err != nil {
			a.status = err.Error()
		} else {
			_ = a.installDocument(loadedDocument{word: word})
		}
	}
	if a.actionNewJSON.Clicked(gtx) {
		a.showNewMenu = false
		_ = a.installDocument(loadedDocument{jsonDoc: jsondoc.NewBlank()})
	}
	buttons := a.createActionItems()
	width := gtx.Dp(unit.Dp(252))
	heightDp := unit.Dp(14)
	for _, item := range buttons {
		if item.separator {
			heightDp += 9
		} else {
			heightDp += 34
		}
	}
	height := gtx.Dp(heightDp)
	gtx.Constraints.Min = image.Pt(width, height)
	gtx.Constraints.Max = gtx.Constraints.Min
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			bounds := image.Rectangle{Max: gtx.Constraints.Max}
			shadow := clip.UniformRRect(bounds.Add(image.Pt(0, gtx.Dp(unit.Dp(3)))), gtx.Dp(unit.Dp(9)))
			paint.FillShape(gtx.Ops, color.NRGBA{R: 36, G: 43, B: 52, A: 22}, shadow.Op(gtx.Ops))
			panel := clip.UniformRRect(bounds, gtx.Dp(unit.Dp(8)))
			paint.FillShape(gtx.Ops, contentSurface, panel.Op(gtx.Ops))
			paint.FillShape(gtx.Ops, homeBorder, clip.Stroke{Path: panel.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
			return layout.Dimensions{Size: bounds.Max}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(7), Bottom: unit.Dp(7), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return material.List(a.theme, &a.createMenuList).Layout(gtx, len(buttons), func(gtx layout.Context, index int) layout.Dimensions {
					item := buttons[index]
					return createActionRow(gtx, a.theme, item)
				})
			})
		}),
	)
}

func createActionRow(gtx layout.Context, theme *material.Theme, item createActionItem) layout.Dimensions {
	if item.separator {
		height := gtx.Dp(unit.Dp(9))
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lineHeight := gtx.Dp(unit.Dp(1))
			gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = lineHeight, lineHeight
			paint.FillShape(gtx.Ops, homeRowBorder, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, lineHeight)}.Op())
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, lineHeight)}
		})
	}
	height := gtx.Dp(unit.Dp(34))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return item.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				bg := color.NRGBA{A: 0}
				if item.click.Hovered() {
					bg = homeHoverFill
				}
				shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(5)))
				paint.FillShape(gtx.Ops, bg, shape.Op(gtx.Ops))
				return layout.Dimensions{Size: gtx.Constraints.Max}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(9), Right: unit.Dp(9)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return homeIcon(gtx, item.icon, 16, color.NRGBA{R: 110, G: 119, B: 130, A: 255})
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return homeLabelStyle(theme, item.label, 13, homeText, font.Normal).Layout(gtx)
						}),
					)
				})
			},
		)
	})
}

type createActionItem struct {
	click     *widget.Clickable
	label     string
	icon      string
	separator bool
}

func (a *App) createActionItems() []createActionItem {
	buttons := []createActionItem{
		{click: &a.actionOpenFile, label: "打开文件…", icon: "file-text"},
		{click: &a.actionOpenFolder, label: "打开文件夹…", icon: "folder-open"},
	}
	if a.hasActiveDocument() {
		buttons = append(buttons, createActionItem{separator: true})
		if a.isDirty() {
			label := "保存"
			if a.path == "" {
				label = "保存…"
			}
			buttons = append(buttons, createActionItem{click: &a.actionSave, label: label, icon: "save"})
		}
		buttons = append(buttons, createActionItem{click: &a.actionSaveAs, label: "另存为…", icon: "sheet-edit"})
	}
	buttons = append(buttons,
		createActionItem{separator: true},
		createActionItem{click: &a.actionNewXLSX, label: "新建 Excel", icon: "file-spreadsheet"},
		createActionItem{click: &a.actionNewDOCX, label: "新建 Word", icon: "file-text"},
		createActionItem{click: &a.actionNewJSON, label: "新建 JSON", icon: "file-code"},
	)
	return buttons
}

func (a *App) createActionLabels() []string {
	items := a.createActionItems()
	labels := make([]string, 0, len(items))
	for _, item := range items {
		if !item.separator {
			labels = append(labels, item.label)
		}
	}
	return labels
}

func (a *App) hasActiveDocument() bool {
	return a.book != nil || a.word != nil || a.jsonDoc != nil
}

func (a *App) saveAsFromMenu() {
	ext := "xlsx"
	if a.word != nil {
		ext = "docx"
	}
	if a.jsonDoc != nil {
		ext = "json"
	}
	a.startSaveAsPicker(ext)
}
