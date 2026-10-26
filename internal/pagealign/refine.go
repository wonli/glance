package pagealign

// RefineBoundary searches within a small window around candidateCut to find
// the exact rune offset that minimizes joint edit distance between curr tail
// and next head. It returns (bestCut, bestScore, ambiguous).
func RefineBoundary(currRunes, nextRunes, docRunes []rune, candidateCut, nextOffset, radius int) (int, int, bool) {
	if candidateCut < 0 {
		candidateCut = 0
	}
	if candidateCut > len(docRunes) {
		candidateCut = len(docRunes)
	}
	if radius <= 0 {
		radius = 8
	}

	window := 16
	minCut := max(0, candidateCut-radius)
	maxCut := min(len(docRunes), candidateCut+radius)

	// Tail of current page
	currTailLen := min(window, len(currRunes))
	currTail := currRunes[len(currRunes)-currTailLen:]

	// Head of next page (starting from nextOffset, to skip leading unaligned headers)
	nextHeadLen := min(window, len(nextRunes)-nextOffset)
	var nextHead []rune
	if nextHeadLen > 0 {
		nextHead = nextRunes[nextOffset : nextOffset+nextHeadLen]
	}

	type scoreItem struct {
		cut       int
		score     int
		leftDist  int
		rightDist int
	}

	bestCut := candidateCut
	bestScore := 999999
	bestLeft := 0
	var items []scoreItem

	for k := minCut; k <= maxCut; k++ {
		leftStart := max(0, k-currTailLen)
		docLeft := docRunes[leftStart:k]

		rightEnd := min(len(docRunes), k+nextHeadLen)
		docRight := docRunes[k:rightEnd]

		leftDist := localEditDistance(currTail, docLeft)
		rightDist := 0
		if len(nextHead) > 0 {
			rightDist = localEditDistance(nextHead, docRight)
		}

		score := leftDist + rightDist
		items = append(items, scoreItem{cut: k, score: score, leftDist: leftDist, rightDist: rightDist})

		if score < bestScore {
			bestScore = score
			bestCut = k
			bestLeft = leftDist
		}
	}

	ambiguous := false
	for _, item := range items {
		if item.cut == bestCut {
			continue
		}
		if item.score == bestScore {
			ambiguous = true
			break
		}
		if item.score == bestScore+1 && item.cut == bestCut-1 {
			// Adjacent rival off by one: if bestCut made no genuine improvement
			// on left distance, it merely absorbed a deletion from right side.
			if bestLeft >= item.leftDist {
				ambiguous = true
				break
			}
		}
	}

	return bestCut, bestScore, ambiguous
}

func localEditDistance(a, b []rune) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	row := make([]int, lb+1)
	for j := range row {
		row[j] = j
	}
	for i := 1; i <= la; i++ {
		prev := row[0]
		row[0] = i
		for j := 1; j <= lb; j++ {
			temp := row[j]
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			row[j] = min(row[j]+1, min(row[j-1]+1, prev+cost))
			prev = temp
		}
	}
	return row[lb]
}
