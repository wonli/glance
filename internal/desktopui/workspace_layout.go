package desktopui

import (
	"image"
	"image/color"
	"runtime"
	"sort"
	"strings"

	"gioui.org/app"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/i18n"
	"glance/internal/workspace"
	"glance/ui"
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
	if a.clientTitlebar {
		a.processWindowControlClicks(gtx)
	}
	a.processWorkspaceTabClicks(gtx)
	ids := a.workspaceTabIDs()
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = gtx.Dp(workspaceTabStripSize), gtx.Dp(workspaceTabStripSize)
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Max
			paint.FillShape(gtx.Ops, workspaceTabStripSurface, clip.Rect{Max: size}.Op())
			return layout.Dimensions{Size: size}
		},
		func(gtx layout.Context) layout.Dimensions {
			if runtime.GOOS == "windows" {
				tabs := func(gtx layout.Context) layout.Dimensions {
					return a.layoutWindowsPinnedWorkspaceTabs(gtx, ids)
				}
				if !a.clientTitlebar {
					return tabs(gtx)
				}
				return layout.Stack{Alignment: layout.NW}.Layout(gtx,
					layout.Expanded(func(gtx layout.Context) layout.Dimensions {
						return layoutWindowMoveArea(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Dimensions{Size: gtx.Constraints.Max}
						})
					}),
					layout.Stacked(tabs),
				)
			}
			if !a.clientTitlebar {
				return a.layoutWorkspaceTabList(gtx, ids)
			}
			return layout.Stack{Alignment: layout.NW}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					return layoutWindowMoveArea(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: gtx.Constraints.Max}
					})
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					controls := func(gtx layout.Context) layout.Dimensions {
						if runtime.GOOS == "darwin" {
							return a.layoutMacWindowControls(gtx)
						}
						return a.layoutStandardWindowControls(gtx)
					}
					tabs := func(gtx layout.Context) layout.Dimensions {
						return a.layoutWorkspaceTabList(gtx, ids)
					}
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(controls),
						layout.Flexed(1, tabs),
					)
				}),
			)
		},
	)
}

// Windows keeps the Glance home tab outside the horizontally scrolling
// document-tab viewport so it remains visible at the left edge.
func (a *App) layoutWindowsPinnedWorkspaceTabs(gtx layout.Context, ids []string) layout.Dimensions {
	return layout.Inset{Top: workspaceTabTopGap}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(16)}.Layout(gtx, a.layoutRecentTab)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return a.layoutWindowsWorkspaceDocumentTabs(gtx, ids)
			}),
		}
		if a.clientTitlebar {
			children = append(children, layout.Rigid(a.layoutStandardWindowControls))
		} else {
			children = append(children, layout.Rigid(a.layoutJSONSaveIndicator))
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
	})
}

func (a *App) layoutWindowsWorkspaceDocumentTabs(gtx layout.Context, ids []string) layout.Dimensions {
	if a.workspaceTabsListPendingReset {
		a.workspaceTabsListPendingReset = false
		a.workspaceTabsList = widget.List{List: layout.List{Axis: layout.Horizontal}}
	}
	a.workspaceTabsList.Axis = layout.Horizontal
	a.workspaceTabsList.ScrollAnyAxis = false
	list := func(gtx layout.Context) layout.Dimensions {
		a.handleWorkspaceTabScroll(gtx)
		a.revealActiveWindowsWorkspaceTab(gtx, ids)
		dims := a.workspaceTabsList.Layout(gtx, len(ids)+1, func(gtx layout.Context, index int) layout.Dimensions {
			return a.layoutWindowsWorkspaceTabListItem(gtx, ids, index)
		})
		pass := pointer.PassOp{}.Push(gtx.Ops)
		area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
		event.Op(gtx.Ops, &a.workspaceTabScrollTag)
		area.Pop()
		pass.Pop()
		return dims
	}
	if !a.clientTitlebar {
		// Keep the viewport full width so the fixed home tab stays aligned with
		// the left edge and the save indicator remains at the far right.
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, list))
	}

	documentItemWidth := gtx.Dp(workspaceDocumentTabWidth) + gtx.Dp(workspaceDocumentTabGap)
	plusItemWidth := gtx.Dp(workspaceDocumentTabGap) + gtx.Dp(workspacePlusTabWidth)
	contentWidth := len(ids)*documentItemWidth + plusItemWidth
	listWidth := min(contentWidth, gtx.Constraints.Max.X)
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = listWidth, listWidth
			return list(gtx)
		}),
		layout.Flexed(1, a.layoutWindowDragHandle),
		layout.Rigid(a.layoutJSONSaveIndicator),
	)
}

