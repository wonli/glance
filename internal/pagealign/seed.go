package pagealign

import "slices"

// Anchor represents a confident alignment between nextRunes and docRunes.
type Anchor struct {
	NextStart int // Start offset in nextRunes
	DocStart  int // Start offset in docRunes
	Length    int // Consecutive matched character length
}

// ImpliedCut returns the candidate boundary in docRunes where nextRunes begins.
func (a Anchor) ImpliedCut() int {
	return a.DocStart
}

// FindAnchors extracts K-gram seeds from nextRunes, searches docRunes,
// extends hits bidirectionally, and returns high-confidence anchors.
func FindAnchors(nextRunes, docRunes []rune, approxDocOffset, seedLen int) []Anchor {
	if len(nextRunes) < seedLen || len(docRunes) < seedLen {
		return nil
	}
	if seedLen <= 0 {
		seedLen = 8
	}

	// Focus seed search around expected position, with generous tolerance.
	searchRadius := max(800, approxDocOffset/2)
	minDoc := max(0, approxDocOffset-searchRadius)
	maxDoc := min(len(docRunes), approxDocOffset+searchRadius+200)
	if minDoc >= maxDoc {
		minDoc = 0
		maxDoc = len(docRunes)
	}

	searchSlice := docRunes[minDoc:maxDoc]

	// Sample seeds from the beginning of nextRunes (up to first 240 runes).
	maxSample := min(len(nextRunes)-seedLen, 240)
	step := 3
	if seedLen <= 6 {
		step = 2
	}

	var anchors []Anchor
	seenCuts := make(map[int]bool)

	for p := 0; p <= maxSample; p += step {
		seed := nextRunes[p : p+seedLen]
		if isTrivialRuneSlice(seed) {
			continue
		}

		// Find occurrences of seed in searchSlice.
		offset := 0
		for {
			idx := indexRuneSlice(searchSlice[offset:], seed)
			if idx < 0 {
				break
			}
			q := minDoc + offset + idx

			// We have a seed match: nextRunes[p:p+seedLen] == docRunes[q:q+seedLen].
			// Extend rightward as far as characters match.
			rExt := seedLen
			for p+rExt < len(nextRunes) && q+rExt < len(docRunes) && nextRunes[p+rExt] == docRunes[q+rExt] {
				rExt++
			}

			// Extend leftward as far as characters match.
			lExt := 0
			for p-lExt-1 >= 0 && q-lExt-1 >= 0 && nextRunes[p-lExt-1] == docRunes[q-lExt-1] {
				lExt++
			}

			startNext := p - lExt
			startDoc := q - lExt
			totalLen := lExt + rExt

			// Only consider anchors with meaningful length (at least seedLen+2).
			if totalLen >= seedLen+2 && !seenCuts[startDoc] {
				seenCuts[startDoc] = true
				anchors = append(anchors, Anchor{
					NextStart: startNext,
					DocStart:  startDoc,
					Length:    totalLen,
				})
			}

			offset += idx + 1
		}
	}

	// Sort anchors by matched length descending (strongest evidence first).
	slices.SortFunc(anchors, func(a, b Anchor) int {
		return b.Length - a.Length
	})

	return anchors
}

// FindTailEnd locates where currRunes ends in docRunes by matching seeds
// from the tail of currRunes.
func FindTailEnd(currRunes, docRunes []rune, seedLen int) (int, bool) {
	if len(currRunes) < seedLen || len(docRunes) < seedLen {
		return 0, false
	}
	if seedLen <= 0 {
		seedLen = 8
	}

	// Sample seeds from the last 120 characters of currRunes.
	tailStart := max(0, len(currRunes)-120)
	tail := currRunes[tailStart:]

	bestEnd := -1
	bestLen := 0

	for p := len(tail) - seedLen; p >= 0; p -= 3 {
		seed := tail[p : p+seedLen]
		if isTrivialRuneSlice(seed) {
			continue
		}
		// Search in docRunes around len(currRunes)
		minDoc := max(0, len(currRunes)-300)
		maxDoc := min(len(docRunes), len(currRunes)+300)
		if minDoc >= maxDoc {
			minDoc = 0
			maxDoc = len(docRunes)
		}

		offset := minDoc
		for {
			idx := indexRuneSlice(docRunes[offset:maxDoc], seed)
			if idx < 0 {
				break
			}
			q := offset + idx
			absP := tailStart + p

			// Extend rightward
			rExt := seedLen
			for absP+rExt < len(currRunes) && q+rExt < len(docRunes) && currRunes[absP+rExt] == docRunes[q+rExt] {
				rExt++
			}
			// Extend leftward
			lExt := 0
			for absP-lExt-1 >= 0 && q-lExt-1 >= 0 && currRunes[absP-lExt-1] == docRunes[q-lExt-1] {
				lExt++
			}

			totalLen := lExt + rExt
			impliedEnd := q + rExt + (len(currRunes) - (absP + rExt))
			if totalLen > bestLen && impliedEnd <= len(docRunes) {
				bestLen = totalLen
				bestEnd = impliedEnd
			}
			offset = q + 1
		}
		if bestLen >= 24 {
			break
		}
	}

	if bestLen >= seedLen+2 {
		return bestEnd, true
	}
	return 0, false
}

func indexRuneSlice(haystack, needle []rune) int {
	if len(needle) == 0 {
		return 0
	}
	if len(haystack) < len(needle) {
		return -1
	}
	first := needle[0]
	maxIdx := len(haystack) - len(needle)
	for i := 0; i <= maxIdx; i++ {
		if haystack[i] == first {
			match := true
			for j := 1; j < len(needle); j++ {
				if haystack[i+j] != needle[j] {
					match = false
					break
				}
			}
			if match {
				return i
			}
		}
	}
	return -1
}

func isTrivialRuneSlice(r []rune) bool {
	if len(r) == 0 {
		return true
	}
	first := r[0]
	for _, c := range r[1:] {
		if c != first {
			return false
		}
	}
	return true
}
