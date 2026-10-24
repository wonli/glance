package application

import (
	"errors"
	"strings"

	"github.com/go-text/typesetting/segmenter"
	"glance/internal/proofreading"
)

// AdjustPageUnits moves visible characters (extended grapheme clusters), or
// shaped lines for coarse input. No font/layout result is needed for fine input.
// The returned count is the signed number of visible characters transferred.
func (c *ProofreadingController) AdjustPageUnits(page proofreading.PageID, steps int, lines bool) (int, error) {
	current, ok := c.Model.Page(page)
	if !ok || current.Reviewed {
		return 0, errors.New("page cannot be adjusted")
	}
	if steps == 0 {
		return 0, nil
	}
	steps = max(-4096, min(4096, steps))
	if !lines {
		return c.adjustFineCached(page, steps)
	}
	owned := c.Model.Text(page)
	count := c.Model.RuneCount(page)
	var preview strings.Builder
	preview.WriteString(owned)
	truncated := false
	if steps > 0 || lines {
		if neighbor, ok := c.Model.NextPage(page); ok && !neighbor.Reviewed {
			text, clipped := c.Model.PagePrefix(neighbor.ID, 32768)
			preview.WriteString(text)
			truncated = clipped
		}
	}
	var sg segmenter.Segmenter
	sg.Init([]rune(preview.String()))
	it := sg.GraphemeIterator()
	boundaries := []int{0}
	for it.Next() {
		g := it.Grapheme()
		boundaries = append(boundaries, g.Offset+len(g.Text))
	}
	if truncated && len(boundaries) > 1 {
		boundaries = boundaries[:len(boundaries)-1]
	}
	end := count
	if lines {
		shaped, ok := c.Layout.Pages[page]
		if !ok || !strings.HasPrefix(shaped.Text, owned) || !strings.HasPrefix(preview.String(), shaped.Text) {
			return 0, ErrProofreadingLayoutPending
		}
		for range absProofreadingSteps(steps) {
			next := end
			if steps > 0 {
				for _, l := range shaped.Lines {
					if l.End > end {
						next = l.End
						break
					}
				}
			} else {
				for i := len(shaped.Lines) - 1; i >= 0; i-- {
					if shaped.Lines[i].Start < end {
						next = shaped.Lines[i].Start
						break
					}
				}
			}
			end = next
		}
		// A line boundary is normally a grapheme boundary; round outward safely.
		if steps > 0 {
			end = min(end, boundaries[len(boundaries)-1])
			for _, b := range boundaries {
				if b >= end {
					end = b
					break
				}
			}
		} else {
			for i := len(boundaries) - 1; i >= 0; i-- {
				if boundaries[i] <= end {
					end = boundaries[i]
					break
				}
			}
		}
	} else if steps > 0 {
		for _, b := range boundaries {
			if b > count && steps > 0 {
				end = b
				steps--
			}
		}
	} else {
		for i := len(boundaries) - 1; i >= 0; i-- {
			if boundaries[i] < count && steps < 0 {
				end = boundaries[i]
				steps++
			}
		}
	}
	if end < count {
		floor := count - c.Model.BoundaryReturnableRunes(page)
		if end < floor {
			// A protected boundary can fall inside a cluster in an older copy.
			// Keep that complete cluster rather than returning only its suffix.
			for _, b := range boundaries {
				if b >= floor {
					end = b
					break
				}
			}
		}
	}
	if lines && steps > 0 && end == count {
		if next, ok := c.Model.NextPage(page); ok && !next.Reviewed && c.Model.RuneCount(next.ID) > 0 {
			return 0, ErrProofreadingLayoutPending
		}
	}
	if err := c.Model.AdjustBoundary(page, end-count); err != nil {
		return 0, err
	}
	after := c.Model.RuneCount(page)
	actual := 0
	for _, b := range boundaries {
		if b > min(count, after) && b <= max(count, after) {
			actual++
		}
	}
	if after < count {
		actual = -actual
	}
	return actual, nil
}

func absProofreadingSteps(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
