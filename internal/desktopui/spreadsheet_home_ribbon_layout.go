package desktopui

import (
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"image"
)

type spreadsheetRibbonGroup struct {
	label   string
	actions []spreadsheetToolbarAction
	widthDP int
}

func spreadsheetHomeRibbonGroups(actions []spreadsheetToolbarAction) []spreadsheetRibbonGroup {
	widths := map[string]int{"剪贴板": 139, "字体": 231, "对齐": 268, "数字": 152, "样式": 260, "单元格": 72, "编辑": 114}
	order := []string{"剪贴板", "字体", "对齐", "数字", "样式", "单元格", "编辑"}
	byGroup := make(map[string][]spreadsheetToolbarAction, len(order))
	for _, action := range actions {
		if action.slot == 20 {
			// The Pen ribbon exposes sorting and filtering through the slot 18 menu.
			continue
		}
		group := action.group
		switch action.slot {
		case toolbarActionFontFamily, toolbarActionFontSize, 4, 5, 6:
			group = "字体"
		case toolbarActionAlignLeft, toolbarActionAlignRight, toolbarActionWrapText, 9:
			group = "对齐"
		case toolbarActionPercentFormat, 10:
			group = "数字"
		case toolbarActionConditionalFormat, toolbarActionTableFormat, toolbarActionCellStyles:
			group = "样式"
		case 12, 13, toolbarActionFormat:
			group = "单元格"
		case toolbarActionSum, toolbarActionFind, 18, 20:
			group = "编辑"
		case 0, 1:
			group = "历史"
		}
		byGroup[group] = append(byGroup[group], action)
	}
	styleOrder := []int{toolbarActionConditionalFormat, toolbarActionTableFormat, toolbarActionCellStyles}
	var orderedStyles []spreadsheetToolbarAction
	for _, slot := range styleOrder {
		for _, action := range byGroup["样式"] {
			if action.slot == slot {
				orderedStyles = append(orderedStyles, action)
			}
		}
	}
	byGroup["样式"] = orderedStyles
	var groups []spreadsheetRibbonGroup
	for _, label := range order {
		if len(byGroup[label]) > 0 {
			groups = append(groups, spreadsheetRibbonGroup{label: label, actions: byGroup[label], widthDP: widths[label]})
		}
	}
	return groups
}

func (a *App) layoutSpreadsheetHomeRibbon(gtx layout.Context, state *documentSessionState, actions []spreadsheetToolbarAction) layout.Dimensions {
	groups := spreadsheetHomeRibbonGroups(actions)
	state.toolbarGroupList.List.Axis = layout.Horizontal
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, a.tokens().Colors.Toolbar.Background, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return a.layoutOfficeRibbonAIEntry(gtx, unit.Dp(64))
						})
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return state.toolbarGroupList.Layout(gtx, len(groups), func(gtx layout.Context, index int) layout.Dimensions {
							return a.layoutSpreadsheetHomeRibbonGroupItem(gtx, state, groups[index], index == len(groups)-1)
						})
					}),
				)
			})
		},
	)
}

func (a *App) layoutSpreadsheetHomeRibbonGroupItem(gtx layout.Context, state *documentSessionState, group spreadsheetRibbonGroup, last bool) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutSpreadsheetHomeRibbonGroup(gtx, state, group, last)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if last {
				return layout.Dimensions{}
			}
			return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(4)), 0)}
		}),
	)
}

func (a *App) layoutSpreadsheetHomeRibbonGroup(gtx layout.Context, state *documentSessionState, group spreadsheetRibbonGroup, last bool) layout.Dimensions {
	width, height := gtx.Dp(unit.Dp(group.widthDP)), gtx.Dp(unit.Dp(72))
	gtx.Constraints = layout.Exact(image.Pt(width, height))
	return layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {

			return layout.Dimensions{Size: image.Pt(width, height)}
		}),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				switch group.label {
				case "剪贴板":
					return a.layoutSpreadsheetClipboardGroup(gtx, state, group.actions)
				case "字体":
					return a.layoutSpreadsheetFontGroup(gtx, state, group.actions)
				case "对齐":
					return a.layoutSpreadsheetAlignmentGroup(gtx, state, group.actions)
				case "数字":
					return a.layoutSpreadsheetNumberGroup(gtx, state, group.actions)
				case "样式":
					return a.layoutSpreadsheetStyleGroup(gtx, state, group.actions)
				case "单元格", "编辑", "历史":
					return a.layoutSpreadsheetCompactActionStack(gtx, state, group.actions)
				default:
					return a.layoutSpreadsheetActionGrid(gtx, state, group.actions, 1, true)
				}
			})
		}),
	)
}

