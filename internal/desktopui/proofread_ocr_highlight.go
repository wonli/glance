package desktopui

import (
	"image/color"
	"unicode/utf8"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"glance/internal/textalign"
)

type proofreadingOCRHighlightState struct {
	Active        bool
	APage         int
	BlockID       int
	BRange        textalign.Range
	SourceAHash   string
	SourceBHash   string
	OCRGeneration uint64
	ModelRevision uint64
}

func proofreadingOCRHighlightLocalRange(segment proofreadingOCRRangeSegment, window proofreadingEditorWindow) (textalign.Range, bool) {
	if window.Start < 0 || window.End <= window.Start || segment.End <= segment.Start {
		return textalign.Range{}, false
	}
	start := max(segment.Start, window.Start)
	end := min(segment.End, window.End)
	if start >= end {
		return textalign.Range{}, false
	}
	return textalign.Range{Start: start - window.Start, End: end - window.Start}, true
}

func proofreadingOCRHighlightCurrent(highlight proofreadingOCRHighlightState, scan *proofreadingOCRScan, state *proofreadingWorkspace, modelRevision uint64) bool {
	if !highlight.Active || scan == nil || state == nil || highlight.APage < 0 || highlight.APage >= len(scan.pages) ||
		highlight.OCRGeneration != scan.generation || highlight.ModelRevision != modelRevision ||
		highlight.SourceAHash == "" || highlight.SourceAHash != state.aSourceHash ||
		highlight.SourceBHash == "" || highlight.SourceBHash != state.sourceBHash ||
		highlight.SourceAHash != scan.sourceAHash || highlight.SourceBHash != scan.sourceBHash ||
		!scan.markdownSnapshotSet || scan.markdownSnapshot != state.bDraftText ||
		highlight.BRange.Start < 0 || highlight.BRange.End <= highlight.BRange.Start ||
		highlight.BRange.End > utf8.RuneCountInString(state.bDraftText) {
		return false
	}
	result := scan.pages[highlight.APage].result
	if result.Status != "matched" && result.Status != "ambiguous" {
		return false
	}
	containsEvidence := func(candidate *textalign.Candidate) bool {
		if candidate == nil {
			return false
		}
		for _, evidence := range candidate.Evidence {
			if evidence.BlockID == highlight.BlockID && evidence.Range == highlight.BRange {
				return true
			}
		}
		return false
	}
	if containsEvidence(result.Selected) {
		return true
	}
	for i := range result.PathChoices {
		if containsEvidence(&result.PathChoices[i]) {
			return true
		}
	}
	return false
}

func (a *App) drawProofreadingOCRMatchHighlight(gtx layout.Context, editor *widget.Editor, page proofreadingXPage, window proofreadingEditorWindow) {
	if a == nil || a.proofreading == nil || editor == nil || a.proofreading.ocrScan == nil {
		return
	}
	state := a.proofreading
	scan := state.ocrScan
	var revision uint64
	if state.controller != nil {
		revision = state.controller.Model.Revision()
	}
	if !proofreadingOCRHighlightCurrent(scan.highlight, scan, state, revision) {
		return
	}
	segments := proofreadingOCRRangeSegments(state.xPages, scan.highlight.BRange)
	for _, segment := range segments {
		if segment.PageIndex != page.PageIndex {
			continue
		}
		local, ok := proofreadingOCRHighlightLocalRange(segment, window)
		if !ok {
			return
		}
		for _, region := range editor.Regions(local.Start, local.End, nil) {
			paint.FillShape(gtx.Ops, proofreadingOCRHighlightColor, clip.Rect(region.Bounds).Op())
		}
		return
	}
}

var proofreadingOCRHighlightColor = color.NRGBA{R: 86, G: 174, B: 132, A: 48}
