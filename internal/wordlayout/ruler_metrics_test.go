package wordlayout

import (
	"context"
	"testing"

	"glance/internal/document"
)

func TestRulerMetricsResolveCharacterUnitsAndSurviveIncrementalLayout(t *testing.T) {
	input := characterIndentTestInput()
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	id := input.Blocks[0].Paragraph.ID
	metric, ok := result.ParagraphIndents[id]
	if !ok || metric.FirstLineIndentEMU != 24*12700 || metric.FirstLineIndentChars != 0 {
		t.Fatalf("resolved ruler metrics %+v present=%v", metric, ok)
	}
	cloned := cloneLayoutResult(result)
	changed := cloned.ParagraphIndents[id]
	changed.FirstLineIndentEMU = 0
	cloned.ParagraphIndents[id] = changed
	if result.ParagraphIndents[id].FirstLineIndentEMU != 24*12700 {
		t.Fatal("published metric map aliases mutable clone")
	}
	input.Previous = &result
	input.Revision++
	input.Blocks = append(input.Blocks, paragraphBlock("next", "more text", document.WordTextStyle{}))
	updated, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ParagraphIndents[id].FirstLineIndentEMU != metric.FirstLineIndentEMU {
		t.Fatal("ruler metrics changed on later paragraph append")
	}
}