func (a *App) processWorkspaceTabClicks(gtx layout.Context) {
	if a.recentTabButton.Clicked(gtx) {
		a.showNewMenu = false
		a.persistActiveSession()
		a.activateRecent()
	}
	if a.newButton.Clicked(gtx) {
		a.showNewMenu = !a.showNewMenu
	}
	for id, closeButton := range a.tabCloseButtons {
		if closeButton != nil && closeButton.Clicked(gtx) {
			a.showNewMenu = false
			a.closeWorkspaceTab(id)
			return
		}
	}
	for id, tabButton := range a.tabButtons {
		if tabButton != nil && tabButton.Clicked(gtx) {
			a.showNewMenu = false
			a.activateWorkspaceTab(id)
			return
		}
	}
}

func (a *App) layoutWorkspaceTabList(gtx layout.Context, ids []string) layout.Dimensions {
	if a.workspaceTabsListPendingReset {
		a.workspaceTabsListPendingReset = false
		a.workspaceTabsList = widget.List{List: layout.List{Axis: layout.Horizontal}}
	}
	a.workspaceTabsList.Axis = layout.Horizontal
	a.workspaceTabsList.ScrollAnyAxis = false
	return layout.Inset{Top: workspaceTabTopGap, Left: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		list := func(gtx layout.Context) layout.Dimensions {
			a.handleWorkspaceTabScroll(gtx)
			a.revealActiveWorkspaceTab(gtx, ids)
			dims := a.workspaceTabsList.Layout(gtx, len(ids)+2, func(gtx layout.Context, index int) layout.Dimensions {
				return a.layoutWorkspaceTabListItem(gtx, ids, index)
			})
			pass := pointer.PassOp{}.Push(gtx.Ops)
			area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
			event.Op(gtx.Ops, &a.workspaceTabScrollTag)
			area.Pop()
			pass.Pop()
			return dims
		}
		if !a.clientTitlebar {
			// Keep the viewport as wide as the tab strip so Background doesn't
			// center a shorter natural width and shift the selected tab.
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, list), layout.Rigid(a.layoutJSONSaveIndicator))
		}

		documentItemWidth := gtx.Dp(workspaceDocumentTabWidth) + gtx.Dp(workspaceDocumentTabGap)
		plusItemWidth := gtx.Dp(workspaceDocumentTabGap) + gtx.Dp(workspacePlusTabWidth)
		contentWidth := gtx.Dp(unit.Dp(112)) + len(ids)*documentItemWidth + plusItemWidth
		listWidth := min(contentWidth, gtx.Constraints.Max.X)
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X, gtx.Constraints.Max.X = listWidth, listWidth
				return list(gtx)
			}),
			layout.Flexed(1, a.layoutWindowDragHandle),
			layout.Rigid(a.layoutJSONSaveIndicator),
		)
	})
}

func (a *App) revealActiveWorkspaceTab(gtx layout.Context, ids []string) {
	a.revealActiveWorkspaceTabInList(gtx, ids, true)
}

func (a *App) workspaceSelectedTabID() string {
	if a.pendingPDFRestoreTabID != "" {
		return a.pendingPDFRestoreTabID
	}
	return a.activeTabID
}

