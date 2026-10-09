package wordlayout

import (
	"context"
	"glance/internal/document"
	"math"
	"testing"
)

func TestLayoutMapsRunRuneOffsetsToPage(t *testing.T) {
	input := layoutTestInput(100, 70, 5, paragraphBlock("p0", "Hello", document.WordTextStyle{FontSize: 10}))
	input.DocumentID = "doc-1"
	input.Revision = 7
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	start, ok0 := result.Locations[SourcePosition{RunID: "p0.run", RuneOffset: 0}]
	middle, ok1 := result.Locations[SourcePosition{RunID: "p0.run", RuneOffset: 1}]
	end, ok5 := result.Locations[SourcePosition{RunID: "p0.run", RuneOffset: 5}]
	if !ok0 || !ok1 || !ok5 || start.PageIndex != 0 || middle.PageIndex != 0 || end.PageIndex != 0 {
		t.Fatalf("source positions = %+v/%+v/%+v, found=%v%v%v", start, middle, end, ok0, ok1, ok5)
	}
	if start.XPT >= middle.XPT || middle.XPT >= end.XPT || math.IsNaN(end.YPT) || math.IsInf(end.YPT, 0) {
		t.Fatalf("rune locations do not advance monotonically: %+v/%+v/%+v", start, middle, end)
	}
}

func TestLayoutMapsEmptyRunCaretToPage(t *testing.T) {
	paragraph := document.WordParagraph{ID: "p0", Alignment: document.WordAlignDefault, Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "empty.run", Text: "", Editable: true}}}}
	input := layoutTestInput(100, 70, 5, document.WordBlock{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &paragraph})
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	position, ok := result.Locations[SourcePosition{RunID: "empty.run", RuneOffset: 0}]
	if !ok || position.PageIndex != 0 || position.XPT != 5 {
		t.Fatalf("empty-run caret location = %+v found=%v; want page 0 at the content start", position, ok)
	}
}

func TestLayoutReportsParagraphLineThatExceedsIndentedWidth(t *testing.T) {
	paragraph := document.WordParagraph{
		ID:        "p0",
		Alignment: document.WordAlignDefault,
		Layout: document.WordParagraphLayout{
			RightIndentEMU: 50800,
			Set:            document.WordParagraphRightIndentSet,
		},
		Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{
			ID: "r0", Text: "中", Style: document.WordTextStyle{FontSize: 10}, Editable: true,
		}}},
	}
	input := layoutTestInput(22, 70, 5, document.WordBlock{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &paragraph})
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.CanPreview {
		t.Fatal("a glyph wider than the remaining indented line width must block output")
	}
	found := false
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.SourceID == "p0" && diagnostic.Code == "paragraph-line-overflow" {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %+v; want a located paragraph-line-overflow error", result.Diagnostics)
	}
}

func TestLayoutReusesScaledGlyphOutlineWithinDocument(t *testing.T) {
	shaper := &countingLayoutShaper{}
	input := layoutTestInput(120, 100, 5,
		paragraphBlock("p0", "AAAA", document.WordTextStyle{FontSize: 10}),
		paragraphBlock("p1", "AA", document.WordTextStyle{FontSize: 10}),
	)
	_, err := NewEngine(shaper).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if shaper.outlineCalls != 1 {
		t.Fatalf("outline lookups = %d; want one per font/glyph/size for this layout", shaper.outlineCalls)
	}
}

func TestLayoutCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewEngine(layoutFakeShaper{}).Layout(ctx, layoutTestInput(100, 70, 5, paragraphBlock("p0", "text", document.WordTextStyle{FontSize: 10})))
	if err == nil {
		t.Fatal("Layout succeeded after context cancellation")
	}
}
