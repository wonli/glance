package desktopui

import (
	"image"
	"image/color"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

var wordGridlineColor = color.NRGBA{R: 145, G: 166, B: 190, A: 42}

const wordGridlineSpacingDP = 24

func drawWordGridlines(gtx layout.Context, app *App, bounds image.Rectangle) {
	if bounds.Empty() {
		return
	}
	pageClip := clip.Rect{Min: bounds.Min, Max: bounds.Max}.Push(gtx.Ops)
	semantic.DescriptionOp(app.textID("word.accessibility.gridlines")).Add(gtx.Ops)

	spacing := max(1, gtx.Dp(unit.Dp(wordGridlineSpacingDP)))
	lineWidth := max(1, gtx.Dp(unit.Dp(1)))
	for x := bounds.Min.X + spacing; x < bounds.Max.X; x += spacing {
		paint.FillShape(gtx.Ops, wordGridlineColor, clip.Rect{
			Min: image.Pt(x, bounds.Min.Y), Max: image.Pt(x+lineWidth, bounds.Max.Y),
		}.Op())
	}
	for y := bounds.Min.Y + spacing; y < bounds.Max.Y; y += spacing {
		paint.FillShape(gtx.Ops, wordGridlineColor, clip.Rect{
			Min: image.Pt(bounds.Min.X, y), Max: image.Pt(bounds.Max.X, y+lineWidth),
		}.Op())
	}
	pageClip.Pop()
}
