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
	widthDP := group.widthDP
	if widthDP <= 0 {
		widthDP = wordRibbonGroupWidth(group.label)
	}
	width := gtx.Dp(unit.Dp(widthDP))
	height := gtx.Dp(unit.Dp(72))
	gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(width, height), image.Pt(width, height)
	return layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			if last || state.wordToolbarTab == wordTabHome {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			}
			x := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(1))
			paint.FillShape(gtx.Ops, a.tokens().Colors.Toolbar.Divider, clip.Rect{Min: image.Pt(x, gtx.Dp(unit.Dp(5))), Max: image.Pt(x+gtx.Dp(unit.Dp(1)), gtx.Constraints.Max.Y-gtx.Dp(unit.Dp(14)))}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return a.layoutWordRibbonGroupContent(gtx, state, group)
		}),
	)
}

func (a *App) layoutWordRibbonGroupItem(gtx layout.Context, state *documentSessionState, group wordRibbonGroup, last bool) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutWordRibbonGroup(gtx, state, group, last)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if last {
				return layout.Dimensions{}
			}
			return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(4)), 0)}
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
		first := min(5, len(group.actions))
		return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceBetween}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layoutWordRibbonActionRow(gtx, state, group.actions[:first], 34, 32, false)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if first == len(group.actions) {
					return layout.Dimensions{}
				}
				return a.layoutWordRibbonActionRow(gtx, state, group.actions[first:], 34, 32, false)
			}),
		)
	case "样式":
		return a.layoutWordStyleGallery(gtx, state, group.actions)
	case "编辑":
		return a.layoutWordRibbonCompactStack(gtx, state, group.actions, 24)
	case "编辑器":
		return a.layoutWordRibbonCompactStack(gtx, state, group.actions, 22)
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
			return a.layoutWordRibbonAction(gtx, state, actions[0], 80, 68, true)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutWordRibbonCompactStack(gtx, state, actions[1:], 24)
			})
		}),
	)
}

func (a *App) layoutWordRibbonCompactStack(gtx layout.Context, state *documentSessionState, actions []ribbonAction, rowHeightDP int) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(actions))
	for _, action := range actions {
		action := action
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutWordRibbonCompactAction(gtx, state, action, pxToRibbonDP(gtx, gtx.Constraints.Max.X), rowHeightDP, true)
		}))
	}
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx, children...)
}

func (a *App) layoutWordRibbonCompactAction(gtx layout.Context, state *documentSessionState, action ribbonAction, widthDP, heightDP int, showLabel bool) layout.Dimensions {
	width, height := gtx.Dp(unit.Dp(widthDP)), gtx.Dp(unit.Dp(heightDP))
	gtx.Constraints = layout.Exact(image.Pt(width, height))
	button := wordToolbarButton(state, action.id)
	enabled := a.wordToolbarActionButtonEnabled(action)
	draw := func(gtx layout.Context) layout.Dimensions {
		bounds := clip.Rect{Max: image.Pt(width, height)}.Push(gtx.Ops)
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(action.label).Add(gtx.Ops)
		semantic.EnabledOp(enabled).Add(gtx.Ops)
		bounds.Pop()
		background := a.tokens().Colors.Toolbar.Background
		if enabled && (button.Hovered() || gtx.Focused(button)) {
			background = a.tokens().Colors.Toolbar.HoverBackground
		}
		paint.FillShape(gtx.Ops, background, clip.UniformRRect(image.Rectangle{Max: image.Pt(width, height)}, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
		icon, _ := a.themedWordToolbarActionIcon(action, enabled)
		iconSize := gtx.Dp(unit.Dp(20))
		if !showLabel {
			iconSize = gtx.Dp(unit.Dp(18))
			if icon == nil {
				return layout.Dimensions{Size: image.Pt(width, height)}
			}
			iconGtx := gtx
			iconGtx.Constraints = layout.Exact(image.Pt(iconSize, iconSize))
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return widget.Image{Src: iconImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: wordToolbarIconScale(20)}.Layout(iconGtx)
			})
		}
		return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints = layout.Exact(image.Pt(iconSize, iconSize))
					if icon == nil {
						return layout.Dimensions{Size: image.Pt(iconSize, iconSize)}
					}
					return widget.Image{Src: iconImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: wordToolbarIconScale(20)}.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = 0
					label := material.Caption(a.theme, action.label)
					label.TextSize = unit.Sp(10)
					label.Color = a.tokens().Colors.Text.PureBlack
					return label.Layout(gtx)
				}),
			)
		})
	}
	if enabled {
		return button.Layout(gtx, draw)
	}
	return draw(gtx)
}