func (a *App) revealActiveWindowsWorkspaceTab(gtx layout.Context, ids []string) {
	selectedTabID := a.workspaceSelectedTabID()
	if selectedTabID == workspace.RecentTabID {
		// The home tab is pinned outside the list. Clear the cached reveal so
		// returning to a document will reveal it even if the user scrolled away.
		a.workspaceTabsRevealedActiveID = ""
		a.workspaceTabsRevealedIndex = -1
		return
	}
	a.revealActiveWorkspaceTabInList(gtx, ids, false)
}

func (a *App) revealActiveWorkspaceTabInList(gtx layout.Context, ids []string, includeRecent bool) {
	index := -1
	itemOffset := 0
	selectedTabID := a.workspaceSelectedTabID()
	if includeRecent && selectedTabID == workspace.RecentTabID {
		index = 0
	} else {
		if includeRecent {
			itemOffset = 1
		}
		for i, id := range ids {
			if id == selectedTabID {
				index = i + itemOffset
				break
			}
		}
	}
	if index < 0 {
		return
	}
	if a.workspaceTabsRevealedActiveID == selectedTabID && a.workspaceTabsRevealedIndex == index {
		return
	}
	viewportWidth := gtx.Constraints.Max.X
	if viewportWidth <= 0 {
		return
	}
	widthCount := len(ids) + 1
	if includeRecent {
		widthCount++
	}
	widths := make([]int, widthCount)
	if includeRecent {
		widths[0] = gtx.Dp(unit.Dp(112))
	}
	for i := range ids {
		widths[i+itemOffset] = gtx.Dp(workspaceDocumentTabWidth) + gtx.Dp(workspaceDocumentTabGap)
	}
	widths[len(widths)-1] = gtx.Dp(workspaceDocumentTabGap) + gtx.Dp(workspacePlusTabWidth)
	prefix := make([]int, len(widths)+1)
	for i, width := range widths {
		prefix[i+1] = prefix[i] + width
	}
	activeStart, activeEnd := prefix[index], prefix[index+1]
	position := a.workspaceTabsList.Position
	first := max(0, min(position.First, len(widths)-1))
	currentScroll := prefix[first] + max(0, position.Offset)
	// Leave a small trailing margin for the close affordance. If the active
	// tab is clipped at either edge, put it at the start of the viewport; this
	// avoids estimating a fractional offset from stale visible-item counts.
	visibleEnd := currentScroll + viewportWidth - gtx.Dp(unit.Dp(24))
	if activeStart < currentScroll || activeEnd > visibleEnd {
		position.First = index
		position.Offset = 0
		position.BeforeEnd = true
		a.workspaceTabsList.Position = position
	}
	a.workspaceTabsRevealedActiveID = selectedTabID
	a.workspaceTabsRevealedIndex = index
}

// Normalize wheel and trackpad axes into one horizontal list offset. A single
// trackpad gesture can report both axes; choosing the dominant axis prevents
// its cross-axis component from canceling or reversing the movement.
func (a *App) handleWorkspaceTabScroll(gtx layout.Context) {
	const scrollLimit = 1 << 30
	filter := pointer.Filter{
		Target: &a.workspaceTabScrollTag,
		Kinds:  pointer.Scroll,
		// Consume both axes here. Splitting a diagonal trackpad gesture between
		// this adapter and the horizontal List can apply opposing deltas and
		// cancel the movement entirely.
		ScrollX: pointer.ScrollRange{Min: -scrollLimit, Max: scrollLimit},
		ScrollY: pointer.ScrollRange{Min: -scrollLimit, Max: scrollLimit},
	}
	for eventValue, ok := gtx.Event(filter); ok; eventValue, ok = gtx.Event(filter) {
		pe, ok := eventValue.(pointer.Event)
		if !ok || pe.Kind != pointer.Scroll || pe.Scroll.X == 0 && pe.Scroll.Y == 0 {
			continue
		}
		// Treat a diagonal gesture as movement on its dominant axis, so its
		// smaller cross-axis component cannot cancel or reverse the scroll.
		delta := pe.Scroll.X
		if absFloat32(pe.Scroll.Y) >= absFloat32(pe.Scroll.X) {
			// A downward wheel/trackpad gesture should reveal later tabs.
			delta = pe.Scroll.Y
		}
		a.workspaceTabScrollRemainder += delta
		pixels := int(a.workspaceTabScrollRemainder)
		a.workspaceTabScrollRemainder -= float32(pixels)
		if pixels == 0 {
			continue
		}
		a.workspaceTabsList.Position.Offset += pixels
		a.workspaceTabsList.Position.BeforeEnd = true
	}
}

