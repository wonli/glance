package textalign

import (
	"context"
	"math/rand"
	"testing"
)

func TestJointCutsMatchExhaustiveEditPaths(t *testing.T) {
	rng := rand.New(rand.NewSource(20261024))
	for trial := 0; trial < 60; trial++ {
		q, r := make([]rune, rng.Intn(5)), make([]rune, rng.Intn(5))
		for i := range q {
			q[i] = rune('甲' + rng.Intn(2))
		}
		for i := range r {
			r[i] = rune('甲' + rng.Intn(2))
		}
		rows := make([]int, len(q)+1)
		for i := range rows {
			rows[i] = i
		}
		got, err := joint(context.Background(), q, r, rows, nil)
		if err != nil {
			t.Fatal(err)
		}
		best := len(q) + len(r) + 1
		want := map[int]Range{}
		type position struct{ i, j int }
		var visited []position
		var walk func(int, int, int)
		walk = func(i, j, cost int) {
			if cost > best {
				return
			}
			visited = append(visited, position{i, j})
			defer func() { visited = visited[:len(visited)-1] }()
			if i == len(q) && j == len(r) {
				if cost < best {
					best = cost
					want = map[int]Range{}
				}
				if cost == best {
					for _, p := range visited {
						span, ok := want[p.i]
						if !ok {
							span = Range{p.j, p.j}
						}
						span.Start = min(span.Start, p.j)
						span.End = max(span.End, p.j)
						want[p.i] = span
					}
				}
				return
			}
			if i < len(q) && j < len(r) {
				delta := 0
				if q[i] != r[j] {
					delta = 1
				}
				walk(i+1, j+1, cost+delta)
			}
			if i < len(q) {
				walk(i+1, j, cost+1)
			}
			if j < len(r) {
				walk(i, j+1, cost+1)
			}
		}
		walk(0, 0, 0)
		for _, row := range rows {
			if got.cuts[row] != want[row] {
				t.Fatalf("q=%q r=%q row%d got%+v want%+v", string(q), string(r), row, got.cuts[row], want[row])
			}
		}
	}
}

func TestJointCannotSplitExpandedEntity(t *testing.T) {
	got, err := joint(context.Background(), []rune("fj"), []rune("fj"), []int{1}, func(j int) bool { return j != 1 })
	if err != nil {
		t.Fatal(err)
	}
	if cut := got.cuts[1]; cut.End >= cut.Start {
		t.Fatalf("accepted forbidden entity cut %+v", cut)
	}
}

func TestJointEmptyInputHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := joint(ctx, nil, nil, []int{0}, nil); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestDuplicateLongPagesDoNotBecomeDistinctAnchors(t *testing.T) {
	body := uniqueText(0x5b00, 40)
	got, err := Align(context.Background(), []Page{textPage(1, body), textPage(2, body)}, body, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got[0].Anchors) > 0 || len(got[1].Anchors) > 0 || got[0].BoundaryAfter.Status != "unresolved" {
		t.Fatal("repeated pages established conflicting anchors")
	}
}

func TestAnchorSupportUsesQueryIdentityNotOnlyBlockID(t *testing.T) {
	c := Candidate{Evidence: []Evidence{{BlockID: 0, start: 10, end: 40, QueryRange: Range{0, 30}, queryOrder: 1}}}
	a := Anchor{BlockID: 0, start: 12, end: 28, QueryRange: Range{2, 18}, queryOrder: 0}
	if supports(c, a) {
		t.Fatal("anchor borrowed support from another OCR block with reused ID")
	}
}
