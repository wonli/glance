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
)

func (a *App) layoutConfirmDialogCard(gtx layout.Context) layout.Dimensions {
	maxWidth := gtx.Dp(unit.Dp(440))
	availableWidth := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(32))
	if availableWidth < maxWidth {
		maxWidth = availableWidth
	}
	if maxWidth <= 0 {
		return layout.Dimensions{}
	}
	gtx.Constraints.Min.X = maxWidth
	gtx.Constraints.Max.X = maxWidth

	child := op.Record(gtx.Ops)
	dims := layout.Inset{Top: unit.Dp(24), Bottom: unit.Dp(24), Left: unit.Dp(24), Right: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(a.layoutConfirmDialogHeader),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(20), Bottom: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					height := gtx.Dp(unit.Dp(1))
					gtx.Constraints.Min.Y = height
					gtx.Constraints.Max.Y = height
					return fill(gtx, color.NRGBA{R: 232, G: 235, B: 239, A: 255})
				})
			}),
			layout.Rigid(a.layoutConfirmDialogActions),
		)
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
	call.Add(gtx.Ops)
	return dims
}

func (a *App) layoutConfirmDialogHeader(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutConfirmDialogIcon(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(14)), 0)}
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := material.Body1(a.theme, a.modal.Title)
					label.TextSize = unit.Sp(17)
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
						label.TextSize = unit.Sp(13)
						label.Color = color.NRGBA{R: 104, G: 113, B: 124, A: 255}
						return label.Layout(gtx)
					})
				}))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		}),
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
			icon, ok := svgIconTintAtSize("confirm", a.modal.ConfirmStyle.Icon, pixels, a.modal.ConfirmStyle.IconForeground)
			if !ok {
				return layout.Dimensions{}
			}
			return widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: float32(gtx.Dp(unit.Dp(20))) / float32(pixels)}.Layout(gtx)
		}),
	)
}

func (a *App) layoutConfirmDialogActions(gtx layout.Context) layout.Dimensions {
	buttons := make([]layout.FlexChild, 0, len(a.modal.Actions))
	for i := range a.modal.Actions {
		action := &a.modal.Actions[i]
		buttons = append(buttons, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layoutConfirmDialogAction(gtx, a.theme, action, func() {
				if a.modal.OnAction != nil {
					a.modal.OnAction(action.ID)
				}
			})
		}))
	}
	return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceStart, Gap: gtx.Dp(unit.Dp(10))}.Layout(gtx, buttons...)
}

func layoutConfirmDialogAction(gtx layout.Context, theme *material.Theme, action *modalDialogAction, onClick func()) layout.Dimensions {
	semantic.Button.Add(gtx.Ops)
	semantic.DescriptionOp(action.Label).Add(gtx.Ops)
	if action.button.Clicked(gtx) {
		onClick()
	}
	return action.button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		background := color.NRGBA{R: 52, G: 120, B: 209, A: 255}
		foreground := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		border := background
		switch action.Style {
		case modalDialogActionSecondary:
			background = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			foreground = color.NRGBA{R: 78, G: 87, B: 98, A: 255}
			border = color.NRGBA{R: 201, G: 208, B: 217, A: 255}
		case modalDialogActionDanger:
			background = color.NRGBA{R: 199, G: 68, B: 68, A: 255}
			foreground = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			border = background
		}
		weight := font.SemiBold
		if action.Style == modalDialogActionSecondary {
			weight = font.Medium
		}
		labelStyle := homeLabelStyle(theme, action.Label, unit.Sp(13), foreground, weight)
		labelOps := op.Record(gtx.Ops)
		labelDims := layout.Inset{Left: unit.Dp(14), Right: unit.Dp(14)}.Layout(gtx, labelStyle.Layout)
		labelCall := labelOps.Stop()
		size := labelDims.Size
		minWidth := gtx.Dp(unit.Dp(88))
		if action.Style == modalDialogActionSecondary {
			minWidth = gtx.Dp(unit.Dp(80))
		}
		if size.X < minWidth {
			size.X = minWidth
		}
		if minHeight := gtx.Dp(unit.Dp(36)); size.Y < minHeight {
			size.Y = minHeight
		}
		bounds := image.Rectangle{Max: size}
		radius := gtx.Dp(unit.Dp(7))
		shape := clip.RRect{Rect: bounds, NW: radius, NE: radius, SW: radius, SE: radius}
		paint.FillShape(gtx.Ops, background, shape.Op(gtx.Ops))
		if action.Style == modalDialogActionSecondary {
			paint.FillShape(gtx.Ops, border, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
		}
		// Gio's label bounds include descent space below the visible CJK glyphs.
		labelOffsetY := (size.Y-labelDims.Size.Y)/2 + gtx.Dp(unit.Dp(3))
		labelOffset := op.Offset(image.Pt((size.X-labelDims.Size.X)/2, labelOffsetY)).Push(gtx.Ops)
		labelCall.Add(gtx.Ops)
		labelOffset.Pop()
		return layout.Dimensions{Size: size}
	})
}
