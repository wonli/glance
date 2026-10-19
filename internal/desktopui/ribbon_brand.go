package desktopui

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

// These three layers are exported from Pen component DwuIp. Keeping each layer
// tintable preserves its exact geometry while all colors come from theme tokens.
func layoutBrandAssistantIcon(gtx layout.Context, size unit.Dp, colors ToolbarColors) layout.Dimensions {
	pixels := gtx.Dp(size)
	gtx.Constraints = layout.Exact(image.Pt(pixels, pixels))
	for i, name := range []string{"ai-back", "ai-front", "ai-star"} {
		tint := colors.AccentAISecondary
		if i == 1 {
			tint = colors.AccentAI
		} else if i == 2 {
			tint = colors.AccentAIStar
		}
		if icon, ok := svgIconTintAtSize("brand", name, pixels, tint); ok {
			widget.Image{Src: iconImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: float32(size) / float32(pixels)}.Layout(gtx)
		}
	}
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func paintRibbonControlState(gtx layout.Context, size image.Point, active, mixed, hovered bool, colors ToolbarColors) {
	background := colors.Background
	if mixed {
		background = colors.MixedBackground
	} else if active {
		background = colors.ActiveBackground
	} else if hovered {
		background = colors.HoverBackground
	}
	bounds := image.Rectangle{Max: size}
	radius := gtx.Dp(4)
	if active {
		paint.FillShape(gtx.Ops, colors.ActiveBorder, clip.UniformRRect(bounds, radius).Op(gtx.Ops))
		bounds = bounds.Inset(gtx.Dp(1))
		radius = gtx.Dp(3)
	}
	paint.FillShape(gtx.Ops, background, clip.UniformRRect(bounds, radius).Op(gtx.Ops))
}
