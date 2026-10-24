package textalign

import "context"

type alignedPair struct {
	query, source int
	equal         bool
}
type jointAlignment struct {
	pairs []alignedPair
	cuts  map[int]Range
}

// joint uses global unit edit distance between fixed unique anchors. Prefix and
// suffix costs enumerate every optimal cut at requested query-page boundaries.
func joint(ctx context.Context, q, r []rune, rows []int, validCut func(int) bool) (jointAlignment, error) {
	if err := ctx.Err(); err != nil {
		return jointAlignment{}, err
	}
	n, m := len(q), len(r)
	cols := m + 1
	// Only boundary rows need distances after the forward pass. Keep a byte
	// per cell for the canonical traceback instead of the full int32 matrix.
	trace := make([]byte, (n+1)*cols)
	checkpoints := make(map[int][]int32, len(rows))
	for _, row := range rows {
		if row >= 0 && row <= n {
			checkpoints[row] = nil
		}
	}
	previous, current := make([]int32, cols), make([]int32, cols)
	for j := 0; j <= m; j++ {
		previous[j] = int32(j)
	}
	save := func(i int, row []int32) {
		if _, ok := checkpoints[i]; ok {
			checkpoints[i] = append([]int32(nil), row...)
		}
	}
	save(0, previous)
	for i := 1; i <= n; i++ {
		if err := ctx.Err(); err != nil {
			return jointAlignment{}, err
		}
		current[0] = int32(i)
		for j := 1; j <= m; j++ {
			cost := int32(0)
			if q[i-1] != r[j-1] {
				cost = 1
			}
			// Preserve the original traceback preference: diagonal, up, left.
			value, direction := previous[j-1]+cost, byte(1)
			if up := previous[j] + 1; up < value {
				value, direction = up, 2
			}
			if left := current[j-1] + 1; left < value {
				value, direction = left, 3
			}
			current[j] = value
			trace[i*cols+j] = direction
		}
		save(i, current)
		previous, current = current, previous
	}
	best := previous[m]
	out := jointAlignment{cuts: map[int]Range{}}
	for j := 0; j <= m; j++ {
		previous[j] = int32(m - j)
	}
	collect := func(i int, suffix []int32) {
		prefix, wanted := checkpoints[i]
		if !wanted {
			return
		}
		cut := Range{m + 1, -1}
		for j := 0; j <= m; j++ {
			if prefix[j]+suffix[j] == best && (validCut == nil || validCut(j)) {
				cut.Start = min(cut.Start, j)
				cut.End = max(cut.End, j)
			}
		}
		out.cuts[i] = cut
	}
	collect(n, previous)
	for i := n - 1; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			return jointAlignment{}, err
		}
		current[m] = int32(n - i)
		for j := m - 1; j >= 0; j-- {
			cost := int32(0)
			if q[i] != r[j] {
				cost = 1
			}
			current[j] = min(previous[j]+1, current[j+1]+1, previous[j+1]+cost)
		}
		collect(i, current)
		previous, current = current, previous
	}
	for i, j := n, m; i > 0 && j > 0; {
		switch trace[i*cols+j] {
		case 1:
			out.pairs = append(out.pairs, alignedPair{i - 1, j - 1, q[i-1] == r[j-1]})
			i--
			j--
		case 2:
			i--
		case 3:
			j--
		}
	}
	return out, nil
}
