package desktopui

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

const (
	wordRibbonCategoryHeightDP = 34
	wordRibbonCommandHeightDP  = 56
	wordRibbonHeightDP         = wordRibbonCategoryHeightDP + wordRibbonCommandHeightDP
)

type wordRibbonGroup struct {
	label   string
	actions []ribbonAction
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
	state.wordToolbarTabList.List.Axis = layout.Horizontal
	height := gtx.Dp(unit.Dp(wordRibbonHeightDP))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, contentSurface, clip.Rect{Max: gtx.Constraints.Max}.Op())
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
	paint.FillShape(gtx.Ops, contentSurface, clip.Rect{Max: gtx.Constraints.Max}.Op())
	paint.FillShape(gtx.Ops, wordStatusTopRule, clip.Rect{Min: image.Pt(0, height-gtx.Dp(unit.Dp(1))), Max: image.Pt(gtx.Constraints.Max.X, height)}.Op())
	for index, tab := range tabs {
		x := 22 + index*59
		a.layoutWordRibbonCategoryTabAt(gtx, state, index, x, 2, wordToolbarTabLabels[tab], tab == state.wordToolbarTab)
	}
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func (a *App) layoutWordRibbonCategoryTabAt(gtx layout.Context, state *documentSessionState, index int, xDP, yDP int, label string, selected bool) {
	width, height := gtx.Dp(unit.Dp(31)), gtx.Dp(unit.Dp(30))
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
		style.Color = color.NRGBA{R: 72, G: 81, B: 92, A: 255}
		if selected {
			style.Color = color.NRGBA{R: 52, G: 91, B: 146, A: 255}
		}
		layout.Center.Layout(gtx, style.Layout)
		return layout.Dimensions{Size: image.Pt(width, height)}
	})
	transform.Pop()
	if selected {
		lineX, lineY := gtx.Dp(unit.Dp(xDP-5)), gtx.Dp(unit.Dp(wordRibbonCategoryHeightDP-2))
		line := op.Offset(image.Pt(lineX, lineY)).Push(gtx.Ops)
		paint.FillShape(gtx.Ops, color.NRGBA{R: 52, G: 91, B: 146, A: 255}, clip.Rect{Max: image.Pt(gtx.Dp(unit.Dp(42)), gtx.Dp(unit.Dp(2)))}.Op())
		line.Pop()
	}
}

func (a *App) layoutWordRibbonCommands(gtx layout.Context, state *documentSessionState) layout.Dimensions {
	height := gtx.Dp(unit.Dp(wordRibbonCommandHeightDP))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, contentSurface, clip.Rect{Max: gtx.Constraints.Max}.Op())
			paint.FillShape(gtx.Ops, wordStatusTopRule, clip.Rect{Min: image.Pt(0, height-gtx.Dp(unit.Dp(1))), Max: image.Pt(gtx.Constraints.Max.X, height)}.Op())
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

var wordPenActionPositions = map[wordToolbarTab][]int{
	wordTabFile:    {34, 76, 118, 179, 221, 263, 324, 366, 408, 469, 511, 553},
	wordTabHome:    {34, 76, 118, 160, 221, 263, 305, 347, 389, 431, 473, 515, 576, 618, 660, 702, 744, 786, 828, 870, 931, 973, 1015, 1076, 1118, 1160, 1202},
	wordTabInsert:  {34, 76, 118, 179, 240, 282, 324, 385, 427, 488, 530, 572, 633, 675, 736, 778},
	wordTabDesign:  {34, 76, 118, 160, 221, 263, 324, 366, 408},
	wordTabLayout:  {34, 76, 118, 160, 221, 263, 324, 366, 408, 469, 511, 553},
	wordTabReview:  {34, 76, 137, 179, 240, 282, 343, 385, 427, 488, 530},
	wordTabView:    {34, 76, 118, 179, 221, 263, 324, 366, 408, 469, 511, 572},
	wordTabPicture: {34, 76, 118, 179, 221},
}

