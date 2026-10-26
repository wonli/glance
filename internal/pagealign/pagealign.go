package pagealign

// PageInput contains the textual content and optional metadata for an OCR page.
type PageInput struct {
	Text string
}

// BoundaryResult represents the discovered boundary cut in the manuscript text.
type BoundaryResult struct {
	// CutIndex is the character (rune) offset in Manuscript where next page begins.
	CutIndex int
	// RuneCount is len(manuscriptRunes) - CutIndex (runes moving to next page).
	RuneCount int
	// Confidence score between 0.0 and 1.0.
	Confidence float64
	// MatchedRunes is the number of confirmed matching characters.
	MatchedRunes int
	// Method indicates which alignment strategy confirmed the boundary.
	Method string
}

// Options configures the boundary alignment behavior.
type Options struct {
	SeedLength   int // Length of N-gram seed (default 8)
	WindowRadius int // Refinement search radius in runes (default 12)
}

// DefaultOptions returns the standard options for page alignment.
func DefaultOptions() Options {
	return Options{
		SeedLength:   8,
		WindowRadius: 12,
	}
}

// FindBoundary locates the exact boundary between currentPage and nextPage within manuscript.
// It returns the boundary result and true if a clear, high-confidence boundary is found.
func FindBoundary(current, next PageInput, manuscript string, opt ...Options) (BoundaryResult, bool) {
	opts := DefaultOptions()
	if len(opt) > 0 {
		if opt[0].SeedLength > 0 {
			opts.SeedLength = opt[0].SeedLength
		}
		if opt[0].WindowRadius > 0 {
			opts.WindowRadius = opt[0].WindowRadius
		}
	}

	cleanCurr := CleanOCRText(current.Text)
	cleanNext := CleanOCRText(next.Text)

	currSource := NewTextSource(cleanCurr)
	nextSource := NewTextSource(cleanNext)
	docSource := NewTextSource(manuscript)

	if len(docSource.Characters) == 0 {
		return BoundaryResult{}, false
	}

	// 1. Exact match when there is no next page (last page of manuscript).
	if len(nextSource.Characters) == 0 {
		if len(currSource.Characters) > 0 && len(docSource.Characters) >= len(currSource.Characters) {
			match := true
			for i, r := range currSource.Characters {
				if docSource.Characters[i] != r {
					match = false
					break
				}
			}
			if match {
				cutRune := len(docSource.Original)
				if len(docSource.Characters) > len(currSource.Characters) {
					cutRune = docSource.Starts[len(currSource.Characters)]
				}
				return BoundaryResult{
					CutIndex:     cutRune,
					RuneCount:    len(docSource.Original) - cutRune,
					Confidence:   1.0,
					MatchedRunes: len(currSource.Characters),
					Method:       "exact_prefix",
				}, true
			}
		}
		return BoundaryResult{}, false
	}

	// 2. Primary Seed-and-Extend algorithm using next-page anchors.
	// Next page MUST provide confirmed anchors in manuscript; we never guess
	// boundary solely on current page tail when next page text is present.
	anchors := FindAnchors(nextSource.Characters, docSource.Characters, len(currSource.Characters), opts.SeedLength)
	tailEnd, tailFound := FindTailEnd(currSource.Characters, docSource.Characters, opts.SeedLength)

	var candidateCut int
	var anchorOffset int
	var method string
	var confidence float64
	var matchedRunes int

	if len(anchors) > 0 && anchors[0].Length >= opts.SeedLength+2 {
		top := anchors[0]
		candidateCut = top.ImpliedCut()
		anchorOffset = top.NextStart
		matchedRunes = top.Length
		method = "seed_anchor"
		confidence = 0.95

		// If tail search also found a cut close to top anchor (within 8 runes),
		// we have a consensus between both pages.
		if tailFound && abs(tailEnd-candidateCut) <= 8 {
			method = "seed_consensus"
			confidence = 0.99
			matchedRunes += 16
		}
	} else {
		// If next page anchors were not found (e.g. OCR typos in short windows),
		// fall back to bounded prefix DP which verifies both pages jointly.
		return fallbackBandedAlignment(currSource, nextSource, docSource, manuscript)
	}

	// 3. Local boundary refinement within radius.
	refinedCut, _, ambiguous := RefineBoundary(
		currSource.Characters,
		nextSource.Characters,
		docSource.Characters,
		candidateCut,
		anchorOffset,
		opts.WindowRadius,
	)
	if ambiguous {
		return BoundaryResult{}, false
	}

	// Map refined character position back to original manuscript rune offset.
	cutRuneIndex := len(docSource.Original)
	if refinedCut < len(docSource.Characters) {
		cutRuneIndex = docSource.Starts[refinedCut]
	}

	runeCount := len(docSource.Original) - cutRuneIndex
	return BoundaryResult{
		CutIndex:     cutRuneIndex,
		RuneCount:    runeCount,
		Confidence:   confidence,
		MatchedRunes: matchedRunes,
		Method:       method,
	}, true
}

