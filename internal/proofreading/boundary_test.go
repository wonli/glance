package proofreading

import (
	"reflect"
	"testing"
)

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

func TestBoundaryRefillPreservesIdentityAndReviewedBarrier(t *testing.T) {
	for _, reviewed := range []bool{false, true} {
		m, _ := New("abcdefghij", []PageID{6, 7, 8, 9, 10})
		must(t, m.AssignInitial([]int{2, 4, 6, 8, 10}))
		if reviewed {
			must(t, m.Review(9, true))
		}
		before := m.Snapshot()
		ids := m.IDs(8)
		must(t, m.AdjustBoundary(6, 2))
		if m.Text(6) != "abcd" || m.Text(7) != "ef" || !reflect.DeepEqual(m.IDs(7), ids) {
			t.Fatal("refill lost content or identity", m.Snapshot())
		}
		if reviewed {
			if m.Text(8) != "" || m.Text(9) != "gh" || m.Text(10) != "ij" {
				t.Fatal("crossed reviewed barrier", m.Snapshot())
			}
		} else if m.Text(8) != "gh" || m.Text(9) != "ij" || m.Text(10) != "" {
			t.Fatal("suffix not shifted", m.Snapshot())
		}
		encoded, err := Encode(m.Snapshot())
		must(t, err)
		restored, err := Decode(encoded)
		must(t, err)
		if !reflect.DeepEqual(restored.Runs, m.Snapshot().Runs) {
			t.Fatal("refill did not persist")
		}
		if !m.Undo() || !reflect.DeepEqual(m.Snapshot().Runs, before.Runs) || !reflect.DeepEqual(m.Snapshot().Pages, before.Pages) {
			t.Fatal("undo failed")
		}
		if !m.Redo() || m.Text(7) != "ef" {
			t.Fatal("redo failed")
		}
	}
}