var wordPenGroupDividers = map[wordToolbarTab][]int{
	wordTabFile:   {166, 311, 456},
	wordTabHome:   {208, 563, 918, 1063},
	wordTabInsert: {166, 227, 372, 475, 620, 723},
	wordTabDesign: {208, 311},
	wordTabLayout: {208, 311, 456},
	wordTabReview: {124, 227, 330, 475},
	wordTabView:   {166, 311, 456, 559},
}

func (a *App) layoutWordRibbonActions(gtx layout.Context, state *documentSessionState, tab wordToolbarTab) layout.Dimensions {
	width, height := gtx.Constraints.Max.X, gtx.Dp(unit.Dp(wordRibbonCommandHeightDP))
	for _, x := range wordPenGroupDividers[tab] {
		px := gtx.Dp(unit.Dp(x))
		paint.FillShape(gtx.Ops, wordStatusTopRule, clip.Rect{Min: image.Pt(px, gtx.Dp(unit.Dp(6))), Max: image.Pt(px+gtx.Dp(unit.Dp(1)), gtx.Dp(unit.Dp(50)))}.Op())
	}
	actions := wordToolbarActions(tab)
	positions := wordPenActionPositions[tab]
	for index, action := range actions {
		if index >= len(positions) {
			break
		}
		a.layoutWordRibbonIconButtonAt(gtx, state, action, positions[index])
	}
	return layout.Dimensions{Size: image.Pt(width, height)}
}

