package desktopui

import (
	"image"
	"image/color"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
)

type standardWindowControl uint8

const (
	windowControlMinimize standardWindowControl = iota
	windowControlMaximize
	windowControlRestore
	windowControlClose
)

func (a *App) layoutStandardWindowControls(gtx layout.Context) layout.Dimensions {
	maximize := windowControlMaximize
	if a.windowMode == app.Maximized {
		maximize = windowControlRestore
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(g layout.Context) layout.Dimensions {
			return a.layoutStandardWindowButton(g, &a.windowMinimizeButton, windowControlMinimize)
		}),
		layout.Rigid(func(g layout.Context) layout.Dimensions {
			return a.layoutStandardWindowButton(g, &a.windowMaximizeButton, maximize)
		}),
		layout.Rigid(func(g layout.Context) layout.Dimensions {
			return a.layoutStandardWindowButton(g, &a.windowCloseButton, windowControlClose)
		}),
	)
}

func (a *App) layoutStandardWindowButton(gtx layout.Context, click *widget.Clickable, kind standardWindowControl) layout.Dimensions {
	size := image.Pt(gtx.Dp(46), min(gtx.Dp(42), gtx.Constraints.Max.Y))
	gtx.Constraints = layout.Exact(size)
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		ink := a.tokens().Colors.Text.PureBlack
		hovered, pressed := click.Hovered(), click.Pressed()
		if hovered {
			// Blend with the titlebar instead of covering its tint with opaque gray.
			bg := color.NRGBA{A: 18}
			if pressed {
				bg = color.NRGBA{R: 204, G: 204, B: 204, A: 255}
			}
			if kind == windowControlClose {
				bg = color.NRGBA{R: 232, G: 17, B: 35, A: 255}
				if pressed {
					bg = color.NRGBA{R: 197, G: 15, B: 31, A: 255}
				}
				ink = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			}
			paint.FillShape(gtx.Ops, bg, clip.Rect{Max: size}.Op())
		}
		descriptions := [...]string{"最小化", "最大化", "还原", "关闭"}
		semantic.DescriptionOp(descriptions[kind]).Add(gtx.Ops)
		drawStandardWindowControlGlyph(gtx, size, kind, ink)
		return layout.Dimensions{Size: size}
	})
}

func drawStandardWindowControlGlyph(gtx layout.Context, size image.Point, kind standardWindowControl, ink color.NRGBA) {
	cx, cy := float32(size.X)/2, float32(size.Y)/2
	d := float32(gtx.Dp(5))
	stroke := float32(max(1, gtx.Dp(1)))
	var path clip.Path
	path.Begin(gtx.Ops)
	switch kind {
	case windowControlMinimize:
		path.MoveTo(f32.Pt(cx-d, cy))
		path.LineTo(f32.Pt(cx+d, cy))
	case windowControlMaximize:
		path.MoveTo(f32.Pt(cx-d, cy-d))
		path.LineTo(f32.Pt(cx+d, cy-d))
		path.LineTo(f32.Pt(cx+d, cy+d))
		path.LineTo(f32.Pt(cx-d, cy+d))
		path.Close()
	case windowControlRestore:
		shift := float32(gtx.Dp(2))
		path.MoveTo(f32.Pt(cx-d+shift, cy-d))
		path.LineTo(f32.Pt(cx+d, cy-d))
		path.LineTo(f32.Pt(cx+d, cy+d-shift))
		path.MoveTo(f32.Pt(cx-d, cy-d+shift))
		path.LineTo(f32.Pt(cx+d-shift, cy-d+shift))
		path.LineTo(f32.Pt(cx+d-shift, cy+d))
		path.LineTo(f32.Pt(cx-d, cy+d))
		path.Close()
	case windowControlClose:
		path.MoveTo(f32.Pt(cx-d, cy-d))
		path.LineTo(f32.Pt(cx+d, cy+d))
		path.MoveTo(f32.Pt(cx+d, cy-d))
		path.LineTo(f32.Pt(cx-d, cy+d))
	}
	paint.FillShape(gtx.Ops, ink, clip.Stroke{Path: path.End(), Width: stroke}.Op())
}
