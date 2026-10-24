package textalign

import "sort"

const sourceGapPenalty = 4

// coherent maximizes sum(block score + matched characters) minus a penalty
// for unexplained source gaps. Two maximum-subarray states retain groups with
// and without a known Markdown heading anchor; OCR order may still differ.
func coherent(evidence []Evidence, requireTitle bool) []Evidence {
	if len(evidence) == 0 {
		return nil
	}
	sort.Slice(evidence, func(i, j int) bool { return evidence[i].start < evidence[j].start })
	best, current, pinned := 0, 0, -1<<30
	from, pinFrom, bestFrom, bestEnd := 0, 0, 0, 0
	for i, e := range evidence {
		value := e.score + e.Matches
		gap := 0
		if i > 0 {
			gap = e.start - evidence[i-1].end
		}
		if e.title {
			pinned = value
			pinFrom = i
			if i > 0 && current-sourceGapPenalty*gap > 0 {
				pinned += current - sourceGapPenalty*gap
				pinFrom = from
			}
		} else if pinned > -1<<29 {
			pinned += value - sourceGapPenalty*gap
		}
		if i == 0 || current-sourceGapPenalty*gap < 0 {
			current = value
			from = i
		} else {
			current += value - sourceGapPenalty*gap
		}
		score, start := current, from
		if requireTitle {
			score, start = pinned, pinFrom
		}
		if score > best {
			best = score
			bestFrom = start
			bestEnd = i + 1
		}
	}
	return evidence[bestFrom:bestEnd]
}