func (a *App) layoutWordRibbonIconButtonAt(gtx layout.Context, state *documentSessionState, action ribbonAction, xDP int) {
	width, height := gtx.Dp(unit.Dp(36)), gtx.Dp(unit.Dp(36))
	x, y := gtx.Dp(unit.Dp(xDP)), gtx.Dp(unit.Dp(10))
	transform := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	buttonGtx := gtx
	buttonGtx.Constraints = layout.Constraints{Min: image.Pt(width, height), Max: image.Pt(width, height)}
	enabled := a.wordToolbarActionEnabled(action)
	button := wordToolbarButton(state, action.id)
	draw := func(gtx layout.Context) layout.Dimensions {
		bounds := clip.Rect{Max: image.Pt(width, height)}.Push(gtx.Ops)
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(action.label).Add(gtx.Ops)
		semantic.EnabledOp(enabled).Add(gtx.Ops)
		bounds.Pop()
		if active := enabled && wordToolbarActionActive(state, action); active || (enabled && button.Hovered()) {
			background := toolbarActiveBackground
			if button.Hovered() {
				background = toolbarHoverBackground
			}
			paint.FillShape(gtx.Ops, background, clip.UniformRRect(image.Rectangle{Max: image.Pt(width, height)}, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
		}
		if icon, ok := wordRibbonIcon(action.icon, enabled); ok {
			iconSize := gtx.Dp(unit.Dp(20))
			iconGtx := gtx
			iconGtx.Constraints = layout.Constraints{Min: image.Pt(iconSize, iconSize), Max: image.Pt(iconSize, iconSize)}
			iconOffset := op.Offset(image.Pt(gtx.Dp(unit.Dp(8)), gtx.Dp(unit.Dp(8)))).Push(gtx.Ops)
			widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: 20.0 / 96.0}.Layout(iconGtx)
			iconOffset.Pop()
		}
		return layout.Dimensions{Size: image.Pt(width, height)}
	}
	if enabled {
		button.Layout(buttonGtx, draw)
	} else {
		draw(buttonGtx)
	}
	transform.Pop()
}

func (a *App) layoutWordRibbonPasteButtonAt(gtx layout.Context, state *documentSessionState, action ribbonAction, xDP, yDP int) {
	x, y := gtx.Dp(unit.Dp(xDP)), gtx.Dp(unit.Dp(yDP))
	width, height := gtx.Dp(unit.Dp(70)), gtx.Dp(unit.Dp(62))
	transform := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	child := gtx
	child.Constraints = layout.Constraints{Min: image.Pt(width, height), Max: image.Pt(width, height)}
	enabled := a.wordToolbarActionEnabled(action)
	button := wordToolbarButton(state, action.id)
	draw := func(gtx layout.Context) layout.Dimensions {
		bounds := clip.Rect{Max: image.Pt(width, height)}.Push(gtx.Ops)
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(action.label).Add(gtx.Ops)
		semantic.EnabledOp(enabled).Add(gtx.Ops)
		bounds.Pop()
		background := contentSurface
		if enabled && (button.Hovered() || gtx.Focused(button)) {
			background = toolbarHoverBackground
		}
		paint.FillShape(gtx.Ops, background, clip.UniformRRect(image.Rectangle{Max: image.Pt(width, height)}, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
		icon, _ := tintedToolbarIcon(action.icon, toolbarIconColor(enabled))
		iconSize := gtx.Dp(unit.Dp(16))
		iconOffset := op.Offset(image.Pt(gtx.Dp(unit.Dp(10)), gtx.Dp(unit.Dp(31)))).Push(gtx.Ops)
		iconGtx := gtx
		iconGtx.Constraints = layout.Constraints{Min: image.Pt(iconSize, iconSize), Max: image.Pt(iconSize, iconSize)}
		if icon != nil {
			widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: 16.0 / 96.0}.Layout(iconGtx)
		}
		iconOffset.Pop()
		label := material.Caption(a.theme, action.label)
		label.TextSize = unit.Sp(10)
		label.Color = toolbarIconColor(enabled)
		labelOffset := op.Offset(image.Pt(gtx.Dp(unit.Dp(32)), gtx.Dp(unit.Dp(30)))).Push(gtx.Ops)
		label.Layout(gtx)
		labelOffset.Pop()
		arrow := material.Caption(a.theme, "⌄")
		arrow.TextSize = unit.Sp(10)
		arrow.Color = toolbarIconColor(enabled)
		arrowOffset := op.Offset(image.Pt(gtx.Dp(unit.Dp(54)), gtx.Dp(unit.Dp(47)))).Push(gtx.Ops)
		arrow.Layout(gtx)
		arrowOffset.Pop()
		return layout.Dimensions{Size: image.Pt(width, height)}
	}
	if enabled {
		button.Layout(child, draw)
	} else {
		draw(child)
	}
	transform.Pop()
}

func (a *App) layoutWordRibbonIconLabelActionAt(gtx layout.Context, state *documentSessionState, action ribbonAction, xDP, yDP int) {
	x, y := gtx.Dp(unit.Dp(xDP)), gtx.Dp(unit.Dp(yDP))
	width, height := gtx.Dp(unit.Dp(52)), gtx.Dp(unit.Dp(62))
	transform := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	child := gtx
	child.Constraints = layout.Constraints{Min: image.Pt(width, height), Max: image.Pt(width, height)}
	enabled := a.wordToolbarActionEnabled(action)
	button := wordToolbarButton(state, action.id)
	draw := func(gtx layout.Context) layout.Dimensions {
		bounds := clip.Rect{Max: image.Pt(width, height)}.Push(gtx.Ops)
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(action.label).Add(gtx.Ops)
		semantic.EnabledOp(enabled).Add(gtx.Ops)
		bounds.Pop()
		background := contentSurface
		if enabled && (button.Hovered() || gtx.Focused(button)) {
			background = toolbarHoverBackground
		}
		paint.FillShape(gtx.Ops, background, clip.UniformRRect(image.Rectangle{Max: image.Pt(width, height)}, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
		icon, _ := tintedToolbarIcon(action.icon, toolbarIconColor(enabled))
		iconSize := gtx.Dp(unit.Dp(22))
		iconOffset := op.Offset(image.Pt(gtx.Dp(unit.Dp(4)), gtx.Dp(unit.Dp(6)))).Push(gtx.Ops)
		iconGtx := gtx
		iconGtx.Constraints = layout.Constraints{Min: image.Pt(iconSize, iconSize), Max: image.Pt(iconSize, iconSize)}
		if icon != nil {
			widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: 22.0 / 96.0}.Layout(iconGtx)
		}
		iconOffset.Pop()
		label := material.Caption(a.theme, action.label)
		label.TextSize = unit.Sp(9)
		label.Color = toolbarIconColor(enabled)
		labelOffset := op.Offset(image.Pt(0, gtx.Dp(unit.Dp(34)))).Push(gtx.Ops)
		label.Layout(gtx)
		labelOffset.Pop()
		return layout.Dimensions{Size: image.Pt(width, height)}
	}
	if enabled {
		button.Layout(child, draw)
	} else {
		draw(child)
	}
	transform.Pop()
}

func (a *App) layoutWordRibbonActionAt(gtx layout.Context, state *documentSessionState, action ribbonAction, xDP, yDP, widthDP, heightDP int, showLabel bool) {
	x, y := gtx.Dp(unit.Dp(xDP)), gtx.Dp(unit.Dp(yDP))
	width, height := gtx.Dp(unit.Dp(widthDP)), gtx.Dp(unit.Dp(heightDP))
	transform := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	child := gtx
	child.Constraints = layout.Constraints{Min: image.Pt(width, height), Max: image.Pt(width, height)}
	a.layoutWordRibbonAction(child, state, action, widthDP, heightDP, showLabel)
	transform.Pop()
}

func (a *App) layoutWordRibbonInlineActionAt(gtx layout.Context, state *documentSessionState, action ribbonAction, xDP, yDP, widthDP, heightDP, iconDP int) {
	x, y := gtx.Dp(unit.Dp(xDP)), gtx.Dp(unit.Dp(yDP))
	width, height := gtx.Dp(unit.Dp(widthDP)), gtx.Dp(unit.Dp(heightDP))
	transform := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	child := gtx
	child.Constraints = layout.Constraints{Min: image.Pt(width, height), Max: image.Pt(width, height)}
	a.layoutWordRibbonInlineAction(child, state, action, widthDP, heightDP, iconDP)
	transform.Pop()
}

func (a *App) layoutWordRibbonInlineAction(gtx layout.Context, state *documentSessionState, action ribbonAction, widthDP, heightDP, iconDP int) layout.Dimensions {
	width, height := gtx.Dp(unit.Dp(widthDP)), gtx.Dp(unit.Dp(heightDP))
	gtx.Constraints = layout.Constraints{Min: image.Pt(width, height), Max: image.Pt(width, height)}
	enabled := a.wordToolbarActionEnabled(action)
	button := wordToolbarButton(state, action.id)
	draw := func(gtx layout.Context) layout.Dimensions {
		bounds := clip.Rect{Max: image.Pt(width, height)}.Push(gtx.Ops)
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(action.label).Add(gtx.Ops)
		semantic.EnabledOp(enabled).Add(gtx.Ops)
		bounds.Pop()
		background := contentSurface
		if enabled && (button.Hovered() || gtx.Focused(button)) {
			background = toolbarHoverBackground
		}
		paint.FillShape(gtx.Ops, background, clip.UniformRRect(image.Rectangle{Max: image.Pt(width, height)}, gtx.Dp(unit.Dp(2))).Op(gtx.Ops))
		icon, _ := tintedToolbarIcon(action.icon, toolbarIconColor(enabled))
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceStart}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				iconSize := gtx.Dp(unit.Dp(iconDP))
				gtx.Constraints.Min = image.Pt(iconSize, iconSize)
				gtx.Constraints.Max = gtx.Constraints.Min
				if icon == nil {
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}
				return widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: float32(iconDP) / 96}.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(7)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := material.Caption(a.theme, action.label)
				label.TextSize = unit.Sp(9)
				label.Color = toolbarIconColor(enabled)
				return label.Layout(gtx)
			}),
		)
	}
	if enabled {
		return button.Layout(gtx, draw)
	}
	return draw(gtx)
}

