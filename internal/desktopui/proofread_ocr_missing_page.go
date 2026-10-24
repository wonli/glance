package desktopui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"

	"glance/internal/ocr"
	"glance/internal/textalign"
)

// proofreadingOCRBPage is an immutable snapshot of one current B page. Start
// and End are half-open rune offsets in the exact B draft used for alignment.
type proofreadingOCRBPage struct {
	Start int
	End   int
	Text  string
}

// proofreadingOCRMissingPageCandidate is evidence for a preview-only fill.
// The hashes and page map fingerprint let the confirmation path reject stale
// page ownership even if the concatenated B text itself did not change.
type proofreadingOCRMissingPageCandidate struct {
	APage       int
	BPage       int
	Preview     string
	SourceAHash string
	SourceBHash string
	DraftHash   string
	PageMapHash string
}

func proofreadingOCRMissingPageCandidates(pages []proofreadingOCRPage, bPages []proofreadingOCRBPage, sourceAHash, sourceBHash, draftHash string) []proofreadingOCRMissingPageCandidate {
	pageMapHash, valid := proofreadingOCRPageMapHash(bPages)
	if !valid || len(pages) == 0 || len(bPages) == 0 {
		return nil
	}

	anchors := make([]int, len(pages))
	for i := range anchors {
		anchors[i] = -1
		if pages[i].result.Status != "matched" || pages[i].result.Selected == nil {
			continue
		}
		if bPage, ok := proofreadingOCRBPageForRange(bPages, pages[i].result.Selected.Range); ok {
			anchors[i] = bPage
		}
	}

	candidates := make([]proofreadingOCRMissingPageCandidate, 0)
	for aPage := range pages {
		page := pages[aPage]
		if page.result.Status != "unmatched" || page.result.Selected != nil {
			continue
		}
		preview := proofreadingOCRPagePreview(page.layout)
		if preview == "" {
			continue
		}

		bPage := -1
		switch {
		case aPage == 0 && len(pages) > 1 && anchors[1] == 1:
			bPage = 0
		case aPage == len(pages)-1 && aPage > 0 && anchors[aPage-1] == len(bPages)-2:
			bPage = len(bPages) - 1
		case aPage > 0 && aPage+1 < len(pages) && anchors[aPage-1] >= 0 && anchors[aPage+1] == anchors[aPage-1]+2:
			bPage = anchors[aPage-1] + 1
		}
		if bPage < 0 || bPage >= len(bPages) || strings.TrimSpace(bPages[bPage].Text) != "" {
			continue
		}

		candidates = append(candidates, proofreadingOCRMissingPageCandidate{
			APage: aPage, BPage: bPage, Preview: preview,
			SourceAHash: sourceAHash, SourceBHash: sourceBHash, DraftHash: draftHash,
			PageMapHash: pageMapHash,
		})
	}
	return candidates
}

func proofreadingOCRBPageForRange(pages []proofreadingOCRBPage, selected textalign.Range) (int, bool) {
	if selected.Start < 0 || selected.End <= selected.Start {
		return 0, false
	}
	found := -1
	for i, page := range pages {
		if selected.Start >= page.Start && selected.End <= page.End {
			if found >= 0 {
				return 0, false
			}
			found = i
		}
	}
	return found, found >= 0
}

func proofreadingOCRPageMapHash(pages []proofreadingOCRBPage) (string, bool) {
	boundary := 0
	for _, page := range pages {
		if page.Start != boundary || page.End < page.Start || page.End-page.Start != utf8.RuneCountInString(page.Text) {
			return "", false
		}
		boundary = page.End
	}
	encoded, err := json.Marshal(pages)
	if err != nil {
		return "", false
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), true
}

func proofreadingOCRPagePreview(page ocr.Layout) string {
	blocks := append([]ocr.LayoutBlock(nil), page.Blocks...)
	sort.SliceStable(blocks, func(i, j int) bool {
		left, right := blocks[i].Order, blocks[j].Order
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		return *left < *right
	})
	text := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if proofreadingOCRFurnitureLabel(block.Label) || strings.TrimSpace(block.Text) == "" {
			continue
		}
		text = append(text, block.Text)
	}
	return strings.Join(text, "\n\n")
}

func proofreadingOCRFurnitureLabel(label string) bool {
	switch label {
	case "number", "header", "footer", "header_image", "footer_image":
		return true
	default:
		return false
	}
}
