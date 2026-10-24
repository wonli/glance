package textalign

import (
	"context"
	"slices"
)

// internalPins is a bounded, heuristic retrieval scope for long continuation
// pages without headings. Two unique, widely separated fragments must agree in
// query and source order. They constrain location, never prove page boundaries.
func (idx *index) internalPins(ctx context.Context, blocks []prepared, stats *SearchStats) ([]Range, error) {
	var q []rune
	for _, b := range blocks {
		q = append(q, b.text...)
	}
	if len(q) < 64 {
		return nil, nil
	}
	positions := map[string][]int{}
	for i := 0; i+4 <= len(q); i++ {
		k := string(q[i : i+4])
		positions[k] = append(positions[k], i)
	}
	budget := 64000
	defer func() { stats.AnchorWork += 64000 - budget }()
	unique := func(needle, hay []rune, at []int) (int, bool) {
		if len(at) > 256 {
			return 0, false
		}
		found := -1
		for _, p := range at {
			if budget < 16 {
				return 0, false
			}
			budget -= 16
			if p+16 <= len(hay) && slices.Equal(needle, hay[p:p+16]) {
				if found >= 0 {
					return 0, false
				}
				found = p
			}
		}
		return found, found >= 0
	}
	for lo, hi := 8, len(q)-24; lo+16 <= hi; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		needle := q[lo : lo+16]
		key := string(needle[:4])
		_, valid := unique(needle, q, positions[key])
		left, ok := unique(needle, idx.source.Text, idx.grams[4][key])
		if !valid || !ok {
			lo += 4
			if budget < 16 {
				break
			}
			continue
		}
		for at := hi; at-lo >= len(q)/2; at -= 4 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			needle = q[at : at+16]
			key = string(needle[:4])
			_, valid = unique(needle, q, positions[key])
			right, ok := unique(needle, idx.source.Text, idx.grams[4][key])
			if valid && ok && right >= left+16 && abs((right-left)-(at-lo)) <= len(q)/2+32 && idx.source.indivisible(left, left+16) && idx.source.indivisible(right, right+16) {
				stats.InternalPins = 2
				return []Range{{left, left + 16}, {right, right + 16}}, nil
			}
			if budget < 16 {
				return nil, nil
			}
		}
		lo += 4
		hi = len(q) - 24
	}
	return nil, nil
}
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
