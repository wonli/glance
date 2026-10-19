package ui

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// NavItem draws a selectable navigation row. The caller owns its Clickable.
type NavItem struct {
	Button   *widget.Clickable
	Label    string
	Trailing string
	Icon     layout.Widget
	Selected bool
	Disabled bool
	Style    NavItemStyle
}

func layoutNavLabel(gtx layout.Context, label material.LabelStyle, offsetY unit.Dp) layout.Dimensions {
	gtx.Constraints.Min.Y = 0
	return (OpticalPlacement{OffsetY: offsetY}).Layout(gtx, label.Layout)
}

// Layout renders the row and keeps the full row inside the clickable bounds.
func (n NavItem) Layout(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	if n.Button == nil {
		return layout.Dimensions{}
	}
	if n.Disabled {
		gtx = gtx.Disabled()
	}
	return n.Button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		width, height := gtx.Constraints.Max.X, gtx.Dp(n.Style.Height)
		if n.Style.Height <= 0 {
			height = gtx.Constraints.Max.Y
			if height == 0 {
				height = gtx.Constraints.Min.Y
			}
		}
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		if width == 0 {
			width = gtx.Constraints.Min.X
		}
		bounds := image.Rect(0, 0, width, height)
		fill := n.Style.Colors.Background
		border := n.Style.Colors.Border
		foreground := n.Style.Colors.Foreground
		if n.Disabled {
			fill, border, foreground = n.Style.Colors.DisabledBackground, n.Style.Colors.DisabledBorder, n.Style.Colors.DisabledForeground
		} else if n.Selected {
			fill = n.Style.Colors.Selected
			border = n.Style.Colors.SelectedBorder
			if n.Style.Colors.SelectedForeground.A > 0 {
				foreground = n.Style.Colors.SelectedForeground
			}
		} else if n.Button.Hovered() && !n.Disabled {
			fill = n.Style.Colors.Hover
			border = n.Style.Colors.HoverBorder
			if n.Style.Colors.HoverForeground.A > 0 {
				foreground = n.Style.Colors.HoverForeground
			}
		}
		shape := clip.UniformRRect(bounds, gtx.Dp(n.Style.Radius))
		paint.FillShape(gtx.Ops, fill, shape.Op(gtx.Ops))
		if border != fill && border.A > 0 {
			paint.FillShape(gtx.Ops, border, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
		}

		content := gtx
		content.Constraints = layout.Exact(bounds.Max)
		return layout.Inset{Left: n.Style.LeftInset, Right: n.Style.RightInset}.Layout(content, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if n.Icon == nil {
						return layout.Dimensions{}
					}
					size := gtx.Dp(n.Style.IconSize)
					gtx.Constraints = layout.Exact(image.Pt(size, size))
					return (OpticalPlacement{OffsetY: n.Style.IconOffsetY}).Layout(gtx, n.Icon)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if n.Icon == nil {
						return layout.Dimensions{}
					}
					return layout.Spacer{Width: n.Style.IconGap}.Layout(gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					label := material.Body2(theme, n.Label)
					textStyle := n.Style.Text
					if n.Selected && n.Style.SelectedText.Size > 0 {
						textStyle = n.Style.SelectedText
					}
					label.TextSize = textStyle.Size
					label.Font.Weight = textStyle.Weight
					label.Color = foreground
					label.MaxLines = 1
					return layoutNavLabel(gtx, label, n.Style.TextOffsetY)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if n.Trailing == "" {
						return layout.Dimensions{}
					}
					return layout.Inset{Left: n.Style.TrailingGap}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						label := material.Body2(theme, n.Trailing)
						label.TextSize = n.Style.TrailingText.Size
						label.Font.Weight = n.Style.TrailingText.Weight
						label.Color = n.Style.TrailingColor
						label.MaxLines = 1
						return layoutNavLabel(gtx, label, n.Style.TrailingOffsetY)
					})
				}),
			)
		})
	})
}