func absFloat32(value float32) float32 {
	if value < 0 {
		return -value
	}
	return value
}

func (a *App) layoutWorkspaceTabListItem(gtx layout.Context, ids []string, index int) layout.Dimensions {
	var item layout.Widget
	if index == 0 {
		item = a.layoutRecentTab
	} else if index == len(ids)+1 {
		item = a.layoutPlusTab
	} else if index > 0 && index <= len(ids) {
		item = func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: workspaceDocumentTabGap}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutDocumentTab(gtx, ids[index-1])
			})
		}
	}
	if item == nil {
		return layout.Dimensions{}
	}
	return item(gtx)
}

func (a *App) layoutWindowsWorkspaceTabListItem(gtx layout.Context, ids []string, index int) layout.Dimensions {
	var item layout.Widget
	if index == len(ids) {
		item = a.layoutPlusTab
	} else if index >= 0 && index < len(ids) {
		item = func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: workspaceDocumentTabGap}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutDocumentTab(gtx, ids[index])
			})
		}
	}
	if item == nil {
		return layout.Dimensions{}
	}
	return item(gtx)
}

func (a *App) processWindowControlClicks(gtx layout.Context) {
	if a.window == nil {
		return
	}
	if a.windowCloseButton.Clicked(gtx) {
		if !a.hideMainWindowToTray() {
			a.window.Perform(system.ActionClose)
		}
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
		style.Color = a.tokens().Colors.Text.PureBlack
		return layout.Center.Layout(gtx, style.Layout)
	})
}

func (a *App) layoutWindowDragHandle(gtx layout.Context) layout.Dimensions {
	return a.windowDecorations.LayoutMove(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
}

func layoutWindowMoveArea(gtx layout.Context, w layout.Widget) layout.Dimensions {
	child := op.Record(gtx.Ops)
	dims := w(gtx)
	call := child.Stop()
	area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
	system.ActionInputOp(system.ActionMove).Add(gtx.Ops)
	area.Pop()
	call.Add(gtx.Ops)
	return dims
}

func layoutWindowMoveAreaAfter(gtx layout.Context, w layout.Widget) layout.Dimensions {
	child := op.Record(gtx.Ops)
	dims := w(gtx)
	call := child.Stop()
	call.Add(gtx.Ops)
	area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
	system.ActionInputOp(system.ActionMove).Add(gtx.Ops)
	area.Pop()
	return dims
}

func (a *App) layoutRecentTab(gtx layout.Context) layout.Dimensions {
	if a.recentTabButton.Clicked(gtx) {
		a.showNewMenu = false
		a.persistActiveSession()
		a.activateRecent()
	}
	active := a.workspaceSelectedTabID() == workspace.RecentTabID
	tab := func(gtx layout.Context) layout.Dimensions {
		return a.paintTab(gtx, &a.recentTabButton, "⌂  Glance", active)
	}
	if active && a.clientTitlebar {
		return layoutWindowMoveAreaAfter(gtx, tab)
	}
	return tab(gtx)
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
		a.showNewMenu = false
		a.activateWorkspaceTab(id)
	}
	if closeButton.Clicked(gtx) {
		a.showNewMenu = false
		a.closeWorkspaceTab(id)
	}
	label := meta.Title
	if state := a.sessions[id]; state != nil && sessionDirty(state) {
		label += " •"
	}
	// Keep this frame's widgets alive even if the close click removed the
	// tab and its widget map entries above.
	return a.paintDocumentTab(gtx, tabButton, closeButton, label, id == a.workspaceSelectedTabID())
}