func (a *App) layoutWordRibbonFieldAt(gtx layout.Context, action ribbonAction, xDP, yDP, widthDP int, value string) {
	x, y := gtx.Dp(unit.Dp(xDP)), gtx.Dp(unit.Dp(yDP))
	width, height := gtx.Dp(unit.Dp(widthDP)), gtx.Dp(unit.Dp(28))
	transform := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	child := gtx
	child.Constraints = layout.Constraints{Min: image.Pt(width, height), Max: image.Pt(width, height)}
	a.layoutWordRibbonField(child, action, widthDP, value)
	transform.Pop()
}

func (a *App) layoutWordStyleGalleryAt(gtx layout.Context, state *documentSessionState, actions []ribbonAction, xDP, yDP, widthDP, heightDP int) {
	x, y := gtx.Dp(unit.Dp(xDP)), gtx.Dp(unit.Dp(yDP))
	width, height := gtx.Dp(unit.Dp(widthDP)), gtx.Dp(unit.Dp(heightDP))
	transform := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	child := gtx
	child.Constraints = layout.Constraints{Min: image.Pt(width, height), Max: image.Pt(width, height)}
	a.layoutWordStyleGallery(child, actions)
	transform.Pop()
}

func wordRibbonGroups(actions []ribbonAction) []wordRibbonGroup {
	var groups []wordRibbonGroup
	for _, action := range actions {
		if len(groups) == 0 || groups[len(groups)-1].label != action.group {
			groups = append(groups, wordRibbonGroup{label: action.group})
		}
		groups[len(groups)-1].actions = append(groups[len(groups)-1].actions, action)
	}
	return groups
}

