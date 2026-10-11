package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

const presentationRulerSize unit.Dp = 24

var (
	presentationRulerBackground = color.NRGBA{R: 250, G: 251, B: 252, A: 255}
	presentationRulerTick       = color.NRGBA{R: 132, G: 141, B: 153, A: 255}
	presentationRulerLabel      = color.NRGBA{R: 82, G: 91, B: 103, A: 255}
)

func (a *App) drawPresentationRulers(gtx layout.Context, view *presentationViewState, slideWidth, slideHeight, scale float64) {
	if view == nil || !view.rulerVisible || scale <= 0 {
		return
	}

	bar := gtx.Dp(presentationRulerSize)
	page := image.Rectangle{Min: view.canvasCardOrigin, Max: view.canvasCardOrigin.Add(view.canvasCardSize)}
	horizontal := image.Rect(page.Min.X, page.Min.Y-bar, page.Max.X, page.Min.Y)
	vertical := image.Rect(page.Min.X-bar, page.Min.Y, page.Min.X, page.Max.Y)
	viewport := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	defer viewport.Pop()

	paint.FillShape(gtx.Ops, presentationRulerBackground, clip.Rect{Min: horizontal.Min, Max: horizontal.Max}.Op())
	paint.FillShape(gtx.Ops, presentationRulerBackground, clip.Rect{Min: vertical.Min, Max: vertical.Max}.Op())
	paint.FillShape(gtx.Ops, pptxBorderColor, clip.Rect{Min: image.Pt(horizontal.Min.X, horizontal.Max.Y-1), Max: horizontal.Max}.Op())
	paint.FillShape(gtx.Ops, pptxBorderColor, clip.Rect{Min: image.Pt(vertical.Max.X-1, vertical.Min.Y), Max: vertical.Max}.Op())

	majorInches := max(1, int(math.Ceil(40/(72*scale))))
	majorPoints := float64(majorInches) * 72
	minorPoints := majorPoints / 4
	maxTicks := 1000

	for tick := 1; float64(tick)*minorPoints <= slideWidth && tick <= maxTicks; tick++ {
		point := float64(tick) * minorPoints
		position := page.Min.X + int(math.Round(point*scale))
		length := gtx.Dp(unit.Dp(4))
		if tick%4 == 0 {
			length = gtx.Dp(unit.Dp(10))
		} else if tick%2 == 0 {
			length = gtx.Dp(unit.Dp(7))
		}
		paint.FillShape(gtx.Ops, presentationRulerTick, clip.Rect{Min: image.Pt(position, horizontal.Max.Y-length), Max: image.Pt(position+1, horizontal.Max.Y)}.Op())
		if tick%4 == 0 {
			inches := int(math.Round(point / 72))
			layoutPresentationRulerLabel(gtx, a.theme, fmt.Sprint(inches), image.Pt(position+gtx.Dp(unit.Dp(3)), horizontal.Min.Y+gtx.Dp(unit.Dp(1))))
		}
	}
	for tick := 1; float64(tick)*minorPoints <= slideHeight && tick <= maxTicks; tick++ {
		point := float64(tick) * minorPoints
		position := page.Min.Y + int(math.Round(point*scale))
		length := gtx.Dp(unit.Dp(4))
		if tick%4 == 0 {
			length = gtx.Dp(unit.Dp(10))
		} else if tick%2 == 0 {
			length = gtx.Dp(unit.Dp(7))
		}
		paint.FillShape(gtx.Ops, presentationRulerTick, clip.Rect{Min: image.Pt(vertical.Max.X-length, position), Max: image.Pt(vertical.Max.X, position+1)}.Op())
		if tick%4 == 0 {
			inches := int(math.Round(point / 72))
			layoutPresentationRulerLabel(gtx, a.theme, fmt.Sprint(inches), image.Pt(vertical.Min.X+gtx.Dp(unit.Dp(2)), position+gtx.Dp(unit.Dp(1))))
		}
	}
}

func layoutPresentationRulerLabel(gtx layout.Context, theme *material.Theme, value string, position image.Point) {
	label := material.Caption(theme, value)
	label.TextSize = unit.Sp(9)
	label.Color = presentationRulerLabel
	recorder := op.Record(gtx.Ops)
	label.Layout(gtx)
	call := recorder.Stop()
	transform := op.Offset(position).Push(gtx.Ops)
	call.Add(gtx.Ops)
	transform.Pop()
}
