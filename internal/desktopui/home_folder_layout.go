package desktopui

import (
	"image"
	"image/color"

	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

const (
	homeFolderMenuItemHeight = 42
	homeFolderMenuHeight     = 2*6 + 3*homeFolderMenuItemHeight + 2*4
	homeFolderMenuItemCount  = 3
)

var homeFolderMenuActionIDs = [homeFolderMenuItemCount]string{"create-child", "rename", "remove"}

type homeFolderRowWidgets struct {
	selectButton widget.Clickable
	expandButton widget.Clickable
	menuButton   widget.Clickable
	menuItems    map[string]*widget.Clickable
}

func (a *App) layoutHomeFolderTree(gtx layout.Context) layout.Dimensions {
	if a.homeFolderRows == nil {
		a.homeFolderRows = make(map[string]*homeFolderRowWidgets)
	}
	if a.homeFolderExpanded == nil {
		a.homeFolderExpanded = make(map[string]bool)
	}
	a.homeFolderTreeList.Axis = layout.Vertical
	if a.homeFolderMenuFolderID != "" {
		a.processHomeFolderMenuKeys(gtx)
	}
	if a.homeFolderBlank.Clicked(gtx) && a.homeFolderMenuFolderID != "" {
		a.dismissHomeFolderMenu()
	}
	snapshot, _ := a.homeFolderSnapshot()
	rows := homeFolderTreeRows(snapshot, a.homeFolderExpanded)
	visible := make(map[string]bool, len(rows))
	for _, row := range rows {
		visible[row.folder.ID] = true
	}
	for id := range a.homeFolderRows {
		if !visible[id] {
			delete(a.homeFolderRows, id)
		}
	}
	dims := layout.Stack{}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return a.homeFolderBlank.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return material.List(a.theme, &a.homeFolderTreeList).Layout(gtx, len(rows), func(gtx layout.Context, index int) layout.Dimensions {
				return a.layoutHomeFolderTreeRow(gtx, rows[index])
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return a.layoutHomeFolderFloatingMenu(gtx, rows)
		}),
	)
	a.updateHomeFolderDropTargets(rows, gtx)
	return dims
}

func (a *App) layoutHomeFolderTreeRow(gtx layout.Context, row homeFolderTreeRow) layout.Dimensions {
	widgets := a.homeFolderRows[row.folder.ID]
	if widgets == nil {
		widgets = &homeFolderRowWidgets{menuItems: make(map[string]*widget.Clickable)}
		a.homeFolderRows[row.folder.ID] = widgets
	}
	for index, id := range homeFolderMenuActionIDs {
		button := a.homeFolderMenuItem(widgets, id)
		if button.Clicked(gtx) {
			a.executeHomeFolderMenuAction(row.folder.ID, index)
		}
	}
	if widgets.expandButton.Clicked(gtx) {
		a.dismissHomeFolderMenu()
		a.toggleHomeFolderExpanded(row.folder.ID)
	}
	if widgets.selectButton.Clicked(gtx) {
		a.selectHomeFolder(row.folder.ID)
	}
	if widgets.menuButton.Clicked(gtx) {
		a.toggleHomeFolderMenu(row.folder.ID)
		gtx.Execute(key.FocusCmd{Tag: &a.homeFolderMenuDismiss})
	}
	for _, target := range []*widget.Clickable{&widgets.expandButton, &widgets.selectButton, &widgets.menuButton} {
		for event, ok := gtx.Event(pointer.Filter{Target: target, Kinds: pointer.Press}); ok; event, ok = gtx.Event(pointer.Filter{Target: target, Kinds: pointer.Press}) {
			if pointerEvent, isPointer := event.(pointer.Event); isPointer && pointerEvent.Buttons.Contain(pointer.ButtonSecondary) {
				a.selectHomeFolder(row.folder.ID)
				a.toggleHomeFolderMenu(row.folder.ID)
				gtx.Execute(key.FocusCmd{Tag: &a.homeFolderMenuDismiss})
			}
		}
	}
	if a.homeFolderMenuFolderID == row.folder.ID {
		gtx.Execute(key.FocusCmd{Tag: &a.homeFolderMenuDismiss})
	}
	return a.layoutHomeFolderTreeRowContents(gtx, row, widgets)
}

