package wordlayout

import (
	"context"
	"math"
	"testing"

	"glance/internal/document"
)

type gridMetricsShaper struct{ layoutFakeShaper }

func (s gridMetricsShaper) Shape(value string, style document.WordTextStyle) (ShapedText, error) {
	out, err := s.layoutFakeShaper.Shape(value, style)
	out.LineGapPT = style.FontSize / 2
	return out, err
}

func TestDocumentGridUsesFontLeadingAndRoundsToGridLines(t *testing.T) {
	line := paragraphLine{ascent: 12, descent: 2, gap: 7}
	for _, test := range []struct {
		name        string
		paragraph   document.WordParagraphLayout
		pitch, want float64
	}{
		{"natural", document.WordParagraphLayout{}, 0, 21},
		{"grid", document.WordParagraphLayout{}, 15.6, 31.2},
		{"exact", document.WordParagraphLayout{LineSpacingRule: "exact", LineSpacingPT: 18}, 15.6, 18},
		{"opt-out", document.WordParagraphLayout{Set: document.WordParagraphSnapToGridSet, SnapToGrid: false}, 15.6, 21},
		{"multiple", document.WordParagraphLayout{LineSpacingRule: "auto", LineSpacingMultiple: 1.5}, 15.6, 46.8},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := lineHeight(line, test.paragraph, test.pitch)
			if math.Abs(got-test.want) > 1e-6 {
				t.Fatalf("line height=%g; want %g", got, test.want)
			}
		})
	}
}

func TestWrappedContinuationUsesWidthWithoutFirstLineIndent(t *testing.T) {
	atoms, err := shapeTextAtoms(context.Background(), layoutFakeShaper{}, make(map[glyphPathCacheKey][]GlyphPathCommand), "body", "一二三四五六七八九十一二三四五六七八九", 0, document.WordTextStyle{FontSize: 14}, false, "body")
	if err != nil {
		t.Fatal(err)
	}
	lines := wrapParagraphAtoms(atoms, 100, document.WordParagraphLayout{FirstLineIndentEMU: 28 * 12700}, 36)
	if len(lines) != 3 {
		t.Fatalf("wrapped lines=%d; want first line 5 characters and two continuation lines of 7", len(lines))
	}
	for i, want := range []int{5, 7, 7} {
		if len(lines[i].atoms) != want {
			t.Fatalf("line %d characters=%d; want %d", i, len(lines[i].atoms), want)
		}
	}
}

func TestEmptyParagraphGridUsesParagraphMarkSize(t *testing.T) {
	atoms, err := shapeTextAtoms(context.Background(), gridMetricsShaper{}, make(map[glyphPathCacheKey][]GlyphPathCommand), "empty", "", 0, document.WordTextStyle{FontSize: 14}, false, "empty")
	if err != nil {
		t.Fatal(err)
	}
	lines := wrapParagraphAtoms(atoms, 100, document.WordParagraphLayout{}, 36, 15.6)
	if len(lines) != 1 || math.Abs(lines[0].height-15.6) > 1e-6 {
		t.Fatalf("empty paragraph lines=%+v; want one 15.6pt grid line from the paragraph mark", lines)
	}
}

func TestDocumentGridPaginatesBodyWithoutEditorPadding(t *testing.T) {
	blocks := []document.WordBlock{}
	for i := 0; i < 5; i++ {
		id := string(rune('a' + i))
		blocks = append(blocks, document.WordBlock{ID: id, Kind: document.WordParagraphBlock, Paragraph: &document.WordParagraph{ID: id, Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: id + ".r", Text: "正文", Style: document.WordTextStyle{FontSize: 14}}}}}})
	}
	input := Input{DocumentID: "grid", Blocks: blocks, DefaultPageSetup: document.WordPageSetup{WidthPT: 200, HeightPT: 150, MarginTopPT: 10, MarginBottomPT: 10, MarginLeftPT: 10, MarginRightPT: 10}, Layout: document.WordLayoutData{Sections: []document.WordSection{{Grid: document.WordDocumentGrid{Type: "lines", LinePitchPT: 15.6}}}}}
	result, err := NewEngine(gridMetricsShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pages) != 2 {
		t.Fatalf("pages=%d; want 2 with real line metrics and document grid", len(result.Pages))
	}
	position := result.Locations[SourcePosition{RunID: "e.r", RuneOffset: 0}]
	if position.PageIndex != 1 {
		t.Fatalf("last body paragraph is on page %d; want second page", position.PageIndex+1)
	}
	input.Previous = &result
	input.Layout.Sections[0].Grid.LinePitchPT = 30
	reflowed, err := NewEngine(gridMetricsShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if reflowed.Locations[SourcePosition{RunID: "b.r", RuneOffset: 0}].YPT == result.Locations[SourcePosition{RunID: "b.r", RuneOffset: 0}].YPT {
		t.Fatal("changing the section grid reused obsolete line geometry")
	}
}
