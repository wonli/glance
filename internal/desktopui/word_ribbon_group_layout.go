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
		icon, _ := wordToolbarActionIcon(action, enabled)
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
						return widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: wordToolbarIconScale(iconSize)}.Layout(iconGtx)
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
			return widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: wordToolbarIconScale(iconSize)}.Layout(iconGtx)
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