func (a *App) layoutHomeFolderMenuDismissInput(gtx layout.Context) {
	if a.homeFolderMenuFolderID == "" || gtx.Constraints.Max.X <= 0 || gtx.Constraints.Max.Y <= 0 {
		return
	}
	if a.homeFolderMenuDismiss.Clicked(gtx) {
		history := a.homeFolderMenuDismiss.History()
		if len(history) != 0 {
			point := history[len(history)-1].Position
			if !point.In(a.homeFolderMenuBounds) && !point.In(a.homeFolderMenuAnchorBounds) {
				a.dismissHomeFolderMenu()
			}
		}
	}
	pass := pointer.PassOp{}.Push(gtx.Ops)
	a.homeFolderMenuDismiss.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
	pass.Pop()
}

func (a *App) layoutHomeFolderFloatingMenu(gtx layout.Context, rows []homeFolderTreeRow) layout.Dimensions {
	if a.homeFolderMenuFolderID == "" {
		return layout.Dimensions{}
	}
	rowHeight := gtx.Dp(unit.Dp(36))
	menuHeight := gtx.Dp(unit.Dp(homeFolderMenuHeight))
	rowIndex, menuTop, visible := homeFolderMenuPosition(
		rows,
		a.homeFolderMenuFolderID,
		a.homeFolderTreeList.Position.First,
		a.homeFolderTreeList.Position.Offset,
		a.homeFolderTreeList.Position.Count,
		rowHeight,
		menuHeight,
		gtx.Constraints.Max.Y,
	)
	if !visible {
		a.dismissHomeFolderMenu()
		return layout.Dimensions{}
	}
	row := rows[rowIndex]
	widgets := a.homeFolderRows[row.folder.ID]
	if widgets == nil {
		return layout.Dimensions{}
	}
	left := 0
	width := gtx.Constraints.Max.X
	if width == 0 {
		return layout.Dimensions{}
	}
	viewportSize := gtx.Constraints.Max
	rowTop := (rowIndex-max(0, a.homeFolderTreeList.Position.First))*rowHeight - a.homeFolderTreeList.Position.Offset
	a.homeFolderMenuAnchorBounds = image.Rectangle{
		Min: a.homeFolderTreeRootOrigin.Add(image.Pt(0, rowTop)),
		Max: a.homeFolderTreeRootOrigin.Add(image.Pt(viewportSize.X, rowTop+rowHeight)),
	}
	a.homeFolderMenuBounds = image.Rectangle{
		Min: a.homeFolderTreeRootOrigin.Add(image.Pt(left, menuTop)),
		Max: a.homeFolderTreeRootOrigin.Add(image.Pt(left+width, menuTop+menuHeight)),
	}
	transform := op.Offset(image.Pt(left, menuTop)).Push(gtx.Ops)
	defer transform.Pop()
	gtx.Constraints = layout.Constraints{
		Min: image.Pt(width, menuHeight),
		Max: image.Pt(width, menuHeight),
	}
	a.layoutHomeFolderMenu(gtx, widgets)
	return layout.Dimensions{Size: viewportSize}
}

func homeFolderMenuPosition(rows []homeFolderTreeRow, folderID string, first, offset, count, rowHeight, menuHeight, viewportHeight int) (int, int, bool) {
	if folderID == "" || count <= 0 || rowHeight <= 0 || menuHeight <= 0 || viewportHeight <= 0 {
		return 0, 0, false
	}
	first = max(0, first)
	end := min(len(rows), first+count)
	for index := first; index < end; index++ {
		if rows[index].folder.ID != folderID {
			continue
		}
		rowTop := (index-first)*rowHeight - offset
		rowBounds := image.Rect(0, rowTop, 1, rowTop+rowHeight)
		if rowBounds.Intersect(image.Rect(0, 0, 1, viewportHeight)).Empty() {
			return 0, 0, false
		}
		menuTop := rowTop + rowHeight
		if menuTop+menuHeight > viewportHeight {
			menuTop = rowTop - menuHeight
		}
		if menuTop < 0 {
			menuTop = 0
		}
		return index, menuTop, true
	}
	return 0, 0, false
}

