package desktopui

import (
	"image"
	"image/color"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func (a *App) layoutWordRibbonActions(gtx layout.Context, state *documentSessionState, tab wordToolbarTab) layout.Dimensions {
	actions := wordToolbarActions(tab)
	iconSize := toolbarIconSize
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(11), Right: unit.Dp(5)}.Layout(gtx, a.layoutAgentRibbonEntry)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				// Keep short command lists full-width so Background does not center them.
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return state.wordToolbarActionList.Layout(gtx, len(actions), func(gtx layout.Context, index int) layout.Dimensions {
					action := actions[index]
					enabled := a.wordToolbarActionButtonEnabled(action)
					button := wordToolbarButton(state, action.id)
					description := a.wordToolbarActionDescription(action)
					if action.id == "view-new-window" && !enabled {
						if _, reason := wordNewWindowAvailability(state, a.desktopRuntime); reason != "" {
							description += "，不可用：" + reason
						}
					}
					active := enabled && wordToolbarActionActive(state, action)
					background := a.tokens().Colors.Surface.Surface
					if active {
						background = a.tokens().Colors.Toolbar.ActiveBackground
					}
					icon, _ := a.themedWordToolbarActionIcon(action, enabled)
					return layoutWordToolbarActionItem(gtx, action, index, actions, a.tokens().Colors.Toolbar.Divider, func(gtx layout.Context) layout.Dimensions {
						btnGtx := gtx
						btnGtx.Constraints.Min = image.Pt(gtx.Dp(toolbarButtonWidth), gtx.Dp(toolbarButtonHeight))
						btnGtx.Constraints.Max = btnGtx.Constraints.Min
						return toolbarIconButtonWithColors(btnGtx, a.theme, button, icon, iconSize, action.label, description, enabled, false, background, a.tokens().Colors)
					})
				})
			})
		}),
	)
}

func layoutWordToolbarActionItem(gtx layout.Context, action ribbonAction, index int, actions []ribbonAction, dividerColor color.NRGBA, child layout.Widget) layout.Dimensions {
	groupEnd := index == len(actions)-1 || actions[index+1].group != action.group
	children := make([]layout.FlexChild, 0, 4)
	children = append(children, layout.Rigid(child))
	if !groupEnd {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(gtx.Dp(toolbarItemGap), 0)}
		}))
	} else if index < len(actions)-1 {
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(12)), 0)}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min = image.Pt(gtx.Dp(unit.Dp(1)), gtx.Dp(unit.Dp(32)))
				gtx.Constraints.Max = gtx.Constraints.Min
				paint.FillShape(gtx.Ops, dividerColor, clip.Rect{Max: gtx.Constraints.Min}.Op())
				return layout.Dimensions{Size: gtx.Constraints.Min}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(12)), 0)}
			}),
		)
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

func (a *App) layoutWordRibbonIconButtonAt(gtx layout.Context, state *documentSessionState, action ribbonAction, xDP int) {
	width, height := gtx.Dp(toolbarButtonWidth), gtx.Dp(toolbarButtonHeight)
	x, y := gtx.Dp(unit.Dp(xDP)), gtx.Dp(unit.Dp(10))
	transform := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	defer transform.Pop()
	buttonGtx := gtx
	buttonGtx.Constraints = layout.Constraints{Min: image.Pt(width, height), Max: image.Pt(width, height)}
	enabled := a.wordToolbarActionButtonEnabled(action)
	button := wordToolbarButton(state, action.id)
	description := a.wordToolbarActionDescription(action)
	if action.id == "view-new-window" && !enabled {
		if _, reason := wordNewWindowAvailability(state, a.desktopRuntime); reason != "" {
			description += "，不可用：" + reason
		}
	}
	active := enabled && wordToolbarActionActive(state, action)
	background := a.tokens().Colors.Surface.Surface
	if active {
		background = a.tokens().Colors.Toolbar.ActiveBackground
	}
	icon, _ := a.themedWordToolbarActionIcon(action, enabled)
	toolbarIconButtonWithColors(buttonGtx, a.theme, button, icon, toolbarIconSize, action.label, description, enabled, false, background, a.tokens().Colors)
}

