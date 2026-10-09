package wordlayout

import (
	"glance/internal/document"
	"strings"
)

type layoutFakeShaper struct{}

func (layoutFakeShaper) Shape(value string, style document.WordTextStyle) (ShapedText, error) {
	size := style.FontSize
	if size <= 0 {
		size = 10
	}
	glyphs := make([]ShapedGlyph, 0, len([]rune(value)))
	for index, r := range []rune(value) {
		advance := size * 0.5
		if r == ' ' || r == '\t' {
			advance = size * 0.3
		} else if r >= 0x2e80 {
			advance = size
		}
		glyphs = append(glyphs, ShapedGlyph{
			FontKey: "test-font", GlyphID: uint32(r), RuneStart: index, RuneEnd: index + 1,
			AdvanceXPT: advance,
		})
	}
	return ShapedText{Glyphs: glyphs, AscentPT: size * 0.8, DescentPT: size * 0.2}, nil
}

func (layoutFakeShaper) FontData(fontKey string) ([]byte, error) {
	if fontKey != "test-font" {
		return nil, ErrUnknownFont
	}
	return []byte("test-font-data"), nil
}

func (layoutFakeShaper) GlyphOutline(fontKey string, glyphID uint32) (GlyphOutline, error) {
	if fontKey != "test-font" {
		return GlyphOutline{}, ErrUnknownFont
	}
	return GlyphOutline{
		UnitsPerEm: 1000,
		Commands: []GlyphOutlineCommand{
			{Op: GlyphPathMoveTo, Points: [3]GlyphOutlinePoint{{X: 0, Y: 0}}},
			{Op: GlyphPathLineTo, Points: [3]GlyphOutlinePoint{{X: 500, Y: 0}}},
			{Op: GlyphPathLineTo, Points: [3]GlyphOutlinePoint{{X: 500, Y: 700}}},
			{Op: GlyphPathLineTo, Points: [3]GlyphOutlinePoint{{X: 0, Y: 700}}},
		},
	}, nil
}

func layoutTestInput(width, height, margin float64, blocks ...document.WordBlock) Input {
	return Input{
		Blocks: blocks,
		DefaultPageSetup: document.WordPageSetup{
			WidthPT: width, HeightPT: height,
			MarginTopPT: margin, MarginRightPT: margin,
			MarginBottomPT: margin, MarginLeftPT: margin,
		},
	}
}

func paragraphBlock(id, text string, style document.WordTextStyle) document.WordBlock {
	paragraph := document.WordParagraph{ID: id, Alignment: document.WordAlignDefault}
	if text != "" {
		paragraph.Inlines = []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: id + ".run", Text: text, Style: style, Editable: true}}}
	}
	return document.WordBlock{ID: id, Kind: document.WordParagraphBlock, Paragraph: &paragraph}
}

type countingShapeCalls struct{ calls int }

func (s *countingShapeCalls) Shape(value string, style document.WordTextStyle) (ShapedText, error) {
	s.calls++
	return layoutFakeShaper{}.Shape(value, style)
}

func (s *countingShapeCalls) FontData(fontKey string) ([]byte, error) {
	return layoutFakeShaper{}.FontData(fontKey)
}

func (s *countingShapeCalls) GlyphOutline(fontKey string, glyphID uint32) (GlyphOutline, error) {
	return layoutFakeShaper{}.GlyphOutline(fontKey, glyphID)
}

func layoutTableCell(id, text string, fontSize float64, span int, merge document.WordTableVerticalMerge) document.WordTableCell {
	cell := document.WordTableCell{ID: id, GridSpan: span, VerticalMerge: merge}
	if text != "" {
		cell.Paragraphs = []document.WordParagraph{{
			ID: id + ".p0", Alignment: document.WordAlignDefault,
			Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: id + ".run", Text: text, Style: document.WordTextStyle{FontSize: fontSize}, Editable: true}}},
		}}
	}
	return cell
}

// Page height 100pt, margins 10pt -> printable height is 80pt
// Image has height 200pt (2540000 EMU), width 100pt (1270000 EMU)

type countingLayoutShaper struct {
	outlineCalls int
}

func (s *countingLayoutShaper) Shape(value string, style document.WordTextStyle) (ShapedText, error) {
	return layoutFakeShaper{}.Shape(value, style)
}

func (s *countingLayoutShaper) FontData(fontKey string) ([]byte, error) {
	return layoutFakeShaper{}.FontData(fontKey)
}

func (s *countingLayoutShaper) GlyphOutline(fontKey string, glyphID uint32) (GlyphOutline, error) {
	s.outlineCalls++
	return layoutFakeShaper{}.GlyphOutline(fontKey, glyphID)
}

func joinPageText(page PageScene) string {
	var value strings.Builder
	for _, fragment := range page.Text {
		value.WriteString(fragment.Text)
	}
	return value.String()
}

// Page: width 200, height 100, margin 10 -> content width 180, height 80
// 2 columns with spacing 20 -> column width = (180 - 20) / 2 = 80 pt
// Col 0: X = 10..90
// Col 1: X = 110..190

// Page 0 should have ColZero and ColOne

// Verify p0 is in col 0 (around X=10) and p1 is in col 1 (around X=110)

// Page 1 should have PageOneColZero in Col 0 (around X=10)
