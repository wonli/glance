package textalign

import "context"

type localMatch struct {
	start, end, matches, edits, score int
	queryStart, queryEnd              int
}

// local implements bounded Smith-Waterman alignment with linear gap penalties.
// It aligns one text block, not the whole book, and permits noisy block ends.
func local(ctx context.Context, q, r []rune) (localMatch, error) {
	cols := len(r) + 1
	trace := make([]byte, (len(q)+1)*cols)
	previous, current := make([]int, cols), make([]int, cols)
	best, bi, bj := 0, 0, 0
	for i := 1; i <= len(q); i++ {
		if i%32 == 0 {
			if err := ctx.Err(); err != nil {
				return localMatch{}, err
			}
		}
		current[0] = 0
		for j := 1; j <= len(r); j++ {
			delta := -2
			if q[i-1] == r[j-1] {
				delta = 3
			}
			value, direction := previous[j-1]+delta, byte(1)
			if previous[j]-2 > value {
				value = previous[j] - 2
				direction = 2
			}
			if current[j-1]-2 > value {
				value = current[j-1] - 2
				direction = 3
			}
			if value <= 0 {
				value = 0
				direction = 0
			}
			current[j] = value
			trace[i*cols+j] = direction
			if value > best {
				best, bi, bj = value, i, j
			}
		}
		previous, current = current, previous
	}
	out := localMatch{end: bj, score: best, queryEnd: bi}
	i, j := bi, bj
	for i > 0 && j > 0 {
		d := trace[i*cols+j]
		if d == 0 {
			break
		}
		switch d {
		case 1:
			if q[i-1] == r[j-1] {
				out.matches++
			} else {
				out.edits++
			}
			i--
			j--
		case 2:
			out.edits++
			i--
		case 3:
			out.edits++
			j--
		}
	}
	out.start = j
	out.queryStart = i
	return out, nil
}

// exact uses KMP so long unchanged blocks avoid quadratic dynamic programming.
func exact(q, r []rune) (localMatch, bool) {
	if len(q) == 0 {
		return localMatch{}, false
	}
	prefix := make([]int, len(q))
	for i, j := 1, 0; i < len(q); i++ {
		for j > 0 && q[i] != q[j] {
			j = prefix[j-1]
		}
		if q[i] == q[j] {
			j++
		}
		prefix[i] = j
	}
	for i, j := 0, 0; i < len(r); i++ {
		for j > 0 && r[i] != q[j] {
			j = prefix[j-1]
		}
		if r[i] == q[j] {
			j++
		}
		if j == len(q) {
			return localMatch{start: i + 1 - len(q), end: i + 1, matches: len(q), score: len(q) * 3, queryEnd: len(q)}, true
		}
	}
	return localMatch{}, false
}
