package desktopui

import (
	"image"
	"image/color"

	"glance/internal/i18n"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/image/math/fixed"
)

func (a *App) proofreadingOCRBatchContentContext(gtx layout.Context) layout.Context {
	if a.proofreading != nil && a.proofreading.ocrStep != nil && a.proofreading.ocrStep.batch.open {
		return gtx.Disabled()
	}
	return gtx
}

func (a *App) processProofreadingOCRBatchControls(gtx layout.Context) {
	step := a.proofreadingOCRStep()
	if step == nil || !step.batch.open || a.modal.Visible {
		return
	}
	b := &step.batch
	wasOpen, wasRunning := b.open, b.running
	if b.pause.Clicked(gtx) && b.running {
		a.pauseProofreadingOCRBatch()
	}
	if b.resume.Clicked(gtx) && !b.running {
		a.resumeProofreadingOCRBatch()
	}
	if b.inspect.Clicked(gtx) && !b.running {
		b.open = false
	}
	if b.end.Clicked(gtx) {
		a.endProofreadingOCRBatch()
	}
	if b.close.Clicked(gtx) && b.checkpoint.Finished {
		b.open = false
	}
	if b.startCurrent.Clicked(gtx) {
		a.startProofreadingOCRBatchFromCurrentPage()
	}
	if b.differences.Clicked(gtx) && b.checkpoint.Finished && len(b.checkpoint.DifferencePages) > 0 {
		b.open = false
		a.proofreading.pageFilter = proofreadingPageFilterAll
		page := b.checkpoint.DifferencePages[0]
		a.setProofreadingCurrentPage(page)
		a.openProofreadingDiffInspectorForPair(proofreadingOCRStepPair{APage: page, BPage: page}, 0)
	}
	if b.open != wasOpen || b.running != wasRunning {
		gtx.Execute(op.InvalidateCmd{})
	}
}

func (a *App) layoutProofreadingOCRBatchAction(gtx layout.Context) layout.Dimensions {
	b := &a.proofreadingOCRStep().batch
	return a.layoutProofreadingToolbarAction(gtx, &b.entry, "proofread.ai.batch.title", "scan-pages", 142, a.pdfDoc != nil, a.requestProofreadingOCRBatch)
}

func (a *App) requestProofreadingSinglePageComparison() {
	step := a.proofreadingOCRStep()
	// The user has explicitly left the paused book task to work on one page.
	if step.batch.suspended {
		step.cancelTask()
		step.batch.suspended, step.batch.started = false, false
		step.diffRunning, step.diffPaused = false, false
	}
	a.compareProofreadingOCRStep()
}

func (a *App) layoutProofreadingOCRBatchDialog(gtx layout.Context) layout.Dimensions {
	if a.proofreading == nil || a.proofreading.ocrStep == nil || !a.proofreading.ocrStep.batch.open {
		return layout.Dimensions{}
	}
	step := a.proofreading.ocrStep
	b := &step.batch
	// Keep keyboard focus out of the underlying editor. Credential dialogs
	// remain above this dialog and retain their own focus when required.
	if !a.modal.Visible {
		for {
			_, ok := gtx.Event(key.Filter{Focus: &b.focusTag, Name: key.NameEscape})
			if !ok {
				break
			}
		}
		gtx.Execute(key.FocusCmd{Tag: &b.focusTag})
	}
	for {
		_, ok := gtx.Event(pointer.Filter{Target: &b.focusTag, Kinds: pointer.Scroll, ScrollX: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20}, ScrollY: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20}})
		if !ok {
			break
		}
	}
	for b.scrim.Clicked(gtx) {
	} // Deliberately never dismiss on the scrim.
	b.scrim.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		event.Op(gtx.Ops, &b.focusTag)
		paint.FillShape(gtx.Ops, color.NRGBA{R: 15, G: 23, B: 42, A: 112}, clip.Rect{Max: gtx.Constraints.Max}.Op())
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		width := min(gtx.Dp(540), max(1, gtx.Constraints.Max.X-gtx.Dp(40)))
		height := min(gtx.Dp(300), max(1, gtx.Constraints.Max.Y-gtx.Dp(40)))
		gtx.Constraints = layout.Exact(image.Pt(width, height))
		for b.card.Clicked(gtx) {
		}
		return b.card.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, contentSurface, clip.UniformRRect(image.Rect(0, 0, width, height), gtx.Dp(12)).Op(gtx.Ops))
			return layout.UniformInset(28).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layoutProofreadingIcon(gtx, "home", "scan-pages", 24, proofreadScanInk)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Left: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return a.layoutProofreadingBatchLabel(gtx, a.textID("proofread.ai.batch.title"), 17, proofreadInk, true)
								})
							}),
						)
					}),
					layout.Rigid(layout.Spacer{Height: 24}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						key := "proofread.ai.batch.processing"
						if !b.running {
							key = "proofread.ai.batch.paused"
						}
						if b.checkpoint.Finished {
							key = "proofread.ai.batch.complete"
						}
						return a.layoutProofreadingBatchLabel(gtx, a.textArgs(key, i18n.Args{"Page": proofreadOCRStepPageLabel(a, min(b.checkpoint.Total-1, b.checkpoint.Page)), "Total": i18n.FormatNumber(a.localeSnapshot().Resolved, float64(b.checkpoint.Total))}), 14, proofreadInk, true)
					}),
					layout.Rigid(layout.Spacer{Height: 12}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						fraction := float32(b.checkpoint.Completed) / float32(max(1, b.checkpoint.Total-b.checkpoint.Start))
						if b.running && step.diffRunning {
							if c, ok := a.currentProofreadingOCRStepComparison(); ok {
								fraction += float32(step.diffCursor) / float32(max(1, len(c.Frames))) / float32(max(1, b.checkpoint.Total-b.checkpoint.Start))
							}
						}
						style := material.ProgressBar(a.theme, fraction)
						style.Color, style.TrackColor, style.Height, style.Radius = proofreadScanInk, color.NRGBA{R: 230, G: 235, B: 242, A: 255}, 6, 3
						return style.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: 12}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutProofreadingBatchLabel(gtx, a.textArgs("proofread.ai.batch.summary", i18n.Args{"Done": i18n.FormatNumber(a.localeSnapshot().Resolved, float64(b.checkpoint.Completed)), "Diff": i18n.FormatNumber(a.localeSnapshot().Resolved, float64(len(b.checkpoint.DifferencePages)))}), 11, proofreadMuted, false)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						size := gtx.Constraints.Max
						message, ink := a.textID("proofread.ai.batch.paused_hint"), proofreadMuted
						if b.running {
							message, ink, _ = a.proofreadingOCRStatusText()
						}
						if b.err != "" {
							message, ink = b.err, color.NRGBA{R: 180, G: 65, B: 55, A: 255}
						}
						if b.checkpoint.Finished {
							message = a.textID("proofread.ai.batch.complete_hint")
						}
						layout.Inset{Top: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return a.layoutProofreadingBatchLabel(gtx, message, 11, ink, false)
						})
						return layout.Dimensions{Size: size}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.layoutProofreadingBatchButtons(gtx, b) }),
				)
			})
		})
	})
}

