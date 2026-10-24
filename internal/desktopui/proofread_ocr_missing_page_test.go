package desktopui

import (
	"testing"

	"glance/internal/ocr"
	"glance/internal/textalign"
)

func TestProofreadingOCRMissingPageMapsOnlyTheUniqueBlankPageBetweenMatchedAnchors(t *testing.T) {
	pages := []proofreadingOCRPage{
		ocrPageWithMatch("matched", 0, 1, "甲"),
		ocrPageWithMatch("unmatched", -1, -1, "OCR 缺文"),
		ocrPageWithMatch("matched", 1, 2, "乙"),
	}
	bPages := []proofreadingOCRBPage{
		{Start: 0, End: 1, Text: "甲"},
		{Start: 1, End: 1, Text: ""},
		{Start: 1, End: 2, Text: "乙"},
	}

	got := proofreadingOCRMissingPageCandidates(pages, bPages, "pdf-hash", "b-file-hash", "draft-hash")
	if len(got) != 1 {
		t.Fatalf("unique blank page should produce one candidate, got %#v", got)
	}
	want := proofreadingOCRMissingPageCandidate{APage: 1, BPage: 1, Preview: "OCR 缺文", SourceAHash: "pdf-hash", SourceBHash: "b-file-hash", DraftHash: "draft-hash"}
	if got[0].APage != want.APage || got[0].BPage != want.BPage || got[0].Preview != want.Preview || got[0].SourceAHash != want.SourceAHash || got[0].SourceBHash != want.SourceBHash || got[0].DraftHash != want.DraftHash {
		t.Fatalf("candidate mismatch: got %#v want %#v", got[0], want)
	}
}

func TestProofreadingOCRMissingPageUsesTextAnchorsAtDocumentEdges(t *testing.T) {
	tests := []struct {
		name  string
		pages []proofreadingOCRPage
		b     []proofreadingOCRBPage
		wantA int
		wantB int
	}{
		{
			name: "first page",
			pages: []proofreadingOCRPage{
				ocrPageWithMatch("unmatched", -1, -1, "卷首缺文"),
				ocrPageWithMatch("matched", 0, 1, "正文"),
			},
			b:     []proofreadingOCRBPage{{Start: 0, End: 0}, {Start: 0, End: 2, Text: "正文"}},
			wantA: 0, wantB: 0,
		},
		{
			name: "last page",
			pages: []proofreadingOCRPage{
				ocrPageWithMatch("matched", 0, 1, "正文"),
				ocrPageWithMatch("unmatched", -1, -1, "卷末缺文"),
			},
			b:     []proofreadingOCRBPage{{Start: 0, End: 2, Text: "正文"}, {Start: 2, End: 2}},
			wantA: 1, wantB: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := proofreadingOCRMissingPageCandidates(test.pages, test.b, "A", "B", "D")
			if len(got) != 1 || got[0].APage != test.wantA || got[0].BPage != test.wantB {
				t.Fatalf("wrong edge mapping: %#v", got)
			}
		})
	}
}

