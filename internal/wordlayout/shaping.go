package wordlayout

import (
	"errors"
	"math"

	"glance/internal/document"
)

var ErrUnknownFont = errors.New("word layout font key is unknown")

// TextShaper returns actual OpenType glyph identities and point-based metrics.
// Glyph font keys let output backends embed exactly the selected fallback face.
type TextShaper interface {
	Shape(text string, style document.WordTextStyle) (ShapedText, error)
	FontData(fontKey string) ([]byte, error)
}

// FontSource is the narrow font-data capability used by output adapters.
type FontSource interface {
	FontData(fontKey string) ([]byte, error)
}

// ShapedText stores visual-order glyphs and the maximum line metrics in points.
type ShapedText struct {
	Glyphs    []ShapedGlyph
	AscentPT  float64
	DescentPT float64
}

// ShapedGlyph is one glyph positioned relative to the previous glyph's pen.
// Rune ranges are logical, zero-based half-open offsets into the input string.
type ShapedGlyph struct {
	FontKey    string
	GlyphID    uint32
	RuneStart  int
	RuneEnd    int
	AdvanceXPT float64
	OffsetXPT  float64
	OffsetYPT  float64
}

// ValidateShapedText checks the geometry invariants shared by layout and output.
// Multiple glyphs may share the same rune range when they form one cluster.
func ValidateShapedText(input string, shaped ShapedText) error {
	if math.IsNaN(shaped.AscentPT) || math.IsInf(shaped.AscentPT, 0) || shaped.AscentPT < 0 {
		return errors.New("shaped text has invalid ascent")
	}
	if math.IsNaN(shaped.DescentPT) || math.IsInf(shaped.DescentPT, 0) || shaped.DescentPT < 0 {
		return errors.New("shaped text has invalid descent")
	}
	runes := []rune(input)
	clusters := make(map[[2]int]struct{})
	for _, glyph := range shaped.Glyphs {
		if glyph.FontKey == "" {
			return errors.New("shaped glyph has no font key")
		}
		if glyph.RuneStart < 0 || glyph.RuneEnd <= glyph.RuneStart || glyph.RuneEnd > len(runes) {
			return errors.New("shaped glyph has an invalid rune range")
		}
		if !finite(glyph.AdvanceXPT) || !finite(glyph.OffsetXPT) || !finite(glyph.OffsetYPT) {
			return errors.New("shaped glyph has invalid point geometry")
		}
		clusters[[2]int{glyph.RuneStart, glyph.RuneEnd}] = struct{}{}
	}
	covered := make([]int, len(runes))
	for cluster := range clusters {
		for index := cluster[0]; index < cluster[1]; index++ {
			covered[index]++
		}
	}
	for _, count := range covered {
		if count != 1 {
			return errors.New("shaped glyph clusters do not cover the input exactly once")
		}
	}
	if len(runes) > 0 && len(clusters) == 0 {
		return errors.New("shaped text has no glyph clusters")
	}
	return nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
