package proofreading

import (
	"reflect"
	"testing"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func model(t *testing.T) *Model {
	t.Helper()
	m, e := New("甲乙丙丁戊己庚辛", []PageID{1, 2, 3})
	must(t, e)
	must(t, m.AssignInitial([]int{4, 6, 8}))
	return m
}
func TestLockedEditAdjustSplitHistory(t *testing.T) {
	m := model(t)
	initial := m.Snapshot()
	ids := m.IDs(1)
	must(t, m.Lock(1, true))
	must(t, m.ReplacePage(1, "甲乙\n\n丙丁"))
	must(t, m.Adjust(1, 2))
	if m.Text(1) != "甲乙\n\n丙丁戊己" {
		t.Fatal(m.Text(1))
	}
	must(t, m.Adjust(1, -100))
	if m.Text(1) != "甲乙\n\n丙丁" {
		t.Fatal("baseline lost", m.Text(1))
	}
	if !reflect.DeepEqual(ids, append(m.IDs(1)[:2], m.IDs(1)[4:]...)) {
		t.Fatal("existing IDs changed")
	}
	must(t, m.Split(1, 3, false))
	if m.Text(1) != "甲乙\n" {
		t.Fatal(m.Text(1))
	}
	must(t, m.Adjust(1, -100))
	if m.Text(1) != "甲乙\n" {
		t.Fatal("old baseline restored")
	}
	final := m.Snapshot()
	for m.Undo() {
	}
	if !reflect.DeepEqual(initial.Runs, m.Snapshot().Runs) {
		t.Fatal("undo content")
	}
	for m.Redo() {
	}
	if !reflect.DeepEqual(final.Runs, m.Snapshot().Runs) {
		t.Fatal("redo content")
	}
	must(t, m.Validate())
}
func TestReviewedBarrierAndLocalEditing(t *testing.T) {
	m := model(t)
	must(t, m.Review(2, true))
	must(t, m.Lock(1, true))
	must(t, m.Adjust(1, 100))
	if m.Text(1) != "甲乙丙丁" {
		t.Fatal("crossed reviewed page")
	}
	if m.Split(1, 2, false) == nil {
		t.Fatal("split into reviewed page")
	}
	must(t, m.ReplacePage(2, "戊\n新己"))
	if m.Text(3) != "庚辛" {
		t.Fatal("neighbour changed")
	}
	if m.Split(2, 1, false) == nil {
		t.Fatal("reviewed page split")
	}
	must(t, m.Validate())
}
func TestTailEmptyPageAndStaleEdit(t *testing.T) {
	m, e := New("abc", []PageID{1, 2})
	must(t, e)
	must(t, m.AssignInitial([]int{0, 2}))
	if m.Text(2) != "abc" {
		t.Fatal("tail dropped")
	}
	must(t, m.ReplacePage(1, "\n"))
	must(t, m.Validate())
	revision := m.Snapshot().Revision
	must(t, m.ReplacePage(2, "abcd"))
	if m.ReplaceAtRevision(2, "bad", revision) == nil {
		t.Fatal("stale edit accepted")
	}
}

func TestStableCaretAnchorsSurviveNewlinesBeforeCaret(t *testing.T) {
	m := model(t)
	anchor, e := AnchorAt(m.Snapshot(), 1, 2)
	must(t, e)
	must(t, m.ReplacePage(1, "甲\n\n乙丙丁"))
	offset, e := OffsetAt(m.Snapshot(), 1, anchor)
	must(t, e)
	if offset != 4 {
		t.Fatal("caret followed numerical offset instead of character identity", offset)
	}
	must(t, m.SplitAt(1, anchor, false))
	if m.Text(1) != "甲\n\n乙" {
		t.Fatal(m.Text(1))
	}
}
