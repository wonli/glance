package application

import (
	"context"
	"glance/internal/document"
	"glance/internal/proofreading"
	"glance/internal/wordlayout"
	"testing"
)

func TestProofreadingControllerRejectsStaleLayout(t *testing.T) {
	m, _ := proofreading.New("abc", []proofreading.PageID{1, 2})
	c := NewProofreadingController(m)
	revision := m.Snapshot().Revision
	if err := c.Replace(1, "a\nbc", revision); err != nil {
		t.Fatal(err)
	}
	if c.AcceptLayout(ProofreadingLayoutSnapshot{Revision: revision}) {
		t.Fatal("stale layout accepted")
	}
}
func TestProofreadingControllerWheelUsesLines(t *testing.T) {
	m, _ := proofreading.New("abcdefgh", []proofreading.PageID{1, 2})
	_ = m.AssignInitial([]int{4, 8})
	_ = m.Lock(1, true)
	c := NewProofreadingController(m)
	c.AcceptLayout(ProofreadingLayoutSnapshot{Revision: m.Snapshot().Revision, Pages: map[proofreading.PageID]ProofreadingPageLayout{1: {Text: "abcdefgh", Lines: []ProofreadingLine{{Start: 0, End: 2, Top: 0, Bottom: 10}, {Start: 2, End: 4, Top: 10, Bottom: 20}, {Start: 4, End: 6, Top: 20, Bottom: 30}, {Start: 6, End: 8, Top: 30, Bottom: 40}}}}})
	top, err := c.AdjustLockedPage(1, 0, 20, 10)
	if err != nil {
		t.Fatal(err)
	}
	if top != 10 || m.Text(1) != "abcdef" {
		t.Fatal(top, m.Text(1))
	}
	_, err = c.AdjustLockedPage(1, 10, 20, -10)
	if err != nil {
		t.Fatal(err)
	}
	if m.Text(1) != "abcd" {
		t.Fatal(m.Text(1))
	}
}