func (a *App) layoutProofreadingBatchLabel(gtx layout.Context, value string, size unit.Sp, ink color.NRGBA, bold bool) layout.Dimensions {
	gtx.Constraints.Min.Y = 0
	label := material.Label(a.theme, size, value)
	label.Color = ink
	label.MaxLines = 2
	if bold {
		label.Font.Weight = font.SemiBold
	}
	return label.Layout(gtx)
}

func (a *App) layoutProofreadingBatchButtons(gtx layout.Context, b *proofreadingOCRBatch) layout.Dimensions {
	type action struct {
		button  *widget.Clickable
		id      string
		primary bool
	}
	buttons := []action{
		{&b.end, "proofread.ai.batch.end", false},
		{&b.startCurrent, "proofread.ai.batch.start_current", false},
		{&b.pause, "proofread.ai.batch.pause", true},
	}
	if !b.running {
		buttons = []action{
			{&b.end, "proofread.ai.batch.end", false},
			{&b.inspect, "proofread.ai.batch.inspect", false},
			{&b.startCurrent, "proofread.ai.batch.start_current", false},
			{&b.resume, "proofread.ai.batch.resume", true},
		}
		if b.err != "" {
			buttons[len(buttons)-1].id = "common.action.retry"
		}
	}
	if b.checkpoint.Finished {
		buttons = []action{
			{&b.close, "common.action.close", false},
			{&b.startCurrent, "proofread.ai.batch.start_current", false},
		}
		if len(b.checkpoint.DifferencePages) > 0 {
			buttons = append(buttons, action{&b.differences, "proofread.ai.batch.differences", true})
		}
	}
	children := []layout.FlexChild{layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
	})}
	for _, item := range buttons {
		txt := a.textID(item.id)
		btnWidth := proofreadingBatchButtonWidth(gtx, a.theme, txt)
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(btnWidth, gtx.Dp(36))
				gtx.Constraints = layout.Exact(size)
				return item.button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					area := clip.Rect{Max: size}.Push(gtx.Ops)
					defer area.Pop()
					semantic.DescriptionOp(txt).Add(gtx.Ops)
					fill, ink := chromeSurface, proofreadInk
					if item.primary {
						fill, ink = proofreadScanInk, contentSurface
					}
					paint.FillShape(gtx.Ops, fill, clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(6)).Op(gtx.Ops))
					label := material.Label(a.theme, 12, txt)
					label.Color = ink
					return layoutProofreadingInspectorFinishLabel(gtx, size, label)
				})
			})
		}))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}

func proofreadingBatchButtonWidth(gtx layout.Context, theme *material.Theme, value string) int {
	label := material.Label(theme, 12, value)
	theme.Shaper.LayoutString(text.Parameters{Font: label.Font, PxPerEm: fixed.I(gtx.Sp(label.TextSize)), MaxWidth: 1 << 20, Locale: gtx.Locale}, value)
	var width fixed.Int26_6
	for glyph, ok := theme.Shaper.NextGlyph(); ok; glyph, ok = theme.Shaper.NextGlyph() {
		width = max(width, glyph.X+glyph.Advance)
	}
	return max(gtx.Dp(80), width.Ceil()+gtx.Dp(24))
}