func (a *App) activateWorkspaceTab(id string) {
	a.invalidatePDFWorkspaceRestore()
	a.persistActiveSession()
	a.ensureDesktopRuntime().drainWorkspaceWriter()
	a.evictInactiveSessionsExcept(id)
	if a.collection != nil {
		if record, ok := a.collection.GetMetadata(id); ok {
			if strings.EqualFold(record.Format, "pdf") {
				if _, found := a.ensureDesktopRuntime().session(record.Tab.DocumentID); !found {
					a.startWorkspacePDFRestore(*record)
					return
				}
			} else if !record.SnapshotPresent && record.Tab.SourcePath != "" {
				// Restore a source file asynchronously when no workspace snapshot exists.
				a.setStatusMessage(i18n.Message{ID: "workspace.status.restore_from_source", Args: i18n.Args{"Path": record.Tab.SourcePath}}, feedbackInfo, notificationSuppressed)
				a.startOpen(record.Tab.SourcePath)
				return
			}
		}
	}
	state, err := a.viewForTabWithError(id)
	if err == nil {
		a.activateSession(id, state)
		return
	}
	title := id
	if meta, ok := a.tabMeta[id]; ok {
		title = meta.Title
		if meta.SourcePath != "" {
			a.setStatusMessage(i18n.Message{ID: "workspace.status.restore_from_source", Args: i18n.Args{"Path": meta.SourcePath}}, feedbackInfo, notificationSuppressed)
			a.startOpen(meta.SourcePath)
			return
		}
	}
	a.setStatusErrorMessage(i18n.Message{ID: "workspace.error.restore_tab", Args: i18n.Args{"Title": title}}, err, notificationToast)
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
			tabContent := func(gtx layout.Context) layout.Dimensions {
				return tab.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						style := material.Body2(a.theme, label)
						style.TextSize = a.tokens().Typography.Body
						style.MaxLines = 1
						style.Color = a.tokens().Colors.Text.PureBlack
						return layout.Center.Layout(gtx, style.Layout)
					})
				})
			}
			if active && a.clientTitlebar {
				return layoutWindowMoveAreaAfter(gtx, tabContent)
			}
			return tabContent(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return close.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(5), Right: unit.Dp(9)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					style := material.Body2(a.theme, "×")
					style.TextSize = a.tokens().Typography.Heading + unit.Sp(1)
					style.Color = a.tokens().Colors.Text.PureBlack
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
			style.TextSize = a.tokens().Typography.Heading + unit.Sp(1)
			style.Color = a.tokens().Colors.Text.PureBlack
			dims = layout.Center.Layout(gtx, style.Layout)
		} else {
			dims = layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				gtx.Constraints.Min.Y = 0
				return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							icon, _ := tintedToolbarIcon("house", color.NRGBA{A: 255})
							if icon == nil {
								return layout.Dimensions{}
							}
							iconSize := gtx.Dp(unit.Dp(18))
							gtx.Constraints.Min = image.Pt(iconSize, iconSize)
							gtx.Constraints.Max = gtx.Constraints.Min
							return (ui.OpticalPlacement{OffsetY: shellOpticalIconOffset(opticalHomeTabIcon)}).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return widget.Image{Src: iconImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: 18.0 / 96.0}.Layout(gtx)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(8)), 0)}
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							style := material.Body2(a.theme, "Glance")
							style.TextSize = a.tokens().Typography.Body
							style.Color = a.tokens().Colors.Text.PureBlack
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
		if label == "+" && active {
			shape := clip.RRect{Rect: image.Rectangle{Max: dims.Size}, NW: gtx.Dp(unit.Dp(6)), NE: gtx.Dp(unit.Dp(6))}
			paintRoundedBorder(gtx.Ops, shape, float32(gtx.Dp(unit.Dp(1))), homeBorder)
		}
		call.Add(gtx.Ops)
		return dims
	})
}
