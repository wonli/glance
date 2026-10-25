package desktopui

import (
	"reflect"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"glance/internal/textalign"
)

func TestProofreadingOCRRangeSegmentsAcrossPagesAndUnicode(t *testing.T) {
	pages := []proofreadingXPage{
		{PageIndex: 4, ContentStart: 0, ContentEnd: 2, Content: "甲𠮷"},
		{PageIndex: 5, ContentStart: 2, ContentEnd: 5, Content: "乙e\u0301"},
		{PageIndex: 6, ContentStart: 5, ContentEnd: 7, Content: "丙丁"},
	}
	got := proofreadingOCRRangeSegments(pages, textalign.Range{Start: 1, End: 6})
	want := []proofreadingOCRRangeSegment{
		{PageIndex: 4, Start: 1, End: 2},
		{PageIndex: 5, Start: 0, End: 3},
		{PageIndex: 6, Start: 0, End: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("range segments = %+v, want %+v", got, want)
	}
}

func TestProofreadingOCRRangeSegmentsSkipsEmptyPagesAndRejectsInvalidRange(t *testing.T) {
	pages := []proofreadingXPage{
		{PageIndex: 0, ContentStart: 0, ContentEnd: 2, Content: "甲乙"},
		{PageIndex: 1, ContentStart: 2, ContentEnd: 2, Content: ""},
		{PageIndex: 2, ContentStart: 2, ContentEnd: 4, Content: "丙丁"},
	}
	for _, test := range []struct {
		name string
		r    textalign.Range
		want []proofreadingOCRRangeSegment
	}{
		{name: "exact first page edge", r: textalign.Range{Start: 0, End: 2}, want: []proofreadingOCRRangeSegment{{PageIndex: 0, Start: 0, End: 2}}},
		{name: "exact second page edge", r: textalign.Range{Start: 2, End: 4}, want: []proofreadingOCRRangeSegment{{PageIndex: 2, Start: 0, End: 2}}},
		{name: "crosses an empty page", r: textalign.Range{Start: 1, End: 3}, want: []proofreadingOCRRangeSegment{{PageIndex: 0, Start: 1, End: 2}, {PageIndex: 2, Start: 0, End: 1}}},
		{name: "reversed", r: textalign.Range{Start: 3, End: 2}},
		{name: "out of bounds", r: textalign.Range{Start: 3, End: 5}},
		{name: "negative", r: textalign.Range{Start: -1, End: 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := proofreadingOCRRangeSegments(pages, test.r); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("range segments = %+v, want %+v", got, test.want)
			}
		})
	}
	gap := []proofreadingXPage{{PageIndex: 0, ContentStart: 0, ContentEnd: 1}, {PageIndex: 1, ContentStart: 2, ContentEnd: 4}}
	if got := proofreadingOCRRangeSegments(gap, textalign.Range{Start: 0, End: 3}); len(got) != 0 {
		t.Fatalf("range crossing unowned content must be rejected, got %+v", got)
	}
}

func TestProofreadingOCRAlignmentSummaryUsesFirstAndLastMatchedPages(t *testing.T) {
	xPages := []proofreadingXPage{
		{PageIndex: 0, ContentStart: 0, ContentEnd: 10},
		{PageIndex: 1, ContentStart: 10, ContentEnd: 20},
		{PageIndex: 2, ContentStart: 20, ContentEnd: 30},
		{PageIndex: 3, ContentStart: 30, ContentEnd: 40},
		{PageIndex: 4, ContentStart: 40, ContentEnd: 50},
	}
	boundary := textalign.Boundary{NextPage: 4, Status: "range", Min: 18, Max: 20, Method: "anchored-edit-distance"}
	results := []proofreadingOCRPage{
		ocrAlignmentPage(1, "ambiguous", textalign.Range{Start: 1, End: 5}, nil),
		ocrAlignmentPage(2, "unmatched", textalign.Range{}, nil),
		ocrAlignmentPage(3, "matched", textalign.Range{Start: 12, End: 18}, &boundary),
		ocrAlignmentPage(4, "matched", textalign.Range{Start: 28, End: 33}, nil),
		ocrAlignmentPage(5, "ambiguous", textalign.Range{Start: 41, End: 45}, nil),
		ocrAlignmentPage(6, "unmatched", textalign.Range{}, nil),
	}
	summary := buildProofreadingOCRAlignmentSummary(results, xPages, 17)
	wantFirst := &proofreadingOCRAlignmentAnchor{APage: 3, BRange: textalign.Range{Start: 12, End: 18}, XFirstPage: 2, XLastPage: 2}
	wantLast := &proofreadingOCRAlignmentAnchor{APage: 4, BRange: textalign.Range{Start: 28, End: 33}, XFirstPage: 3, XLastPage: 4}
	wantProvisionalFirst := &proofreadingOCRAlignmentAnchor{APage: 1, BRange: textalign.Range{Start: 1, End: 5}, XFirstPage: 1, XLastPage: 1}
	wantProvisionalLast := &proofreadingOCRAlignmentAnchor{APage: 5, BRange: textalign.Range{Start: 41, End: 45}, XFirstPage: 5, XLastPage: 5}
	if !reflect.DeepEqual(summary.First, wantFirst) || !reflect.DeepEqual(summary.Last, wantLast) || !reflect.DeepEqual(summary.ProvisionalFirst, wantProvisionalFirst) || !reflect.DeepEqual(summary.ProvisionalLast, wantProvisionalLast) {
		t.Fatalf("unexpected alignment summary: %+v", summary)
	}
	if summary.ModelRevision != 17 {
		t.Fatalf("model revision=%d, want 17", summary.ModelRevision)
	}
	if results[2].result.BoundaryAfter == nil || *results[2].result.BoundaryAfter != boundary {
		t.Fatalf("summary changed the boundary evidence: %+v", results[2].result.BoundaryAfter)
	}
}