func fallbackBandedAlignment(currSource, nextSource, docSource TextSource, manuscript string) (BoundaryResult, bool) {
	a := currSource.Characters
	b := docSource
	n := nextSource.Characters
	if len(a) < 16 || len(n) < 16 {
		return BoundaryResult{}, false
	}
	budget := min(256, max(32, len(a)/4))
	distances := proofPrefixDistances(a, b.Characters, budget+1)
	if distances == nil {
		return BoundaryResult{}, false
	}

	window := min(64, len(n))
	nextBudget := max(2, window/8)
	bestEnd, bestScore := -1, budget+nextBudget+3
	for end := max(1, len(a)-budget-1); end < len(distances) && end < len(b.Characters); end++ {
		if distances[end] > budget+1 {
			continue
		}
		nextDists := proofPrefixDistances(n[:window], b.Characters[end:], nextBudget+1)
		if nextDists == nil {
			continue
		}
		nextScore := nextBudget + 2
		for j := max(1, window-nextBudget-1); j < len(nextDists); j++ {
			nextScore = min(nextScore, nextDists[j])
		}
		score := distances[end] + nextScore
		if score < bestScore {
			bestEnd = end
			bestScore = score
		}
	}
	if bestEnd < 0 {
		return BoundaryResult{}, false
	}
	cutRuneIndex := b.Starts[bestEnd]
	return BoundaryResult{
		CutIndex:     cutRuneIndex,
		RuneCount:    len(b.Original) - cutRuneIndex,
		Confidence:   0.80,
		MatchedRunes: len(a) - distances[bestEnd],
		Method:       "banded_dp_fallback",
	}, true
}

func proofPrefixDistances(query, target []rune, budget int) []int {
	if len(query) == 0 || budget < 0 || len(target) < len(query)-budget {
		return nil
	}
	m := min(len(target), len(query)+budget)
	inf := budget + 1
	prev, curr := make([]int, m+1), make([]int, m+1)
	for j := range prev {
		prev[j], curr[j] = inf, inf
	}
	for j := 0; j <= min(m, budget); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(query); i++ {
		lo, hi := max(1, i-budget), min(m, i+budget)
		if lo > hi {
			return nil
		}
		curr[0] = min(i, inf)
		if lo > 1 {
			curr[lo-1] = inf
		}
		if hi < m {
			curr[hi+1] = inf
		}
		rowBest := inf
		for j := lo; j <= hi; j++ {
			cost := 0
			if query[i-1] != target[j-1] {
				cost = 1
			}
			curr[j] = min(inf, prev[j-1]+cost, prev[j]+1, curr[j-1]+1)
			rowBest = min(rowBest, curr[j])
		}
		if rowBest > budget {
			return nil
		}
		prev, curr = curr, prev
	}
	return prev
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