func wordRibbonGroupWidth(label string) int {
	switch label {
	case "剪贴板":
		return 158
	case "字体":
		return 442
	case "段落":
		return 350
	case "样式":
		return 132
	case "编辑":
		return 112
	case "编辑器":
		return 230
	case "历史":
		return 88
	case "文档":
		return 104
	case "插图":
		return 192
	case "图片":
		return 362
	default:
		return 176
	}
}

func (a *App) layoutWordRibbonGroup(gtx layout.Context, state *documentSessionState, group wordRibbonGroup, last bool) layout.Dimensions {
	width := gtx.Dp(unit.Dp(wordRibbonGroupWidth(group.label)))
	height := gtx.Dp(unit.Dp(wordRibbonCommandHeightDP - 9))
	gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(width, height), image.Pt(width, height)
	return layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			if last {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			}
			x := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(1))
			paint.FillShape(gtx.Ops, ribbonDividerColor, clip.Rect{Min: image.Pt(x, gtx.Dp(unit.Dp(5))), Max: image.Pt(x+gtx.Dp(unit.Dp(1)), gtx.Constraints.Max.Y-gtx.Dp(unit.Dp(14)))}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(7), Top: unit.Dp(2), Bottom: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceBetween}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return a.layoutWordRibbonGroupContent(gtx, state, group)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						style := material.Caption(a.theme, group.label)
						style.Color = color.NRGBA{R: 111, G: 119, B: 128, A: 255}
						return layout.Center.Layout(gtx, style.Layout)
					}),
				)
			})
		}),
	)
}

func (a *App) layoutWordRibbonGroupContent(gtx layout.Context, state *documentSessionState, group wordRibbonGroup) layout.Dimensions {
	switch group.label {
	case "剪贴板":
		return a.layoutWordClipboardRibbonGroup(gtx, state, group.actions)
	case "字体":
		return a.layoutWordFontRibbonGroup(gtx, state, group.actions)
	case "段落":
		return a.layoutWordRibbonActionRow(gtx, state, group.actions, 36, 28, false)
	case "样式":
		return a.layoutWordStyleGallery(gtx, group.actions)
	case "编辑":
		return a.layoutWordRibbonActionRow(gtx, state, group.actions, 38, 32, true)
	case "编辑器":
		return a.layoutWordRibbonActionRow(gtx, state, group.actions, 49, 52, true)
	case "历史":
		return a.layoutWordRibbonActionRow(gtx, state, group.actions, 32, 44, false)
	case "文档":
		return a.layoutWordRibbonActionRow(gtx, state, group.actions, 68, 56, true)
	default:
		if len(group.actions) <= 3 {
			return a.layoutWordRibbonActionRow(gtx, state, group.actions, 48, 58, true)
		}
		buttonWidth := max(24, (wordRibbonGroupWidth(group.label)-18)/len(group.actions))
		return a.layoutWordRibbonActionRow(gtx, state, group.actions, 42, buttonWidth, false)
	}
}