func (a *App) layoutHomeFolderTreeRowContents(gtx layout.Context, row homeFolderTreeRow, widgets *homeFolderRowWidgets) layout.Dimensions {
	height := gtx.Dp(unit.Dp(36))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			bg := color.NRGBA{A: 0}
			if a.homeActiveDropFolderID == row.folder.ID {
				bg = homeDropTargetFill
			} else if a.homeSelectedFolderID == row.folder.ID {
				bg = homeSelectedFill
			} else if widgets.selectButton.Hovered() {
				bg = homeHoverFill
			}
			paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(6))).Op(gtx.Ops))
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(12 + row.depth*20), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						glyph := "folder"
						if a.homeFolderExpanded[row.folder.ID] {
							glyph = "folder-open"
						}
						fg := color.NRGBA{R: 122, G: 131, B: 142, A: 255}
						if a.homeSelectedFolderID == row.folder.ID || a.homeFolderExpanded[row.folder.ID] {
							fg = color.NRGBA{R: 60, G: 70, B: 82, A: 255}
						}
						icon := func(gtx layout.Context) layout.Dimensions {
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return homeIcon(gtx, glyph, 20, fg)
							})
						}
						if !row.hasChildren {
							return icon(gtx)
						}
						return widgets.expandButton.Layout(gtx, icon)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return widgets.selectButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.Y = 0
							fontSize := 14
							if row.depth == 0 {
								fontSize = 15
							}
							return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(6), Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return homeTextLabel(gtx, a.theme, row.folder.Name, unit.Sp(fontSize), homeText, a.homeSelectedFolderID == row.folder.ID)
							})
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !widgets.menuButton.Hovered() && !gtx.Focused(&widgets.menuButton) && a.homeFolderMenuFolderID != row.folder.ID && !widgets.selectButton.Hovered() && !widgets.expandButton.Hovered() {
							return layout.Dimensions{}
						}
						button := func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(unit.Dp(24)), gtx.Dp(unit.Dp(24))))
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return homeIcon(gtx, "ellipsis", 16, homeMuted) })
						}
						return widgets.menuButton.Layout(gtx, button)
					}),
				)
			})
		},
	)
}

func (a *App) processHomeFolderMenuKeys(gtx layout.Context) {
	for event, ok := gtx.Event(key.Filter{Focus: &a.homeFolderMenuDismiss}); ok; event, ok = gtx.Event(key.Filter{Focus: &a.homeFolderMenuDismiss}) {
		keyEvent, isKey := event.(key.Event)
		if !isKey || keyEvent.State != key.Press {
			continue
		}
		switch keyEvent.Name {
		case key.NameEscape:
			folderID := a.homeFolderMenuFolderID
			anchor := a.homeFolderRows[folderID]
			a.dismissHomeFolderMenu()
			if anchor != nil {
				gtx.Execute(key.FocusCmd{Tag: &anchor.menuButton})
			}
			return
		case key.NameUpArrow:
			a.homeFolderMenuIndex = homeFolderMenuNextSelection(a.homeFolderMenuIndex, -1)
		case key.NameDownArrow:
			a.homeFolderMenuIndex = homeFolderMenuNextSelection(a.homeFolderMenuIndex, 1)
		case key.NameEnter, key.NameReturn:
			index := a.homeFolderMenuIndex
			if index < 0 {
				index = 0
			}
			a.executeHomeFolderMenuAction(a.homeFolderMenuFolderID, index)
			return
		}
	}
}

func homeFolderMenuNextSelection(current, direction int) int {
	if direction == 0 {
		return current
	}
	if current < 0 || current >= homeFolderMenuItemCount {
		if direction < 0 {
			return homeFolderMenuItemCount - 1
		}
		return 0
	}
	next := (current + direction) % homeFolderMenuItemCount
	if next < 0 {
		next += homeFolderMenuItemCount
	}
	return next
}

func (a *App) executeHomeFolderMenuAction(folderID string, index int) {
	if folderID == "" || index < 0 || index >= homeFolderMenuItemCount {
		return
	}
	a.dismissHomeFolderMenu()
	switch homeFolderMenuActionIDs[index] {
	case "create-child":
		a.showCreateHomeFolderDialog(folderID)
	case "rename":
		a.showRenameHomeFolderDialog(folderID)
	case "remove":
		a.showRemoveHomeFolderConfirm(folderID)
	}
}

func (a *App) layoutHomeFolderMenu(gtx layout.Context, widgets *homeFolderRowWidgets) layout.Dimensions {
	// The menu is laid out from a fixed-height tree row, so size it independently.
	menuHeight := gtx.Dp(unit.Dp(homeFolderMenuHeight))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = menuHeight, menuHeight
	return homeRoundedPanel(gtx, homeSurface, homeBorder, unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				a.homeFolderMenuItemLayout(gtx, widgets, "create-child", "新建子文件夹"),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return a.layoutHomeFolderMenuItem(gtx, widgets, "rename", "重命名")
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return a.layoutHomeFolderMenuItem(gtx, widgets, "remove", "从列表移除")
					})
				}),
			)
		})
	})
}

