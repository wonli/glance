package desktopui

import (
	"glance/internal/proofreading"
	"reflect"
	"testing"
)

func TestProofreadingAdjacentBoundaryPublishOnlyTouchesTwoBoxes(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	s.modelDirtyPages = nil
	original := s.bDraftText
	third := s.xPages[2]
	if moved, err := s.controller.AdjustPageUnits(1, -1, false); err != nil || moved != -1 {
		t.Fatal(moved, err)
	}
	a.proofreadingBoundaryChanged(0)
	if s.xPages[0].Content != "abc" || s.xPages[1].Content != "defgh" {
		t.Fatal(s.xPages)
	}
	if !reflect.DeepEqual(s.xPages[2], third) || s.bDraftText != original {
		t.Fatal("unrelated box or immutable input stream changed")
	}
	if len(s.modelDirtyPages) != 2 || !s.modelDirtyPages[1] || !s.modelDirtyPages[2] {
		t.Fatal("scheduled more than adjacent boxes", s.modelDirtyPages)
	}
	if len(s.modelLayoutActivePages) != 2 || s.modelLayoutActivePages[0] != proofreading.PageID(1) || s.modelLayoutActivePages[1] != proofreading.PageID(2) || s.modelLayoutNeedsFull {
		t.Fatal("boundary requested whole-book layout", s.modelLayoutActivePages)
	}
}

func TestProofreadingBoundaryRefillPublishesFollowingBoxes(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	s.modelDirtyPages = nil
	if moved, err := s.controller.AdjustPageUnits(1, 100, false); err != nil || moved != 4 {
		t.Fatal(moved, err)
	}
	a.proofreadingBoundaryChanged(0)
	for i, p := range s.xPages {
		if p.Content != s.controller.Model.Text(proofreading.PageID(i+1)) {
			t.Fatal("stale box after refill", i, p.Content, s.controller.Model.Snapshot())
		}
	}
	if !s.modelDirtyPages[3] {
		t.Fatal("refilled suffix was not scheduled for layout", s.modelDirtyPages)
	}
}

func TestProofreadingRefillLayoutSurvivesNextScrollFrame(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	if _, err := s.controller.AdjustPageUnits(1, 4, false); err != nil {
		t.Fatal(err)
	}
	a.proofreadingBoundaryChanged(0)
	if _, err := s.controller.AdjustPageUnits(1, 1, false); err != nil {
		t.Fatal(err)
	}
	a.proofreadingBoundaryChanged(0)
	found := false
	for _, id := range s.modelLayoutActivePages {
		if id == 3 {
			found = true
		}
	}
	if !found {
		t.Fatal("next frame dropped pending suffix layout", s.modelLayoutActivePages)
	}
}
