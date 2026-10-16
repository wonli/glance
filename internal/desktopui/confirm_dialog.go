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

var (
	modalDialogSaveConfirmStyle = modalDialogConfirmStyle{
		Icon:           "save",
		IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
		IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
	}
	modalDialogDangerConfirmStyle = modalDialogConfirmStyle{
		Icon:           "trash-2",
		IconBackground: color.NRGBA{R: 253, G: 238, B: 238, A: 255},
		IconForeground: color.NRGBA{R: 199, G: 68, B: 68, A: 255},
	}
	modalDialogWarningConfirmStyle = modalDialogConfirmStyle{
		Icon:           "triangle-alert",
		IconBackground: color.NRGBA{R: 253, G: 238, B: 238, A: 255},
		IconForeground: color.NRGBA{R: 199, G: 68, B: 68, A: 255},
	}
	modalDialogPrintConfirmStyle = modalDialogConfirmStyle{
		Icon:           "printer",
		IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
		IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
	}
	modalDialogRestoreConfirmStyle = modalDialogConfirmStyle{
		Icon:           "version-history",
		IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
		IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
	}
	modalDialogInfoConfirmStyle = modalDialogConfirmStyle{
		Icon:           "review-word-count",
		IconBackground: color.NRGBA{R: 234, G: 242, B: 252, A: 255},
		IconForeground: color.NRGBA{R: 52, G: 120, B: 209, A: 255},
	}
)

func (a *App) layoutConfirmDialogCard(gtx layout.Context) layout.Dimensions {
	cardWidth := 440
	if a.modal.CardWidthDp > 0 {
		cardWidth = a.modal.CardWidthDp
	}
	maxWidth := gtx.Dp(unit.Dp(cardWidth))
	availableWidth := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(32))
	if availableWidth < maxWidth {
		maxWidth = availableWidth
	}
	if maxWidth <= 0 {
		return layout.Dimensions{}
	}
	gtx.Constraints.Min.X = maxWidth
	gtx.Constraints.Max.X = maxWidth
	gtx.Constraints.Min.Y = 0
	actionContentWidth := max(0, maxWidth-gtx.Dp(unit.Dp(48)))

	child := op.Record(gtx.Ops)
	dims := layout.Inset{Top: unit.Dp(24), Bottom: unit.Dp(24), Left: unit.Dp(24), Right: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(a.layoutConfirmDialogHeader),
		}

		if a.modal.HasInput || a.modal.HasSecondInput || a.modal.HasForm {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, a.layoutModalInputs)
			}))
		}

		if a.modal.CustomLayout != nil {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, a.modal.CustomLayout)
			}))
		}

		if len(a.modal.Actions) > 0 && !a.modal.CustomActionsInLayout {
			children = append(children,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(20), Bottom: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						if gtx.Constraints.Max.X > actionContentWidth {
							gtx.Constraints.Max.X = actionContentWidth
						}
						if gtx.Constraints.Min.X > gtx.Constraints.Max.X {
							gtx.Constraints.Min.X = gtx.Constraints.Max.X
						}
						height := gtx.Dp(unit.Dp(1))
						gtx.Constraints.Min.Y = height
						gtx.Constraints.Max.Y = height
						return fill(gtx, color.NRGBA{R: 232, G: 235, B: 239, A: 255})
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if gtx.Constraints.Max.X > actionContentWidth {
						gtx.Constraints.Max.X = actionContentWidth
					}
					if gtx.Constraints.Min.X > gtx.Constraints.Max.X {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
					}
					return a.layoutConfirmDialogActions(gtx)
				}),
			)
		}

		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
	call := child.Stop()

	bounds := image.Rectangle{Max: dims.Size}
	radius := gtx.Dp(unit.Dp(12))
	for _, shadow := range []struct {
		offset int
		alpha  uint8
	}{{10, 10}, {6, 12}, {3, 20}} {
		shadowBounds := bounds.Add(image.Pt(0, gtx.Dp(unit.Dp(shadow.offset))))
		shadowShape := clip.RRect{Rect: shadowBounds, NW: radius, NE: radius, SW: radius, SE: radius}
		paint.FillShape(gtx.Ops, color.NRGBA{R: 31, G: 41, B: 55, A: shadow.alpha}, shadowShape.Op(gtx.Ops))
	}
	card := clip.RRect{Rect: bounds, NW: radius, NE: radius, SW: radius, SE: radius}
	paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, card.Op(gtx.Ops))
	paint.FillShape(gtx.Ops, color.NRGBA{R: 216, G: 222, B: 230, A: 255}, clip.Stroke{Path: card.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
	contentClip := card.Push(gtx.Ops)
	call.Add(gtx.Ops)
	contentClip.Pop()
	return dims
}

func (a *App) layoutModalInputs(gtx layout.Context) layout.Dimensions {
	var inputChildren []layout.FlexChild
	if a.modal.HasInput {
		inputChildren = append(inputChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			bottom := unit.Dp(0)
			if a.modal.HasSecondInput || a.modal.HasForm {
				bottom = unit.Dp(10)
			}
			return layout.Inset{Bottom: bottom}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layoutModalInputField(gtx, a.theme, &a.modal.Input, a.modal.InputHint)
			})
		}))
	}
	if a.modal.HasSecondInput {
		inputChildren = append(inputChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			bottom := unit.Dp(0)
			if a.modal.HasForm {
				bottom = unit.Dp(10)
			}
			return layout.Inset{Bottom: bottom}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layoutModalInputField(gtx, a.theme, &a.modal.SecondInput, a.modal.SecondInputHint)
			})
		}))
	}
	if a.modal.HasForm {
		for index := 0; index < a.modal.FormInputCount; index++ {
			fieldIndex := index
			inputChildren = append(inputChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				bottom := unit.Dp(10)
				if fieldIndex == a.modal.FormInputCount-1 {
					bottom = unit.Dp(0)
				}
				return layout.Inset{Bottom: bottom}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.Caption(a.theme, a.modal.FormLabels[fieldIndex])
							label.TextSize = a.tokens().Typography.Secondary
							label.Font.Weight = font.SemiBold
							label.Color = color.NRGBA{R: 78, G: 87, B: 98, A: 255}
							return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, label.Layout)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layoutModalInputField(gtx, a.theme, &a.modal.FormInputs[fieldIndex], a.modal.FormHints[fieldIndex])
						}),
					)
				})
			}))
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, inputChildren...)
}