func (a *App) layoutWordClipboardRibbonGroup(gtx layout.Context, state *documentSessionState, actions []ribbonAction) layout.Dimensions {
	if len(actions) == 0 {
		return layout.Dimensions{}
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutWordRibbonAction(gtx, state, actions[0], 52, 56, true)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutWordRibbonActionRow(gtx, state, actions[1:], 36, 29, false)
		}),
	)
}

func (a *App) layoutWordFontRibbonGroup(gtx layout.Context, state *documentSessionState, actions []ribbonAction) layout.Dimensions {
	var fields, marks []ribbonAction
	for _, action := range actions {
		if action.id == "font-family" || action.id == "font-size" {
			fields = append(fields, action)
		} else {
			marks = append(marks, action)
		}
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceEnd}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if len(fields) == 0 {
				return layout.Dimensions{}
			}
			return a.layoutWordRibbonField(gtx, fields[0], 146, "等线 (正文)")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if len(fields) < 2 {
				return layout.Dimensions{}
			}
			return a.layoutWordRibbonField(gtx, fields[1], 54, "11")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutWordRibbonActionRow(gtx, state, marks, 28, 26, false)
		}),
	)
}

func (a *App) layoutWordRibbonActionRow(gtx layout.Context, state *documentSessionState, actions []ribbonAction, heightDP, widthDP int, labels bool) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(actions))
	for _, action := range actions {
		action := action
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutWordRibbonAction(gtx, state, action, widthDP, heightDP, labels)
		}))
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceAround}.Layout(gtx, children...)
}

func (a *App) layoutWordRibbonAction(gtx layout.Context, state *documentSessionState, action ribbonAction, widthDP, heightDP int, showLabel bool) layout.Dimensions {
	width, height := gtx.Dp(unit.Dp(widthDP)), gtx.Dp(unit.Dp(heightDP))
	gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(width, height), image.Pt(width, height)
	enabled := a.wordToolbarActionEnabled(action)
	button := wordToolbarButton(state, action.id)
	draw := func(gtx layout.Context) layout.Dimensions {
		semantics := clip.Rect{Max: image.Pt(width, height)}.Push(gtx.Ops)
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(action.label).Add(gtx.Ops)
		semantic.EnabledOp(enabled).Add(gtx.Ops)
		semantics.Pop()
		background := contentSurface
		if enabled && wordToolbarActionActive(state, action) {
			background = toolbarActiveBackground
		}
		if enabled && (button.Hovered() || gtx.Focused(button)) {
			background = toolbarHoverBackground
		}
		paint.FillShape(gtx.Ops, background, clip.UniformRRect(image.Rectangle{Max: image.Pt(width, height)}, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
		if action.id == "font-family" || action.id == "font-size" {
			value := "等线 (正文)"
			if action.id == "font-size" {
				value = "11"
			}
			return drawWordRibbonDropdown(gtx, a.theme, action, width, height, value)
		}
		icon, _ := tintedToolbarIcon(action.icon, toolbarIconColor(enabled))
		if showLabel {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle, Spacing: layout.SpaceAround}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if icon == nil {
						return layout.Dimensions{}
					}
					iconSize := min(gtx.Dp(unit.Dp(22)), min(width-gtx.Dp(unit.Dp(4)), height-gtx.Dp(unit.Dp(18))))
					if iconSize <= 0 {
						return layout.Dimensions{}
					}
					iconGtx := gtx
					iconGtx.Constraints.Min = image.Pt(iconSize, iconSize)
					iconGtx.Constraints.Max = iconGtx.Constraints.Min
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: float32(iconSize) / 96.0}.Layout(iconGtx)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := material.Caption(a.theme, action.label)
					label.TextSize = unit.Sp(9)
					label.Color = toolbarIconColor(enabled)
					return layout.Inset{Left: unit.Dp(1), Right: unit.Dp(1)}.Layout(gtx, label.Layout)
				}),
			)
		}
		if icon == nil {
			return layout.Dimensions{Size: image.Pt(width, height)}
		}
		iconSize := min(gtx.Dp(unit.Dp(18)), min(width-gtx.Dp(unit.Dp(4)), height-gtx.Dp(unit.Dp(4))))
		if iconSize <= 0 {
			return layout.Dimensions{Size: image.Pt(width, height)}
		}
		iconGtx := gtx
		iconGtx.Constraints.Min = image.Pt(iconSize, iconSize)
		iconGtx.Constraints.Max = iconGtx.Constraints.Min
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: float32(iconSize) / 96.0}.Layout(iconGtx)
		})
	}
	if enabled {
		return button.Layout(gtx, draw)
	}
	return draw(gtx)
}

