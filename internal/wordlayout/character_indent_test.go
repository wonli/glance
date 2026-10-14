package wordlayout

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"testing"

	"glance/internal/document"
)

func TestCharacterIndentUsesShapedIdeographicSpaceWidth(t *testing.T) {
	input := characterIndentTestInput()
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.CanPreview || len(result.Pages) == 0 {
		t.Fatalf("page preview blocked: %+v", result.Diagnostics)
	}
	starts := paragraphLineStarts(result, "p0.run")
	if len(starts) < 2 {
		t.Fatalf("paragraph did not wrap: %v", starts)
	}
	if got := starts[0].x - starts[1].x; math.Abs(got-24) > 0.01 {
		t.Fatalf("first-line indent = %.2fpt; want two shaped 12pt ideographic spaces", got)
	}
	if hasDiagnostic(result, "character-indent-width-fallback") {
		t.Fatalf("shaped ideographic-space metric unexpectedly fell back: %+v", result.Diagnostics)
	}
}

func TestCharacterIndentFallsBackToFontSizeWithWarning(t *testing.T) {
	input := characterIndentTestInput()
	result, err := NewEngine(noIdeographicSpaceShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.CanPreview || len(result.Pages) == 0 {
		t.Fatalf("font-size fallback blocked page preview: %+v", result.Diagnostics)
	}
	if !hasDiagnostic(result, "character-indent-width-fallback") {
		t.Fatalf("diagnostics = %+v; want character-width fallback warning", result.Diagnostics)
	}
	starts := paragraphLineStarts(result, "p0.run")
	if len(starts) < 2 {
		t.Fatalf("paragraph did not wrap: %v", starts)
	}
	if got := starts[0].x - starts[1].x; math.Abs(got-24) > 0.01 {
		t.Fatalf("fallback first-line indent = %.2fpt; want two 12pt font-size units", got)
	}
}

func TestEMUFirstLineIndentShiftsOnlyFirstLine(t *testing.T) {
	input := layoutTestInput(120, 200, 10, paragraphBlock("p0", strings.Repeat("A", 40), document.WordTextStyle{}))
	input.Blocks[0].Paragraph.Layout = document.WordParagraphLayout{
		FirstLineIndentEMU: 24 * 12700,
		Set:                document.WordParagraphFirstLineIndentSet,
	}
	input.Layout.Defaults.Text = document.WordTextStyle{FontFamily: "Test Sans", FontSize: 12}
	input.Layout.Defaults.TextSet = document.WordTextFontFamilySet | document.WordTextFontSizeSet
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	starts := paragraphLineStarts(result, "p0.run")
	if len(starts) < 2 {
		t.Fatalf("paragraph did not wrap: %v", starts)
	}
	if got := starts[0].x - starts[1].x; math.Abs(got-24) > 0.01 {
		t.Fatalf("first-line indent = %.2fpt; want 24pt", got)
	}
}

func characterIndentTestInput() Input {
	input := layoutTestInput(120, 200, 10, paragraphBlock("p0", strings.Repeat("A", 40), document.WordTextStyle{}))
	paragraph := input.Blocks[0].Paragraph
	paragraph.Layout = document.WordParagraphLayout{
		FirstLineIndentChars: 200,
		Set:                  document.WordParagraphFirstLineIndentSet | document.WordParagraphFirstLineIndentCharsSet,
	}
	input.Layout.Defaults.Text = document.WordTextStyle{FontFamily: "Test Sans", FontSize: 12}
	input.Layout.Defaults.TextSet = document.WordTextFontFamilySet | document.WordTextFontSizeSet
	return input
}

func paragraphLineStarts(result Result, runID string) []struct{ baseline, x float64 } {
	byBaseline := make(map[float64]float64)
	for _, page := range result.Pages {
		for _, fragment := range page.Text {
			if fragment.RunID != runID || len(fragment.Glyphs) == 0 {
				continue
			}
			if current, ok := byBaseline[fragment.BaselineYPT]; !ok || fragment.XPT < current {
				byBaseline[fragment.BaselineYPT] = fragment.XPT
			}
		}
	}
	lines := make([]struct{ baseline, x float64 }, 0, len(byBaseline))
	for baseline, x := range byBaseline {
		lines = append(lines, struct{ baseline, x float64 }{baseline, x})
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].baseline < lines[j].baseline })
	return lines
}

func hasDiagnostic(result Result, code string) bool {
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

type noIdeographicSpaceShaper struct{}

func (noIdeographicSpaceShaper) Shape(value string, style document.WordTextStyle) (ShapedText, error) {
	if value == "　" {
		return ShapedText{}, errors.New("test font does not shape an ideographic space")
	}
	return layoutFakeShaper{}.Shape(value, style)
}

func (noIdeographicSpaceShaper) FontData(fontKey string) ([]byte, error) {
	return layoutFakeShaper{}.FontData(fontKey)
}

func (noIdeographicSpaceShaper) GlyphOutline(fontKey string, glyphID uint32) (GlyphOutline, error) {
	return layoutFakeShaper{}.GlyphOutline(fontKey, glyphID)
}
