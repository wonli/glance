package desktopui

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/i18n"
	"glance/internal/proofreading"
)

var proofreadingOCRMissingPageColor = color.NRGBA{R: 110, G: 77, B: 174, A: 255}

func (a *App) refreshProofreadingOCRMissingPageCandidates() {
	if a == nil || a.proofreading == nil || a.proofreading.ocrScan == nil {
		return
	}
	state, scan := a.proofreading, a.proofreading.ocrScan
	for i := range scan.pages {
		scan.pages[i].missingPage = nil
	}
	if scan.stage != proofreadingOCRComplete || !scan.markdownSnapshotSet || scan.markdownSnapshot != state.bDraftText || scan.sourceAHash != state.aSourceHash || scan.sourceBHash != state.sourceBHash {
		return
	}
	bPages := make([]proofreadingOCRBPage, len(state.xPages))
	for i, page := range state.xPages {
		bPages[i] = proofreadingOCRBPage{Start: page.ContentStart, End: page.ContentEnd, Text: page.Content}
	}
	candidates := proofreadingOCRMissingPageCandidates(scan.pages, bPages, scan.sourceAHash, scan.sourceBHash, proofreadingOCRDraftSnapshotHash(scan.markdownSnapshot))
	for i := range candidates {
		candidate := candidates[i]
		if candidate.APage >= 0 && candidate.APage < len(scan.pages) {
			scan.pages[candidate.APage].missingPage = &candidate
		}
	}
}

func (a *App) proofreadingOCRMissingPageCandidateForBPage(bPage int) (*proofreadingOCRMissingPageCandidate, *proofreadingOCRPage) {
	if a == nil || a.proofreading == nil || a.proofreading.ocrScan == nil {
		return nil, nil
	}
	for i := range a.proofreading.ocrScan.pages {
		page := &a.proofreading.ocrScan.pages[i]
		if page.missingPage != nil && page.missingPage.BPage == bPage {
			return page.missingPage, page
		}
	}
	return nil, nil
}

func (a *App) openProofreadingOCRMissingPagePreview(bPage int) bool {
	if a == nil || a.proofreading == nil || a.modal.Visible {
		return false
	}
	state := a.proofreading
	a.refreshProofreadingOCRMissingPageCandidates()
	candidate, _ := a.proofreadingOCRMissingPageCandidateForBPage(bPage)
	if candidate == nil || state.controller == nil {
		return false
	}
	copy := *candidate
	state.ocrPreview = &copy
	state.ocrPreviewList.Position = layout.Position{}
	args := i18n.Args{
		"APage": i18n.FormatNumber(a.localeSnapshot().Resolved, float64(copy.APage+1)),
		"BPage": i18n.FormatNumber(a.localeSnapshot().Resolved, float64(copy.BPage+1)),
	}
	a.modal.ShowCustomMessage(
		"proofread.ocr.missing-page",
		i18n.Message{ID: "proofread.ai.missing_page.title"},
		i18n.Message{ID: "proofread.ai.missing_page.body", Args: args},
		520,
		func(gtx layout.Context) layout.Dimensions { return a.layoutProofreadingOCRMissingPagePreview(gtx) },
		func(action string) {
			switch action {
			case "fill":
				if !a.confirmProofreadingOCRMissingPageFill() {
					a.modal.Hide()
				}
			case "cancel", "close":
				state.ocrPreview = nil
				a.modal.Hide()
			}
		},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
		modalDialogActionSpec{ID: "fill", LabelRef: i18n.Message{ID: "proofread.ai.missing_page.fill"}},
	)
	return true
}

