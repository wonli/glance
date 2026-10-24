package textalign

import (
	"context"
	"sort"
)

type jointSpan struct {
	left, right Anchor
	alignment   jointAlignment
	rows        map[int]int
}

func refine(ctx context.Context, idx *index, pages []Page, results []Result, opt Options) error {
	queries := make([]pageQuery, len(pages))
	var anchors []Anchor
	for i, p := range pages {
		results[i].hasUnread = unread(p)
		if err := ctx.Err(); err != nil {
			return err
		}
		size := 0
		for _, b := range p.Blocks {
			size += len(b.Text)
		}
		if size > 128<<10 || len(p.Blocks) > 512 {
			continue
		}
		queries[i] = queryFor(p)
		if len(queries[i].text) > 12000 {
			queries[i] = pageQuery{}
			continue
		}
		a, err := idx.anchors(ctx, i, p, &results[i], queries[i])
		if err != nil {
			return err
		}
		results[i].Anchors = a
		anchors = append(anchors, a...)
	}
	// Repeated OCR pages can have unique-within-page phrases at the same B
	// location. Reject both conflicting page anchor sets rather than choosing one.
	conflicts := map[int]bool{}
	for i := 0; i+1 < len(anchors); i++ {
		a, b := anchors[i], anchors[i+1]
		if a.page != b.page && a.end > b.start {
			conflicts[a.page] = true
			conflicts[b.page] = true
		}
	}
	clean := anchors[:0]
	for _, a := range anchors {
		if conflicts[a.page] {
			results[a.page].Anchors = nil
		} else {
			clean = append(clean, a)
		}
	}
	anchors = clean
	budget := opt.MaxJointCells
	if budget <= 0 {
		budget = 2_000_000
	}
	budget = min(budget, 8_000_000)
	blocked := map[int]string{}
	contextState := func(page int, state string) {
		if state == "bounded" || results[page].Context != "bounded" {
			results[page].Context = state
		}
	}
	var spans []jointSpan
	changed := false
	for ai := 0; ai+1 < len(anchors); ai++ {
		left, right := anchors[ai], anchors[ai+1]
		if left.page == right.page {
			continue
		}
		lo, hi := left.page, right.page
		if hi-lo > 7 || left.end > right.start {
			continue
		}
		consecutive, ordered := true, true
		for p := lo; p <= hi; p++ {
			if p > lo && pages[p].Number != pages[p-1].Number+1 {
				consecutive = false
			}
			if len(queries[p].text) == 0 || (results[p].Selected != nil && results[p].Selected.Reordered) {
				ordered = false
			}
		}
		if !consecutive {
			continue
		}
		if !ordered {
			for p := lo; p <= hi; p++ {
				contextState(p, "unsupported")
				if p < hi {
					blocked[p] = "reading order or OCR text is unavailable"
				}
			}
			continue
		}
		var query []rune
		offsets := map[int]int{}
		rows := map[int]int{}
		for p := lo; p <= hi; p++ {
			from, to := 0, len(queries[p].text)
			if p == lo {
				from = left.queryEnd
			}
			if p == hi {
				to = right.queryStart
			}
			offsets[p] = len(query) - from
			query = append(query, queries[p].text[from:to]...)
			if p < hi {
				rows[p] = len(query)
			}
		}
		ref := idx.source.Text[left.end:right.start]
		cells := 2 * (len(query) + 1) * (len(ref) + 1)
		if cells > budget {
			for p := lo; p <= hi; p++ {
				contextState(p, "limited")
				if p < hi {
					blocked[p] = "joint alignment budget reached"
				}
			}
			continue
		}
		budget -= cells
		results[lo].Search.JointCells += cells
		wanted := make([]int, 0, len(rows))
		for _, row := range rows {
			wanted = append(wanted, row)
		}
		alignment, err := joint(ctx, query, ref, wanted, func(j int) bool { return idx.source.indivisible(left.end+j, left.end+j) })
		if err != nil {
			return err
		}
		spans = append(spans, jointSpan{left, right, alignment, rows})
		for p := lo; p <= hi; p++ {
			contextState(p, "bounded")
		}
		// Only whole intermediate pages get new candidates. Endpoint tails and heads
		// establish context without pretending to describe their entire pages.
		for p := lo + 1; p < hi; p++ {
			r := &results[p]
			kept := r.Candidates[:0]
			for _, c := range r.Candidates {
				if c.start >= left.end && c.end <= right.start {
					kept = append(kept, c)
				}
			}
			if len(kept) != len(r.Candidates) {
				changed = true
			}
			r.Candidates = kept
			c, ok := jointCandidate(idx.source, queries[p], alignment, offsets[p], left.end)
			if ok {
				exists := false
				for _, old := range r.Candidates {
					if old.start == c.start && old.end == c.end && old.Score >= c.Score {
						exists = true
						break
					}
				}
				if !exists {
					r.Candidates = append(r.Candidates, c)
					changed = true
				}
			}
		}
	}
	if len(spans) > 0 {
		for i := range results {
			if len(results[i].Anchors) == 0 {
				continue
			}
			r := &results[i]
			kept := r.Candidates[:0]
			for _, c := range r.Candidates {
				valid := true
				for _, a := range r.Anchors {
					if !supports(c, a) {
						valid = false
						break
					}
				}
				if valid {
					kept = append(kept, c)
				}
			}
			if len(kept) != len(r.Candidates) {
				changed = true
			}
			r.Candidates = kept
		}
	}
	if changed {
		if err := resolvePaths(ctx, pages, results, opt); err != nil {
			return err
		}
	}
	inferBoundaries(queries, results)
	for _, span := range spans {
		left, right := span.left, span.right
		if !anchorPresent(results[left.page], left) || !anchorPresent(results[right.page], right) {
			continue
		}
		for p, row := range span.rows {
			a, b := &results[p], &results[p+1]
			leftKnown := placed(*a) || (p == left.page && anchorPresent(*a, left))
			rightKnown := placed(*b) || (p+1 == right.page && anchorPresent(*b, right))
			if a.BoundaryAfter == nil || !leftKnown || !rightKnown {
				continue
			}
			cut, ok := span.alignment.cuts[row]
			if !ok || cut.End < cut.Start {
				a.BoundaryAfter.Reason = "no optimal cut respects source character boundaries"
				continue
			}
			minimum := left.Range.End
			if cut.Start > 0 {
				minimum = idx.source.ends[left.end+cut.Start-1]
			}
			maximum := right.Range.Start
			if cut.End < len(idx.source.Text[left.end:right.start]) {
				maximum = idx.source.starts[left.end+cut.End]
			}
			if minimum > maximum {
				continue
			}
			a.BoundaryAfter = &Boundary{NextPage: b.Page, Status: "range", Min: minimum, Max: maximum, Method: "anchored-edit-distance", Reason: "envelope of text-model optimal cuts; page ownership remains unverified"}
			if minimum == maximum {
				a.BoundaryAfter.Status = "point"
			}
		}
	}
	for page, reason := range blocked {
		results[page].BoundaryAfter = &Boundary{NextPage: results[page+1].Page, Status: "unresolved", Method: "anchored-edit-distance", Reason: reason}
	}

	return nil
}

