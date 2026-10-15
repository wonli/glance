package desktopui

import (
	"image"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

const (
	wordRibbonCategoryHeightDP    = 34
	wordRibbonCategoryTabWidthDP  = 31
	wordRibbonCategoryTabHeightDP = 30
	wordRibbonCommandHeightDP     = 88
	wordRibbonHeightDP            = wordRibbonCategoryHeightDP + wordRibbonCommandHeightDP
	spreadsheetRibbonLeftInsetDP  = 16
)

type wordRibbonGroup struct {
	label   string
	actions []ribbonAction
	widthDP int
}

func wordRibbonGroupsForTab(tab wordToolbarTab, actions []ribbonAction) []wordRibbonGroup {
	groups := wordRibbonGroups(actions)
	if tab == wordTabHome {
		paragraphActions := make(map[string]ribbonAction)
		for _, action := range actions {
			if action.group == "段落" {
				paragraphActions[action.id] = action
			}
		}
		paragraphOrder := []ribbonAction{
			paragraphActions["bullet-list"],
			paragraphActions["numbered-list"],
			{id: "multilevel-list", label: "多级列表", group: "段落", icon: "list"},
			paragraphActions["decrease-indent"],
			paragraphActions["increase-indent"],
			paragraphActions["align-left"],
			paragraphActions["align-center"],
			paragraphActions["align-right"],
			paragraphActions["align-justify"],
			{id: "paragraph-filter", label: "筛选", group: "段落", icon: "list-filter"},
		}
		for index := range groups {
			if groups[index].label == "段落" {
				groups[index].actions = paragraphOrder
				break
			}
		}
		visibleGroups := groups[:0]
		for _, group := range groups {
			if group.label != "编辑器" {
				visibleGroups = append(visibleGroups, group)
			}
		}
		groups = visibleGroups
		insert := len(groups)
		for i, group := range groups {
			if group.label == "编辑" {
				insert = i
				break
			}
		}
		groups = append(groups, wordRibbonGroup{})
		copy(groups[insert+1:], groups[insert:])
		groups[insert] = wordRibbonGroup{
			label: "样式",
			actions: []ribbonAction{
				{id: "style-body", label: "正文", group: "样式"},
				{id: "style-heading", label: "标题 1", group: "样式"},
				{id: "style-heading-2", label: "标题 2", group: "样式"},
			},
		}
	}
	if tab == wordTabHome {
		widths := map[string]int{"剪贴板": 155, "字体": 231, "段落": 176, "样式": 263, "编辑": 66, "编辑器": 230}
		for i := range groups {
			groups[i].widthDP = widths[groups[i].label]
		}
	} else {
		for i := range groups {
			groups[i].widthDP = wordRibbonGroupWidth(groups[i].label)
		}
	}
	return groups
}

func wordRibbonActionAnchorOffset(groups []wordRibbonGroup, actionID string) int {
	groupStart := 0
	for _, group := range groups {
		for index, action := range group.actions {
			if action.id != actionID {
				continue
			}
			switch group.label {
			case "剪贴板":
				if index == 0 {
					return groupStart
				}
				return groupStart + 84
			case "字体":
				switch action.id {
				case "font-family":
					return groupStart
				case "font-size-decrease":
					return groupStart + 170
				case "font-size-increase":
					return groupStart + 198
				case "bold":
					return groupStart
				case "italic":
					return groupStart + 26
				case "underline":
					return groupStart + 52
				case "text-highlight":
					return groupStart + 78
				case "font-color":
					return groupStart + 104
				}
			case "段落":
				return groupStart + (index%5)*30
			case "编辑", "编辑器":
				return groupStart
			default:
				return groupStart + index*44
			}
		}
		width := group.widthDP
		if width <= 0 {
			width = wordRibbonGroupWidth(group.label)
		}
		groupStart += width
		if groupStart > 0 {
			groupStart += 4
		}
	}
	return 0
}

func (a *App) layoutWordRibbon(gtx layout.Context) layout.Dimensions {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return layout.Dimensions{}
	}
	if state.wordToolbarTab == wordTabPicture && state.selectedWordImageID == "" {
		state.wordToolbarTab = wordTabHome
	}
	state.wordToolbarActionList.List.Axis = layout.Horizontal
	state.wordToolbarGroupList.List.Axis = layout.Horizontal
	state.wordToolbarTabList.List.Axis = layout.Horizontal
	height := gtx.Dp(unit.Dp(wordRibbonHeightDP))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, a.tokens().Colors.Surface.Surface, clip.Rect{Max: gtx.Constraints.Max}.Op())
			bounds := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
			semantic.DescriptionOp("Word 功能区").Add(gtx.Ops)
			bounds.Pop()
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.layoutWordRibbonCategories(gtx, state)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.layoutWordRibbonCommands(gtx, state)
				}),
			)
		},
	)
}

