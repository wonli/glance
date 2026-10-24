package proofreading

import "testing"

func TestHistoryIdentityNeverReusedAfterUndo(t *testing.T) {
	m, _ := New("a", []PageID{1})
	if err := m.ReplacePage(1, "ab"); err != nil {
		t.Fatal(err)
	}
	next := m.Snapshot().Next
	m.Undo()
	if m.Snapshot().Next < next {
		t.Fatal("undo reused historical character identities")
	}
}

func TestHistoryRestoreKeepsUndoAndRejectsLateCandidate(t *testing.T) {
	m, _ := New("old", []PageID{1})
	m.ReplacePage(1, "current")
	candidate, _ := New("historical", []PageID{1})
	if err := m.AcceptRestored(candidate, m.Revision()); err != nil {
		t.Fatal(err)
	}
	if !m.Undo() || m.Text(1) != "current" {
		t.Fatal("history restore discarded prior undo")
	}
	if err := m.AcceptRestored(candidate, 0); err == nil {
		t.Fatal("late restore overwrote current model")
	}
}
