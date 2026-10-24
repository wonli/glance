package application

import (
	"glance/internal/proofreading"
	"testing"
)

func TestProofreadingUnitAdjustmentKeepsGraphemesAndWhitespace(t *testing.T) {
	m, _ := proofreading.New("甲乙e\u0301 👩‍💻\n尾", []proofreading.PageID{1, 2})
	if err := m.AssignInitial([]int{2, 11}); err != nil {
		t.Fatal(err)
	}
	c := NewProofreadingController(m)
	for _, want := range []string{"甲乙e\u0301", "甲乙e\u0301 ", "甲乙e\u0301 👩‍💻", "甲乙e\u0301 👩‍💻\n"} {
		n, err := c.AdjustPageUnits(1, 1, false)
		if err != nil || n != 1 || m.Text(1) != want {
			t.Fatalf("got %q, units=%d err=%v want %q", m.Text(1), n, err, want)
		}
	}
	for _, want := range []string{"甲乙e\u0301 👩‍💻", "甲乙e\u0301 ", "甲乙e\u0301", "甲乙"} {
		n, err := c.AdjustPageUnits(1, -1, false)
		if err != nil || n != -1 || m.Text(1) != want {
			t.Fatalf("got %q, units=%d err=%v want %q", m.Text(1), n, err, want)
		}
	}
	rev := m.Snapshot().Revision
	n, err := c.AdjustPageUnits(1, -1, false)
	if n != -1 || err != nil || m.Snapshot().Revision == rev || m.Text(1) != "甲" {
		t.Fatal("original page boundary could not be corrected", n, err)
	}
}

func TestProofreadingUnitAdjustmentHonorsReviewedBarrier(t *testing.T) {
	m, _ := proofreading.New("甲乙丙丁", []proofreading.PageID{1, 2})
	_ = m.AssignInitial([]int{2, 4})
	_ = m.Review(2, true)
	c := NewProofreadingController(m)
	rev := m.Snapshot().Revision
	n, err := c.AdjustPageUnits(1, 1, false)
	if err != nil || n != 0 || m.Snapshot().Revision != rev || m.Text(1) != "甲乙" {
		t.Fatal(n, err, m.Snapshot())
	}
	if _, err = c.AdjustPageUnits(2, 1, false); err == nil {
		t.Fatal("reviewed target allowed adjustment")
	}
}

func TestProofreadingUnitReverseKeepsClusterAcrossOriginalBoundary(t *testing.T) {
	m, _ := proofreading.New("e\u0301尾", []proofreading.PageID{1, 2})
	_ = m.AssignInitial([]int{1, 3})
	c := NewProofreadingController(m)
	if _, err := c.AdjustPageUnits(1, 1, false); err != nil {
		t.Fatal(err)
	}
	if m.Text(1) != "e\u0301" {
		t.Fatal(m.Text(1))
	}
	moved, err := c.AdjustPageUnits(1, -1, false)
	if err != nil || moved != -1 || m.Text(1) != "" || m.Text(2) != "e\u0301尾" {
		t.Fatal("original boundary prevented continuous correction or split a cluster", moved, err, m.Text(1))
	}
}

func TestProofreadingContinuousBoundaryCanCorrectOriginalAndEarlierManualPages(t *testing.T) {
	m, _ := proofreading.New("甲乙丙丁戊己", []proofreading.PageID{1, 2, 3})
	_ = m.AssignInitial([]int{2, 4, 6})
	c := NewProofreadingController(m)
	if n, err := c.AdjustPageUnits(1, -1, false); err != nil || n != -1 || m.Text(1) != "甲" || m.Text(2) != "乙丙丁" {
		t.Fatal(n, err, m.Snapshot())
	}
	if n, err := c.AdjustPageUnits(1, 2, false); err != nil || n != 2 || m.Text(1) != "甲乙丙" {
		t.Fatal(n, err, m.Snapshot())
	}
	_ = m.ReplacePage(1, "甲乙丙新")
	if n, err := c.AdjustPageUnits(1, -1, false); err != nil || n != 0 || m.Text(1) != "甲乙丙新" {
		t.Fatal("typing not protected", n, err)
	}
	if !m.Undo() || m.Text(1) != "甲乙丙" {
		t.Fatal("typing undo")
	}
	if !m.Undo() || m.Text(1) != "甲" {
		t.Fatal("boundary undo")
	}
}

func TestProofreadingContinuousBoundaryRefillsEmptyNextBox(t *testing.T) {
	m, _ := proofreading.New("甲乙丙丁戊己庚辛", []proofreading.PageID{1, 2, 3, 4})
	_ = m.AssignInitial([]int{2, 4, 6, 8})
	c := NewProofreadingController(m)
	if moved, err := c.AdjustPageUnits(1, 100, false); err != nil || moved != 2 || m.Text(1) != "甲乙丙丁" || m.Text(2) != "戊己" || m.Text(3) != "庚辛" || m.Text(4) != "" {
		t.Fatal("exhausted neighbor was not refilled", moved, err, m.Snapshot())
	}
	if !m.Undo() || m.Text(1) != "甲乙" || m.Text(2) != "丙丁" || m.Text(3) != "戊己" || m.Text(4) != "庚辛" {
		t.Fatal("refill was not one undo step", m.Snapshot())
	}
	if !m.Redo() {
		t.Fatal("redo")
	}
	if moved, err := c.AdjustPageUnits(1, 1, false); err != nil || moved != 1 || m.Text(1) != "甲乙丙丁戊" || m.Text(2) != "己" {
		t.Fatal("could not continue collecting", moved, err, m.Snapshot())
	}
}