func TestProofreadingOCRMissingPageRejectsAmbiguousPageGaps(t *testing.T) {
	matched := func(start, end int, text string) proofreadingOCRPage {
		return ocrPageWithMatch("matched", start, end, text)
	}
	unmatched := func(text string) proofreadingOCRPage { return ocrPageWithMatch("unmatched", -1, -1, text) }
	tests := []struct {
		name  string
		pages []proofreadingOCRPage
		b     []proofreadingOCRBPage
	}{
		{
			name:  "two blank B pages",
			pages: []proofreadingOCRPage{matched(0, 1, "甲"), unmatched("缺文"), matched(1, 2, "乙")},
			b: []proofreadingOCRBPage{
				{Start: 0, End: 1, Text: "甲"}, {Start: 1, End: 1}, {Start: 1, End: 1}, {Start: 1, End: 2, Text: "乙"},
			},
		},
		{
			name:  "multiple missing A pages",
			pages: []proofreadingOCRPage{matched(0, 1, "甲"), unmatched("缺一"), unmatched("缺二"), matched(1, 2, "乙")},
			b: []proofreadingOCRBPage{
				{Start: 0, End: 1, Text: "甲"}, {Start: 1, End: 1}, {Start: 1, End: 1}, {Start: 1, End: 2, Text: "乙"},
			},
		},
		{
			name:  "nonempty target page",
			pages: []proofreadingOCRPage{matched(0, 1, "甲"), unmatched("缺文"), matched(3, 4, "乙")},
			b: []proofreadingOCRBPage{
				{Start: 0, End: 1, Text: "甲"}, {Start: 1, End: 3, Text: "已有"}, {Start: 3, End: 4, Text: "乙"},
			},
		},
		{
			name:  "unmatched page has no OCR text",
			pages: []proofreadingOCRPage{matched(0, 1, "甲"), unmatched("  \n "), matched(1, 2, "乙")},
			b: []proofreadingOCRBPage{
				{Start: 0, End: 1, Text: "甲"}, {Start: 1, End: 1}, {Start: 1, End: 2, Text: "乙"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := proofreadingOCRMissingPageCandidates(test.pages, test.b, "A", "B", "D"); len(got) != 0 {
				t.Fatalf("ambiguous or unsafe gap produced candidates: %#v", got)
			}
		})
	}
}

func TestProofreadingOCRMissingPageRejectsUncertainAlignmentStatuses(t *testing.T) {
	for _, status := range []string{"matched", "ambiguous", "limited"} {
		t.Run(status, func(t *testing.T) {
			pages := []proofreadingOCRPage{
				ocrPageWithMatch("matched", 0, 1, "甲"),
				ocrPageWithMatch(status, 1, 2, "不可直接填入"),
				ocrPageWithMatch("matched", 2, 3, "乙"),
			}
			b := []proofreadingOCRBPage{{Start: 0, End: 1, Text: "甲"}, {Start: 1, End: 2, Text: ""}, {Start: 2, End: 3, Text: "乙"}}
			if got := proofreadingOCRMissingPageCandidates(pages, b, "A", "B", "D"); len(got) != 0 {
				t.Fatalf("status %q produced a fill candidate: %#v", status, got)
			}
		})
	}
}

func TestProofreadingOCRMissingPageRequiresNonFurnitureBodyText(t *testing.T) {
	pages := []proofreadingOCRPage{
		ocrPageWithMatch("matched", 0, 1, "甲"),
		{layout: ocr.Layout{Blocks: []ocr.LayoutBlock{
			{Label: "header", Text: "唐诗汇评"},
			{Label: "footer", Text: "卷四"},
			{Label: "number", Text: "三十"},
		}}, result: textalign.Result{Status: "unmatched"}},
		ocrPageWithMatch("matched", 1, 2, "乙"),
	}
	b := []proofreadingOCRBPage{{Start: 0, End: 1, Text: "甲"}, {Start: 1, End: 1}, {Start: 1, End: 2, Text: "乙"}}
	if got := proofreadingOCRMissingPageCandidates(pages, b, "A", "B", "D"); len(got) != 0 {
		t.Fatalf("page furniture alone must not produce body text for filling: %#v", got)
	}

	pages[1].layout.Blocks = append(pages[1].layout.Blocks, ocr.LayoutBlock{Label: "content", Text: "正文"})
	got := proofreadingOCRMissingPageCandidates(pages, b, "A", "B", "D")
	if len(got) != 1 || got[0].Preview != "正文" {
		t.Fatalf("candidate should retain body text and omit known furniture: %#v", got)
	}
}

func TestProofreadingOCRMissingPageOrdersVerticalOCRBlocksForPreview(t *testing.T) {
	one, two := 1, 2
	pages := []proofreadingOCRPage{
		ocrPageWithMatch("matched", 0, 1, "甲"),
		{layout: ocr.Layout{Blocks: []ocr.LayoutBlock{{Text: "後列", Order: &two}, {Text: "前列", Order: &one}}}, result: textalign.Result{Status: "unmatched"}},
		ocrPageWithMatch("matched", 1, 2, "乙"),
	}
	b := []proofreadingOCRBPage{{Start: 0, End: 1, Text: "甲"}, {Start: 1, End: 1}, {Start: 1, End: 2, Text: "乙"}}
	got := proofreadingOCRMissingPageCandidates(pages, b, "A", "B", "D")
	if len(got) != 1 || got[0].Preview != "前列\n\n後列" {
		t.Fatalf("vertical OCR preview lost reading order: %#v", got)
	}
}

func TestProofreadingOCRMissingPageDoesNotAssumePhysicalPageNumbersWhenCountsDiffer(t *testing.T) {
	pages := []proofreadingOCRPage{
		ocrPageWithMatch("matched", 0, 1, "首"),
		ocrPageWithMatch("unmatched", -1, -1, "漏页"),
		ocrPageWithMatch("matched", 1, 2, "尾"),
	}
	b := []proofreadingOCRBPage{
		{Start: 0, End: 1, Text: "首"}, {Start: 1, End: 1}, {Start: 1, End: 2, Text: "尾"}, {Start: 2, End: 2},
	}
	got := proofreadingOCRMissingPageCandidates(pages, b, "A", "B", "D")
	if len(got) != 1 || got[0].APage != 1 || got[0].BPage != 1 {
		t.Fatalf("mapping must use text anchors across unequal page counts: %#v", got)
	}
}

func TestProofreadingOCRMissingPageRejectsInvalidPageMapsAndCrossPageAnchors(t *testing.T) {
	tests := []struct {
		name  string
		pages []proofreadingOCRPage
		b     []proofreadingOCRBPage
	}{
		{
			name: "noncontiguous page ranges",
			pages: []proofreadingOCRPage{
				ocrPageWithMatch("matched", 0, 1, "甲"),
				ocrPageWithMatch("unmatched", -1, -1, "缺文"),
				ocrPageWithMatch("matched", 2, 3, "乙"),
			},
			b: []proofreadingOCRBPage{{Start: 0, End: 1, Text: "甲"}, {Start: 2, End: 2}, {Start: 2, End: 3, Text: "乙"}},
		},
		{
			name: "one anchor range spans two B pages",
			pages: []proofreadingOCRPage{
				ocrPageWithMatch("matched", 0, 2, "甲乙"),
				ocrPageWithMatch("unmatched", -1, -1, "缺文"),
				ocrPageWithMatch("matched", 2, 3, "丙"),
			},
			b: []proofreadingOCRBPage{{Start: 0, End: 1, Text: "甲"}, {Start: 1, End: 2, Text: "乙"}, {Start: 2, End: 2}, {Start: 2, End: 3, Text: "丙"}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := proofreadingOCRMissingPageCandidates(test.pages, test.b, "A", "B", "D"); len(got) != 0 {
				t.Fatalf("invalid page ownership produced a candidate: %#v", got)
			}
		})
	}
}

func TestProofreadingOCRMissingPageFingerprintTracksCurrentPageOwnership(t *testing.T) {
	pages := []proofreadingOCRPage{
		ocrPageWithMatch("matched", 0, 1, "甲"),
		ocrPageWithMatch("unmatched", -1, -1, "缺文"),
		ocrPageWithMatch("matched", 1, 2, "乙"),
	}
	one := []proofreadingOCRBPage{{Start: 0, End: 1, Text: "甲"}, {Start: 1, End: 1}, {Start: 1, End: 2, Text: "乙"}}
	two := []proofreadingOCRBPage{{Start: 0, End: 0}, {Start: 0, End: 1, Text: "甲"}, {Start: 1, End: 1}, {Start: 1, End: 2, Text: "乙"}}
	first := proofreadingOCRMissingPageCandidates(pages, one, "A", "B", "D")
	second := proofreadingOCRMissingPageCandidates(pages, two, "A", "B", "D")
	if len(first) != 1 || len(second) != 1 || second[0].BPage != 2 {
		t.Fatalf("page ownership change should move the candidate to the current target: first=%#v second=%#v", first, second)
	}
	if first[0].PageMapHash == "" || first[0].PageMapHash == second[0].PageMapHash {
		t.Fatalf("candidate page map fingerprint did not change: first=%q second=%q", first[0].PageMapHash, second[0].PageMapHash)
	}
}

func ocrPageWithMatch(status string, start, end int, text string) proofreadingOCRPage {
	page := proofreadingOCRPage{layout: ocr.Layout{Blocks: []ocr.LayoutBlock{{Text: text}}}, result: textalign.Result{Status: status}}
	if start >= 0 && end > start {
		page.result.Selected = &textalign.Candidate{Range: textalign.Range{Start: start, End: end}}
	}
	return page
}
