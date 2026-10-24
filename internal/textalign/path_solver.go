package textalign

import (
	"context"
	"sort"
)

type candidateNode struct {
	page, choice int
	candidate    Candidate
}
type route struct {
	score float64
	node  int
}
type routeTree struct{ values []route }

func newRouteTree(n int) *routeTree {
	t := &routeTree{values: make([]route, n+1)}
	for i := range t.values {
		t.values[i].node = -1
	}
	return t
}
func better(a, b route) bool { return a.score > b.score || (a.score == b.score && a.node < b.node) }
func (t *routeTree) put(at int, value route) {
	for at < len(t.values) {
		if better(value, t.values[at]) {
			t.values[at] = value
		}
		at += at & -at
	}
}
func (t *routeTree) get(at int) route {
	best := route{node: -1}
	for at > 0 {
		if better(t.values[at], best) {
			best = t.values[at]
		}
		at -= at & -at
	}
	return best
}

type pathSolver struct {
	nodes        []candidateNode
	groups       [][]int
	starts, ends []int
	used, limit  int
}

func newPathSolver(results []Result, limit int) *pathSolver {
	s := &pathSolver{groups: make([][]int, len(results)), limit: limit}
	for i, r := range results {
		for j, c := range r.Candidates {
			s.groups[i] = append(s.groups[i], len(s.nodes))
			s.nodes = append(s.nodes, candidateNode{i, j, c})
			s.starts = append(s.starts, c.start)
			s.ends = append(s.ends, c.end)
		}
	}
	s.starts = uniqueSorted(s.starts)
	s.ends = uniqueSorted(s.ends)
	return s
}
func uniqueSorted(v []int) []int {
	sort.Ints(v)
	n := 0
	for _, x := range v {
		if n == 0 || x != v[n-1] {
			v[n] = x
			n++
		}
	}
	return v[:n]
}
func (s *pathSolver) charge() bool {
	if s.used >= s.limit {
		return false
	}
	s.used++
	return true
}

// forward uses a Fenwick prefix maximum over B end positions. Updates are
// batched by page, so two candidates from the same A page can never connect.
// It finds the best path over all retained candidates, including skipped pages.
func (s *pathSolver) forward(ctx context.Context, exclude int) (route, []float64, []int, bool, error) {
	if s.used >= s.limit {
		return route{}, nil, nil, false, nil
	}
	tree := newRouteTree(len(s.ends))
	scores := make([]float64, len(s.nodes))
	previous := make([]int, len(s.nodes))
	for i := range previous {
		previous[i] = -1
	}
	for page, group := range s.groups {
		if err := ctx.Err(); err != nil {
			return route{}, nil, nil, false, err
		}
		if page == exclude {
			continue
		}
		for _, id := range group {
			if !s.charge() {
				return route{}, nil, nil, false, nil
			}
			c := s.nodes[id].candidate
			before := tree.get(sort.Search(len(s.ends), func(i int) bool { return s.ends[i] > c.start }))
			scores[id] = before.score + c.Score
			previous[id] = before.node
		}
		for _, id := range group {
			c := s.nodes[id].candidate
			tree.put(sort.SearchInts(s.ends, c.end)+1, route{scores[id], id})
		}
	}
	return tree.get(len(s.ends)), scores, previous, true, nil
}
func (s *pathSolver) suffix(ctx context.Context) ([]float64, bool, error) {
	tree := newRouteTree(len(s.starts))
	suffix := make([]float64, len(s.nodes))
	for page := len(s.groups) - 1; page >= 0; page-- {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		for _, id := range s.groups[page] {
			if !s.charge() {
				return nil, false, nil
			}
			c := s.nodes[id].candidate
			suffix[id] = tree.get(len(s.starts) - sort.SearchInts(s.starts, c.end)).score
		}
		for _, id := range s.groups[page] {
			c := s.nodes[id].candidate
			tree.put(len(s.starts)-sort.SearchInts(s.starts, c.start), route{suffix[id] + c.Score, id})
		}
	}
	return suffix, true, nil
}
