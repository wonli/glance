package wordlayout

import (
	"math"
	"testing"

	"glance/internal/document"
)

type contractFakeTextShaper struct{}

func (contractFakeTextShaper) Shape(value string, style document.WordTextStyle) (ShapedText, error) {
	glyphs := make([]ShapedGlyph, 0, len([]rune(value)))
	for index := range []rune(value) {
		glyphs = append(glyphs, ShapedGlyph{
			FontKey:    "fake-regular",
			GlyphID:    uint32(index + 1),
			RuneStart:  index,
			RuneEnd:    index + 1,
			AdvanceXPT: 7,
		})
	}
	return ShapedText{Glyphs: glyphs, AscentPT: 9, DescentPT: 3}, nil
}

func (contractFakeTextShaper) FontData(fontKey string) ([]byte, error) {
	if fontKey != "fake-regular" {
		return nil, ErrUnknownFont
	}
	return []byte("fake font bytes"), nil
}

func TestTextShaperReturnsGlyphClustersAndMetrics(t *testing.T) {
	var shaper TextShaper = contractFakeTextShaper{}
	value := "A中ffi"
	shaped, err := shaper.Shape(value, document.WordTextStyle{FontFamily: "A test family", FontSize: 12})
	if err != nil {
		t.Fatal(err)
	}
	if shaped.AscentPT <= 0 || shaped.DescentPT < 0 || math.IsNaN(shaped.AscentPT) || math.IsNaN(shaped.DescentPT) || math.IsInf(shaped.AscentPT, 0) || math.IsInf(shaped.DescentPT, 0) {
		t.Fatalf("invalid text metrics: %+v", shaped)
	}
	runes := []rune(value)
	clusters := make(map[[2]int]struct{})
	previousStart := -1
	for index, glyph := range shaped.Glyphs {
		if glyph.FontKey == "" || glyph.GlyphID == 0 {
			t.Errorf("glyph %d is missing a font key or glyph ID: %+v", index, glyph)
		}
		if glyph.RuneStart < 0 || glyph.RuneEnd <= glyph.RuneStart || glyph.RuneEnd > len(runes) {
			t.Fatalf("glyph %d has invalid cluster range [%d,%d) for %d runes", index, glyph.RuneStart, glyph.RuneEnd, len(runes))
		}
		if glyph.RuneStart < previousStart {
			t.Fatalf("glyph clusters are not in logical order: previous start %d, current %+v", previousStart, glyph)
		}
		previousStart = glyph.RuneStart
		if glyph.AdvanceXPT < 0 || math.IsNaN(glyph.AdvanceXPT) || math.IsInf(glyph.AdvanceXPT, 0) || math.IsNaN(glyph.OffsetXPT) || math.IsInf(glyph.OffsetXPT, 0) || math.IsNaN(glyph.OffsetYPT) || math.IsInf(glyph.OffsetYPT, 0) {
			t.Fatalf("glyph %d has invalid point geometry: %+v", index, glyph)
		}
		clusters[[2]int{glyph.RuneStart, glyph.RuneEnd}] = struct{}{}
	}
	covered := make([]int, len(runes))
	for cluster := range clusters {
		for runeIndex := cluster[0]; runeIndex < cluster[1]; runeIndex++ {
			covered[runeIndex]++
		}
	}
	for index, count := range covered {
		if count != 1 {
			t.Errorf("input rune %d (%q) is covered %d times; want exactly once", index, runes[index], count)
		}
	}
	fontBytes, err := shaper.FontData(shaped.Glyphs[0].FontKey)
	if err != nil || len(fontBytes) == 0 {
		t.Fatalf("FontData(%q) = %d bytes, %v; want embedded font data", shaped.Glyphs[0].FontKey, len(fontBytes), err)
	}
}
