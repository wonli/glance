package desktopui

import (
	"glance/internal/application"
	"glance/internal/proofreading"
	"strings"
	"testing"
)

func proofreadingModelFixture(t *testing.T) (*App, *proofreadingWorkspace) {
	t.Helper()
	m, err := proofreading.New("abcdefghijkl", []proofreading.PageID{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.AssignInitial([]int{4, 8, 12}); err != nil {
		t.Fatal(err)
	}
	a := New(nil, "")
	a.managedBackgroundClosed = true
	s := &proofreadingWorkspace{controller: application.NewProofreadingController(m), modelInitialAssigned: true, bDocument: &proofreadingObject{}, bDraftInitialized: true, pageFrames: []proofreadingPageFrame{{PageIndex: 0, Width: 400, Height: 600}, {PageIndex: 1, Width: 400, Height: 600}, {PageIndex: 2, Width: 400, Height: 600}}}
	s.controller.Layout.Pages = map[proofreading.PageID]application.ProofreadingPageLayout{}
	a.proofreading = s
	a.refreshProofreadingModelView()
	a.syncProofreadingEditors()
	return a, s
}
func TestProofreadingNewlinesThenScrollRetainsLockedText(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	if err := s.controller.Model.Lock(1, true); err != nil {
		t.Fatal(err)
	}
	a.applyProofreadingPageEdit(s.xPages[0], "ab\n\n\ncd")
	if err := s.controller.Model.Adjust(1, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.controller.Model.Adjust(1, -100); err != nil {
		t.Fatal(err)
	}
	a.refreshProofreadingModelView()
	if s.xPages[0].Content != "ab\n\n\ncd" || s.xPages[1].Content != "efgh" {
		t.Fatal(s.xPages)
	}
}
func TestProofreadingSplitDownUsesNativeCaretAndHistory(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	_ = s.controller.Model.Lock(1, true)
	s.xPageMenuPage = 0
	s.xPageEditors[0].SetCaret(2, 2)
	a.splitProofreadingPage(false)
	if s.xPages[0].Content != "ab" || s.xPages[1].Content != "cdefgh" {
		t.Fatal(s.xPages)
	}
	a.undoProofreadingPagination()
	if s.xPages[0].Content != "abcd" || s.xPages[1].Content != "efgh" {
		t.Fatal(s.xPages)
	}
	a.redoProofreading()
	if s.xPages[0].Content != "ab" {
		t.Fatal(s.xPages)
	}
}

func TestProofreadingSplitUsesStableCaretAfterPageViewportScroll(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	long := strings.Repeat("x", 200)
	if err := s.controller.Model.ReplacePage(1, long); err != nil {
		t.Fatal(err)
	}
	if err := s.controller.Model.Lock(1, true); err != nil {
		t.Fatal(err)
	}
	a.refreshProofreadingModelView()
	s.xPageMenuPage = 0
	s.xEditorWindows[0].Start = 100
	s.xPageEditors[0].SetCaret(2, 2)
	anchor, err := proofreading.AnchorAt(s.controller.Model.Snapshot(), 1, 150)
	if err != nil {
		t.Fatal(err)
	}
	s.xEditorCarets[0] = proofreadingEditorCaret{Start: anchor, End: anchor, Valid: true}
	a.splitProofreadingPage(false)
	if got := s.controller.Model.Text(1); got != long[:150] {
		t.Fatalf("split ignored the stable caret after the page viewport moved: kept %d characters, want 150", len([]rune(got)))
	}
}
func TestProofreadingReviewedEditDoesNotChangeNeighbour(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	_ = s.controller.Model.Review(1, true)
	a.applyProofreadingPageEdit(s.xPages[0], "a\n\nnew\nbcd")
	if s.xPages[1].Content != "efgh" {
		t.Fatal(s.xPages)
	}
	s.xPageMenuPage = 0
	s.xPageEditors[0].SetCaret(2, 2)
	a.splitProofreadingPage(false)
	if s.xPages[1].Content != "efgh" {
		t.Fatal("reviewed page split")
	}
}