func (a *App) layoutWordFontRibbonGroup(gtx layout.Context, state *documentSessionState, actions []ribbonAction) layout.Dimensions {
	fontFamily := findRibbonAction(actions, "font-family")
	fontSize := ribbonAction{id: "font-size", label: "字号", group: "字体"}
	increase := findRibbonAction(actions, "font-size-increase")
	decrease := findRibbonAction(actions, "font-size-decrease")
	marks := []ribbonAction{
		findRibbonAction(actions, "bold"), findRibbonAction(actions, "italic"), findRibbonAction(actions, "underline"),
		{id: "strikethrough", label: "删除线", icon: "strikethrough"}, findRibbonAction(actions, "text-highlight"), findRibbonAction(actions, "font-color"),
	}
	return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceBetween}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.layoutWordRibbonField(gtx, fontFamily, 108, a.fontPickerDisplayFamilyName(a.localizedSelectedWordFontFamilyName(state)))
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.layoutWordRibbonField(gtx, fontSize, 54, a.localizedSelectedWordFontSizeDisplay(state))
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.layoutWordRibbonCompactAction(gtx, state, increase, 28, 34, false)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.layoutWordRibbonCompactAction(gtx, state, decrease, 28, 34, false)
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutWordRibbonActionRow(gtx, state, marks, 34, 35, false)
		}),
	)
}

func findRibbonAction(actions []ribbonAction, id string) ribbonAction {
	for _, action := range actions {
		if action.id == id {
			return action
		}
	}
	return ribbonAction{id: id, label: id, icon: id}
}

func (a *App) layoutWordRibbonActionRow(gtx layout.Context, state *documentSessionState, actions []ribbonAction, heightDP, widthDP int, labels bool) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(actions))
	for index, action := range actions {
		if index > 0 && state.wordToolbarTab == wordTabHome {
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout))
		}
		action := action
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutWordRibbonAction(gtx, state, action, widthDP, heightDP, labels)
		}))
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceEnd}.Layout(gtx, children...)
}

func (a *App) layoutWordRibbonAction(gtx layout.Context, state *documentSessionState, action ribbonAction, widthDP, heightDP int, showLabel bool) layout.Dimensions {
	action = a.localizedWordAction(action)
	width, height := gtx.Dp(unit.Dp(widthDP)), gtx.Dp(unit.Dp(heightDP))
	gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(width, height), image.Pt(width, height)
	enabled := a.wordToolbarActionButtonEnabled(action)
	button := wordToolbarButton(state, action.id)
	draw := func(gtx layout.Context) layout.Dimensions {
		semantics := clip.Rect{Max: image.Pt(width, height)}.Push(gtx.Ops)
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(action.label).Add(gtx.Ops)
		semantic.EnabledOp(enabled).Add(gtx.Ops)
		semantics.Pop()
		active := enabled && wordToolbarActionActive(state, action)
		colors := officeAccentTokens(a.tokens().Colors, a.tokens().Colors.OfficeAccent.Word)
		paintRibbonControlState(gtx, image.Pt(width, height), active, false, enabled && (button.Hovered() || gtx.Focused(button)), colors.Toolbar)
		if action.id == "font-family" || action.id == "font-size" {
			value := a.fontPickerDisplayFamilyName(a.localizedSelectedWordFontFamilyName(state))
			if action.id == "font-size" {
				value = a.localizedSelectedWordFontSizeDisplay(state)
			}
			return drawWordRibbonDropdown(gtx, a.theme, action, width, height, value, enabled, colors)
		}
		icon, _ := a.themedWordToolbarActionIcon(action, enabled)
		if active {
			icon, _ = wordToolbarActionIconWithTint(action, colors.Toolbar.ActiveGlyph)
		}
		if showLabel {
			label := action.label
			if action.displayLabel != "" {
				label = action.displayLabel
			}
			if penWordRibbonDropdowns[action.id] {
				label += " ▾"
			}
			return layoutOfficeRibbonCommandContents(gtx, a.theme, icon, label)
		}
		if icon == nil {
			return layout.Dimensions{Size: image.Pt(width, height)}
		}
		iconSize := min(gtx.Dp(unit.Dp(20)), min(width-gtx.Dp(unit.Dp(4)), height-gtx.Dp(unit.Dp(4))))
		if iconSize <= 0 {
			return layout.Dimensions{Size: image.Pt(width, height)}
		}
		iconGtx := gtx
		iconGtx.Constraints.Min = image.Pt(iconSize, iconSize)
		iconGtx.Constraints.Max = iconGtx.Constraints.Min
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return widget.Image{Src: iconImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: wordToolbarIconScale(iconSize)}.Layout(iconGtx)
		})
	}
	if enabled {
		return button.Layout(gtx, draw)
	}
	return draw(gtx)
}

