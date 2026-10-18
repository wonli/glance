package desktopui

import (
	"image"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// Both Word and Excel use the same font family and size field treatment.
func drawOfficeRibbonFontDropdown(gtx layout.Context, theme *material.Theme, label string, width, height int, value string, enabled bool, colors ColorTokens) layout.Dimensions {
	bounds := clip.Rect{Max: image.Pt(width, height)}.Push(gtx.Ops)
	semantic.DescriptionOp(label).Add(gtx.Ops)
	semantic.EnabledOp(enabled).Add(gtx.Ops)
	bounds.Pop()
	paint.FillShape(gtx.Ops, colors.Surface.Border, clip.UniformRRect(image.Rectangle{Max: image.Pt(width, height)}, gtx.Dp(3)).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, colors.Surface.Surface, clip.UniformRRect(image.Rectangle{Max: image.Pt(width, height)}.Inset(gtx.Dp(1)), gtx.Dp(2)).Op(gtx.Ops))
	style := material.Caption(theme, value)
	style.TextSize = unit.Sp(12)
	style.Color = colors.Text.PureBlack
	return layout.Inset{Left: unit.Dp(6), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = 0
				return style.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutPenRibbonGlyph(gtx, "chevron-down", 12, style.Color)
			}),
		)
	})
}
