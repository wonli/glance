package proofreading

import "testing"

func TestAdjacentBoundaryReviewedBoxOwnsLocalEditsAfterReopen(t *testing.T) {
	m, _ := New("甲乙丙丁戊己", []PageID{1, 2, 3})
	must(t, m.AssignInitial([]int{2, 4, 6}))
	must(t, m.Review(2, true))
	must(t, m.ReplacePage(2, "新丙"))
	encoded, err := Encode(m.Snapshot())
	must(t, err)
	snapshot, err := Decode(encoded)
	must(t, err)
	n, err := Restore(snapshot)
	must(t, err)
	must(t, n.AdjustBoundary(1, 100))
	must(t, n.AdjustBoundary(1, -100))
	if n.Text(1) != "甲乙" || n.Text(2) != "新丙" || n.Text(3) != "戊己" {
		t.Fatal("finalized box edits leaked back to allocation", n.Snapshot())
	}
	if n.AdjustBoundary(2, 1) == nil {
		t.Fatal("finalized box allowed cross-box adjustment")
	}
}