func (a *App) layoutWordRibbonField(gtx layout.Context, action ribbonAction, widthDP int, value string) layout.Dimensions {
	width, height := gtx.Dp(unit.Dp(widthDP)), gtx.Dp(unit.Dp(34))
	gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(width, height), image.Pt(width, height)
	state := a.documentSessionState
	button := wordToolbarButton(state, action.id)
	enabled := a.wordToolbarActionButtonEnabled(action)
	draw := func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, a.tokens().Colors.Toolbar.Divider, clip.UniformRRect(image.Rectangle{Max: image.Pt(width, height)}, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
		return drawWordRibbonDropdown(gtx, a.theme, action, width, height, value, enabled, a.tokens().Colors)
	}
	if enabled {
		return button.Layout(gtx, draw)
	}
	return draw(gtx)
}

func drawWordRibbonDropdown(gtx layout.Context, theme *material.Theme, action ribbonAction, width, height int, value string, enabled bool, colors ColorTokens) layout.Dimensions {
	return drawOfficeRibbonFontDropdown(gtx, theme, action.label, width, height, value, enabled, colors)
}

func (a *App) layoutWordStyleGallery(gtx layout.Context, state *documentSessionState, actions []ribbonAction) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(actions))
	for _, action := range actions {
		action := action
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			width, height := gtx.Constraints.Max.X, gtx.Dp(unit.Dp(60))
			gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(width, height), image.Pt(width, height)
			button := wordToolbarButton(state, action.id)
			enabled := a.wordToolbarActionButtonEnabled(action)
			draw := func(gtx layout.Context) layout.Dimensions {
				cardBounds := image.Rectangle{Max: image.Pt(width, height)}
				active := enabled && wordToolbarActionActive(state, action)
				colors := officeAccentTokens(a.tokens().Colors, a.tokens().Colors.OfficeAccent.Word)
				borderColor := colors.Surface.Border
				background := colors.Surface.Surface
				if active {
					borderColor = colors.Toolbar.ActiveBorder
					background = colors.Toolbar.ActiveBackground
				} else if enabled && (button.Hovered() || gtx.Focused(button)) {
					background = colors.Toolbar.HoverBackground
				}
				paint.FillShape(gtx.Ops, borderColor, clip.UniformRRect(cardBounds, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
				inner := cardBounds.Inset(gtx.Dp(unit.Dp(1)))
				paint.FillShape(gtx.Ops, background, clip.UniformRRect(inner, gtx.Dp(unit.Dp(2))).Op(gtx.Ops))
				semantics := clip.Rect{Max: image.Pt(width, height)}.Push(gtx.Ops)
				semantic.Button.Add(gtx.Ops)
				semantic.DescriptionOp(action.label).Add(gtx.Ops)
				semantic.EnabledOp(enabled).Add(gtx.Ops)
				semantics.Pop()
				preview := material.Body1(a.theme, "AaBbCc")
				preview.TextSize = unit.Sp(15)
				preview.Color = a.tokens().Colors.Toolbar.Label
				if action.id == "style-heading" {
					preview.Color = color.NRGBA{R: 43, G: 87, B: 154, A: 255}
					preview.TextSize = unit.Sp(19)
				}
				if action.id == "style-heading-2" {
					preview.TextSize = unit.Sp(17)
				}
				label := material.Caption(a.theme, action.label)
				label.TextSize = unit.Sp(12)
				label.Color = a.tokens().Colors.Text.PureBlack
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
			}
			if enabled {
				return button.Layout(gtx, draw)
			}
			return draw(gtx)
		}))
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceAround}.Layout(gtx, children...)
}
