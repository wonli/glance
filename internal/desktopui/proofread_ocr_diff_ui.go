package desktopui

import (
	"image"
	"image/color"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

var (
	proofreadScanInk       = color.NRGBA{R: 26, G: 115, B: 232, A: 255}
	proofreadScanFill      = color.NRGBA{R: 26, G: 115, B: 232, A: 80}
	proofreadDiffSameFill  = color.NRGBA{R: 231, G: 245, B: 237, A: 255}
	proofreadDiffSameInk   = color.NRGBA{R: 40, G: 112, B: 76, A: 255}
	proofreadDiffAOnlyFill = color.NRGBA{R: 252, G: 235, B: 232, A: 255}
	proofreadDiffAOnlyInk  = color.NRGBA{R: 166, G: 61, B: 53, A: 255}
	proofreadDiffBOnlyFill = color.NRGBA{R: 232, G: 241, B: 253, A: 255}
	proofreadDiffBOnlyInk  = color.NRGBA{R: 43, G: 99, B: 164, A: 255}
)

const proofreadingOCRDiffInitialDelay = 30 * time.Millisecond
const proofreadingOCRDiffMatchFrameDelay = 4 * time.Millisecond
const proofreadingOCRDiffDifferenceFrameDelay = 12 * time.Millisecond

func startProofreadingOCRDiffPlayback(step *proofreadingOCRStepState, comparison proofreadingOCRStepComparison, now time.Time) {
	if step == nil {
		return
	}
	step.diffPair = proofreadingOCRStepPair{APage: comparison.APage, BPage: comparison.BPage}
	step.diffDraftHash = comparison.DraftHash
	step.diffCursor = 0
	step.diffMatches = 0
	step.diffAOnly = 0
	step.diffBOnly = 0
	step.diffPaused = false
	step.diffRunning = len(comparison.Frames) > 0
	step.diffNextAt = time.Time{}
	if step.diffRunning {
		step.diffNextAt = now.Add(proofreadingOCRDiffInitialDelay)
	}
}

func (a *App) advanceProofreadingOCRDiffPlayback(gtx layout.Context, step *proofreadingOCRStepState) {
	if a == nil || step == nil {
		return
	}
	comparison, ok := a.currentProofreadingOCRStepComparison()
	if !ok || !comparison.HasEvidence || len(comparison.Frames) == 0 {
		step.diffRunning = false
		step.diffPaused = false
		step.diffCursor = 0
		step.diffDraftHash = ""
		step.diffNextAt = time.Time{}
		return
	}
	pair := proofreadingOCRStepPair{APage: comparison.APage, BPage: comparison.BPage}
	if step.diffPair != pair || step.diffDraftHash != comparison.DraftHash {
		step.diffPair = pair
		step.diffDraftHash = comparison.DraftHash
		step.diffCursor = len(comparison.Frames)
		step.diffRunning = false
		step.diffPaused = false
		step.diffNextAt = time.Time{}
		return
	}
	if !step.diffRunning || step.diffPaused {
		return
	}
	if gtx.Now.Before(step.diffNextAt) {
		gtx.Execute(op.InvalidateCmd{})
		return
	}
	if step.diffCursor >= len(comparison.Frames) {
		step.diffRunning = false
		step.diffPaused = false
		step.diffNextAt = time.Time{}
		gtx.Execute(op.InvalidateCmd{})
		return
	}
	// Advance by elapsed playback time, not by display frames. Bound catch-up
	// work so a delayed UI frame cannot monopolize the window loop.
	for advanced := 0; advanced < 64 && step.diffCursor < len(comparison.Frames) && !gtx.Now.Before(step.diffNextAt); advanced++ {
		frame := comparison.Frames[step.diffCursor]
		step.diffCursor++
		switch frame.Kind {
		case proofreadingOCRDiffEqual:
			step.diffMatches++
		case proofreadingOCRDiffAOnly:
			step.diffAOnly++
		case proofreadingOCRDiffBOnly:
			step.diffBOnly++
		case proofreadingOCRDiffReplace:
			step.diffAOnly++
			step.diffBOnly++
		}
		delay := proofreadingOCRDiffMatchFrameDelay
		if frame.Kind != proofreadingOCRDiffEqual {
			delay = proofreadingOCRDiffDifferenceFrameDelay
		}
		step.diffNextAt = step.diffNextAt.Add(delay)
	}

	gtx.Execute(op.InvalidateCmd{})
}

func proofreadingOCRCurrentDiffFrame(step *proofreadingOCRStepState, comparison proofreadingOCRStepComparison) (proofreadingOCRDiffFrame, bool) {
	if step == nil || !(step.diffRunning || step.diffPaused) || step.diffCursor <= 0 ||
		step.diffPair != (proofreadingOCRStepPair{APage: comparison.APage, BPage: comparison.BPage}) ||
		step.diffDraftHash != comparison.DraftHash || step.diffCursor > len(comparison.Frames) {
		return proofreadingOCRDiffFrame{}, false
	}
	return comparison.Frames[step.diffCursor-1], true
}

func proofreadingOCRScannedDiffFrames(step *proofreadingOCRStepState, comparison proofreadingOCRStepComparison) []proofreadingOCRDiffFrame {
	if step != nil && (step.diffRunning || step.diffPaused) &&
		step.diffPair == (proofreadingOCRStepPair{APage: comparison.APage, BPage: comparison.BPage}) && step.diffDraftHash == comparison.DraftHash {
		if step.diffCursor <= 0 {
			return nil
		}
		return comparison.Frames[:min(step.diffCursor, len(comparison.Frames))]
	}
	return comparison.Frames
}

func proofreadingOCRDiffFrameInk(kind proofreadingOCRDiffOpKind) color.NRGBA {
	switch kind {
	case proofreadingOCRDiffAOnly:
		return proofreadDiffAOnlyInk
	case proofreadingOCRDiffBOnly:
		return proofreadDiffBOnlyInk
	default:
		return proofreadDiffSameInk
	}
}

func proofreadingOCRDiffFrameBInk(kind proofreadingOCRDiffOpKind) color.NRGBA {
	if kind == proofreadingOCRDiffReplace {
		return proofreadDiffBOnlyInk
	}
	return proofreadingOCRDiffFrameInk(kind)
}

func proofreadingOCRDiffFrameAInk(kind proofreadingOCRDiffOpKind) color.NRGBA {
	if kind == proofreadingOCRDiffReplace {
		return proofreadDiffAOnlyInk
	}
	return proofreadingOCRDiffFrameInk(kind)
}

func (a *App) layoutProofreadingOCRDiffHeaderControls(gtx layout.Context) layout.Dimensions {
	return layout.Dimensions{}
}

func layoutProofreadingOCRDiffLegend(gtx layout.Context, a *App) layout.Dimensions {
	items := []struct {
		labelID string
		fill    color.NRGBA
	}{
		{labelID: "proofread.ai.step.diff.legend.same", fill: proofreadDiffSameFill},
		{labelID: "proofread.ai.step.diff.legend.a_only", fill: proofreadDiffAOnlyFill},
		{labelID: "proofread.ai.step.diff.legend.b_only", fill: proofreadDiffBOnlyFill},
	}
	children := make([]layout.FlexChild, 0, len(items)*2)
	for index, item := range items {
		item := item
		if index > 0 {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(gtx.Dp(5), 0)}
			}))
		}
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := gtx.Dp(8)
				paint.FillShape(gtx.Ops, item.fill, clip.UniformRRect(image.Rect(0, 0, size, size), max(1, size/3)).Op(gtx.Ops))
				return layout.Dimensions{Size: image.Pt(size, size)}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					label := material.Label(a.theme, 8, a.textID(item.labelID))
					label.Color = proofreadMuted
					label.MaxLines = 1
					return label.Layout(gtx)
				})
			}),
		)
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