func layoutModalInputField(gtx layout.Context, theme *material.Theme, editor *widget.Editor, hint string) layout.Dimensions {
	height := gtx.Dp(unit.Dp(36))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			bounds := image.Rectangle{Max: gtx.Constraints.Max}
			shape := clip.RRect{
				Rect: bounds,
				NW:   gtx.Dp(unit.Dp(7)),
				NE:   gtx.Dp(unit.Dp(7)),
				SW:   gtx.Dp(unit.Dp(7)),
				SE:   gtx.Dp(unit.Dp(7)),
			}
			paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, shape.Op(gtx.Ops))
			paint.FillShape(gtx.Ops, color.NRGBA{R: 201, G: 208, B: 217, A: 255}, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
			return layout.Dimensions{Size: bounds.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = 0
				style := material.Editor(theme, editor, hint)
				style.TextSize = defaultShellTokens.Typography.Body
				style.Color = color.NRGBA{R: 31, G: 37, B: 45, A: 255}
				style.HintColor = color.NRGBA{R: 122, G: 131, B: 142, A: 255}
				return style.Layout(gtx)
			})
		},
	)
}

func (a *App) layoutConfirmDialogHeader(gtx layout.Context) layout.Dimensions {
	hasIcon := false
	if a.modal.ConfirmStyle.Icon != "" {
		_, hasIcon = findDialogIcon(a.modal.ConfirmStyle.IconFamily, a.modal.ConfirmStyle.Icon, 96, a.modal.ConfirmStyle.IconForeground)
	}

	titleSection := func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := material.Body1(a.theme, a.modal.Title)
				label.TextSize = a.tokens().Typography.Heading
				label.Color = color.NRGBA{R: 31, G: 37, B: 45, A: 255}
				label.Font.Weight = font.SemiBold
				label.MaxLines = 2
				return label.Layout(gtx)
			}),
		}
		if a.modal.Message != "" {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					label := material.Body2(a.theme, a.modal.Message)
					label.TextSize = a.tokens().Typography.Body
					label.Color = color.NRGBA{R: 104, G: 113, B: 124, A: 255}
					return label.Layout(gtx)
				})
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	}

	if !hasIcon {
		return titleSection(gtx)
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutConfirmDialogIcon(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(14)), 0)}
		}),
		layout.Flexed(1, titleSection),
	)
}

