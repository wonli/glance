package desktopui

import (
	"image"
	"image/color"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget/material"

	"glance/internal/i18n"
)

func (a *App) layoutProofreadingOCRAction(gtx layout.Context) layout.Dimensions {
	scan := a.proofreading.ocrScan
	if scan == nil {
		scan = newProofreadingOCRScan()
		a.proofreading.ocrScan = scan
	}
	if scan.button.Clicked(gtx) {
		if scan.active() {
			scan.cancelTask()
			scan.err = ""
			scan.notice = ""
		} else if scan.authRejected {
			a.openProofreadingOCRTokenDialog()
		} else {
			a.requestProofreadingOCRScan()
		}
	}
	labelID := "proofread.ai.scan.start"
	if scan.active() {
		labelID = "proofread.ai.scan.cancel"
	} else if scan.authRejected {
		labelID = "proofread.ai.token.replace"
	} else if scan.stage == proofreadingOCRComplete {
		labelID = "proofread.ai.scan.rescan"
	} else if a.proofreading.ocrJob != nil && a.proofreading.ocrJob.terminal() {
		labelID = "proofread.ai.scan.rescan"
	} else if _, ok := proofreadingOCRResumeJob(a.proofreading.ocrJob, a.proofreading.aSourceHash, a.proofreading.sourceBHash, a.proofreading.bDraftText); ok {
		labelID = "proofread.ai.scan.resume"
	}
	size := image.Pt(gtx.Dp(142), gtx.Dp(32))
	return scan.button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints = layout.Exact(size)
		fill := color.NRGBA{R: 251, G: 243, B: 242, A: 255}
		border := color.NRGBA{R: 230, G: 189, B: 185, A: 255}
		ink := color.NRGBA{R: 166, G: 67, B: 61, A: 255}
		if scan.active() {
			fill = color.NRGBA{R: 255, G: 237, B: 235, A: 255}
			border = color.NRGBA{R: 222, G: 132, B: 125, A: 255}
		}
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, fill, clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(6)).Op(gtx.Ops))
				proofreadRoundedBorder(gtx, size, 6, border, max(1, gtx.Dp(1)))
				return layout.Dimensions{Size: size}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					label := material.Label(a.theme, 11, a.textID(labelID))
					label.Color = ink
					return label.Layout(gtx)
				})
			},
		)
	})
}

func (a *App) layoutProofreadingOCRStatus(gtx layout.Context) layout.Dimensions {
	scan := a.proofreading.ocrScan
	if scan == nil {
		scan = newProofreadingOCRScan()
		a.proofreading.ocrScan = scan
	}
	statusID := scan.notice
	args := i18n.Args{}
	ink := color.NRGBA{R: 104, G: 120, B: 135, A: 255}
	if scan.err != "" {
		statusID = "proofread.ai.status.failed"
		args["Error"] = scan.err
		ink = color.NRGBA{R: 166, G: 67, B: 61, A: 255}
	} else if statusID == "" {
		switch scan.stage {
		case proofreadingOCRAuthenticating:
			statusID = "proofread.ai.status.authenticating"
		case proofreadingOCRUploading:
			statusID = "proofread.ai.status.uploading"
		case proofreadingOCRPending:
			statusID = "proofread.ai.status.pending"
		case proofreadingOCRRunning:
			statusID = "proofread.ai.status.running"
			args["Page"] = i18n.FormatNumber(a.localeSnapshot().Resolved, float64(scan.current+1))
			args["Total"] = i18n.FormatNumber(a.localeSnapshot().Resolved, float64(scan.total))
			args["Extracted"] = i18n.FormatNumber(a.localeSnapshot().Resolved, float64(scan.extracted))
		case proofreadingOCRMatching:
			statusID = "proofread.ai.status.matching"
		case proofreadingOCRComplete:
			statusID = "proofread.ai.status.complete"
			matched, ambiguous, unmatched, limited := 0, 0, 0, 0
			for _, page := range scan.pages {
				switch page.result.Status {
				case "matched":
					matched++
				case "ambiguous":
					ambiguous++
				case "limited":
					limited++
				default:
					unmatched++
				}
			}
			args["Pages"] = i18n.FormatNumber(a.localeSnapshot().Resolved, float64(len(scan.pages)))
			args["Matched"] = i18n.FormatNumber(a.localeSnapshot().Resolved, float64(matched))
			args["Ambiguous"] = i18n.FormatNumber(a.localeSnapshot().Resolved, float64(ambiguous))
			args["Unmatched"] = i18n.FormatNumber(a.localeSnapshot().Resolved, float64(unmatched))
			args["Limited"] = i18n.FormatNumber(a.localeSnapshot().Resolved, float64(limited))
		case proofreadingOCRCancelled:
			statusID = "proofread.ai.status.cancelled"
		default:
			statusID = "proofread.ai.status.ready"
		}
	}
	if statusID == "" {
		statusID = "proofread.ai.status.ready"
	}
	label := material.Label(a.theme, 10, a.textArgs(statusID, args))
	label.Color = ink
	label.MaxLines = 1
	return label.Layout(gtx)
}

func (a *App) drawProofreadingOCRThumbnailStatus(gtx layout.Context, size image.Point, pageIndex int) {
	scan := a.proofreading.ocrScan
	if scan == nil || pageIndex < 0 || pageIndex >= len(scan.pages) {
		return
	}
	page := scan.pages[pageIndex]
	ink := proofreadingOCRStatusColor(page.result.Status)
	description := proofreadingOCRStatusMessage(page.result.Status)
	if page.missingPage != nil {
		ink = proofreadingOCRMissingPageColor
		description = "proofread.ai.status.page_missing"
	}
	semantic.DescriptionOp(a.textID(description)).Add(gtx.Ops)
	box := image.Rect(size.X-gtx.Dp(13), gtx.Dp(5), size.X-gtx.Dp(5), gtx.Dp(13))
	paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Ellipse(box.Inset(-gtx.Dp(1))).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, ink, clip.Ellipse(box).Op(gtx.Ops))
}

func proofreadingOCRStatusMessage(status string) string {
	switch status {
	case "matched":
		return "proofread.ai.status.page_matched"
	case "ambiguous":
		return "proofread.ai.status.page_ambiguous"
	case "limited":
		return "proofread.ai.status.page_limited"
	default:
		return "proofread.ai.status.page_unmatched"
	}
}
