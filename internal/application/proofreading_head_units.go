package application

import (
	"errors"
	"strings"

	"github.com/go-text/typesetting/segmenter"
	"glance/internal/proofreading"
)

// AdjustPageHeadUnits returns a count relative to the viewed page: down pulls
// the previous suffix into this head; up sends this prefix to the previous box.
// Coarse upward input follows this page's displayed lines, not the unfilled
// remainder of the previous box's last line.
func (c *ProofreadingController) AdjustPageHeadUnits(page proofreading.PageID, steps int, lines bool) (int, error) {
	current, ok := c.Model.Page(page)
	previous, hasPrevious := c.Model.PreviousPage(page)
	if !ok || current.Reviewed || hasPrevious && previous.Reviewed {
		return 0, errors.New("page cannot be adjusted")
	}
	if !hasPrevious || steps == 0 {
		return 0, nil
	}
	if !lines || steps > 0 {
		moved, err := c.AdjustPageUnits(previous.ID, -steps, lines)
		return -moved, err
	}
	owned := c.Model.Text(page)
	count := c.Model.RuneCount(page)
	if count == 0 {
		return 0, nil
	}
	shaped, ok := c.Layout.Pages[page]
	if !ok || !strings.HasPrefix(shaped.Text, owned) {
		return 0, ErrProofreadingLayoutPending
	}
	remaining := min(4096, -steps)
	end := 0
	for _, line := range shaped.Lines {
		if line.End <= end {
			continue
		}
		end = min(count, line.End)
		remaining--
		if remaining == 0 || end == count {
			break
		}
	}
	if end == 0 {
		return 0, ErrProofreadingLayoutPending
	}
	var sg segmenter.Segmenter
	sg.Init([]rune(owned))
	it := sg.GraphemeIterator()
	units := 0
	for it.Next() {
		g := it.Grapheme()
		units++
		if g.Offset+len(g.Text) >= end {
			end = g.Offset + len(g.Text)
			break
		}
	}
	if err := c.Model.AdjustBoundary(previous.ID, end); err != nil {
		return 0, err
	}
	return -units, nil
}
