package desktopui

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"

	"glance/internal/document"
)

// drawPresentationBackground paints the resolved slide color followed by its
// optional image. The image uses stretch-to-fill behavior, matching a PPTX
// background blip fill's slide-sized viewport.
func drawPresentationBackground(gtx layout.Context, slide document.PresentationSlide, size image.Point) {
	bgColor := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	if slide.BgColorHex != "" {
		bgColor = parsePresentationHexColor(slide.BgColorHex, bgColor)
	}
	paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: size}.Op())
	if slide.BackgroundImage == nil || size.X <= 0 || size.Y <= 0 {
		return
	}
	imageGtx := gtx
	imageGtx.Constraints = layout.Exact(size)
	widget.Image{
		Src:      paint.NewImageOp(slide.BackgroundImage),
		Fit:      widget.Fill,
		Position: layout.Center,
	}.Layout(imageGtx)
}

// drawPresentationStaticElements paints read-only master/layout elements.
// It deliberately has no presentation view state, so these elements cannot
// register click, selection, or text-editing interactions.
func (a *App) drawPresentationStaticElements(gtx layout.Context, elements []document.PresentationElement, scale float64) {
	for _, elem := range elements {
		a.drawPresentationThumbnailElement(gtx, elem, scale)
	}
}