func TestProofreadingControllerUpAtAdjustmentStartDoesNotAcquireFollowingText(t *testing.T) {
	m, _ := proofreading.New("abcdefgh", []proofreading.PageID{1, 2})
	if err := m.Lock(1, true); err != nil {
		t.Fatal(err)
	}
	anchor, err := proofreading.AnchorAt(m.Snapshot(), 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.SplitAt(1, anchor, false); err != nil {
		t.Fatal(err)
	}
	if got := m.Text(1); got != "ab" {
		t.Fatalf("split setup left unexpected locked-page text %q", got)
	}
	c := NewProofreadingController(m)
	c.AcceptLayout(ProofreadingLayoutSnapshot{
		Revision: m.Snapshot().Revision,
		Pages: map[proofreading.PageID]ProofreadingPageLayout{
			1: {Text: "abcdefgh", Lines: []ProofreadingLine{
				{Start: 0, End: 2, Top: 0, Bottom: 10},
				{Start: 2, End: 4, Top: 10, Bottom: 20},
				{Start: 4, End: 6, Top: 20, Bottom: 30},
				{Start: 6, End: 8, Top: 30, Bottom: 40},
			}},
		},
	})
	top, err := c.AdjustLockedPage(1, 0, 40, -10)
	if err != nil {
		t.Fatal(err)
	}
	if top != 0 || m.Text(1) != "ab" {
		t.Fatalf("upward scroll at the adjustment start acquired text: top=%v page=%q", top, m.Text(1))
	}
}

func TestProofreadingControllerReverseReturnsAcquiredTextOnShortPage(t *testing.T) {
	m, _ := proofreading.New("abcdefgh", []proofreading.PageID{1, 2})
	if err := m.AssignInitial([]int{2, 8}); err != nil {
		t.Fatal(err)
	}
	if err := m.Lock(1, true); err != nil {
		t.Fatal(err)
	}
	c := NewProofreadingController(m)
	lines := []ProofreadingLine{{Start: 0, End: 2, Top: 0, Bottom: 10}, {Start: 2, End: 4, Top: 10, Bottom: 20}, {Start: 4, End: 6, Top: 20, Bottom: 30}, {Start: 6, End: 8, Top: 30, Bottom: 40}}
	c.AcceptLayout(ProofreadingLayoutSnapshot{Revision: m.Snapshot().Revision, Pages: map[proofreading.PageID]ProofreadingPageLayout{1: {Text: "abcdefgh", Lines: lines}}})
	top, err := c.AdjustLockedPage(1, 0, 40, 5)
	if err != nil {
		t.Fatal(err)
	}
	if m.Text(1) != "abcdefgh" {
		t.Fatalf("forward scroll failed to acquire text: %q", m.Text(1))
	}
	c.Layout.Revision = m.Snapshot().Revision
	_, err = c.AdjustLockedPage(1, top, 40, -5)
	if err != nil {
		t.Fatal(err)
	}
	if m.Text(1) != "ab" || m.Text(2) != "cdefgh" {
		t.Fatalf("reverse at adjustment start must return acquired characters, keeping locked base: page1=%q page2=%q", m.Text(1), m.Text(2))
	}
}

func TestProofreadingRealLayoutMapsNewlinesAndWrap(t *testing.T) {
	lines, err := proofreadingShapeLines(context.Background(), "甲乙丙丁\n\n戊己", 25, ProofreadingTextStyle{Size: 10}, proofreadingTestShaper{})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) < 4 {
		t.Fatalf("missing wrapped/empty lines: %+v", lines)
	}
	end := 0
	top := -1.0
	for _, line := range lines {
		if line.Start != end || line.End < line.Start || line.Top <= top || line.Bottom <= line.Top {
			t.Fatalf("invalid line mapping: %+v", lines)
		}
		end = line.End
		top = line.Top
	}
	if end != 8 {
		t.Fatalf("text tail lost: %+v", lines)
	}
}
func TestProofreadingReflowLayoutStopsAtBarrier(t *testing.T) {
	m, _ := proofreading.New("甲乙丙丁戊己庚辛壬癸", []proofreading.PageID{1, 2, 3})
	_ = m.AssignInitial([]int{4, 6, 10})
	_ = m.Review(2, true)
	geometry := []ProofreadingPageGeometry{{Page: 1, Width: 12, Height: 12}, {Page: 2, Width: 12, Height: 12}, {Page: 3, Width: 12, Height: 12}}
	result, err := ReflowProofreading(context.Background(), m.Snapshot(), geometry, ProofreadingTextStyle{Size: 10}, proofreadingTestShaper{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.ReflowDerived(result.Ends); err != nil {
		t.Fatal(err)
	}
	if m.Text(1) != "甲乙丙丁" || m.Text(2) != "戊己" || m.Text(3) != "庚辛壬癸" {
		t.Fatal("independent page boundary changed")
	}
}

type proofreadingTestShaper struct{}

func (proofreadingTestShaper) Shape(value string, style document.WordTextStyle) (wordlayout.ShapedText, error) {
	glyphs := []wordlayout.ShapedGlyph{}
	for i, r := range []rune(value) {
		glyphs = append(glyphs, wordlayout.ShapedGlyph{FontKey: "test", GlyphID: uint32(r), RuneStart: i, RuneEnd: i + 1, AdvanceXPT: style.FontSize})
	}
	return wordlayout.ShapedText{Glyphs: glyphs, AscentPT: style.FontSize * 0.8, DescentPT: style.FontSize * 0.2}, nil
}
func (proofreadingTestShaper) FontData(string) ([]byte, error) { return nil, nil }
func (proofreadingTestShaper) GlyphOutline(string, uint32) (wordlayout.GlyphOutline, error) {
	return wordlayout.GlyphOutline{UnitsPerEm: 1000}, nil
}
