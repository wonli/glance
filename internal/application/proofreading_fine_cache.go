package application

import (
	"sort"

	"github.com/go-text/typesetting/segmenter"
	"glance/internal/proofreading"
)

type proofreadingFineCache struct {
	valid           bool
	page, next      proofreading.PageID
	owned, neighbor proofreading.ContentToken
	boundaries      []int
}

// Consecutive boundary moves keep the concatenated pair's text unchanged. Shape
// grapheme boundaries once per pair, then move across them without resegmenting
// both full pages for every wheel threshold. External edits or refill invalidate
// the two immutable root identities.
func (c *ProofreadingController) adjustFineCached(page proofreading.PageID, steps int) (int, error) {
	neighbor, hasNext := c.Model.NextPage(page)
	if !hasNext || neighbor.Reviewed {
		return 0, nil
	}
	cache := &c.fine
	ownToken, nextToken := c.Model.ContentToken(page), c.Model.ContentToken(neighbor.ID)
	count := c.Model.RuneCount(page)
	if !cache.valid || cache.page != page || cache.next != neighbor.ID || cache.owned != ownToken || cache.neighbor != nextToken {
		owned := c.Model.Text(page)
		next, truncated := c.Model.PagePrefix(neighbor.ID, 32768)
		var sg segmenter.Segmenter
		sg.Init([]rune(owned + next))
		it := sg.GraphemeIterator()
		bounds := []int{0}
		for it.Next() {
			g := it.Grapheme()
			bounds = append(bounds, g.Offset+len(g.Text))
		}
		if truncated && len(bounds) > 1 {
			bounds = bounds[:len(bounds)-1]
		}
		*cache = proofreadingFineCache{valid: true, page: page, next: neighbor.ID, owned: ownToken, neighbor: nextToken, boundaries: bounds}
	}
	bounds := cache.boundaries
	end := count
	if steps > 0 {
		at := sort.Search(len(bounds), func(i int) bool { return bounds[i] > count })
		if at < len(bounds) {
			end = bounds[min(len(bounds)-1, at+steps-1)]
		}
	} else {
		at := sort.SearchInts(bounds, count) - 1
		if at >= 0 {
			end = bounds[max(0, at+steps+1)]
		}
		floor := count - c.Model.BoundaryReturnableRunes(page)
		if end < floor {
			at = sort.SearchInts(bounds, floor)
			if at < len(bounds) {
				end = bounds[at]
			}
		}
	}
	nextCount := c.Model.RuneCount(neighbor.ID)
	if err := c.Model.AdjustBoundary(page, end-count); err != nil {
		cache.valid = false
		return 0, err
	}
	after := c.Model.RuneCount(page)
	actual := sort.Search(len(bounds), func(i int) bool { return bounds[i] > max(count, after) }) - sort.Search(len(bounds), func(i int) bool { return bounds[i] > min(count, after) })
	if after < count {
		actual = -actual
	}
	cache.owned = c.Model.ContentToken(page)
	cache.neighbor = c.Model.ContentToken(neighbor.ID)
	// Exhausting the donor replaces the pair's text. Reaching a bounded preview
	// also requires fresh lookahead on the next threshold.
	if after-count == nextCount && nextCount > 0 || after >= bounds[len(bounds)-1] {
		cache.valid = false
	}
	return actual, nil
}