func TestProofreadingCoarseAdjustmentUsesShapedLineAndWaitsForLayout(t *testing.T) {
	m, _ := proofreading.New("甲乙丙丁戊己", []proofreading.PageID{1, 2})
	_ = m.AssignInitial([]int{2, 6})
	c := NewProofreadingController(m)
	if _, err := c.AdjustPageUnits(1, 1, true); err != ErrProofreadingLayoutPending {
		t.Fatal("coarse input used stale layout", err)
	}
	c.Layout = ProofreadingLayoutSnapshot{Revision: m.Snapshot().Revision, Pages: map[proofreading.PageID]ProofreadingPageLayout{1: {Text: "甲乙丙丁戊己", Lines: []ProofreadingLine{{Start: 0, End: 3}, {Start: 3, End: 6}}}}}
	n, err := c.AdjustPageUnits(1, 1, true)
	if err != nil || n != 1 || m.Text(1) != "甲乙丙" {
		t.Fatal(n, err, m.Text(1))
	}
	c.Layout.Revision = m.Revision()
	n, err = c.AdjustPageUnits(1, -1, true)
	if err != nil || n != -3 || m.Text(1) != "" || m.Text(2) != "甲乙丙丁戊己" {
		t.Fatal("coarse reverse did not use adjacent boundary", n, err, m.Snapshot())
	}
}

func TestProofreadingCoarseRejectsStaleNeighborPreview(t *testing.T) {
	m, _ := proofreading.New("abcdef", []proofreading.PageID{1, 2})
	_ = m.AssignInitial([]int{2, 6})
	c := NewProofreadingController(m)
	c.Layout = ProofreadingLayoutSnapshot{Pages: map[proofreading.PageID]ProofreadingPageLayout{1: {Text: "abcdef", Lines: []ProofreadingLine{{Start: 0, End: 4}, {Start: 4, End: 6}}}}}
	_ = m.ReplacePage(2, "\nxyz")
	c.Layout.Revision = m.Revision()
	if _, err := c.AdjustPageUnits(1, 1, true); err != ErrProofreadingLayoutPending || m.Text(1) != "ab" {
		t.Fatal("stale neighbor line geometry changed ownership", err, m.Snapshot())
	}
}

func TestProofreadingFineCacheInvalidatesOnNeighborEditAndRefill(t *testing.T) {
	m, _ := proofreading.New("甲乙丙丁戊己", []proofreading.PageID{1, 2, 3})
	_ = m.AssignInitial([]int{2, 4, 6})
	c := NewProofreadingController(m)
	if n, err := c.AdjustPageUnits(1, 1, false); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if err := m.ReplacePage(2, "e\u0301👩‍💻"); err != nil {
		t.Fatal(err)
	}
	if n, err := c.AdjustPageUnits(1, 1, false); err != nil || n != 1 || m.Text(1) != "甲乙丙e\u0301" {
		t.Fatal("stale segmentation after edit", n, err, m.Text(1))
	}
	if n, err := c.AdjustPageUnits(1, 1, false); err != nil || n != 1 || m.Text(2) != "戊己" {
		t.Fatal("refill", n, err, m.Snapshot())
	}
	if n, err := c.AdjustPageUnits(1, 1, false); err != nil || n != 1 || m.Text(2) != "己" {
		t.Fatal("stale segmentation after refill", n, err, m.Snapshot())
	}
	if !m.Undo() {
		t.Fatal("undo")
	}
	if n, err := c.AdjustPageUnits(1, 1, false); err != nil || n != 1 || m.Text(2) != "己" {
		t.Fatal("stale cache after undo", n, err, m.Snapshot())
	}
}

func TestProofreadingHeadCoarseUsesCurrentFirstLine(t *testing.T) {
	m, _ := proofreading.New("abcdefgh甲乙丙丁戊己庚辛壬癸后续", []proofreading.PageID{1, 2, 3})
	_ = m.AssignInitial([]int{8, 18, 20})
	c := NewProofreadingController(m)
	c.Layout = ProofreadingLayoutSnapshot{Revision: m.Revision(), Pages: map[proofreading.PageID]ProofreadingPageLayout{
		1: {Text: "abcdefgh甲乙丙丁戊己庚辛壬癸", Lines: []ProofreadingLine{{Start: 0, End: 10}, {Start: 10, End: 18}}},
		2: {Text: "甲乙丙丁戊己庚辛壬癸", Lines: []ProofreadingLine{{Start: 0, End: 5}, {Start: 5, End: 10}}},
	}}
	if moved, err := c.AdjustPageHeadUnits(2, -1, true); err != nil || moved != -5 || m.Text(1) != "abcdefgh甲乙丙丁戊" || m.Text(2) != "己庚辛壬癸" {
		t.Fatal("used previous partial line instead of current first line", moved, err, m.Snapshot())
	}
	if !m.Undo() {
		t.Fatal("undo")
	}
	if moved, err := c.AdjustPageHeadUnits(2, 1, false); err != nil || moved != 1 || m.Text(1) != "abcdefg" || m.Text(2) != "h甲乙丙丁戊己庚辛壬癸" {
		t.Fatal("down did not pull previous suffix", moved, err, m.Snapshot())
	}
}
