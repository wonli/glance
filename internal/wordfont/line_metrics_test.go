package wordfont

import (
	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/shaping"
	"glance/internal/document"
	"golang.org/x/image/math/fixed"
	"math"
	"testing"
)

func TestWordShaperPreservesFontLineGapAcrossFallbackSegments(t *testing.T) {
	s, err := newWordTextShaperWithCacheDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	style := document.WordTextStyle{FontFamily: "Times New Roman", FontSize: 14}
	got, err := s.Shape("A中B", style)
	if err != nil {
		t.Fatal(err)
	}
	input := shaping.Input{Text: []rune("A中B"), RunStart: 0, RunEnd: 3, Direction: di.DirectionLTR, Size: fixed.I(14)}
	fontMap := &wordTextFontMap{owner: s, candidates: s.orderedFonts(style)}
	var segmenter shaping.Segmenter
	var harfbuzz shaping.HarfbuzzShaper
	want := 0.0
	for _, segment := range segmenter.Split(input, fontMap) {
		out := harfbuzz.Shape(segment)
		want = math.Max(want, fixedToPoints(out.LineBounds.Gap))
	}
	if got.LineGapPT != want {
		t.Fatalf("shaper discarded the selected font's leading: gap=%g; want %g", got.LineGapPT, want)
	}
}
