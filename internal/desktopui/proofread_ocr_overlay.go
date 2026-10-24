package desktopui

import (
	"image"
	"image/color"
	"math"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

func (a *App) drawProofreadingOCRPageOverlay(gtx layout.Context, size image.Point, pageIndex int) {
	state := a.proofreading
	if state == nil || state.ocrScan == nil || size.X <= 0 || size.Y <= 0 {
		return
	}
	scan := state.ocrScan
	if pageIndex >= 0 && pageIndex < len(scan.pages) {
		drawProofreadingOCRBlocks(gtx, size, scan.pages[pageIndex])
	}
	if scan.active() && scan.stage != proofreadingOCRMatching && scan.current == pageIndex {
		drawProofreadingLaser(gtx, size, scan.startedAt)
	}
}

func drawProofreadingOCRBlocks(gtx layout.Context, size image.Point, page proofreadingOCRPage) {
	if page.layout.Width <= 0 || page.layout.Height <= 0 {
		return
	}
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	defer area.Pop()
	ink := proofreadingOCRStatusColor(page.result.Status)
	stroke := max(1, gtx.Dp(1))
	for _, block := range page.layout.Blocks {
		x0 := int(block.Bounds[0] / float64(page.layout.Width) * float64(size.X))
		y0 := int(block.Bounds[1] / float64(page.layout.Height) * float64(size.Y))
		x1 := int(block.Bounds[2] / float64(page.layout.Width) * float64(size.X))
		y1 := int(block.Bounds[3] / float64(page.layout.Height) * float64(size.Y))
		minX, maxX := max(0, min(x0, x1)), min(size.X, max(x0, x1))
		minY, maxY := max(0, min(y0, y1)), min(size.Y, max(y0, y1))
		if maxX-minX < 2 || maxY-minY < 2 {
			continue
		}
		bounds := image.Rect(minX, minY, maxX, maxY)
		paint.FillShape(gtx.Ops, color.NRGBA{R: ink.R, G: ink.G, B: ink.B, A: 24}, clip.Rect(bounds).Op())
		for _, band := range []image.Rectangle{
			image.Rect(minX, minY, maxX, min(maxY, minY+stroke)),
			image.Rect(minX, max(minY, maxY-stroke), maxX, maxY),
			image.Rect(minX, minY, min(maxX, minX+stroke), maxY),
			image.Rect(max(minX, maxX-stroke), minY, maxX, maxY),
		} {
			paint.FillShape(gtx.Ops, ink, clip.Rect(band).Op())
		}
	}
}

func proofreadingOCRStatusColor(status string) color.NRGBA {
	switch status {
	case "matched":
		return color.NRGBA{R: 38, G: 145, B: 106, A: 220}
	case "ambiguous":
		return color.NRGBA{R: 211, G: 137, B: 40, A: 225}
	case "limited":
		return color.NRGBA{R: 113, G: 125, B: 138, A: 225}
	default:
		return color.NRGBA{R: 191, G: 76, B: 68, A: 225}
	}
}

func drawProofreadingLaser(gtx layout.Context, size image.Point, started time.Time) {
	if started.IsZero() || size.X <= 0 || size.Y <= 0 {
		return
	}
	const sweepDuration = 1450 * time.Millisecond
	elapsed := gtx.Now.Sub(started)
	if elapsed < 0 {
		elapsed = 0
	}
	phase := math.Mod(float64(elapsed), float64(sweepDuration)) / float64(sweepDuration)
	y := int(phase * float64(size.Y))
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	defer area.Pop()
	for _, band := range []struct {
		h   int
		col color.NRGBA
	}{{gtx.Dp(18), color.NRGBA{R: 246, G: 61, B: 72, A: 22}}, {gtx.Dp(7), color.NRGBA{R: 246, G: 61, B: 72, A: 55}}, {gtx.Dp(2), color.NRGBA{R: 255, G: 58, B: 69, A: 230}}} {
		h := max(1, band.h)
		top := y - h/2
		paint.FillShape(gtx.Ops, band.col, clip.Rect{Min: image.Pt(0, top), Max: image.Pt(size.X, top+h)}.Op())
	}
	gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(16 * time.Millisecond)})
}
