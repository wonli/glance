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

// IconTextButton lays out an icon and a label in a stable clickable area.
// The caller owns and retains Button between frames.
type IconTextButton struct {
	Button   *widget.Clickable
	Label    string
	Icon     layout.Widget
	Style    IconTextButtonStyle
	Selected bool
	Disabled bool
}

// Layout draws the button. Offsets move rendered content without changing the
// button's reported size or pointer bounds.
func (b IconTextButton) Layout(gtx layout.Context, theme *material.Theme) layout.Dimensions {
	if b.Button == nil {
		return layout.Dimensions{}
	}
	if b.Disabled {
		gtx = gtx.Disabled()
	}
	return b.Button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := image.Pt(gtx.Dp(b.Style.Width), gtx.Dp(b.Style.Height))
		if b.Style.Width <= 0 {
			size.X = gtx.Constraints.Max.X
		}
		if size.X < gtx.Constraints.Min.X {
			size.X = gtx.Constraints.Min.X
		}
		if gtx.Constraints.Max.X > 0 && size.X > gtx.Constraints.Max.X {
			size.X = gtx.Constraints.Max.X
		}
		if b.Style.Height <= 0 {
			size.Y = gtx.Constraints.Max.Y
			if size.Y == 0 {
				size.Y = gtx.Constraints.Min.Y
			}
		}
		bounds := image.Rectangle{Max: size}
		fill := b.Style.Colors.Background
		border := b.Style.Colors.Border
		foreground := b.Style.Colors.Foreground
		if b.Disabled {
			fill, border, foreground = b.Style.Colors.DisabledBackground, b.Style.Colors.DisabledBorder, b.Style.Colors.DisabledForeground
		} else if b.Selected {
			fill, border = b.Style.Colors.Selected, b.Style.Colors.SelectedBorder
			if b.Style.Colors.SelectedForeground.A > 0 {
				foreground = b.Style.Colors.SelectedForeground
			}
		} else if b.Button.Hovered() {
			fill = b.Style.Colors.Hover
			border = b.Style.Colors.HoverBorder
			if b.Style.Colors.HoverForeground.A > 0 {
				foreground = b.Style.Colors.HoverForeground
			}
		}
		shape := clip.UniformRRect(bounds, gtx.Dp(b.Style.Radius))
		paint.FillShape(gtx.Ops, fill, shape.Op(gtx.Ops))
		if border != fill && border.A > 0 {
			paint.FillShape(gtx.Ops, border, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
		}

		content := gtx
		content.Constraints = layout.Exact(size)
		return layout.Center.Layout(content, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if b.Icon == nil {
						return layout.Dimensions{}
					}
					iconSize := gtx.Dp(b.Style.IconSize)
					gtx.Constraints = layout.Exact(image.Pt(iconSize, iconSize))
					return (OpticalPlacement{OffsetY: b.Style.IconOffsetY}).Layout(gtx, b.Icon)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if b.Icon == nil || b.Label == "" {
						return layout.Dimensions{}
					}
					return layout.Spacer{Width: b.Style.Gap}.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					style := material.Body2(theme, b.Label)
					style.TextSize = b.Style.Text.Size
					style.Font.Weight = b.Style.Text.Weight
					style.Color = foreground
					style.MaxLines = 1
					return (OpticalPlacement{OffsetY: b.Style.TextOffsetY}).Layout(gtx, style.Layout)
				}),
			)
		})
	})
}
