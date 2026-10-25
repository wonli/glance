package desktopui

import (
	"image"
	"image/color"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/i18n"
	"glance/internal/textalign"
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

func (a *App) proofreadingOCRStatusText() (string, color.NRGBA, bool) {
	if a == nil || a.proofreading == nil {
		return "", color.NRGBA{}, false
	}
	if a.proofreading.aiProofreading {
		step := a.proofreading.ocrStep
		if step == nil {
			return "", color.NRGBA{}, false
		}
		if step.err != "" {
			return a.textArgs("proofread.ai.step.failed", i18n.Args{"Error": step.err}), color.NRGBA{R: 166, G: 67, B: 61, A: 255}, true
		}
		if step.active() {
			args := i18n.Args{
				"APage":     proofreadOCRStepPageLabel(a, max(0, step.page)),
				"BPage":     proofreadOCRStepPageLabel(a, max(0, step.bPage)),
				"Extracted": i18n.FormatNumber(a.localeSnapshot().Resolved, float64(step.extracted)),
				"Total":     i18n.FormatNumber(a.localeSnapshot().Resolved, float64(max(1, step.total))),
			}
			switch step.phase {
			case proofreadingOCRStepCheckingCache:
				return a.textArgs("proofread.ai.step.cache_checking", args), proofreadBlue, true
			case proofreadingOCRStepAuthenticating:
				return a.textArgs("proofread.ai.step.authenticating", args), proofreadBlue, true
			case proofreadingOCRStepRendering:
				return a.textArgs("proofread.ai.step.rendering", args), proofreadBlue, true
			case proofreadingOCRStepUploading:
				return a.textArgs("proofread.ai.step.uploading", args), proofreadBlue, true
			case proofreadingOCRStepRunning:
				return a.textArgs("proofread.ai.step.running", args), proofreadBlue, true
			case proofreadingOCRStepComparing, proofreadingOCRStepCheckingBoundary:
				return a.textArgs("proofread.ai.step.comparing", args), proofreadBlue, true
			}
		}
		comparison, hasComparison := a.currentProofreadingOCRStepComparison()
		if hasComparison {
			if step.diffRunning || step.diffPaused {
				progress := min(len(comparison.Frames), max(0, step.diffCursor))
				msg := a.textArgs("proofread.ai.step.diff.animation.progress", i18n.Args{
					"Done":  i18n.FormatNumber(a.localeSnapshot().Resolved, float64(progress)),
					"Total": i18n.FormatNumber(a.localeSnapshot().Resolved, float64(len(comparison.Frames))),
				})
				return msg, proofreadBlue, true
			}
			if !comparison.HasEvidence {
				return a.textID("proofread.ai.step.no_evidence"), color.NRGBA{R: 180, G: 110, B: 30, A: 255}, true
			}
			args := i18n.Args{
				"Matches": i18n.FormatNumber(a.localeSnapshot().Resolved, float64(comparison.Matches)),
				"AOnly":   i18n.FormatNumber(a.localeSnapshot().Resolved, float64(comparison.AOnly)),
				"BOnly":   i18n.FormatNumber(a.localeSnapshot().Resolved, float64(comparison.BOnly)),
			}
			msg := a.textArgs("proofread.ai.step.compared", args)
			if comparison.AOnly > 0 || comparison.BOnly > 0 {
				return msg, color.NRGBA{R: 196, G: 56, B: 46, A: 255}, true
			}
			return msg, color.NRGBA{R: 38, G: 145, B: 106, A: 255}, true
		}
		return "", color.NRGBA{}, false
	}
	if a.proofreading.ocrScan == nil {
		return "", color.NRGBA{}, false
	}
	scan := a.proofreading.ocrScan
	if a.validateProofreadingOCRLink() {
		link := scan.link
		choices := proofreadingOCRBlockChoice(scan.pages[link.APage], link.BlockID)
		args := i18n.Args{
			"APage": i18n.FormatNumber(a.localeSnapshot().Resolved, float64(link.APage+1)),
			"First": i18n.FormatNumber(a.localeSnapshot().Resolved, float64(link.XFirstPage+1)),
			"Last":  i18n.FormatNumber(a.localeSnapshot().Resolved, float64(link.XLastPage+1)),
		}
		statusID := "proofread.ai.link.active"
		if len(choices) > 1 {
			current := 0
			for i, choice := range choices {
				if choice.PathChoiceIndex == link.PathChoiceIndex && choice.Range == link.BRange {
					current = i
					break
				}
			}
			statusID = "proofread.ai.link.choice"
			args["Choice"] = i18n.FormatNumber(a.localeSnapshot().Resolved, float64(current+1))
			args["Choices"] = i18n.FormatNumber(a.localeSnapshot().Resolved, float64(len(choices)))
		}
		return a.textArgs(statusID, args), color.NRGBA{R: 38, G: 145, B: 106, A: 255}, true
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
		case proofreadingOCRCheckingCache:
			statusID = "proofread.ai.status.cache_checking"
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
			statusID = "proofread.ai.status.matching.path"
			args["Completed"] = i18n.FormatNumber(a.localeSnapshot().Resolved, float64(scan.alignment.CompletedPages))
			args["Total"] = i18n.FormatNumber(a.localeSnapshot().Resolved, float64(max(1, scan.alignment.TotalPages)))
			if scan.alignment.Phase == textalign.AlignmentPhaseCandidates {
				statusID = "proofread.ai.status.matching.progress"
				args["Page"] = i18n.FormatNumber(a.localeSnapshot().Resolved, float64(max(1, scan.alignment.Page)))
			}
		case proofreadingOCRComplete:
			args = proofreadingOCRSummaryArgs(a, scan)
			switch {
			case scan.cacheErr != "":
				statusID = "proofread.ai.status.cache_save_failed"
				args["Error"] = scan.cacheErr
				ink = color.NRGBA{R: 166, G: 67, B: 61, A: 255}
			case scan.cacheHit:
				statusID = "proofread.ai.status.cache_reused"
			default:
				statusID = "proofread.ai.status.cache_saved"
			}
		case proofreadingOCRCancelled:
			statusID = "proofread.ai.status.cancelled"
		default:
			return "", color.NRGBA{}, false
		}
	}
	if statusID == "proofread.ai.status.cache_save_failed" && scan.cacheErr != "" {
		args["Error"] = scan.cacheErr
		ink = color.NRGBA{R: 166, G: 67, B: 61, A: 255}
	}
	message := a.textArgs(statusID, args)
	if scan.stage == proofreadingOCRComplete {
		if evidence := proofreadingOCRStatusEvidence(a, scan); evidence != "" {
			message += " · " + evidence
		}
	}
	return message, ink, true
}