func (a *App) layoutWordRibbonField(gtx layout.Context, action ribbonAction, widthDP int, value string) layout.Dimensions {
	width, height := gtx.Dp(unit.Dp(widthDP)), gtx.Dp(unit.Dp(28))
	gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(width, height), image.Pt(width, height)
	paint.FillShape(gtx.Ops, ribbonDividerColor, clip.UniformRRect(image.Rectangle{Max: image.Pt(width, height)}, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
	return drawWordRibbonDropdown(gtx, a.theme, action, width, height, value)
}

func drawWordRibbonDropdown(gtx layout.Context, theme *material.Theme, action ribbonAction, width, height int, value string) layout.Dimensions {
	bounds := clip.Rect{Max: image.Pt(width, height)}.Push(gtx.Ops)
	semantic.DescriptionOp(action.label).Add(gtx.Ops)
	semantic.EnabledOp(false).Add(gtx.Ops)
	bounds.Pop()
	paint.FillShape(gtx.Ops, contentSurface, clip.UniformRRect(image.Rectangle{Max: image.Pt(width, height)}, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
	textColor := disabledToolbarText
	style := material.Caption(theme, value)
	style.TextSize = unit.Sp(10)
	style.Color = textColor
	return layout.Inset{Left: unit.Dp(6), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, style.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				chevron := material.Caption(theme, "⌄")
				chevron.Color = textColor
				return chevron.Layout(gtx)
			}),
		)
	})
}

func (a *App) layoutWordStyleGallery(gtx layout.Context, actions []ribbonAction) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(actions))
	for _, action := range actions {
		action := action
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			width, height := gtx.Dp(unit.Dp(38)), gtx.Dp(unit.Dp(56))
			gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(width, height), image.Pt(width, height)
			paint.FillShape(gtx.Ops, contentSurface, clip.UniformRRect(image.Rectangle{Max: image.Pt(width, height)}, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
			bounds := clip.Rect{Max: image.Pt(width, height)}.Push(gtx.Ops)
			semantic.DescriptionOp(action.label).Add(gtx.Ops)
			semantic.EnabledOp(false).Add(gtx.Ops)
			bounds.Pop()
			preview := material.Body1(a.theme, "Aa")
			preview.TextSize = unit.Sp(14)
			preview.Color = disabledToolbarText
			if action.id == "style-heading" {
				preview.Font.Weight = font.Bold
			}
			label := material.Caption(a.theme, action.label)
			label.TextSize = unit.Sp(8)
			label.Color = disabledToolbarText
			previewGtx := gtx
			previewHeight := gtx.Dp(unit.Dp(18))
			previewGtx.Constraints = layout.Constraints{Min: image.Pt(width, previewHeight), Max: image.Pt(width, previewHeight)}
			previewOffset := op.Offset(image.Pt(0, gtx.Dp(unit.Dp(8)))).Push(gtx.Ops)
			layout.Center.Layout(previewGtx, preview.Layout)
			previewOffset.Pop()
			labelGtx := gtx
			labelHeight := gtx.Dp(unit.Dp(14))
			labelGtx.Constraints = layout.Constraints{Min: image.Pt(width, labelHeight), Max: image.Pt(width, labelHeight)}
			labelOffset := op.Offset(image.Pt(0, gtx.Dp(unit.Dp(30)))).Push(gtx.Ops)
			layout.Center.Layout(labelGtx, label.Layout)
			labelOffset.Pop()
			return layout.Dimensions{Size: image.Pt(width, height)}
		}))
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceAround}.Layout(gtx, children...)
}