func (a *App) homeFolderMenuItemLayout(gtx layout.Context, widgets *homeFolderRowWidgets, id, label string) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return a.layoutHomeFolderMenuItem(gtx, widgets, id, label)
	})
}

func (a *App) layoutHomeFolderMenuItem(gtx layout.Context, widgets *homeFolderRowWidgets, id, label string) layout.Dimensions {
	button := a.homeFolderMenuItem(widgets, id)
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		height := gtx.Dp(unit.Dp(homeFolderMenuItemHeight))
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				bg := color.NRGBA{A: 0}
				if button.Hovered() || a.homeFolderMenuIndex == homeFolderMenuItemIndex(id) {
					bg = homeHoverFill
				}
				paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(6))).Op(gtx.Ops))
				return layout.Dimensions{Size: gtx.Constraints.Max}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							iconName := homeFolderMenuIconName(id)
							fg := homeText
							if id == "remove" {
								fg = color.NRGBA{R: 196, G: 73, B: 73, A: 255}
							}
							return homeIcon(gtx, iconName, 18, fg)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.Y = 0
							// This top inset shifts text ink by 3.5dp after the inset box is centered, matching the folder SVG's visual center.
							return layout.Inset{Left: unit.Dp(10), Top: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.Y = 0
								fg := homeText
								if id == "remove" {
									fg = color.NRGBA{R: 196, G: 73, B: 73, A: 255}
								}
								return homeTextLabel(gtx, a.theme, label, 14, fg, false)
							})
						}),
					)
				})
			},
		)
	})
}

func homeFolderMenuItemIndex(id string) int {
	for index, actionID := range homeFolderMenuActionIDs {
		if actionID == id {
			return index
		}
	}
	return -1
}

func homeFolderMenuIconName(id string) string {
	switch id {
	case "create-child":
		return "folder-plus"
	case "rename":
		return "pencil"
	case "remove":
		return "folder-minus"
	default:
		return ""
	}
}

func (a *App) homeFolderMenuItem(row *homeFolderRowWidgets, id string) *widget.Clickable {
	if row.menuItems == nil {
		row.menuItems = make(map[string]*widget.Clickable)
	}
	if row.menuItems[id] == nil {
		row.menuItems[id] = new(widget.Clickable)
	}
	return row.menuItems[id]
}

func (a *App) layoutHomeFolderContentPage(gtx layout.Context) layout.Dimensions {
	path := a.homeFolderDisplayPath(a.homeSelectedFolderID)
	folderID := a.homeSelectedFolderID
	allItems := a.homeFolderRecents(folderID)
	items := a.filterHomeRecents(allItems, true, nil)
	a.loadMoreHomeFolderItems(folderID, len(items))
	visibleCount := min(len(items), a.visibleHomeFolderPageSize(folderID))
	var precedingHeight int
	trackedRigid := func(child layout.Widget) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := child(gtx)
			precedingHeight += dims.Size.Y
			return dims
		})
	}
	children := []layout.FlexChild{
		trackedRigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.Y = 0
						return homeTextLabel(gtx, a.theme, path, 24, homeText, true)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return homeTextLabel(gtx, a.theme, "Glance / "+path, 12, homeMuted, false)
						})
					}),
				)
			})
		}),
		trackedRigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(16)}.Layout(gtx, a.layoutHomeFilters)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			a.homeRecentTableRootOrigin = a.homeContentRootOrigin.Add(image.Pt(0, precedingHeight))
			a.homeRecentListRootOrigin = a.homeRecentTableRootOrigin.Add(image.Pt(0, gtx.Dp(unit.Dp(48))))
			return a.layoutHomeTableHeader(gtx)
		}),
	}
	for index := 0; index < visibleCount; index++ {
		index := index
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutHomeRecentRow(gtx, items[index], index)
		}))
	}
	if visibleCount == 0 {
		message := homeFolderEmptyStateMessage(len(allItems) > 0)
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(24), Left: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = 0
				return homeTextLabel(gtx, a.theme, message, 14, homeMuted, false)
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}