func (a *App) layoutProofreadingOCRFollowControl(gtx layout.Context) layout.Dimensions {
	if a == nil || a.proofreading == nil || a.proofreading.ocrScan == nil || a.proofreading.ocrScan.stage != proofreadingOCRMatching {
		return layout.Dimensions{}
	}
	scan := a.proofreading.ocrScan
	if scan.followButton.Clicked(gtx) {
		a.setProofreadingOCRFollow(!scan.follow)
	}
	labelID := "proofread.ai.follow.resume"
	if scan.follow {
		labelID = "proofread.ai.follow.pause"
	}
	semantic.DescriptionOp(a.textID(labelID)).Add(gtx.Ops)
	size := image.Pt(gtx.Dp(92), gtx.Dp(24))
	gtx.Constraints = layout.Exact(size)
	button := material.Button(a.theme, &scan.followButton, a.textID(labelID))
	button.TextSize = unit.Sp(9)
	button.Background = color.NRGBA{R: 235, G: 240, B: 246, A: 255}
	button.Color = color.NRGBA{R: 65, G: 91, B: 121, A: 255}
	button.Inset = layout.Inset{Left: unit.Dp(4), Right: unit.Dp(4), Top: unit.Dp(2), Bottom: unit.Dp(2)}
	return button.Layout(gtx)
}