func (a *App) layoutConfirmDialogIcon(gtx layout.Context) layout.Dimensions {
	size := gtx.Dp(unit.Dp(40))
	gtx.Constraints.Min = image.Pt(size, size)
	gtx.Constraints.Max = gtx.Constraints.Min
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			bounds := image.Rectangle{Max: gtx.Constraints.Max}
			radius := gtx.Dp(unit.Dp(10))
			shape := clip.RRect{Rect: bounds, NW: radius, NE: radius, SW: radius, SE: radius}
			paint.FillShape(gtx.Ops, a.modal.ConfirmStyle.IconBackground, shape.Op(gtx.Ops))
			return layout.Dimensions{Size: bounds.Max}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			pixels := 96
			icon, ok := findDialogIcon(a.modal.ConfirmStyle.IconFamily, a.modal.ConfirmStyle.Icon, pixels, a.modal.ConfirmStyle.IconForeground)
			if !ok {
				return layout.Dimensions{}
			}
			return widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: float32(gtx.Dp(unit.Dp(20))) / float32(pixels)}.Layout(gtx)
		}),
	)
}

func (a *App) layoutConfirmDialogActions(gtx layout.Context) layout.Dimensions {
	actions := make([]*modalDialogAction, 0, len(a.modal.Actions))
	for i := range a.modal.Actions {
		actions = append(actions, &a.modal.Actions[i])
	}
	return a.layoutModalActionButtons(gtx, actions, 10)
}

func layoutConfirmDialogAction(gtx layout.Context, theme *material.Theme, action *modalDialogAction, onClick func()) layout.Dimensions {
	width := measureConfirmDialogActionWidth(gtx, theme, action)
	if available := gtx.Constraints.Max.X; available > 0 && width > available {
		width = available
	}
	return layoutConfirmDialogActionAtWidth(gtx, theme, action, width, onClick)
}

func (a *App) layoutModalActionButtons(gtx layout.Context, actions []*modalDialogAction, gapDp int) layout.Dimensions {
	if len(actions) == 0 || gtx.Constraints.Max.X <= 0 {
		return layout.Dimensions{}
	}
	gap := gtx.Dp(unit.Dp(gapDp))
	availableWidth := gtx.Constraints.Max.X
	widths := make([]int, len(actions))
	for index, action := range actions {
		widths[index] = measureConfirmDialogActionWidth(gtx, a.theme, action)
		if widths[index] > availableWidth {
			widths[index] = availableWidth
		}
	}
	rows := planDialogActionRows(widths, availableWidth, gap)
	rowChildren := make([]layout.FlexChild, 0, len(rows))
	for _, row := range rows {
		rowIndexes := row
		buttons := make([]layout.FlexChild, 0, len(rowIndexes))
		for _, actionIndex := range rowIndexes {
			index := actionIndex
			action := actions[index]
			width := widths[index]
			buttons = append(buttons, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutConfirmDialogActionAtWidth(gtx, a.theme, action, width, func() {
					if a.modal.OnAction != nil {
						a.modal.OnAction(action.ID)
					}
				})
			}))
		}
		rowChildren = append(rowChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceStart, Gap: gap}.Layout(gtx, buttons...)
		}))
	}
	return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceStart, Gap: gtx.Dp(unit.Dp(8))}.Layout(gtx, rowChildren...)
}

