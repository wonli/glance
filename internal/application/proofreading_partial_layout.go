package application

import (
	"context"
	"fmt"
	"glance/internal/proofreading"
	"strings"
)

// LayoutProofreadingOwnedPages refreshes view geometry after an explicit
// boundary adjustment. It does not repartition or reshape the rest of the book.
func LayoutProofreadingOwnedPages(ctx context.Context, s proofreading.Snapshot, geometry []ProofreadingPageGeometry, style ProofreadingTextStyle, previous ProofreadingLayoutSnapshot, pages []proofreading.PageID, shape ProofreadingLineShaper) (ProofreadingLayoutSnapshot, error) {
	result := ProofreadingLayoutSnapshot{Revision: s.Revision, StyleRevision: style.Revision, Pages: make(map[proofreading.PageID]ProofreadingPageLayout, len(previous.Pages))}
	for id, p := range previous.Pages {
		result.Pages[id] = p
	}
	indexes := make(map[proofreading.PageID]int, len(s.Pages))
	for i, p := range s.Pages {
		indexes[p.ID] = i
	}
	for _, id := range pages {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		i, ok := indexes[id]
		if !ok || i >= len(geometry) {
			return result, fmt.Errorf("missing proofreading page geometry")
		}
		last := i
		if !s.Pages[i].Reviewed && i+1 < len(s.Pages) && !s.Pages[i+1].Reviewed {
			last = i + 1
		}
		var text strings.Builder
		following := 4096
		for _, r := range s.Runs {
			j := indexes[r.Page]
			if j < i {
				continue
			}
			if j > last {
				break
			}
			if j == i {
				text.WriteString(r.Text)
				continue
			}
			end := len(r.Text)
			for at := range r.Text {
				if following == 0 {
					end = at
					break
				}
				following--
			}
			text.WriteString(r.Text[:end])
			if following == 0 {
				break
			}
		}
		value := text.String()
		lines, err := shape(ctx, value, geometry[i].Width, style)
		if err != nil {
			return result, err
		}
		height := 0.0
		if len(lines) > 0 {
			height = lines[len(lines)-1].Bottom
		}
		result.Pages[id] = ProofreadingPageLayout{Text: value, Lines: lines, Height: height}
	}
	return result, nil
}