func layoutProofreadingOCRDiffActionButton(gtx layout.Context, a *App, button *widget.Clickable, labelID string, width unit.Dp, background, ink color.NRGBA) layout.Dimensions {
	gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(width), gtx.Dp(23)))
	style := material.Button(a.theme, button, a.textID(labelID))
	style.TextSize = unit.Sp(8)
	style.Background = background
	style.Color = ink
	style.Inset = layout.Inset{Left: unit.Dp(3), Right: unit.Dp(3), Top: unit.Dp(2), Bottom: unit.Dp(2)}
	return style.Layout(gtx)
}

func (a *App) drawProofreadingOCRStepEmptyPage(
	gtx layout.Context,
	page proofreadingXPage,
	marginX, marginY float64,
	step *proofreadingOCRStepState,
	comparison proofreadingOCRStepComparison,
) {
	if a == nil || page.Width <= 0 || page.Height <= 0 || page.ContentStart != page.ContentEnd {
		return
	}
	preview := comparison.AOnlyText
	if step != nil && (step.diffRunning || step.diffPaused) {
		preview = ""
		if frame, ok := proofreadingOCRCurrentDiffFrame(step, comparison); ok && frame.Kind == proofreadingOCRDiffAOnly {
			preview = string(frame.AChar)
		}
	}
	a.drawProofreadingOCRStepEmptyPagePreview(gtx, page, marginX, marginY, preview)
}

func proofreadingOCRDiffAnchorRegions(editor *widget.Editor, page proofreadingXPage, window proofreadingEditorWindow, position int) []widget.Region {
	length := max(0, page.ContentEnd-page.ContentStart)
	local := max(0, min(length, position-page.ContentStart))
	if local == length {
		local--
	}
	if local < 0 || local >= length {
		return nil
	}
	rangeInWindow, valid := proofreadingOCRHighlightLocalRange(proofreadingOCRRangeSegment{
		PageIndex: page.PageIndex, Start: local, End: local + 1,
	}, window)
	if !valid {
		return nil
	}
	return editor.Regions(rangeInWindow.Start, rangeInWindow.End, nil)
}

func (a *App) drawProofreadingOCRStepEmptyPagePreview(gtx layout.Context, page proofreadingXPage, marginX, marginY float64, value string) {
	if a == nil || value == "" {
		return
	}
	contentSize := image.Pt(max(1, int(page.Width-2*marginX)), max(1, int(page.Height-2*marginY)))
	area := clip.Rect{Max: contentSize}.Push(gtx.Ops)
	defer area.Pop()
	labelText := a.textID("proofread.ai.step.diff.row.a_only") + "\n" + value
	child := gtx
	child.Metric = unit.Metric{PxPerDp: 1, PxPerSp: 1}
	child.Constraints = layout.Constraints{Max: contentSize}
	label := material.Label(a.theme, 11, labelText)
	label.Color = proofreadDiffAOnlyInk
	label.Font.Weight = font.SemiBold
	label.WrapPolicy = text.WrapGraphemes
	label.LineHeightScale = 1.28
	label.MaxLines = 10
	label.Truncator = "…"
	label.Layout(child)
}

func drawProofreadingOCRStepAOnlyBlockHighlights(gtx layout.Context, size image.Point, page proofreadingOCRPage, step *proofreadingOCRStepState, comparison proofreadingOCRStepComparison) {
	drawProofreadingOCRCharacterBoxes(gtx, size, step, comparison)
}