func (a *App) layoutWordRibbonCategories(gtx layout.Context, state *documentSessionState) layout.Dimensions {
	height := gtx.Dp(unit.Dp(wordRibbonCategoryHeightDP))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	tabs := wordToolbarTabs(state)
	paint.FillShape(gtx.Ops, a.tokens().Colors.Surface.Surface, clip.Rect{Max: gtx.Constraints.Max}.Op())
	for index, tab := range tabs {
		x := 22 + index*(wordRibbonCategoryTabWidthDP+28)
		a.layoutWordRibbonCategoryTabAt(gtx, state, index, x, 2, wordToolbarTabLabels[tab], tab == state.wordToolbarTab)
	}
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func (a *App) layoutWordRibbonCategoryTabAt(gtx layout.Context, state *documentSessionState, index int, xDP, yDP int, label string, selected bool) {
	width, height := gtx.Dp(unit.Dp(wordRibbonCategoryTabWidthDP)), gtx.Dp(unit.Dp(wordRibbonCategoryTabHeightDP))
	x, y := gtx.Dp(unit.Dp(xDP)), gtx.Dp(unit.Dp(yDP))
	transform := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	buttonGtx := gtx
	buttonGtx.Constraints = layout.Constraints{Min: image.Pt(width, height), Max: image.Pt(width, height)}
	state.wordToolbarTabs[index].Layout(buttonGtx, func(gtx layout.Context) layout.Dimensions {
		bounds := clip.Rect{Max: image.Pt(width, height)}.Push(gtx.Ops)
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(label).Add(gtx.Ops)
		bounds.Pop()
		style := material.Body2(a.theme, label)
		style.TextSize = unit.Sp(13)
		style.Color = a.tokens().Colors.Text.PureBlack
		shift := op.Offset(image.Pt(0, gtx.Dp(unit.Dp(3)))).Push(gtx.Ops)
		layout.Center.Layout(gtx, style.Layout)
		shift.Pop()
		return layout.Dimensions{Size: image.Pt(width, height)}
	})
	transform.Pop()
	if selected {
		lineX, lineY := gtx.Dp(unit.Dp(xDP)), gtx.Dp(unit.Dp(yDP+wordRibbonCategoryTabHeightDP-1))
		line := op.Offset(image.Pt(lineX, lineY)).Push(gtx.Ops)
		paint.FillShape(gtx.Ops, a.tokens().Colors.Text.Brand, clip.Rect{Max: image.Pt(width, gtx.Dp(unit.Dp(1)))}.Op())
		line.Pop()
	}
}

func (a *App) layoutWordRibbonCommands(gtx layout.Context, state *documentSessionState) layout.Dimensions {
	height := gtx.Dp(unit.Dp(wordRibbonCommandHeightDP))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, a.tokens().Colors.Toolbar.Background, clip.Rect{Max: gtx.Constraints.Max}.Op())
			paint.FillShape(gtx.Ops, a.tokens().Colors.Toolbar.Divider, clip.Rect{Min: image.Pt(0, height-gtx.Dp(unit.Dp(1))), Max: image.Pt(gtx.Constraints.Max.X, height)}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return a.layoutWordRibbonActions(gtx, state, state.wordToolbarTab)
		},
	)
}

func (a *App) layoutWordHomeRibbonContent(gtx layout.Context, state *documentSessionState) layout.Dimensions {
	return a.layoutWordRibbonActions(gtx, state, wordTabHome)
}

func (a *App) layoutWordHomeRibbonControls(gtx layout.Context, state *documentSessionState) layout.Dimensions {
	return a.layoutWordRibbonActions(gtx, state, wordTabHome)
}

var (
	wordPenActionPositions = make(map[wordToolbarTab][]int)
	wordPenGroupDividers   = make(map[wordToolbarTab][]int)
)

func init() {
	allTabs := []wordToolbarTab{
		wordTabFile,
		wordTabHome,
		wordTabInsert,
		wordTabDesign,
		wordTabLayout,
		wordTabReferences,
		wordTabReview,
		wordTabView,
		wordTabPicture,
	}
	buttonWidth := int(toolbarButtonWidth)
	gap := int(toolbarItemGap)
	dividerMargin := 12
	dividerWidth := 1
	for _, tab := range allTabs {
		actions := wordToolbarActions(tab)
		if len(actions) == 0 {
			continue
		}
		positions := make([]int, len(actions))
		var dividers []int
		x := 34
		for i, action := range actions {
			if i > 0 && action.group != actions[i-1].group {
				dividerX := x - gap + dividerMargin
				dividers = append(dividers, dividerX)
				x = dividerX + dividerWidth + dividerMargin
			}
			positions[i] = x
			x += buttonWidth + gap
		}
		wordPenActionPositions[tab] = positions
		wordPenGroupDividers[tab] = dividers
	}
}
