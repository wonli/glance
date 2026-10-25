package desktopui

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"glance/internal/i18n"
)

var (
	proofreadDiffSameFill  = color.NRGBA{R: 231, G: 245, B: 237, A: 255}
	proofreadDiffSameInk   = color.NRGBA{R: 40, G: 112, B: 76, A: 255}
	proofreadDiffAOnlyFill = color.NRGBA{R: 252, G: 235, B: 232, A: 255}
	proofreadDiffAOnlyInk  = color.NRGBA{R: 166, G: 61, B: 53, A: 255}
	proofreadDiffBOnlyFill = color.NRGBA{R: 232, G: 241, B: 253, A: 255}
	proofreadDiffBOnlyInk  = color.NRGBA{R: 43, G: 99, B: 164, A: 255}
)

type proofreadingOCRDiffDisplayRow struct {
	label string
	text  string
	color color.NRGBA
}

func (a *App) layoutProofreadingOCRDiffSummary(gtx layout.Context) layout.Dimensions {
	if a == nil || a.proofreading == nil || !a.proofreading.aiProofreading {
		return layout.Dimensions{}
	}
	comparison, ok := a.currentProofreadingOCRStepComparison()
	if !ok {
		return layout.Dimensions{}
	}
	step := a.proofreadingOCRStep()
	if step == nil {
		return layout.Dimensions{}
	}
	return layout.Stack{Alignment: layout.SE}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: unit.Dp(12), Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				width := max(gtx.Dp(250), gtx.Constraints.Max.X/2-gtx.Dp(12))
				size := image.Pt(width, gtx.Dp(174))
				gtx.Constraints = layout.Exact(size)
				return a.layoutProofreadingOCRDiffSummaryCard(gtx, step, comparison)
			})
		}),
	)
}

func (a *App) layoutProofreadingOCRDiffSummaryCard(gtx layout.Context, step *proofreadingOCRStepState, comparison proofreadingOCRStepComparison) layout.Dimensions {
	size := gtx.Constraints.Max
	fill := color.NRGBA{R: 255, G: 255, B: 255, A: 250}
	border := color.NRGBA{R: 170, G: 184, B: 198, A: 255}
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, fill, clip.UniformRRect(image.Rect(0, 0, size.X, size.Y), gtx.Dp(8)).Op(gtx.Ops))
			proofreadRoundedBorder(gtx, size, 8, border, max(1, gtx.Dp(1)))
			return layout.Dimensions{Size: size}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(7), Left: unit.Dp(9), Right: unit.Dp(9)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						label := material.Label(a.theme, 10, a.textArgs("proofread.ai.step.diff.title", i18n.Args{
							"APage": proofreadOCRStepPageLabel(a, comparison.APage),
							"BPage": proofreadOCRStepPageLabel(a, comparison.BPage),
						}))
						label.Color = proofreadInk
						label.Font.Weight = font.SemiBold
						label.MaxLines = 1
						return label.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(0, gtx.Dp(6))}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutProofreadingOCRDiffCounters(gtx, comparison)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return a.layoutProofreadingOCRDiffTextList(gtx, step, comparison)
					}),
				)
			})
		},
	)
}

func (a *App) layoutProofreadingOCRDiffCounters(gtx layout.Context, comparison proofreadingOCRStepComparison) layout.Dimensions {
	if !comparison.HasEvidence {
		return layout.Dimensions{}
	}
	format := func(value int) string {
		return i18n.FormatNumber(a.localeSnapshot().Resolved, float64(value))
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layoutProofreadingOCRDiffCounterBadge(gtx, a, "proofread.ai.step.diff.same", format(comparison.Matches), proofreadDiffSameFill, proofreadDiffSameInk)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(gtx.Dp(4), 0)}
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layoutProofreadingOCRDiffCounterBadge(gtx, a, "proofread.ai.step.diff.a_only", format(comparison.AOnly), proofreadDiffAOnlyFill, proofreadDiffAOnlyInk)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(gtx.Dp(4), 0)}
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layoutProofreadingOCRDiffCounterBadge(gtx, a, "proofread.ai.step.diff.b_only", format(comparison.BOnly), proofreadDiffBOnlyFill, proofreadDiffBOnlyInk)
		}),
	)
}

