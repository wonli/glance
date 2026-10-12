package desktopui

import (
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"image"
	"image/color"
)

// Measure the message before painting: list rows have unbounded height.
func agentMessageBubble(gtx layout.Context, user bool, child layout.Widget) layout.Dimensions {
	return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := gtx.Constraints.Min
		radius := gtx.Dp(10)
		small := gtx.Dp(3)
		shape := clip.RRect{Rect: image.Rectangle{Max: size}, NW: radius, NE: radius, SW: radius, SE: radius}
		fill := color.NRGBA{R: 250, G: 251, B: 252, A: 255}
		if user {
			shape.SE = small
			fill = agentSidebarUserBubble
		} else {
			shape.NW = small
		}
		paint.FillShape(gtx.Ops, fill, shape.Op(gtx.Ops))
		if !user {
			paint.FillShape(gtx.Ops, color.NRGBA{R: 224, G: 229, B: 235, A: 255}, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(1))}.Op())
		}
		return layout.Dimensions{Size: size}
	}, child)
}

func agentHistoryMeasuredPanel(gtx layout.Context, fill, border color.NRGBA, radius unit.Dp, child layout.Widget) layout.Dimensions {
	return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := gtx.Constraints.Min
		shape := clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(radius))
		paint.FillShape(gtx.Ops, fill, shape.Op(gtx.Ops))
		paint.FillShape(gtx.Ops, border, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(1))}.Op())
		return layout.Dimensions{Size: size}
	}, child)
}