func (a *App) layoutProofreadingOCRLinkControls(gtx layout.Context) layout.Dimensions {
	if a == nil || a.proofreading == nil || a.proofreading.ocrScan == nil || !a.validateProofreadingOCRLink() {
		return layout.Dimensions{}
	}
	scan := a.proofreading.ocrScan
	choices := proofreadingOCRBlockChoice(scan.pages[scan.link.APage], scan.link.BlockID)
	if scan.linkUnlink.Clicked(gtx) {
		a.clearProofreadingOCRLink(true)
		return layout.Dimensions{}
	}
	if len(choices) > 1 {
		if scan.linkPrevious.Clicked(gtx) {
			a.moveProofreadingOCRLinkChoice(-1)
		}
		if scan.linkNext.Clicked(gtx) {
			a.moveProofreadingOCRLinkChoice(1)
		}
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if len(choices) <= 1 {
				return layout.Dimensions{}
			}
			return layoutProofreadingOCRLinkButton(gtx, a, &scan.linkPrevious, "proofread.ai.link.previous", "‹")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if len(choices) <= 1 {
				return layout.Dimensions{}
			}
			return layoutProofreadingOCRLinkButton(gtx, a, &scan.linkNext, "proofread.ai.link.next", "›")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layoutProofreadingOCRLinkButton(gtx, a, &scan.linkUnlink, "proofread.ai.link.unlink", "×")
		}),
	)
}

func layoutProofreadingOCRLinkButton(gtx layout.Context, a *App, button *widget.Clickable, labelID, fallback string) layout.Dimensions {
	width := gtx.Dp(28)
	if labelID == "proofread.ai.link.unlink" {
		width = gtx.Dp(48)
	}
	gtx.Constraints = layout.Constraints{Min: image.Pt(width, 0), Max: image.Pt(width, gtx.Dp(25))}
	label := fallback
	if a != nil {
		semantic.DescriptionOp(a.textID(labelID)).Add(gtx.Ops)
		if labelID == "proofread.ai.link.unlink" {
			label = a.textID(labelID)
		}
	}
	style := material.Button(a.theme, button, label)
	style.TextSize = unit.Sp(10)
	style.Background = color.NRGBA{R: 232, G: 241, B: 238, A: 255}
	style.Color = color.NRGBA{R: 44, G: 119, B: 92, A: 255}
	style.Inset = layout.Inset{Left: unit.Dp(3), Right: unit.Dp(3), Top: unit.Dp(2), Bottom: unit.Dp(2)}
	return style.Layout(gtx)
}

func proofreadingOCRSummaryArgs(a *App, scan *proofreadingOCRScan) i18n.Args {
	args := i18n.Args{}
	if a == nil || scan == nil {
		return args
	}
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
	locale := a.localeSnapshot().Resolved
	args["Pages"] = i18n.FormatNumber(locale, float64(len(scan.pages)))
	args["Matched"] = i18n.FormatNumber(locale, float64(matched))
	args["Ambiguous"] = i18n.FormatNumber(locale, float64(ambiguous))
	args["Unmatched"] = i18n.FormatNumber(locale, float64(unmatched))
	args["Limited"] = i18n.FormatNumber(locale, float64(limited))
	return args
}

