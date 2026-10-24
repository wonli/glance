package textalign

import (
	"context"
	"slices"
)

type queryPart struct {
	block         Block
	text          []rune
	offset, order int
}
type pageQuery struct {
	text  []rune
	parts []queryPart
}

func queryFor(page Page) pageQuery {
	var q pageQuery
	for _, b := range page.Blocks {
		if furniture(b.Label) {
			continue
		}
		text := normalized(b.Text)
		if len(text) == 0 {
			continue
		}
		q.parts = append(q.parts, queryPart{b, text, len(q.text), len(q.parts)})
		q.text = append(q.text, text...)
	}
	return q
}
func unread(page Page) bool {
	for _, b := range page.Blocks {
		if b.Text == "" && (b.Label == "image" || b.Label == "header_image" || b.Label == "seal") {
			return true
		}
	}
	return false
}
func supports(c Candidate, a Anchor) bool {
	for _, e := range c.Evidence {
		if e.BlockID == a.BlockID && e.queryOrder == a.queryOrder && e.start <= a.start && e.end >= a.end && e.QueryRange.Start <= a.QueryRange.Start && e.QueryRange.End >= a.QueryRange.End {
			return true
		}
	}
	return false
}

// anchors require uniqueness in both documents and support from every retained
// near-best placement. A small endpoint variation need not invalidate an interior.
func (idx *index) anchors(ctx context.Context, page int, p Page, r *Result, q pageQuery) ([]Anchor, error) {
	base := r.Selected
	if base == nil && len(r.supported) > 0 {
		base = &r.Candidates[r.supported[0]]
	}
	if base == nil || r.Status == "limited" || !r.pathChecked || base.Joint || base.Reordered || base.Coverage < 0.9 || base.SourceCoverage < 0.9 || unread(p) || len(q.text) < 24 {
		return nil, nil
	}
	grams := map[string][]int{}
	for at := 0; at+4 <= len(q.text); at++ {
		key := string(q.text[at : at+4])
		grams[key] = append(grams[key], at)
	}
	type probe struct{ part, at int }
	var probes []probe
	for i, b := range q.parts {
		for at := 0; at+16 <= len(b.text); at += 4 {
			probes = append(probes, probe{i, at})
		}
		if len(b.text) >= 16 && (len(b.text)-16)%4 != 0 {
			probes = append(probes, probe{i, len(b.text) - 16})
		}
	}
	var found []Anchor
	budget := 64000
	unique := func(needle []rune, hay []rune, positions []int) (int, bool) {
		if len(positions) > 256 {
			return 0, false
		}
		hit := -1
		for _, at := range positions {
			if budget < 16 {
				return 0, false
			}
			budget -= 16
			if at+16 <= len(hay) && slices.Equal(needle, hay[at:at+16]) {
				if hit >= 0 {
					return 0, false
				}
				hit = at
			}
		}
		return hit, hit >= 0
	}
	for lo, hi := 0, len(probes)-1; lo <= hi && len(found) < 2; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pos := lo
		if len(found) == 1 {
			pos = hi
			hi--
		} else {
			lo++
		}
		probe := probes[pos]
		margin := min(8, (len(q.text)-16)/2)
		if q.parts[probe.part].offset+probe.at < margin || q.parts[probe.part].offset+probe.at+16 > len(q.text)-margin {
			continue
		}
		b := q.parts[probe.part]
		needle := b.text[probe.at : probe.at+16]
		key := string(needle[:4])
		_, ok := unique(needle, q.text, grams[key])
		if !ok {
			if budget < 16 {
				break
			}
			continue
		}
		start, ok := unique(needle, idx.source.Text, idx.grams[4][key])
		if !ok {
			if budget < 16 {
				break
			}
			continue
		}
		if !idx.source.indivisible(start, start+16) {
			continue
		}
		a := Anchor{BlockID: b.block.ID, Range: idx.source.OriginalRange(start, start+16), QueryRange: Range{probe.at, probe.at + 16}, start: start, end: start + 16, queryStart: b.offset + probe.at, queryEnd: b.offset + probe.at + 16, page: page, queryOrder: b.order}
		accepted := true
		for _, choice := range r.supported {
			if !supports(r.Candidates[choice], a) {
				accepted = false
				break
			}
		}
		if !accepted {
			continue
		}
		if len(found) > 0 && (a.queryStart < found[0].queryEnd || a.start < found[0].end) {
			continue
		}
		found = append(found, a)
	}
	r.Search.AnchorWork += 64000 - budget
	return found, nil
}
