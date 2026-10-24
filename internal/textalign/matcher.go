package textalign

import (
	"context"
	"sort"
)

type cachedMatch struct {
	region Range
	match  localMatch
}
type matcher struct {
	source       *Source
	parents      []Range
	cache        map[int][]cachedMatch
	budget, used int
	stats        *SearchStats
}

func newMatcher(s *Source, windows []window, budget int, stats *SearchStats) *matcher {
	regions := make([]Range, len(windows))
	longest := 0
	for i, w := range windows {
		regions[i] = Range{w.start, w.end}
		longest = max(longest, w.end-w.start)
	}
	sort.Slice(regions, func(i, j int) bool {
		if regions[i].Start == regions[j].Start {
			return regions[i].End < regions[j].End
		}
		return regions[i].Start < regions[j].Start
	})
	var parents []Range
	for _, r := range regions {
		if len(parents) > 0 {
			last := &parents[len(parents)-1]
			if r.Start <= last.End && max(last.End, r.End)-last.Start <= 2*longest {
				last.End = max(last.End, r.End)
				continue
			}
		}
		parents = append(parents, r)
	}
	return &matcher{source: s, parents: parents, cache: map[int][]cachedMatch{}, budget: budget, stats: stats}
}
func contains(a, b Range) bool        { return a.Start <= b.Start && a.End >= b.End }
func fits(m localMatch, r Range) bool { return m.score == 0 || (m.start >= r.Start && m.end <= r.End) }

// match reuses a superset optimum only when its entire alignment fits the
// requested interval. No alignment in that subset can have a greater score.
func (m *matcher) match(ctx context.Context, key int, q []rune, request Range) (localMatch, bool, error) {
	for _, cached := range m.cache[key] {
		if contains(cached.region, request) && fits(cached.match, request) {
			m.stats.CacheHits++
			return cached.match, true, nil
		}
	}
	parent := request
	for _, r := range m.parents {
		if contains(r, request) {
			parent = r
			break
		}
	}
	tried := false
	for _, cached := range m.cache[key] {
		if cached.region == parent {
			tried = true
			break
		}
	}
	if parent != request && !tried {
		value, ok, err := m.evaluate(ctx, q, parent)
		if err != nil {
			return localMatch{}, false, err
		}
		if ok {
			m.cache[key] = append(m.cache[key], cachedMatch{parent, value})
			if fits(value, request) {
				return value, true, nil
			}
		}
	}
	value, ok, err := m.evaluate(ctx, q, request)
	if ok {
		m.cache[key] = append(m.cache[key], cachedMatch{request, value})
	}
	return value, ok, err
}
func (m *matcher) evaluate(ctx context.Context, q []rune, region Range) (localMatch, bool, error) {
	if err := ctx.Err(); err != nil {
		return localMatch{}, false, err
	}
	linear := len(q) + region.End - region.Start
	if linear > m.budget-m.used {
		return localMatch{}, false, nil
	}
	m.used += linear
	r := m.source.Text[region.Start:region.End]
	// Histogram intersection is an admissible upper bound on exact character
	// matches. A subset cannot satisfy the block's minimum if its superset fails.
	need := make(map[rune]int, len(q))
	for _, c := range q {
		need[c]++
	}
	possible := 0
	for _, c := range r {
		if need[c] > 0 {
			need[c]--
			possible++
		}
	}
	if possible < minimumMatches(len(q)) {
		return localMatch{}, true, nil
	}
	value, ok := exact(q, r)
	if !ok {
		cells := (len(q) + 1) * (len(r) + 1)
		if cells > m.budget-m.used {
			return localMatch{}, false, nil
		}
		m.used += cells
		m.stats.LocalCalls++
		var err error
		value, err = local(ctx, q, r)
		if err != nil {
			return localMatch{}, false, err
		}
	}
	if value.score > 0 {
		value.start += region.Start
		value.end += region.Start
	}
	return value, true, nil
}
