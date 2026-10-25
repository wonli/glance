package desktopui

import "glance/internal/textalign"

type proofreadingOCRRangeSegment struct {
	PageIndex int
	Start     int
	End       int
}

type proofreadingOCRAlignmentAnchor struct {
	APage      int
	BRange     textalign.Range
	XFirstPage int
	XLastPage  int
}

type proofreadingOCRAlignmentSummary struct {
	First            *proofreadingOCRAlignmentAnchor
	Last             *proofreadingOCRAlignmentAnchor
	ProvisionalFirst *proofreadingOCRAlignmentAnchor
	ProvisionalLast  *proofreadingOCRAlignmentAnchor
	ModelRevision    uint64
}

// proofreadingOCRRangeSegments projects a global B rune range into the local
// half-open rune intervals owned by X pages. Invalid or incompletely covered
// ranges are rejected as a whole so callers never show a partial false match.
func proofreadingOCRRangeSegments(pages []proofreadingXPage, r textalign.Range) []proofreadingOCRRangeSegment {
	if r.Start < 0 || r.End <= r.Start || len(pages) == 0 {
		return nil
	}
	previousEnd := -1
	for _, page := range pages {
		if page.PageIndex < 0 || page.ContentStart < 0 || page.ContentEnd < page.ContentStart || page.ContentStart < previousEnd {
			return nil
		}
		previousEnd = page.ContentEnd
	}

	covered := r.Start
	segments := make([]proofreadingOCRRangeSegment, 0, 2)
	for _, page := range pages {
		start := max(r.Start, page.ContentStart)
		end := min(r.End, page.ContentEnd)
		if start >= end {
			continue
		}
		if start != covered {
			return nil
		}
		segments = append(segments, proofreadingOCRRangeSegment{
			PageIndex: page.PageIndex,
			Start:     start - page.ContentStart,
			End:       end - page.ContentStart,
		})
		covered = end
		if covered == r.End {
			return segments
		}
	}
	return nil
}

func buildProofreadingOCRAlignmentSummary(results []proofreadingOCRPage, xPages []proofreadingXPage, modelRevision uint64) proofreadingOCRAlignmentSummary {
	summary := proofreadingOCRAlignmentSummary{ModelRevision: modelRevision}
	for _, page := range results {
		var candidate *textalign.Candidate
		switch page.result.Status {
		case "matched":
			candidate = page.result.Selected
		case "ambiguous":
			if page.result.Selected != nil {
				candidate = page.result.Selected
			} else if len(page.result.PathChoices) > 0 {
				candidate = &page.result.PathChoices[0]
			}
		default:
			continue
		}
		if candidate == nil || page.result.Page <= 0 {
			continue
		}
		segments := proofreadingOCRRangeSegments(xPages, candidate.Range)
		if len(segments) == 0 {
			continue
		}
		anchor := &proofreadingOCRAlignmentAnchor{
			APage:      page.result.Page,
			BRange:     candidate.Range,
			XFirstPage: segments[0].PageIndex + 1,
			XLastPage:  segments[len(segments)-1].PageIndex + 1,
		}
		switch page.result.Status {
		case "matched":
			if summary.First == nil || anchor.APage < summary.First.APage {
				summary.First = anchor
			}
			if summary.Last == nil || anchor.APage > summary.Last.APage {
				summary.Last = anchor
			}
		case "ambiguous":
			if summary.ProvisionalFirst == nil || anchor.APage < summary.ProvisionalFirst.APage {
				summary.ProvisionalFirst = anchor
			}
			if summary.ProvisionalLast == nil || anchor.APage > summary.ProvisionalLast.APage {
				summary.ProvisionalLast = anchor
			}
		}
	}
	return summary
}

func (a *App) refreshProofreadingOCRAlignmentSummary() {
	if a == nil || a.proofreading == nil || a.proofreading.ocrScan == nil {
		return
	}
	state := a.proofreading
	scan := state.ocrScan
	scan.alignmentSummary = proofreadingOCRAlignmentSummary{}
	if scan.stage != proofreadingOCRComplete || !scan.markdownSnapshotSet || scan.markdownSnapshot != state.bDraftText ||
		scan.sourceAHash != state.aSourceHash || scan.sourceBHash != state.sourceBHash {
		return
	}
	var revision uint64
	if state.controller != nil {
		revision = state.controller.Model.Revision()
	}
	scan.alignmentSummary = buildProofreadingOCRAlignmentSummary(scan.pages, state.xPages, revision)
}