func (a *App) layoutWordRibbonPasteButtonAt(gtx layout.Context, state *documentSessionState, action ribbonAction, xDP, yDP int) {
	x, y := gtx.Dp(unit.Dp(xDP)), gtx.Dp(unit.Dp(yDP))
	width, height := gtx.Dp(unit.Dp(70)), gtx.Dp(unit.Dp(62))
	transform := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	child := gtx
	child.Constraints = layout.Constraints{Min: image.Pt(width, height), Max: image.Pt(width, height)}
	enabled := a.wordToolbarActionButtonEnabled(action)
	button := wordToolbarButton(state, action.id)
	draw := func(gtx layout.Context) layout.Dimensions {
		bounds := clip.Rect{Max: image.Pt(width, height)}.Push(gtx.Ops)
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(action.label).Add(gtx.Ops)
		semantic.EnabledOp(enabled).Add(gtx.Ops)
		bounds.Pop()
		background := a.tokens().Colors.Surface.Surface
		if enabled && (button.Hovered() || gtx.Focused(button)) {
			background = a.tokens().Colors.Toolbar.HoverBackground
		}
		paint.FillShape(gtx.Ops, background, clip.UniformRRect(image.Rectangle{Max: image.Pt(width, height)}, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
		icon, _ := a.themedWordToolbarActionIcon(action, enabled)
		iconSize := gtx.Dp(unit.Dp(16))
		iconOffset := op.Offset(image.Pt(gtx.Dp(unit.Dp(10)), gtx.Dp(unit.Dp(31)))).Push(gtx.Ops)
		iconGtx := gtx
		iconGtx.Constraints = layout.Constraints{Min: image.Pt(iconSize, iconSize), Max: image.Pt(iconSize, iconSize)}
		if icon != nil {
			widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: wordToolbarIconScale(16)}.Layout(iconGtx)
		}
		iconOffset.Pop()
		label := material.Caption(a.theme, action.label)
		label.TextSize = unit.Sp(10)
		label.Color = a.toolbarIconColor(enabled)
		labelOffset := op.Offset(image.Pt(gtx.Dp(unit.Dp(32)), gtx.Dp(unit.Dp(30)))).Push(gtx.Ops)
		label.Layout(gtx)
		labelOffset.Pop()
		arrow := material.Caption(a.theme, "⌄")
		arrow.TextSize = unit.Sp(10)
		arrow.Color = a.toolbarIconColor(enabled)
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
	enabled := a.wordToolbarActionButtonEnabled(action)
	button := wordToolbarButton(state, action.id)
	draw := func(gtx layout.Context) layout.Dimensions {
		bounds := clip.Rect{Max: image.Pt(width, height)}.Push(gtx.Ops)
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(action.label).Add(gtx.Ops)
		semantic.EnabledOp(enabled).Add(gtx.Ops)
		bounds.Pop()
		background := a.tokens().Colors.Surface.Surface
		if enabled && (button.Hovered() || gtx.Focused(button)) {
			background = a.tokens().Colors.Toolbar.HoverBackground
		}
		paint.FillShape(gtx.Ops, background, clip.UniformRRect(image.Rectangle{Max: image.Pt(width, height)}, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
		icon, _ := a.themedWordToolbarActionIcon(action, enabled)
		iconSize := gtx.Dp(unit.Dp(22))
		iconOffset := op.Offset(image.Pt(gtx.Dp(unit.Dp(4)), gtx.Dp(unit.Dp(6)))).Push(gtx.Ops)
		iconGtx := gtx
		iconGtx.Constraints = layout.Constraints{Min: image.Pt(iconSize, iconSize), Max: image.Pt(iconSize, iconSize)}
		if icon != nil {
			widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: wordToolbarIconScale(22)}.Layout(iconGtx)
		}
		iconOffset.Pop()
		label := material.Caption(a.theme, action.label)
		label.TextSize = unit.Sp(9)
		label.Color = a.toolbarIconColor(enabled)
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
	enabled := a.wordToolbarActionButtonEnabled(action)
	button := wordToolbarButton(state, action.id)
	draw := func(gtx layout.Context) layout.Dimensions {
		bounds := clip.Rect{Max: image.Pt(width, height)}.Push(gtx.Ops)
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(action.label).Add(gtx.Ops)
		semantic.EnabledOp(enabled).Add(gtx.Ops)
		bounds.Pop()
		background := a.tokens().Colors.Surface.Surface
		if enabled && (button.Hovered() || gtx.Focused(button)) {
			background = a.tokens().Colors.Toolbar.HoverBackground
		}
		paint.FillShape(gtx.Ops, background, clip.UniformRRect(image.Rectangle{Max: image.Pt(width, height)}, gtx.Dp(unit.Dp(2))).Op(gtx.Ops))
		icon, _ := a.themedWordToolbarActionIcon(action, enabled)
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceStart}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				iconSize := gtx.Dp(unit.Dp(iconDP))
				gtx.Constraints.Min = image.Pt(iconSize, iconSize)
				gtx.Constraints.Max = gtx.Constraints.Min
				if icon == nil {
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}
				return widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: wordToolbarIconScale(iconDP)}.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(7)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := material.Caption(a.theme, action.label)
				label.TextSize = unit.Sp(9)
				label.Color = a.toolbarIconColor(enabled)
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
