package desktopui

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
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

	"glance/internal/format/docx"
	"glance/internal/format/jsondoc"
	"glance/internal/format/xlsx"
	"glance/internal/workspace"
)

const (
	workspaceTabTopGap    unit.Dp = 12
	workspaceTabHeight    unit.Dp = 36
	workspaceTabStripSize unit.Dp = workspaceTabTopGap + workspaceTabHeight
)

func (a *App) layoutWorkspaceTabs(gtx layout.Context) layout.Dimensions {
	ids := a.workspaceTabIDs()
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = gtx.Dp(workspaceTabStripSize), gtx.Dp(workspaceTabStripSize)
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return fill(gtx, color.NRGBA{R: 239, G: 242, B: 246, A: 255})
		},
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, color.NRGBA{R: 202, G: 208, B: 216, A: 255}, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(1)))}.Op())
			a.processWindowControlClicks(gtx)
			if !a.clientTitlebar {
				return a.layoutWorkspaceTabList(gtx, ids)
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				func() layout.FlexChild {
					if runtime.GOOS == "darwin" {
						return layout.Rigid(a.layoutMacWindowControls)
					}
					return layout.Rigid(func(layout.Context) layout.Dimensions { return layout.Dimensions{} })
				}(),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return a.layoutWorkspaceTabList(gtx, ids)
				}),
				layout.Rigid(a.layoutWindowDragHandle),
				func() layout.FlexChild {
					if runtime.GOOS == "darwin" {
						return layout.Rigid(func(layout.Context) layout.Dimensions { return layout.Dimensions{} })
					}
					return layout.Rigid(a.layoutStandardWindowControls)
				}(),
			)
		},
	)
}

func (a *App) layoutWorkspaceTabList(gtx layout.Context, ids []string) layout.Dimensions {
	return layout.Inset{Top: workspaceTabTopGap}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return material.List(a.theme, &a.workspaceTabsList).Layout(gtx, len(ids)+2, func(gtx layout.Context, index int) layout.Dimensions {
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
	return a.paintTab(gtx, &a.recentTabButton, "⌂  最近", a.activeTabID == workspace.RecentTabID)
}

func (a *App) layoutPlusTab(gtx layout.Context) layout.Dimensions {
	if a.newButton.Clicked(gtx) {
		a.showNewMenu = !a.showNewMenu
	}
	return a.paintTab(gtx, &a.newButton, "+", false)
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
	if a.tabButtons[id] == nil {
		a.tabButtons[id] = new(widget.Clickable)
	}
	if a.tabCloseButtons[id] == nil {
		a.tabCloseButtons[id] = new(widget.Clickable)
	}
	if a.tabButtons[id].Clicked(gtx) {
		a.persistActiveSession()
		if state := a.sessions[id]; state != nil {
			a.activateSession(id, state)
		}
	}
	if a.tabCloseButtons[id].Clicked(gtx) {
		a.closeWorkspaceTab(id)
	}
	label := meta.Title
	if state := a.sessions[id]; state != nil && state.book != nil && state.book.Dirty() || state != nil && state.word != nil && state.word.Dirty() || state != nil && state.jsonDoc != nil && state.jsonDoc.Dirty() {
		label += " •"
	}
	return layout.Inset{Left: unit.Dp(2), Right: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return a.paintDocumentTab(gtx, a.tabButtons[id], a.tabCloseButtons[id], label, id == a.activeTabID)
	})
}

func (a *App) paintDocumentTab(gtx layout.Context, tab, close *widget.Clickable, label string, active bool) layout.Dimensions {
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = gtx.Dp(workspaceTabHeight), gtx.Dp(workspaceTabHeight)
	runes := []rune(label)
	if len(runes) > 24 {
		label = string(runes[:21]) + "…"
	}
	child := op.Record(gtx.Ops)
	dims := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return tab.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					style := material.Body2(a.theme, label)
					style.Color = color.NRGBA{R: 52, G: 57, B: 65, A: 255}
					return layout.Center.Layout(gtx, style.Layout)
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return close.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(5), Right: unit.Dp(9)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					style := material.Body2(a.theme, "×")
					style.Color = color.NRGBA{R: 91, G: 101, B: 116, A: 255}
					return layout.Center.Layout(gtx, style.Layout)
				})
			})
		}),
	)
	dims.Size.Y = gtx.Constraints.Max.Y
	call := child.Stop()
	bg := color.NRGBA{R: 234, G: 237, B: 241, A: 255}
	if active {
		bg = pageBackground
	}
	paint.FillShape(gtx.Ops, bg, clip.RRect{Rect: image.Rect(0, 0, dims.Size.X, dims.Size.Y), NW: gtx.Dp(unit.Dp(7)), NE: gtx.Dp(unit.Dp(7))}.Op(gtx.Ops))
	call.Add(gtx.Ops)
	return dims
}

