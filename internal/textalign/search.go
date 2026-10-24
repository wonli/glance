package textalign

import (
	"context"
	"math"
	"sort"
)

type index struct {
	source *Source
	grams  map[int]map[string][]int
}

func newIndex(s *Source) *index {
	idx := &index{source: s, grams: map[int]map[string][]int{}}
	for _, k := range []int{2, 4} {
		postings := map[string][]int{}
		for i := 0; i+k <= len(s.Text); i++ {
			key := string(s.Text[i : i+k])
			if len(postings[key]) < 257 {
				postings[key] = append(postings[key], i)
			}
		}
		idx.grams[k] = postings
	}
	return idx
}

type window struct {
	start, end int
	votes      float64
}
type prepared struct {
	block  Block
	text   []rune
	prefix int
	titles []Range
}

func (idx *index) candidates(ctx context.Context, page Page, opt Options, stats *SearchStats) ([]Candidate, bool, error) {
	var blocks []prepared
	if len(page.Blocks) > 512 {
		return nil, true, nil
	}
	total := 0
	bytes := 0
	for _, b := range page.Blocks {
		bytes += len(b.Text)
		if bytes > 128<<10 {
			return nil, true, nil
		}
	}
	for _, b := range page.Blocks {
		if furniture(b.Label) {
			continue
		}
		q := normalized(b.Text)
		if len(q) > 0 {
			p := prepared{block: b, text: q, prefix: total}
			blocks = append(blocks, p)
			total += len(q)
		}
	}
	if total == 0 {
		return nil, false, nil
	}
	// Absolute safety caps are independent of caller-supplied tuning.
	if total > 12000 || len(blocks) > 512 {
		return nil, true, nil
	}
	truncated := false
	titleBudget := 64000
	type titleLookup struct {
		ranges  []Range
		limited bool
	}
	titleCache := map[string]titleLookup{}
	for i := range blocks {
		b := &blocks[i]
		if b.block.Label != "doc_title" && b.block.Label != "paragraph_title" {
			continue
		}
		key := string(b.text)
		lookup, ok := titleCache[key]
		if !ok {
			var err error
			lookup.ranges, lookup.limited, err = idx.titleRanges(ctx, b.text, &titleBudget)
			if err != nil {
				return nil, false, err
			}
			titleCache[key] = lookup
		}
		b.titles = lookup.ranges
		truncated = truncated || lookup.limited
	}
	stats.TitleWork = 64000 - titleBudget
	var titles []Range
	for _, b := range blocks {
		titles = append(titles, b.titles...)
	}
	pins, err := idx.internalPins(ctx, blocks, stats)
	if err != nil {
		return nil, false, err
	}
	pad := min(32, max(8, total/3))
	possibleWindow := func(start, end int) bool {
		if len(pins) > 0 && (start > pins[0].Start || end < pins[len(pins)-1].End) {
			return false
		}
		if len(titles) == 0 {
			return true
		}
		for _, title := range titles {
			if start <= title.Start && end >= title.End {
				return true
			}
		}
		return false
	}
	seeds := map[int]float64{}
	for _, b := range blocks {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		k := 4
		if len(b.text) < k {
			k = 2
		}
		if len(b.text) < k {
			continue
		}
		for at := 0; at+k <= len(b.text); at++ {
			hits := idx.grams[k][string(b.text[at:at+k])]
			if len(hits) > 256 {
				truncated = true
				continue
			}
			weight := 1 / math.Max(1, float64(len(hits)))
			for _, hit := range hits {
				for _, seed := range []int{hit - at - b.prefix, hit - at} {
					seeds[max(0, seed)] += weight
				}
			}
		}
	}
	var ranked []window
	for start, votes := range seeds {
		end := min(len(idx.source.Text), start+total+pad)
		if !possibleWindow(start, end) && !possibleWindow(max(0, start-pad), end) {
			continue
		}
		ranked = append(ranked, window{start: start, votes: votes})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].votes == ranked[j].votes {
			return ranked[i].start < ranked[j].start
		}
		return ranked[i].votes > ranked[j].votes
	})
	if len(ranked) > 64 {
		ranked = ranked[:64]
		truncated = true
	}
	unique := map[[2]int]bool{}
	var windows []window
	for _, seed := range ranked {
		// Both an exact start and a padded start are tried so a preceding repeated
		// title cannot hide the second occurrence in the same local-alignment window.
		for _, start := range []int{seed.start, max(0, seed.start-pad)} {
			end := min(len(idx.source.Text), seed.start+total+pad)
			key := [2]int{start, end}
			if start >= end || unique[key] || !possibleWindow(start, end) {
				continue
			}
			unique[key] = true
			windows = append(windows, window{start, end, seed.votes})
		}
	}
	budget := opt.MaxCells
	if budget <= 0 {
		budget = 4_000_000
	}
	budget = min(budget, 16_000_000)
	matches := newMatcher(idx.source, windows, budget, stats)
	stats.Windows = len(windows)
	defer func() { stats.WorkUnits = matches.used }()
	found := map[[2]int]Candidate{}
	for _, w := range windows {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		c := Candidate{start: len(idx.source.Text)}
		requireTitle := len(titles) > 0
		for bi, b := range blocks {
			// Single-character OCR fragments contribute no independent locator evidence.
			if len(b.text) < 2 {
				continue
			}
			// Match only unused source intervals so repeated OCR blocks can use
			// separate identical occurrences without counting any B text twice.
			occupied := append([]Evidence(nil), c.Evidence...)
			sort.Slice(occupied, func(i, j int) bool { return occupied[i].start < occupied[j].start })
			segments := make([]Range, 0, len(occupied)+1)
			from := w.start
			for _, e := range occupied {
				if from < e.start {
					segments = append(segments, Range{from, e.start})
				}
				from = e.end
			}
			if from < w.end {
				segments = append(segments, Range{from, w.end})
			}
			if len(b.titles) > 0 {
				var anchored []Range
				for _, seg := range segments {
					for _, title := range b.titles {
						r := Range{max(seg.Start, title.Start), min(seg.End, title.End)}
						if r.End > r.Start {
							anchored = append(anchored, r)
						}
					}
				}
				segments = anchored
			}
			m := localMatch{}
			for _, segment := range segments {
				trial, ok, err := matches.match(ctx, bi, b.text, segment)
				if err != nil {
					return nil, false, err
				}
				if !ok {
					truncated = true
					break
				}
				if trial.score > m.score {
					m = trial
				}
			}
			minimum := minimumMatches(len(b.text))
			if len(b.titles) > 0 {
				minimum = len(b.text)
			}
			if m.matches < minimum {
				continue
			}
			start, end := m.start, m.end
			if !idx.source.indivisible(start, end) {
				continue
			}
			c.Evidence = append(c.Evidence, Evidence{BlockID: b.block.ID, Range: idx.source.OriginalRange(start, end), Bounds: b.block.Bounds, QueryCharacters: len(b.text), Matches: m.matches, Edits: m.edits, QueryRange: Range{m.queryStart, m.queryEnd}, start: start, end: end, score: m.score, queryOrder: bi, title: len(b.titles) > 0})
		}
		c.Evidence = coherent(c.Evidence, requireTitle)
		matched := 0
		c.Score = 0
		c.start = len(idx.source.Text)
		c.end = 0
		c.Reordered = false
		sort.Slice(c.Evidence, func(i, j int) bool { return c.Evidence[i].queryOrder < c.Evidence[j].queryOrder })
		for i, e := range c.Evidence {
			matched += e.Matches
			c.Score += float64(e.score)
			c.start = min(c.start, e.start)
			c.end = max(c.end, e.end)
			if i > 0 && e.start < c.Evidence[i-1].start {
				c.Reordered = true
			}
		}
		if matched < 4 || float64(matched)/float64(total) < 0.25 {
			continue
		}
		c.Coverage = float64(matched) / float64(total)
		// Unrecognized query characters reduce the value of a partial accidental hit.
		c.Score -= float64(total - matched)
		consumed := 0
		for _, e := range c.Evidence {
			consumed += e.end - e.start
		}
		c.SourceCoverage = float64(consumed) / float64(c.end-c.start)
		// Do not improve a page by scavenging repeated names in unrelated B
		// paragraphs. Unexplained B characters between blocks remain evidence
		// against that candidate, not silently assigned text.
		c.Score -= sourceGapPenalty * float64(c.end-c.start-consumed)
		if c.Score <= 0 {
			continue
		}
		c.Range = idx.source.OriginalRange(c.start, c.end)
		key := [2]int{c.start, c.end}
		old, ok := found[key]
		if !ok || c.Score > old.Score {
			found[key] = c
		}
	}
	var candidates []Candidate
	for _, c := range found {
		candidates = append(candidates, c)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			if candidates[i].start == candidates[j].start {
				return candidates[i].end < candidates[j].end
			}
			return candidates[i].start < candidates[j].start
		}
		return candidates[i].Score > candidates[j].Score
	})
	limit := opt.MaxCandidates
	if limit <= 0 {
		limit = 12
	}
	limit = min(limit, 64)
	if len(candidates) > limit {
		candidates = candidates[:limit]
		truncated = true
	}
	return candidates, truncated, nil
}

func furniture(label string) bool {
	switch label {
	case "number", "header", "footer", "header_image", "footer_image":
		return true
	}
	return false
}

func minimumMatches(n int) int {
	if n <= 12 {
		return max(2, (n*85+99)/100)
	}
	return max(2, (n+1)/2)
}

// A recognized document title can anchor to a real Markdown heading when one
// exists. A mention of the same book inside CIP/body text is not that heading.
func (idx *index) titleRanges(ctx context.Context, q []rune, budget *int) ([]Range, bool, error) {
	if len(q) < 2 {
		return nil, false, nil
	}
	var out []Range
	for _, h := range idx.source.headings {
		from := h.Start
		for from < h.End {
			if err := ctx.Err(); err != nil {
				return nil, false, err
			}
			cost := len(q) + h.End - from
			if cost > *budget || len(out) >= 256 {
				return out, true, nil
			}
			*budget -= cost
			m, ok := exact(q, idx.source.Text[from:h.End])
			if !ok {
				break
			}
			out = append(out, Range{from + m.start, from + m.end})
			from += m.start + 1
		}
	}
	return out, false, nil
}
