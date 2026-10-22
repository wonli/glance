package proofreading

import "testing"

func TestAdjustmentNeedsNoLockAndSurvivesReflow(t *testing.T) {
	m, _ := New("甲乙丙丁戊己", []PageID{1, 2, 3})
	must(t, m.AssignInitial([]int{2, 4, 6}))
	must(t, m.Adjust(1, 1))
	if m.Text(1) != "甲乙丙" || m.Text(2) != "丁" {
		t.Fatal(m.Snapshot())
	}
	must(t, m.Reflow(map[PageID]int{1: 1, 2: 4, 3: 6}))
	if m.Text(1) != "甲乙丙" {
		t.Fatal("reflow erased manual boundary", m.Snapshot())
	}
	must(t, m.Adjust(2, 1))
	if m.Text(1) != "甲乙丙" || m.Text(2) != "丁戊" {
		t.Fatal("switching target changed previous page", m.Snapshot())
	}
	must(t, m.Adjust(1, -10))
	// Page 2 is now independent: returning into it must stop at the barrier.
	if m.Text(1) != "甲乙丙" {
		t.Fatal("returned into independent page")
	}
}

func TestAutomaticAdjustmentProtectsBaseAndTypingAndUndo(t *testing.T) {
	m, _ := New("甲乙丙丁", []PageID{1, 2})
	must(t, m.AssignInitial([]int{2, 4}))
	must(t, m.Adjust(1, 1))
	if !m.Undo() || m.Text(1) != "甲乙" || m.Text(2) != "丙丁" {
		t.Fatal("first adjustment was not one undo step")
	}
	if !m.Redo() || m.Text(1) != "甲乙丙" {
		t.Fatal("redo")
	}
	must(t, m.Adjust(1, -100))
	if m.Text(1) != "甲乙" {
		t.Fatal("base text returned")
	}
	must(t, m.ReplacePage(1, "甲乙新"))
	must(t, m.Adjust(1, 2))
	must(t, m.Adjust(1, -100))
	if m.Text(1) != "甲乙新" || m.Text(2) != "丙丁" {
		t.Fatal("typing lost", m.Snapshot())
	}
	data, err := Encode(m.Snapshot())
	must(t, err)
	restored, err := Decode(data)
	must(t, err)
	n, err := Restore(restored)
	must(t, err)
	must(t, n.Reflow(map[PageID]int{1: 1, 2: 5}))
	if n.Text(1) != "甲乙新" {
		t.Fatal("saved manual boundary lost")
	}
}

func TestManualReverseAfterFollowingPagesReflowPreservesOrder(t *testing.T) {
	m, _ := New("abcdef", []PageID{1, 2, 3})
	must(t, m.AssignInitial([]int{2, 4, 6}))
	must(t, m.Adjust(1, 3))
	must(t, m.Reflow(map[PageID]int{1: 5, 2: 6, 3: 6}))
	must(t, m.Adjust(1, -1))
	if m.Text(1) != "abcd" || m.Text(2) != "ef" || m.Text(3) != "" {
		t.Fatal(m.Snapshot())
	}
	must(t, m.Adjust(1, -100))
	if m.Text(1) != "ab" || m.Text(2) != "cdef" {
		t.Fatal(m.Snapshot())
	}
	must(t, m.Validate())
}
