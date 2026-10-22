package application

import (
	"context"
	"glance/internal/proofreading"
	"testing"
)

func TestProofreadingBoundaryLayoutShapesOnlyChangedPages(t *testing.T) {
	m, _ := proofreading.New("甲乙丙丁戊己", []proofreading.PageID{1, 2, 3})
	_ = m.AssignInitial([]int{2, 4, 6})
	_ = m.AdjustBoundary(1, -1)
	previous := ProofreadingLayoutSnapshot{Pages: map[proofreading.PageID]ProofreadingPageLayout{3: {Text: "戊己", Height: 77}}}
	calls := 0
	shape := func(_ context.Context, value string, _ float64, _ ProofreadingTextStyle) ([]ProofreadingLine, error) {
		calls++
		return []ProofreadingLine{{Start: 0, End: len([]rune(value)), Bottom: 20}}, nil
	}
	got, err := LayoutProofreadingOwnedPages(context.Background(), m.Snapshot(), []ProofreadingPageGeometry{{Page: 1, Width: 100}, {Page: 2, Width: 100}, {Page: 3, Width: 100}}, ProofreadingTextStyle{}, previous, []proofreading.PageID{1, 2}, shape)
	if err != nil || calls != 2 || got.Pages[1].Text != "甲乙丙丁" || got.Pages[2].Text != "乙丙丁戊己" || got.Pages[3].Height != 77 {
		t.Fatal(err, calls, got)
	}
	if got.Revision != m.Revision() || previous.Pages[1].Text != "" {
		t.Fatal("mutated previous published layout")
	}
}
