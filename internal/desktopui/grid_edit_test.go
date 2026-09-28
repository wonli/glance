package desktopui

import (
	"errors"
	"testing"
)

func TestCellEditBeginCommitCancel(t *testing.T) {
	s := cellEditState{}
	s.selectCell("C4", "原值")
	if !s.begin() {
		t.Fatal("begin failed")
	}
	s.setDraft("草稿")
	if s.Original != "原值" || s.Draft != "草稿" {
		t.Fatalf("state=%+v", s)
	}
	s.cancel()
	if s.Editing || s.Draft != "原值" || s.Original != "原值" {
		t.Fatalf("cancel=%+v", s)
	}
	_ = s.begin()
	s.setDraft("更新")
	calls := 0
	if err := s.commit(func(address, value string) error {
		calls++
		if address != "C4" || value != "更新" {
			t.Fatalf("%s=%s", address, value)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || s.Editing || s.Original != "更新" {
		t.Fatalf("commit=%+v calls=%d", s, calls)
	}
}
func TestCellEditNoOpDoesNotDirty(t *testing.T) {
	s := cellEditState{}
	s.selectCell("A1", "same")
	_ = s.begin()
	calls := 0
	if err := s.commit(func(string, string) error { calls++; return nil }); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("no-op called apply %d times", calls)
	}
}
func TestCellEditRejectsFormulaAndRetainsDraftOnError(t *testing.T) {
	s := cellEditState{}
	s.selectCell("A1", "10")
	_ = s.begin()
	s.setDraft("=SUM(A1:A2)")
	want := errors.New("formulas unsupported")
	err := s.commit(func(_, value string) error {
		if value != "=SUM(A1:A2)" {
			t.Fatalf("draft=%q", value)
		}
		return want
	})
	if !errors.Is(err, want) || !s.Editing || s.Draft != "=SUM(A1:A2)" || s.Original != "10" {
		t.Fatalf("state=%+v err=%v", s, err)
	}
}
func TestCellSelectionSyncsNameAndValue(t *testing.T) {
	s := cellEditState{}
	s.selectCell("B7", "员工")
	if s.Address != "B7" || s.Draft != "员工" {
		t.Fatalf("state=%+v", s)
	}
}