func layoutProofreadingOCRDiffCounterBadge(gtx layout.Context, a *App, messageID, count string, fill, ink color.NRGBA) layout.Dimensions {
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(21))
	gtx.Constraints = layout.Exact(size)
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, fill, clip.UniformRRect(image.Rect(0, 0, size.X, size.Y), gtx.Dp(4)).Op(gtx.Ops))
			return layout.Dimensions{Size: size}
		},
		func(gtx layout.Context) layout.Dimensions {
			label := material.Label(a.theme, 8, a.textArgs(messageID, i18n.Args{"Count": count}))
			label.Color = ink
			label.MaxLines = 1
			return layout.Center.Layout(gtx, label.Layout)
		},
	)
}

func (a *App) layoutProofreadingOCRDiffTextList(gtx layout.Context, step *proofreadingOCRStepState, comparison proofreadingOCRStepComparison) layout.Dimensions {
	if !comparison.HasEvidence {
		return a.layoutProofreadingXLabel(gtx, a.textID("proofread.ai.step.diff.no_evidence"), unit.Sp(9), proofreadMuted, false)
	}
	rows := []proofreadingOCRDiffDisplayRow{
		{label: a.textID("proofread.ai.step.diff.row.same"), text: proofreadingOCRDiffDisplayText(comparison.MatchedText, a), color: proofreadDiffSameInk},
		{label: a.textID("proofread.ai.step.diff.row.a_only"), text: proofreadingOCRDiffDisplayText(comparison.AOnlyText, a), color: proofreadDiffAOnlyInk},
		{label: a.textID("proofread.ai.step.diff.row.b_only"), text: proofreadingOCRDiffDisplayText(comparison.BOnlyText, a), color: proofreadDiffBOnlyInk},
	}
	if !comparison.Complete {
		rows = append([]proofreadingOCRDiffDisplayRow{{label: a.textID("proofread.ai.step.diff.incomplete"), color: color.NRGBA{R: 159, G: 108, B: 31, A: 255}}}, rows...)
	}
	step.diffList.Axis = layout.Vertical
	return step.diffList.Layout(gtx, len(rows), func(gtx layout.Context, index int) layout.Dimensions {
		row := rows[index]
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(2), Right: unit.Dp(3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := material.Label(a.theme, 9, row.label)
					label.Color = proofreadMuted
					label.Font.Weight = font.SemiBold
					label.MaxLines = 1
					return label.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := material.Label(a.theme, 10, row.text)
					label.Color = row.color
					label.WrapPolicy = text.WrapGraphemes
					return label.Layout(gtx)
				}),
			)
		})
	})
}

func proofreadingOCRDiffDisplayText(value string, a *App) string {
	if value != "" {
		return value
	}
	if a == nil {
		return "—"
	}
	return a.textID("proofread.ai.step.diff.none")
}

func drawProofreadingOCRStepAOnlyBlockHighlights(gtx layout.Context, size image.Point, page proofreadingOCRPage, comparison proofreadingOCRStepComparison) {
	if len(comparison.AOnlyBlockIDs) == 0 || size.X <= 0 || size.Y <= 0 || page.layout.Width <= 0 || page.layout.Height <= 0 {
		return
	}
	blockIDs := make(map[int]struct{}, len(comparison.AOnlyBlockIDs))
	for _, id := range comparison.AOnlyBlockIDs {
		blockIDs[id] = struct{}{}
	}
	stroke := max(gtx.Dp(2), 2)
	ink := proofreadDiffAOnlyInk
	fill := proofreadDiffAOnlyFill
	fill.A = 60
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	defer area.Pop()
	for _, block := range page.layout.Blocks {
		if _, marked := blockIDs[block.ID]; !marked {
			continue
		}
		bounds := proofreadingOCRBlockPixelBounds(size, page.layout.Width, page.layout.Height, block.Bounds)
		if bounds.Dx() < 2 || bounds.Dy() < 2 {
			continue
		}
		paint.FillShape(gtx.Ops, fill, clip.Rect(bounds).Op())
		for _, band := range []image.Rectangle{
			image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Max.X, min(bounds.Max.Y, bounds.Min.Y+stroke)),
			image.Rect(bounds.Min.X, max(bounds.Min.Y, bounds.Max.Y-stroke), bounds.Max.X, bounds.Max.Y),
			image.Rect(bounds.Min.X, bounds.Min.Y, min(bounds.Max.X, bounds.Min.X+stroke), bounds.Max.Y),
			image.Rect(max(bounds.Min.X, bounds.Max.X-stroke), bounds.Min.Y, bounds.Max.X, bounds.Max.Y),
		} {
			paint.FillShape(gtx.Ops, ink, clip.Rect(band).Op())
		}
	}
}