func (a *App) confirmProofreadingOCRMissingPageFill() bool {
	if a == nil || a.proofreading == nil || a.proofreading.ocrPreview == nil {
		return false
	}
	state := a.proofreading
	preview := *state.ocrPreview
	a.refreshProofreadingOCRMissingPageCandidates()
	current, _ := a.proofreadingOCRMissingPageCandidateForBPage(preview.BPage)
	valid := state.controller != nil && current != nil && *current == preview &&
		preview.SourceAHash == state.aSourceHash && preview.SourceBHash == state.sourceBHash &&
		preview.DraftHash == proofreadingOCRDraftSnapshotHash(state.bDraftText) &&
		preview.PageMapHash == proofreadingOCRCurrentPageMapHash(state.xPages) &&
		preview.BPage >= 0 && preview.BPage < len(state.xPages) && strings.TrimSpace(state.xPages[preview.BPage].Content) == "" &&
		state.ocrScan != nil && state.ocrScan.markdownSnapshot == state.bDraftText &&
		state.ocrScan.sourceAHash == state.aSourceHash && state.ocrScan.sourceBHash == state.sourceBHash
	if !valid {
		state.ocrPreview = nil
		if state.ocrScan != nil {
			state.ocrScan.notice = "proofread.ai.status.missing_page_stale"
		}
		return false
	}
	if err := state.controller.Model.ReplacePage(proofreading.PageID(preview.BPage+1), preview.Preview); err != nil {
		state.draftPersistError = err.Error()
		state.ocrPreview = nil
		return false
	}
	state.ocrPreview = nil
	a.modal.Hide()
	a.proofreadingModelChanged(false)
	return true
}

func proofreadingOCRCurrentPageMapHash(pages []proofreadingXPage) string {
	bPages := make([]proofreadingOCRBPage, len(pages))
	for i, page := range pages {
		bPages[i] = proofreadingOCRBPage{Start: page.ContentStart, End: page.ContentEnd, Text: page.Content}
	}
	hash, _ := proofreadingOCRPageMapHash(bPages)
	return hash
}

func (a *App) layoutProofreadingOCRMissingPagePreview(gtx layout.Context) layout.Dimensions {
	if a == nil || a.proofreading == nil || a.proofreading.ocrPreview == nil {
		return layout.Dimensions{}
	}
	state := a.proofreading
	height := gtx.Dp(unit.Dp(248))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			bounds := image.Rectangle{Max: gtx.Constraints.Max}
			shape := clip.RRect{Rect: bounds, NW: gtx.Dp(unit.Dp(7)), NE: gtx.Dp(unit.Dp(7)), SW: gtx.Dp(unit.Dp(7)), SE: gtx.Dp(unit.Dp(7))}
			paint.FillShape(gtx.Ops, color.NRGBA{R: 248, G: 249, B: 251, A: 255}, shape.Op(gtx.Ops))
			paint.FillShape(gtx.Ops, color.NRGBA{R: 218, G: 223, B: 230, A: 255}, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
			return layout.Dimensions{Size: bounds.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			state.ocrPreviewList.Axis = layout.Vertical
			return layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(12), Left: unit.Dp(14), Right: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return state.ocrPreviewList.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
					label := material.Body2(a.theme, state.ocrPreview.Preview)
					label.Color = color.NRGBA{R: 43, G: 49, B: 58, A: 255}
					return label.Layout(gtx)
				})
			})
		},
	)
}

func (a *App) layoutProofreadingOCRMissingPageBadge(gtx layout.Context, size image.Point, bPage int) layout.Dimensions {
	candidate, page := a.proofreadingOCRMissingPageCandidateForBPage(bPage)
	if candidate == nil || page == nil {
		return layout.Dimensions{}
	}
	if page.missingPageButton.Clicked(gtx) {
		a.openProofreadingOCRMissingPagePreview(bPage)
	}
	gtx.Constraints = layout.Exact(size)
	return layout.Stack{Alignment: layout.NE}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: size} }),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(7), Right: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				width, height := gtx.Dp(unit.Dp(108)), gtx.Dp(unit.Dp(23))
				gtx.Constraints = layout.Exact(image.Pt(width, height))
				return page.missingPageButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					paint.FillShape(gtx.Ops, proofreadingOCRMissingPageColor, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(5))).Op(gtx.Ops))
					semantic.DescriptionOp(a.textID("proofread.ai.status.page_missing")).Add(gtx.Ops)
					label := material.Label(a.theme, 9, a.textID("proofread.ai.missing_page.view"))
					label.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
					label.MaxLines = 1
					return layout.Center.Layout(gtx, label.Layout)
				})
			})
		}),
	)
}
