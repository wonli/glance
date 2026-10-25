package desktopui

import (
	"image"
	"image/color"
	"math"
	"time"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"glance/internal/textalign"
)

func (a *App) drawProofreadingOCRPageOverlay(gtx layout.Context, size image.Point, pageIndex int) {
	state := a.proofreading
	if state == nil || state.ocrScan == nil || size.X <= 0 || size.Y <= 0 {
		return
	}
	if state.aiProofreading {
		step := state.ocrStep
		if step != nil {
			if page, ok := step.pages[pageIndex]; ok && step.pageSourceHash[pageIndex] == state.aSourceHash {
				comparison, hasComparison := a.currentProofreadingOCRStepComparison()
				if hasComparison && comparison.APage == pageIndex {
					page.result = comparison.Result
				}
				drawProofreadingOCRBlocks(gtx, size, page, -1, time.Time{})
				if hasComparison && comparison.APage == pageIndex {
					drawProofreadingOCRStepAOnlyBlockHighlights(gtx, size, page, comparison)
				}
			}
			if step.active() && step.page == pageIndex {
				drawProofreadingLaser(gtx, size, step.startedAt)
				gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(16 * time.Millisecond)})
			}
		}
		return
	}
	scan := state.ocrScan
	if pageIndex >= 0 && pageIndex < len(scan.pages) {
		page := &scan.pages[pageIndex]
		activeBlock := -1
		var pulseAt time.Time
		if scan.link.Active && scan.link.APage == pageIndex {
			activeBlock = scan.link.BlockID
			pulseAt = scan.link.PulseAt
		}
		drawProofreadingOCRBlocks(gtx, size, *page, activeBlock, pulseAt)
		if scan.stage == proofreadingOCRComplete {
			a.layoutProofreadingOCRBlockTargets(gtx, size, pageIndex, page)
		}
	}
	if proofreadingOCRAnimationShouldRun(scan, pageIndex, true, true) {
		drawProofreadingLaser(gtx, size, scan.matchingStartedAt)
	}
}

func proofreadingOCRAnimationShouldRun(scan *proofreadingOCRScan, pageIndex int, pageVisible, proofreadingViewActive bool) bool {
	return scan != nil && scan.stage == proofreadingOCRMatching && scan.follow &&
		scan.alignment.Phase == textalign.AlignmentPhaseCandidates && scan.current == pageIndex &&
		pageVisible && proofreadingViewActive
}

func proofreadingOCRPulseAlpha(now, pulseAt time.Time, steady uint8) uint8 {
	const duration = 750 * time.Millisecond
	if pulseAt.IsZero() {
		return steady
	}
	elapsed := now.Sub(pulseAt)
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed >= duration {
		return steady
	}
	return steady + uint8((float64(255-steady)*float64(duration-elapsed))/float64(duration))
}

func proofreadingOCRPulseActive(now, pulseAt time.Time) bool {
	return !pulseAt.IsZero() && now.Sub(pulseAt) < 750*time.Millisecond
}