func (a *App) layoutSpreadsheetClipboardGroup(gtx layout.Context, state *documentSessionState, actions []spreadsheetToolbarAction) layout.Dimensions {
	var paste *spreadsheetToolbarAction
	var compact []spreadsheetToolbarAction
	for i := range actions {
		if actions[i].slot == 21 {
			paste = &actions[i]
		} else {
			compact = append(compact, actions[i])
		}
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if paste == nil {
				return layout.Dimensions{}
			}
			return a.layoutSpreadsheetRibbonAction(gtx, state, *paste, 68, 68, spreadsheetRibbonActionVertical)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutSpreadsheetCompactActionStack(gtx, state, compact)
			})
		}),
	)
}

func (a *App) layoutSpreadsheetFontGroup(gtx layout.Context, state *documentSessionState, actions []spreadsheetToolbarAction) layout.Dimensions {
	var family, size *spreadsheetToolbarAction
	for i := range actions {
		switch actions[i].slot {
		case toolbarActionFontFamily:
			family = &actions[i]
		case toolbarActionFontSize:
			size = &actions[i]
		}
	}
	return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceBetween}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if family == nil {
						return layout.Dimensions{}
					}
					return a.layoutSpreadsheetRibbonAction(gtx, state, *family, 108, 34, spreadsheetRibbonActionField)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if size == nil {
						return layout.Dimensions{}
					}
					return a.layoutSpreadsheetRibbonAction(gtx, state, *size, 54, 34, spreadsheetRibbonActionField)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.layoutSpreadsheetRibbonPlaceholder(gtx, spreadsheetRibbonCell{label: "减小字号", icon: "a-arrow-up"}, 28, 34, spreadsheetRibbonActionIconOnly)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.layoutSpreadsheetRibbonPlaceholder(gtx, spreadsheetRibbonCell{label: "增大字号", icon: "a-arrow-down"}, 28, 34, spreadsheetRibbonActionIconOnly)
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			items := []spreadsheetRibbonCell{
				{slot: 4, hasAction: true, label: "加粗"},
				{slot: 5, hasAction: true, label: "斜体"},
				{slot: 6, hasAction: true, label: "下划线"},
				{label: "删除线", icon: "strikethrough"},
				{label: "文本高亮", icon: "highlighter"},
				{label: "字体颜色", icon: "baseline"},
			}
			return a.layoutSpreadsheetRibbonRow(gtx, state, actions, items, pxToRibbonDP(gtx, gtx.Constraints.Max.X), 34, spreadsheetRibbonActionIconOnly)
		}),
	)
}