func (a *App) paintTab(gtx layout.Context, button *widget.Clickable, label string, active bool) layout.Dimensions {
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = gtx.Dp(workspaceTabHeight), gtx.Dp(workspaceTabHeight)
		runes := []rune(label)
		if len(runes) > 24 {
			label = string(runes[:21]) + "…"
		}
		child := op.Record(gtx.Ops)
		dims := layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			style := material.Body2(a.theme, label)
			style.Color = color.NRGBA{R: 52, G: 57, B: 65, A: 255}
			return layout.Center.Layout(gtx, style.Layout)
		})
		dims.Size.Y = gtx.Constraints.Max.Y
		call := child.Stop()
		bg := color.NRGBA{R: 234, G: 237, B: 241, A: 255}
		if active {
			bg = pageBackground
		}
		paint.FillShape(gtx.Ops, bg, clip.RRect{Rect: image.Rect(0, 0, dims.Size.X, dims.Size.Y), NW: gtx.Dp(unit.Dp(7)), NE: gtx.Dp(unit.Dp(7))}.Op(gtx.Ops))
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
	state := a.sessions[id]
	if state == nil {
		return
	}
	if (state.book != nil && state.book.Dirty()) || (state.word != nil && state.word.Dirty()) || (state.jsonDoc != nil && state.jsonDoc.Dirty()) {
		a.status = "该标签有未保存更改；请先保存后再关闭"
		return
	}
	wasActive := a.activeTabID == id
	if a.collection != nil {
		if _, persisted := a.collection.Get(id); persisted {
			if err := a.collection.CloseTab(id); err != nil {
				a.status = err.Error()
				return
			}
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
		if s := a.sessions[next]; s != nil {
			a.activateSession(next, s)
		} else {
			a.activateRecent()
		}
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

func (a *App) layoutRecent(gtx layout.Context) layout.Dimensions {
	children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions { return material.H6(a.theme, "最近打开").Layout(gtx) })}
	if a.workspaceError != "" {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Body2(a.theme, "工作区恢复失败："+a.workspaceError).Layout(gtx)
		}))
	}
	var recents []workspace.Recent
	if a.collection != nil {
		recents = a.collection.Recents()
	}
	if len(recents) == 0 {
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return material.Body1(a.theme, "选择 + 打开文件，或新建 Excel / Word / JSON 文档").Layout(gtx)
			})
		}))
	} else {
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return material.List(a.theme, &a.recentList).Layout(gtx, len(recents), func(gtx layout.Context, i int) layout.Dimensions {
				item := recents[i]
				button := a.recentButtons[item.Path]
				if button == nil {
					button = new(widget.Clickable)
					a.recentButtons[item.Path] = button
				}
				_, statErr := os.Stat(item.Path)
				if button.Clicked(gtx) && statErr == nil {
					a.startOpen(item.Path)
				}
				return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					label := filepath.Base(item.Path) + "   " + item.Path
					if statErr != nil {
						label += "（不可用）"
					}
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							style := material.Button(a.theme, button, label)
							if statErr != nil {
								style.Background = color.NRGBA{R: 180, G: 184, B: 192, A: 255}
							}
							return style.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							remove := a.recentRemoveButtons[item.Path]
							if remove == nil {
								remove = new(widget.Clickable)
								a.recentRemoveButtons[item.Path] = remove
							}
							if remove.Clicked(gtx) && statErr != nil {
								if err := a.collection.RemoveRecent(item.Path); err != nil {
									a.status = err.Error()
								}
								delete(a.recentButtons, item.Path)
								delete(a.recentRemoveButtons, item.Path)
							}
							if statErr != nil {
								return material.Button(a.theme, remove, "移除").Layout(gtx)
							}
							return layout.Dimensions{}
						}),
					)
				})
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceAround}.Layout(gtx, children...)
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
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = gtx.Dp(unit.Dp(40)), gtx.Dp(unit.Dp(40))
	return material.List(a.theme, &a.createActionsList).Layout(gtx, len(buttons), func(gtx layout.Context, index int) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return material.Button(a.theme, buttons[index].click, buttons[index].label).Layout(gtx)
		})
	})
}

type createActionItem struct {
	click *widget.Clickable
	label string
}

func (a *App) createActionItems() []createActionItem {
	buttons := []createActionItem{{&a.actionOpenFile, "打开文件…"}, {&a.actionOpenFolder, "打开文件夹…"}}
	if a.hasActiveDocument() {
		if a.isDirty() {
			label := "保存"
			if a.path == "" {
				label = "保存…"
			}
			buttons = append(buttons, createActionItem{&a.actionSave, label})
		}
		buttons = append(buttons, createActionItem{&a.actionSaveAs, "另存为…"})
	}
	buttons = append(buttons, createActionItem{&a.actionNewXLSX, "新建 Excel"}, createActionItem{&a.actionNewDOCX, "新建 Word"}, createActionItem{&a.actionNewJSON, "新建 JSON"})
	return buttons
}

func (a *App) createActionLabels() []string {
	items := a.createActionItems()
	labels := make([]string, len(items))
	for i, item := range items {
		labels[i] = item.label
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