func planDialogActionRows(widths []int, availableWidth, gap int) [][]int {
	if availableWidth <= 0 || len(widths) == 0 {
		return nil
	}
	if gap < 0 {
		gap = 0
	}
	var rows [][]int
	var row []int
	rowWidth := 0
	for index, width := range widths {
		if width < 0 {
			width = 0
		}
		if width > availableWidth {
			width = availableWidth
		}
		withGap := 0
		if len(row) > 0 {
			withGap = gap
		}
		if len(row) > 0 && rowWidth+withGap+width > availableWidth {
			rows = append(rows, row)
			row = nil
			rowWidth = 0
			withGap = 0
		}
		row = append(row, index)
		rowWidth += withGap + width
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	return rows
}

func measureConfirmDialogActionWidth(gtx layout.Context, theme *material.Theme, action *modalDialogAction) int {
	availableWidth := gtx.Constraints.Max.X
	if availableWidth <= 0 {
		return 0
	}
	measureGtx := gtx
	measureGtx.Ops = new(op.Ops)
	measureGtx.Constraints.Min.X = 0
	measureGtx.Constraints.Max.X = availableWidth
	labelDims := layout.Inset{Left: unit.Dp(14), Right: unit.Dp(14)}.Layout(measureGtx, dialogActionLabelStyle(theme, action, dialogActionForeground(action)).Layout)
	width := labelDims.Size.X
	minWidth := gtx.Dp(unit.Dp(88))
	if dialogActionStyle(action) == modalDialogActionSecondary {
		minWidth = gtx.Dp(unit.Dp(80))
	}
	if width < minWidth {
		width = minWidth
	}
	if action.WidthDp > 0 {
		requested := gtx.Dp(unit.Dp(action.WidthDp))
		if requested > width {
			width = requested
		}
	}
	if width > availableWidth {
		width = availableWidth
	}
	return width
}

func dialogActionStyle(action *modalDialogAction) modalDialogActionStyle {
	style := action.Style
	if style == modalDialogActionDefault {
		style = resolveDialogActionStyle(modalDialogActionSpec{ID: action.ID, Label: action.Label})
	}
	return style
}

func dialogActionForeground(action *modalDialogAction) color.NRGBA {
	if dialogActionStyle(action) == modalDialogActionSecondary {
		return color.NRGBA{R: 78, G: 87, B: 98, A: 255}
	}
	return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
}

func dialogActionLabelStyle(theme *material.Theme, action *modalDialogAction, foreground color.NRGBA) material.LabelStyle {
	weight := font.SemiBold
	if dialogActionStyle(action) == modalDialogActionSecondary {
		weight = font.Medium
	}
	textSize := unit.Sp(13)
	if action.TextSizeSp > 0 {
		textSize = unit.Sp(action.TextSizeSp)
	}
	style := homeLabelStyle(theme, action.Label, textSize, foreground, weight)
	style.MaxLines = 2
	return style
}

func layoutConfirmDialogActionAtWidth(gtx layout.Context, theme *material.Theme, action *modalDialogAction, width int, onClick func()) layout.Dimensions {
	if width < 0 {
		width = 0
	}
	gtx.Constraints.Min.X = width
	gtx.Constraints.Max.X = width
	semantic.Button.Add(gtx.Ops)
	semantic.DescriptionOp(action.Label).Add(gtx.Ops)
	if action.button.Clicked(gtx) {
		onClick()
	}
	return action.button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		background := color.NRGBA{R: 52, G: 120, B: 209, A: 255}
		foreground := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		border := background
		style := dialogActionStyle(action)
		switch style {
		case modalDialogActionSecondary:
			background = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			foreground = color.NRGBA{R: 78, G: 87, B: 98, A: 255}
			border = color.NRGBA{R: 201, G: 208, B: 217, A: 255}
		case modalDialogActionDanger:
			background = color.NRGBA{R: 199, G: 68, B: 68, A: 255}
			foreground = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			border = background
		}
		labelStyle := dialogActionLabelStyle(theme, action, foreground)
		labelOps := op.Record(gtx.Ops)
		labelDims := layout.Inset{Left: unit.Dp(14), Right: unit.Dp(14)}.Layout(gtx, labelStyle.Layout)
		labelCall := labelOps.Stop()
		size := image.Pt(gtx.Constraints.Max.X, labelDims.Size.Y)
		if minHeight := gtx.Dp(unit.Dp(36)); size.Y < minHeight {
			size.Y = minHeight
		}
		bounds := image.Rectangle{Max: size}
		radius := gtx.Dp(unit.Dp(7))
		shape := clip.RRect{Rect: bounds, NW: radius, NE: radius, SW: radius, SE: radius}
		paint.FillShape(gtx.Ops, background, shape.Op(gtx.Ops))
		if style == modalDialogActionSecondary {
			paint.FillShape(gtx.Ops, border, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
		}

		labelOffsetY := (size.Y-labelDims.Size.Y)/2 + gtx.Dp(unit.Dp(3))
		labelOffset := op.Offset(image.Pt((size.X-labelDims.Size.X)/2, labelOffsetY)).Push(gtx.Ops)
		labelCall.Add(gtx.Ops)
		labelOffset.Pop()
		return layout.Dimensions{Size: size}
	})
}