func (a *App) layoutSpreadsheetAlignmentGroup(gtx layout.Context, state *documentSessionState, actions []spreadsheetToolbarAction) layout.Dimensions {
	contentWidthDP := pxToRibbonDP(gtx, gtx.Constraints.Max.X)
	sideWidthDP := min(112, max(1, contentWidthDP-152))
	primaryWidthDP := max(1, contentWidthDP-sideWidthDP)
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			width, height := gtx.Dp(unit.Dp(primaryWidthDP)), gtx.Dp(unit.Dp(72))
			gtx.Constraints = layout.Exact(image.Pt(width, height))
			firstRow := []spreadsheetRibbonCell{
				{label: "顶端对齐", icon: "align-start-vertical", iconFamily: "ribbon"},
				{label: "垂直居中", icon: "align-center-vertical"},
				{label: "底端对齐", icon: "align-end-vertical"},
				{label: "文字方向", icon: "text-wrap"},
			}
			secondRow := []spreadsheetRibbonCell{
				{slot: toolbarActionAlignLeft, hasAction: true},
				{slot: 9, hasAction: true},
				{slot: toolbarActionAlignRight, hasAction: true},
				{label: "两端对齐", icon: "text-align-justify"},
				{label: "筛选", icon: "list-filter"},
			}
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceBetween}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.layoutSpreadsheetRibbonRow(gtx, state, actions, firstRow, primaryWidthDP, 34, spreadsheetRibbonActionIconOnly)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.layoutSpreadsheetRibbonRow(gtx, state, actions, secondRow, primaryWidthDP, 34, spreadsheetRibbonActionIconOnly)
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			width, height := gtx.Dp(unit.Dp(sideWidthDP)), gtx.Dp(unit.Dp(72))
			gtx.Constraints = layout.Exact(image.Pt(width, height))
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceBetween}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.layoutSpreadsheetRibbonRow(gtx, state, actions, []spreadsheetRibbonCell{{slot: toolbarActionWrapText, hasAction: true}}, sideWidthDP, 34, spreadsheetRibbonActionCompact)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.layoutSpreadsheetRibbonPlaceholder(gtx, spreadsheetRibbonCell{label: "合并后居中", displayLabel: "合并后居中 ▾", icon: "merge"}, sideWidthDP, 34, spreadsheetRibbonActionCompact)
				}),
			)
		}),
	)
}

func (a *App) layoutSpreadsheetNumberGroup(gtx layout.Context, state *documentSessionState, actions []spreadsheetToolbarAction) layout.Dimensions {
	var numberFormat *spreadsheetToolbarAction
	for i := range actions {
		if actions[i].slot == 10 {
			numberFormat = &actions[i]
			break
		}
	}
	widthDP := pxToRibbonDP(gtx, gtx.Constraints.Max.X)
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if numberFormat == nil {
				return layout.Dimensions{}
			}
			return a.layoutSpreadsheetRibbonAction(gtx, state, *numberFormat, widthDP, 34, spreadsheetRibbonActionField)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			items := []spreadsheetRibbonCell{
				{label: "货币格式", icon: "banknote"},
				{slot: toolbarActionPercentFormat, hasAction: true},
				{label: "小数位数", icon: "hash"},
			}
			return a.layoutSpreadsheetRibbonRow(gtx, state, actions, items, widthDP, 34, spreadsheetRibbonActionIconOnly)
		}),
	)
}

func (a *App) layoutSpreadsheetStyleGroup(gtx layout.Context, state *documentSessionState, actions []spreadsheetToolbarAction) layout.Dimensions {
	widths := []int{80, 92, 80}
	children := make([]layout.FlexChild, 0, len(actions))
	for index, action := range actions {
		action, index := action, index
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			width := widths[min(index, len(widths)-1)]
			return a.layoutSpreadsheetRibbonAction(gtx, state, action, width, 68, spreadsheetRibbonActionVertical)
		}))
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceAround}.Layout(gtx, children...)
}

func (a *App) layoutSpreadsheetCompactActionStack(gtx layout.Context, state *documentSessionState, actions []spreadsheetToolbarAction) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(actions))
	rowHeight := 72 / max(1, len(actions))
	for _, action := range actions {
		action := action
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutSpreadsheetRibbonAction(gtx, state, action, pxToRibbonDP(gtx, gtx.Constraints.Max.X), rowHeight, spreadsheetRibbonActionCompact)
		}))
	}
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx, children...)
}

func spreadsheetHomeActionAnchorOffset(groups []spreadsheetRibbonGroup, slot int) int {
	groupStart := 0
	for _, group := range groups {
		for index, action := range group.actions {
			if action.slot != slot {
				continue
			}
			switch group.label {
			case "剪贴板":
				if slot == 21 {
					return groupStart
				}
				return groupStart + 68
			case "字体":
				if slot == toolbarActionFontFamily {
					return groupStart
				}
				if slot == toolbarActionFontSize {
					return groupStart + 108
				}
				return groupStart + 162 + (index-2)*30
			case "对齐":
				return groupStart + index*48
			case "数字":
				return groupStart + index*60
			case "样式":
				return groupStart + index*85
			case "单元格", "编辑", "历史":
				return groupStart
			}
		}
		groupStart += group.widthDP + 4
	}
	return 0
}