func proofreadingOCRStatusEvidence(a *App, scan *proofreadingOCRScan) string {
	if a == nil || scan == nil || len(scan.pages) == 0 {
		return ""
	}
	state := a.proofreading
	var xPages []proofreadingXPage
	if state != nil {
		xPages = state.xPages
	}
	summary := buildProofreadingOCRAlignmentSummary(scan.pages, xPages, 0)
	anchorArgs := i18n.Args{}
	anchorID := "proofread.ai.evidence.no_anchor"
	switch {
	case summary.First != nil && summary.Last != nil:
		anchorID = "proofread.ai.evidence.anchors"
		anchorArgs = proofreadingOCRAnchorArgs(a, summary.First, summary.Last)
	case summary.ProvisionalFirst != nil && summary.ProvisionalLast != nil:
		anchorID = "proofread.ai.evidence.provisional_anchors"
		anchorArgs = proofreadingOCRAnchorArgs(a, summary.ProvisionalFirst, summary.ProvisionalLast)
	}
	anchors := a.textArgs(anchorID, anchorArgs)
	point, rangeCount, unresolved := 0, 0, 0
	for _, page := range scan.pages {
		if page.result.BoundaryAfter == nil {
			continue
		}
		switch page.result.BoundaryAfter.Status {
		case "point":
			point++
		case "range":
			rangeCount++
		default:
			unresolved++
		}
	}
	boundaryID := "proofread.ai.evidence.boundary"
	boundaryArgs := i18n.Args{
		"Point":      i18n.FormatNumber(a.localeSnapshot().Resolved, float64(point)),
		"Range":      i18n.FormatNumber(a.localeSnapshot().Resolved, float64(rangeCount)),
		"Unresolved": i18n.FormatNumber(a.localeSnapshot().Resolved, float64(unresolved)),
	}
	if point+rangeCount+unresolved == 0 {
		boundaryID = "proofread.ai.evidence.no_boundary"
		boundaryArgs = i18n.Args{}
	}
	return a.textArgs("proofread.ai.evidence.summary", i18n.Args{
		"Anchors":    anchors,
		"Boundaries": a.textArgs(boundaryID, boundaryArgs),
	})
}

func proofreadingOCRAnchorArgs(a *App, first, last *proofreadingOCRAlignmentAnchor) i18n.Args {
	if a == nil || first == nil || last == nil {
		return i18n.Args{}
	}
	locale := a.localeSnapshot().Resolved
	return i18n.Args{
		"FirstA":    i18n.FormatNumber(locale, float64(first.APage)),
		"FirstB":    i18n.FormatNumber(locale, float64(first.XFirstPage)),
		"FirstLast": i18n.FormatNumber(locale, float64(first.XLastPage)),
		"LastA":     i18n.FormatNumber(locale, float64(last.APage)),
		"LastB":     i18n.FormatNumber(locale, float64(last.XFirstPage)),
		"LastLast":  i18n.FormatNumber(locale, float64(last.XLastPage)),
	}
}

func (a *App) drawProofreadingOCRThumbnailStatus(gtx layout.Context, size image.Point, pageIndex int) {
	if a == nil || a.proofreading == nil || pageIndex < 0 {
		return
	}
	if a.proofreading.aiProofreading {
		step := a.proofreading.ocrStep
		if step == nil || step.pageSourceHash[pageIndex] != a.proofreading.aSourceHash {
			return
		}
		if _, ok := step.pages[pageIndex]; !ok {
			return
		}
		ink := color.NRGBA{R: 74, G: 113, B: 153, A: 220}
		description := "proofread.ai.step.thumbnail.recognized"
		if comparison, ok := step.comparisons[proofreadingOCRStepPair{APage: pageIndex, BPage: pageIndex}]; ok && proofreadingOCRStepComparisonValid(a.proofreading, comparison) {
			ink = proofreadingOCRStatusColor(comparison.Result.Status)
			description = "proofread.ai.step.thumbnail.compared"
		}
		semantic.DescriptionOp(a.textID(description)).Add(gtx.Ops)
		box := image.Rect(size.X-gtx.Dp(13), gtx.Dp(5), size.X-gtx.Dp(5), gtx.Dp(13))
		paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Ellipse(box.Inset(-gtx.Dp(1))).Op(gtx.Ops))
		paint.FillShape(gtx.Ops, ink, clip.Ellipse(box).Op(gtx.Ops))
		return
	}
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
	case "recognized":
		return "proofread.ai.step.thumbnail.recognized"
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
