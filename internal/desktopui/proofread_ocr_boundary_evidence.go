package desktopui

import (
	"glance/internal/ocr"
	"glance/internal/pagealign"
)

// Align the entire OCR page to a bounded manuscript prefix. Errors at either
// end, substitutions, insertions and deletions share an edit budget proportional
// to text length. Expand next-page context when a short window cannot identify
// a unique best joint alignment. Nothing here changes manuscript characters.
func proofreadingOCRAlignedPageTail(current, next ocr.Layout, text string) (proofreadingOCROverflowTail, bool) {
	if current.Width <= 0 || current.Height <= 0 || next.Width <= 0 || next.Height <= 0 {
		return proofreadingOCROverflowTail{}, false
	}
	currText := proofreadingOCRPageText(current).text
	nextText := proofreadingOCRPageText(next).text
	if res, ok := pagealign.FindBoundary(pagealign.PageInput{Text: currText}, pagealign.PageInput{Text: nextText}, text); ok {
		return proofreadingOCROverflowTail{
			Start:        res.CutIndex,
			RuneCount:    res.RuneCount,
			MatchedRunes: res.MatchedRunes,
		}, true
	}
	a := newProofreadingOCRDiffSource(currText).characters
	b := newProofreadingOCRDiffSource(text)
	n := newProofreadingOCRDiffSource(nextText).characters
	if len(a) < 16 || len(a) > 4096 || len(n) < 16 {
		return proofreadingOCROverflowTail{}, false
	}
	budget := min(256, max(32, len(a)/4))
	distances := proofreadingOCRPrefixEndDistances(a, b.characters, budget+1)
	if distances == nil {
		return proofreadingOCROverflowTail{}, false
	}
	type candidateScore struct {
		end       int
		score     int
		dist      int
		nextScore int
	}
	for _, requested := range []int{16, 32, 64, 128, 256} {
		window := min(requested, len(n))
		nextBudget := max(2, window/8)
		bestEnd, bestScore, bestMatched := -1, budget+nextBudget+3, 0
		bestCurrent, bestNext := 0, 0
		var candidates []candidateScore
		for end := max(1, len(a)-budget-1); end < len(distances) && end < len(b.characters); end++ {
			if distances[end] > budget+1 {
				continue
			}
			nextDistances := proofreadingOCRPrefixEndDistances(n[:window], b.characters[end:], nextBudget+1)
			if nextDistances == nil {
				continue
			}
			nextScore := nextBudget + 2
			for j := max(1, window-nextBudget-1); j < len(nextDistances); j++ {
				nextScore = min(nextScore, nextDistances[j])
			}
			if nextScore > nextBudget+1 {
				continue
			}
			score := distances[end] + nextScore
			candidates = append(candidates, candidateScore{
				end:       end,
				score:     score,
				dist:      distances[end],
				nextScore: nextScore,
			})
			if score < bestScore {
				bestEnd, bestScore, bestMatched = end, score, min(len(a)-distances[end], window-nextScore)
				bestCurrent, bestNext = distances[end], nextScore
			}
		}
		if bestEnd < 0 || bestCurrent > budget || bestNext > nextBudget {
			if window == len(n) {
				break
			}
			continue
		}
		ambiguous := false
		for _, c := range candidates {
			if c.end == bestEnd {
				continue
			}
			if c.score == bestScore {
				ambiguous = true
				break
			}
			if c.score == bestScore+1 && c.end == bestEnd-1 {
				// Adjacent rival off by one character: verify that bestEnd made genuine progress
				// on the side that changed. If the rival is at bestEnd-1 (character assigned to next page),
				// bestEnd must have improved page 1 distance (not merely absorbed a deletion from next page).
				if bestCurrent >= c.dist {
					ambiguous = true
					break
				}
			}
		}
		if !ambiguous {
			start := b.starts[bestEnd]
			return proofreadingOCROverflowTail{Start: start, RuneCount: len([]rune(text)) - start, MatchedRunes: bestMatched}, true
		}
		if window == len(n) {
			break
		}
	}
	return proofreadingOCROverflowTail{}, false
}

// Banded prefix edit distances: align all of query from target's beginning,
// leaving the later manuscript free. Work is O(len(query)*budget); a 50k tail
// never becomes a matrix axis. Only endpoints inside the final band are valid.
func proofreadingOCRPrefixEndDistances(query, target []rune, budget int) []int {
	if len(query) == 0 || budget < 0 || len(target) < len(query)-budget {
		return nil
	}
	m := min(len(target), len(query)+budget)
	inf := budget + 1
	previous, current := make([]int, m+1), make([]int, m+1)
	for j := range previous {
		previous[j], current[j] = inf, inf
	}
	for j := 0; j <= min(m, budget); j++ {
		previous[j] = j
	}
	for i := 1; i <= len(query); i++ {
		lo, hi := max(1, i-budget), min(m, i+budget)
		if lo > hi {
			return nil
		}
		current[0] = min(i, inf)
		if lo > 1 {
			current[lo-1] = inf
		}
		if hi < m {
			current[hi+1] = inf
		}
		rowBest := inf
		for j := lo; j <= hi; j++ {
			cost := 0
			if query[i-1] != target[j-1] {
				cost = 1
			}
			current[j] = min(inf, previous[j-1]+cost, previous[j]+1, current[j-1]+1)
			rowBest = min(rowBest, current[j])
		}
		if rowBest > budget {
			return nil
		}
		previous, current = current, previous
	}
	return previous
}