func jointCandidate(source *Source, q pageQuery, a jointAlignment, offset, referenceStart int) (Candidate, bool) {
	c := Candidate{Joint: true, start: len(source.Text)}
	matched, edits := 0, 0
	for _, part := range q.parts {
		e := Evidence{BlockID: part.block.ID, Bounds: part.block.Bounds, QueryCharacters: len(part.text), start: len(source.Text), queryOrder: part.order, QueryRange: Range{len(part.text), 0}}
		for _, pair := range a.pairs {
			at := pair.query - offset - part.offset
			if at < 0 || at >= len(part.text) {
				continue
			}
			if !pair.equal {
				e.Edits++
				continue
			}
			e.Matches++
			e.start = min(e.start, referenceStart+pair.source)
			e.end = max(e.end, referenceStart+pair.source+1)
			e.QueryRange.Start = min(e.QueryRange.Start, at)
			e.QueryRange.End = max(e.QueryRange.End, at+1)
		}
		if e.Matches == 0 {
			continue
		}
		if !source.indivisible(e.start, e.end) {
			return Candidate{}, false
		}
		e.Range = source.OriginalRange(e.start, e.end)
		e.score = 3*e.Matches - 2*e.Edits
		c.Evidence = append(c.Evidence, e)
		c.start = min(c.start, e.start)
		c.end = max(c.end, e.end)
		matched += e.Matches
		edits += e.Edits
	}
	if matched < 4 || len(q.text) == 0 || float64(matched)/float64(len(q.text)) < 0.25 {
		return Candidate{}, false
	}
	c.Score = float64(4*matched - len(q.text) - 2*edits)
	if c.Score <= 0 {
		return Candidate{}, false
	}
	c.Coverage = float64(matched) / float64(len(q.text))
	c.Range = source.OriginalRange(c.start, c.end)
	consumed := 0
	for _, e := range c.Evidence {
		consumed += e.end - e.start
	}
	c.SourceCoverage = float64(consumed) / float64(c.end-c.start)
	c.Score -= float64(sourceGapPenalty * (c.end - c.start - consumed))
	if c.Score <= 0 {
		return Candidate{}, false
	}
	sort.Slice(c.Evidence, func(i, j int) bool { return c.Evidence[i].queryOrder < c.Evidence[j].queryOrder })
	return c, true
}

func anchorPresent(r Result, a Anchor) bool {
	if r.Status == "limited" || r.Context == "limited" || r.Context == "unsupported" {
		return false
	}
	for _, c := range r.Candidates {
		if supports(c, a) {
			return true
		}
	}
	return false
}
