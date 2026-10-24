package textalign

import (
	"context"
	"math/rand"
	"testing"
)

// An exhaustive oracle enumerates every skip and placement on small graphs;
// it is independent of the prefix/suffix-max implementation used for books.
func exhaustive(results []Result, exclude, forcePage, forceChoice int) float64 {
	best := -1.0
	var visit func(int, int, float64)
	visit = func(page, end int, score float64) {
		if page == len(results) {
			if score > best {
				best = score
			}
			return
		}
		if page != forcePage {
			visit(page+1, end, score)
		}
		if page == exclude {
			return
		}
		for choice, c := range results[page].Candidates {
			if page == forcePage && choice != forceChoice {
				continue
			}
			if c.start >= end {
				visit(page+1, c.end, score+c.Score)
			}
		}
	}
	visit(0, 0, 0)
	return best
}

func TestPathSolverMatchesExhaustiveAlternatives(t *testing.T) {
	rng := rand.New(rand.NewSource(20261024))
	for trial := 0; trial < 80; trial++ {
		results := make([]Result, 4)
		for i := range results {
			for j := 0; j < 3; j++ {
				start := rng.Intn(12)
				results[i].Candidates = append(results[i].Candidates, Candidate{start: start, end: start + 1 + rng.Intn(4), Score: float64(1 + rng.Intn(9))})
			}
		}
		s := newPathSolver(results, 100000)
		best, forward, _, ok, err := s.forward(context.Background(), -1)
		if err != nil || !ok {
			t.Fatalf("solver: %v", err)
		}
		if want := exhaustive(results, -1, -1, -1); best.score != want {
			t.Fatalf("trial%d best=%v want=%v", trial, best.score, want)
		}
		suffix, ok, err := s.suffix(context.Background())
		if err != nil || !ok {
			t.Fatal(err)
		}
		for id, n := range s.nodes {
			if want := exhaustive(results, -1, n.page, n.choice); forward[id]+suffix[id] != want {
				t.Fatalf("trial%d through p%d c%d=%v want%v", trial, n.page, n.choice, forward[id]+suffix[id], want)
			}
		}
		for page := range results {
			got, _, _, ok, err := s.forward(context.Background(), page)
			if err != nil || !ok {
				t.Fatal(err)
			}
			if want := exhaustive(results, page, -1, -1); got.score != want {
				t.Fatalf("trial%d exclude%d=%v want%v", trial, page, got.score, want)
			}
		}
	}
}

func TestPathBudgetCannotClaimUnique(t *testing.T) {
	page := Page{Number: 1, Blocks: []Block{{Text: "天地玄黄宇宙洪荒"}}}
	got, err := Align(context.Background(), []Page{page}, "天地玄黄宇宙洪荒", Options{MaxPathWork: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Status != "limited" {
		t.Fatalf("unchecked alternatives reported %s", got[0].Status)
	}
}