func TestProofreadingOCRAlignmentSummaryRecomputesAfterOwnershipChange(t *testing.T) {
	results := []proofreadingOCRPage{ocrAlignmentPage(1, "matched", textalign.Range{Start: 8, End: 12}, nil)}
	before := []proofreadingXPage{
		{PageIndex: 0, ContentStart: 0, ContentEnd: 10},
		{PageIndex: 1, ContentStart: 10, ContentEnd: 20},
	}
	after := []proofreadingXPage{
		{PageIndex: 0, ContentStart: 0, ContentEnd: 8},
		{PageIndex: 1, ContentStart: 8, ContentEnd: 20},
	}
	first := buildProofreadingOCRAlignmentSummary(results, before, 4)
	second := buildProofreadingOCRAlignmentSummary(results, after, 5)
	if first.First == nil || first.First.XFirstPage != 1 || first.First.XLastPage != 2 {
		t.Fatalf("initial page ownership span = %+v", first.First)
	}
	if second.First == nil || second.First.XFirstPage != 2 || second.First.XLastPage != 2 || second.ModelRevision != 5 {
		t.Fatalf("summary did not follow current page ownership/revision: %+v", second)
	}
}

func TestProofreadingOCRAlignmentSummaryHasNoConfirmedAnchorWhenAllPagesAreAmbiguousOrUnmatched(t *testing.T) {
	xPages := []proofreadingXPage{{PageIndex: 0, ContentStart: 0, ContentEnd: 10}}
	results := []proofreadingOCRPage{
		ocrAlignmentPage(1, "ambiguous", textalign.Range{Start: 1, End: 3}, nil),
		ocrAlignmentPage(2, "unmatched", textalign.Range{}, nil),
		ocrAlignmentPage(3, "ambiguous", textalign.Range{Start: 5, End: 8}, nil),
	}
	summary := buildProofreadingOCRAlignmentSummary(results, xPages, 2)
	if summary.First != nil || summary.Last != nil {
		t.Fatalf("ambiguous pages were promoted to confirmed anchors: %+v", summary)
	}
	if summary.ProvisionalFirst == nil || summary.ProvisionalFirst.APage != 1 || summary.ProvisionalLast == nil || summary.ProvisionalLast.APage != 3 {
		t.Fatalf("ambiguous endpoints were not kept provisional: %+v", summary)
	}
}

func TestProofreadingOCRAlignmentSummaryPublishedWhenResultsArrive(t *testing.T) {
	markdown := "甲乙"
	scan := newProofreadingOCRScan()
	scan.stage = proofreadingOCRMatching
	scan.markdownSnapshot = markdown
	scan.markdownSnapshotSet = true
	scan.sourceAHash = "pdf-hash"
	scan.sourceBHash = "b-hash"
	scan.results <- proofreadingOCRResult{pages: []proofreadingOCRPage{
		ocrAlignmentPage(2, "matched", textalign.Range{Start: 0, End: 1}, nil),
		ocrAlignmentPage(4, "matched", textalign.Range{Start: 1, End: 2}, nil),
	}}
	a := &App{proofreading: &proofreadingWorkspace{
		aSourceHash: "pdf-hash", sourceBHash: "b-hash", bDraftText: markdown,
		xPages: []proofreadingXPage{
			{PageIndex: 0, ContentStart: 0, ContentEnd: 1, Content: "甲"},
			{PageIndex: 1, ContentStart: 1, ContentEnd: 2, Content: "乙"},
		},
		ocrScan: scan,
	}}
	var ops op.Ops
	a.processProofreadingOCRScan(layout.Context{Ops: &ops})
	if scan.stage != proofreadingOCRComplete || scan.alignmentSummary.First == nil || scan.alignmentSummary.Last == nil {
		t.Fatalf("OCR completion did not publish its endpoint summary: stage=%v summary=%+v", scan.stage, scan.alignmentSummary)
	}
	if scan.alignmentSummary.First.APage != 2 || scan.alignmentSummary.Last.APage != 4 || scan.alignmentSummary.First.XFirstPage != 1 || scan.alignmentSummary.Last.XFirstPage != 2 {
		t.Fatalf("published summary has wrong A/X endpoint pages: %+v", scan.alignmentSummary)
	}
}

func ocrAlignmentPage(page int, status string, r textalign.Range, boundary *textalign.Boundary) proofreadingOCRPage {
	result := textalign.Result{Page: page, Status: status, BoundaryAfter: boundary}
	if status == "matched" {
		result.Selected = &textalign.Candidate{Range: r}
		result.PathChoices = []textalign.Candidate{{Range: r}}
	} else if status == "ambiguous" {
		result.PathChoices = []textalign.Candidate{{Range: r}}
	}
	return proofreadingOCRPage{result: result}
}