func drawProofreadingOCRBlocks(gtx layout.Context, size image.Point, page proofreadingOCRPage, activeBlockID int, pulseAt time.Time) {
	if page.layout.Width <= 0 || page.layout.Height <= 0 {
		return
	}
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	defer area.Pop()
	ink := proofreadingOCRStatusColor(page.result.Status)
	stroke := max(1, gtx.Dp(1))
	for _, block := range page.layout.Blocks {
		bounds := proofreadingOCRBlockPixelBounds(size, page.layout.Width, page.layout.Height, block.Bounds)
		if bounds.Dx() < 2 || bounds.Dy() < 2 {
			continue
		}
		paint.FillShape(gtx.Ops, color.NRGBA{R: ink.R, G: ink.G, B: ink.B, A: 24}, clip.Rect(bounds).Op())
		for _, band := range []image.Rectangle{
			image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Max.X, min(bounds.Max.Y, bounds.Min.Y+stroke)),
			image.Rect(bounds.Min.X, max(bounds.Min.Y, bounds.Max.Y-stroke), bounds.Max.X, bounds.Max.Y),
			image.Rect(bounds.Min.X, bounds.Min.Y, min(bounds.Max.X, bounds.Min.X+stroke), bounds.Max.Y),
			image.Rect(max(bounds.Min.X, bounds.Max.X-stroke), bounds.Min.Y, bounds.Max.X, bounds.Max.Y),
		} {
			paint.FillShape(gtx.Ops, ink, clip.Rect(band).Op())
		}
		if block.ID == activeBlockID {
			active := color.NRGBA{R: 32, G: 166, B: 124, A: proofreadingOCRPulseAlpha(gtx.Now, pulseAt, 150)}
			width := max(gtx.Dp(2), stroke+1)
			for _, band := range []image.Rectangle{
				image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Max.X, min(bounds.Max.Y, bounds.Min.Y+width)),
				image.Rect(bounds.Min.X, max(bounds.Min.Y, bounds.Max.Y-width), bounds.Max.X, bounds.Max.Y),
				image.Rect(bounds.Min.X, bounds.Min.Y, min(bounds.Max.X, bounds.Min.X+width), bounds.Max.Y),
				image.Rect(max(bounds.Min.X, bounds.Max.X-width), bounds.Min.Y, bounds.Max.X, bounds.Max.Y),
			} {
				paint.FillShape(gtx.Ops, active, clip.Rect(band).Op())
			}
		}
	}
	if activeBlockID >= 0 && proofreadingOCRPulseActive(gtx.Now, pulseAt) {
		gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(16 * time.Millisecond)})
	}
}

func proofreadingOCRBlockPixelBounds(size image.Point, layoutWidth, layoutHeight int, bounds [4]float64) image.Rectangle {
	if size.X <= 0 || size.Y <= 0 || layoutWidth <= 0 || layoutHeight <= 0 {
		return image.Rectangle{}
	}
	x0 := int(bounds[0] / float64(layoutWidth) * float64(size.X))
	y0 := int(bounds[1] / float64(layoutHeight) * float64(size.Y))
	x1 := int(bounds[2] / float64(layoutWidth) * float64(size.X))
	y1 := int(bounds[3] / float64(layoutHeight) * float64(size.Y))
	return image.Rect(max(0, min(x0, x1)), max(0, min(y0, y1)), min(size.X, max(x0, x1)), min(size.Y, max(y0, y1)))
}

func (a *App) layoutProofreadingOCRBlockTargets(gtx layout.Context, size image.Point, pageIndex int, page *proofreadingOCRPage) {
	if a == nil || a.proofreading == nil || a.proofreading.ocrScan == nil || page == nil || pageIndex < 0 || pageIndex >= len(a.proofreading.ocrScan.pages) {
		return
	}
	for _, block := range page.layout.Blocks {
		if len(proofreadingOCRBlockChoice(*page, block.ID)) == 0 {
			continue
		}
		bounds := proofreadingOCRBlockPixelBounds(size, page.layout.Width, page.layout.Height, block.Bounds)
		if bounds.Dx() < 2 || bounds.Dy() < 2 {
			continue
		}
		button := page.blockButton(block.ID)
		if button == nil {
			continue
		}
		clicked := button.Clicked(gtx)
		offset := op.Offset(bounds.Min).Push(gtx.Ops)
		child := gtx
		child.Constraints = layout.Exact(bounds.Size())
		button.Layout(child, func(gtx layout.Context) layout.Dimensions {
			semantic.DescriptionOp(a.textID("proofread.ai.link.block")).Add(gtx.Ops)
			return layout.Dimensions{Size: bounds.Size()}
		})
		offset.Pop()
		if clicked {
			a.activateProofreadingOCRBlock(pageIndex, block.ID)
		}
	}
}

func proofreadingOCRStatusColor(status string) color.NRGBA {
	switch status {
	case "recognized":
		return color.NRGBA{R: 74, G: 113, B: 153, A: 185}
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
