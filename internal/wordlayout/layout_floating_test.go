package wordlayout

import (
	"context"
	"math"
	"reflect"
	"testing"

	"glance/internal/document"
)

func testTightFloat(id string, x, y, width, height float64) document.WordInline {
	return document.WordInline{Kind: document.WordImageInline, Image: &document.WordImage{
		ID: id, WidthEMU: int64(width * emuPerPoint), HeightEMU: int64(height * emuPerPoint), Resolved: true,
		Floating: &document.WordImageFloating{
			XOffsetEMU: int64(x * emuPerPoint), YOffsetEMU: int64(y * emuPerPoint), WrapType: "tight",
			WrapPolygon: []document.WordImageWrapPoint{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}},
		},
	}}
}

func TestLayoutTightFloatWrapsAnchorAndFollowingParagraphs(t *testing.T) {
	anchor := document.WordParagraph{ID: "anchor", Inlines: []document.WordInline{
		testTightFloat("float", 80, 0, 40, 30),
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "anchor.run", Text: "abcdefghijklmnopqrst", Style: document.WordTextStyle{FontSize: 10}}},
	}}
	next := paragraphBlock("next", "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789", document.WordTextStyle{FontSize: 10})
	input := layoutTestInput(200, 120, 10,
		document.WordBlock{ID: "anchor", Kind: document.WordParagraphBlock, Paragraph: &anchor},
		next,
	)
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pages) != 1 || len(result.Pages[0].Images) != 1 {
		t.Fatalf("float layout = %d pages, images %+v; want one page and one image", len(result.Pages), result.Pages)
	}
	image := result.Pages[0].Images[0]
	if image.ImageID != "float" || image.Bounds != (Rect{XPT: 90, YPT: 10, WidthPT: 40, HeightPT: 30}) {
		t.Fatalf("float placement = %+v; want the text-relative 90,10 40×30pt bounds", image)
	}
	var firstNext, firstRightSide, firstFullWidth *TextFragment
	for index := range result.Pages[0].Text {
		fragment := &result.Pages[0].Text[index]
		if fragment.RunID != "next.run" {
			continue
		}
		if fragment.RuneStart == 0 {
			firstNext = fragment
		}
		if fragment.RuneStart == 16 {
			firstRightSide = fragment
		}
		if fragment.RuneStart == 56 {
			firstFullWidth = fragment
		}
	}
	if firstNext == nil || math.Abs(firstNext.XPT-10) > 0.01 || firstNext.BaselineYPT < 28 || firstNext.BaselineYPT > 29 {
		t.Fatalf("following paragraph's first glyph = %+v; want the left interval beside the float", firstNext)
	}
	if firstRightSide == nil || math.Abs(firstRightSide.XPT-130) > 0.01 || math.Abs(firstRightSide.BaselineYPT-firstNext.BaselineYPT) > epsilonPT {
		t.Fatalf("following paragraph's right-side glyph = %+v; want the same baseline to continue in the right interval", firstRightSide)
	}
	if firstFullWidth == nil || math.Abs(firstFullWidth.XPT-10) > 0.01 || firstFullWidth.BaselineYPT < 48 || firstFullWidth.BaselineYPT > 49 {
		t.Fatalf("following paragraph's post-float glyph = %+v; want full-width flow below the image", firstFullWidth)
	}
}

func TestLayoutTightFloatFlowsOneBaselineAcrossBothIntervals(t *testing.T) {
	anchor := document.WordParagraph{ID: "anchor", Inlines: []document.WordInline{
		testTightFloat("float", 70, 0, 40, 30),
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "anchor.run", Text: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz", Style: document.WordTextStyle{FontSize: 10}}},
	}}
	input := layoutTestInput(200, 100, 10, document.WordBlock{ID: "anchor", Kind: document.WordParagraphBlock, Paragraph: &anchor})
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pages) != 1 {
		t.Fatalf("page count = %d; want one page", len(result.Pages))
	}
	var firstLeft, firstRight *TextFragment
	for index := range result.Pages[0].Text {
		fragment := &result.Pages[0].Text[index]
		if fragment.RunID != "anchor.run" {
			continue
		}
		if fragment.RuneStart == 0 {
			firstLeft = fragment
		}
		if fragment.XPT >= 120-epsilonPT && (firstLeft == nil || math.Abs(fragment.BaselineYPT-firstLeft.BaselineYPT) <= epsilonPT) {
			firstRight = fragment
		}
	}
	if firstLeft == nil || math.Abs(firstLeft.XPT-10) > epsilonPT {
		t.Fatalf("first left-side fragment = %+v; want it at the left text edge", firstLeft)
	}
	if firstRight == nil {
		t.Fatalf("text fragments = %+v; want the same baseline to continue in the right interval beside the float", result.Pages[0].Text)
	}
}

func TestFloatAwareFlowKeepsInlineImageThatFillsTheBodyWidth(t *testing.T) {
	anchor := document.WordParagraph{ID: "anchor", Inlines: []document.WordInline{testTightFloat("float", 30, 0, 20, 10)}}
	fullWidthImage := document.WordInline{Kind: document.WordImageInline, Image: &document.WordImage{
		ID: "full-width", WidthEMU: 80 * int64(emuPerPoint), HeightEMU: 12 * int64(emuPerPoint), Resolved: true,
	}}
	imageParagraph := document.WordParagraph{ID: "image-paragraph", Inlines: []document.WordInline{fullWidthImage}}
	input := layoutTestInput(90, 80, 5,
		document.WordBlock{ID: "anchor", Kind: document.WordParagraphBlock, Paragraph: &anchor},
		document.WordBlock{ID: "image-paragraph", Kind: document.WordParagraphBlock, Paragraph: &imageParagraph},
	)
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range result.Pages {
		for _, placement := range page.Images {
			if placement.ImageID == "full-width" {
				return
			}
		}
	}
	t.Fatalf("page images = %+v, diagnostics = %+v; want the full-width inline image after the float", result.Pages, result.Diagnostics)
}

func TestFloatAwareBaselineReservesCombinedAscentAndDescent(t *testing.T) {
	anchor := document.WordParagraph{ID: "anchor", Inlines: []document.WordInline{
		testTightFloat("float", 20, 0, 20, 50),
		{Kind: document.WordImageInline, Image: &document.WordImage{
			ID: "tall-inline", WidthEMU: 20 * int64(emuPerPoint), HeightEMU: 20 * int64(emuPerPoint), Resolved: true,
		}},
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "anchor.run", Text: "abcdefghij", Style: document.WordTextStyle{FontSize: 10}}},
	}}
	input := layoutTestInput(90, 120, 5, document.WordBlock{ID: "anchor", Kind: document.WordParagraphBlock, Paragraph: &anchor})
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	var baselines []float64
	for _, fragment := range result.Pages[0].Text {
		if fragment.RunID != "anchor.run" {
			continue
		}
		if len(baselines) == 0 || math.Abs(fragment.BaselineYPT-baselines[len(baselines)-1]) > epsilonPT {
			baselines = append(baselines, fragment.BaselineYPT)
		}
	}
	if len(baselines) < 2 {
		t.Fatalf("text baselines = %v; want at least two lines around the float", baselines)
	}
	if got := baselines[1] - baselines[0]; got < 10-epsilonPT {
		t.Fatalf("baseline gap = %.2fpt; want 10pt to include the tall image ascent and text descent", got)
	}
}

func TestLayoutTightFloatMovesTextBelowWhenNoLineIntervalFits(t *testing.T) {
	anchor := document.WordParagraph{ID: "anchor", Inlines: []document.WordInline{
		testTightFloat("float", 0, 15, 80, 30),
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "anchor.run", Text: "A", Style: document.WordTextStyle{FontSize: 10}}},
	}}
	next := paragraphBlock("next", "B", document.WordTextStyle{FontSize: 10})
	input := layoutTestInput(90, 100, 5,
		document.WordBlock{ID: "anchor", Kind: document.WordParagraphBlock, Paragraph: &anchor},
		next,
	)
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pages) != 1 || len(result.Pages[0].Images) != 1 || result.Pages[0].Images[0].Bounds.YPT != 20 {
		t.Fatalf("float placement = %+v; want one image starting 15pt below the anchor paragraph", result.Pages)
	}
	for _, fragment := range result.Pages[0].Text {
		if fragment.RunID == "next.run" && fragment.BaselineYPT < 58 {
			t.Fatalf("text entered a fully excluded line band: %+v", fragment)
		}
	}
	if len(result.Pages[0].Text) < 2 {
		t.Fatalf("page text = %+v; want both anchor and following text", result.Pages[0].Text)
	}
}

func TestFloatingImageAfterPageBreakFallsBackInSourceOrder(t *testing.T) {
	anchor := document.WordParagraph{ID: "anchor", Inlines: []document.WordInline{
		{Kind: document.WordPageBreakInline, PageBreak: &document.WordPageBreak{ID: "page-break"}},
		testTightFloat("float", 20, 0, 30, 20),
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "anchor.run", Text: "after break", Style: document.WordTextStyle{FontSize: 10}}},
	}}
	input := layoutTestInput(120, 100, 10,
		paragraphBlock("prefix", "before break", document.WordTextStyle{FontSize: 10}),
		document.WordBlock{ID: "anchor", Kind: document.WordParagraphBlock, Paragraph: &anchor},
	)
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatalf("layout with a leading page break: %v", err)
	}
	if len(result.Pages) != 2 || len(result.Pages[0].Images) != 0 || len(result.Pages[1].Images) != 1 || result.Pages[1].Images[0].ImageID != "float" {
		t.Fatalf("page count/images = %d / %+v; want the float on the single page after the break", len(result.Pages), result.Pages)
	}
	if len(result.Pages[1].Text) == 0 || result.Pages[1].Text[0].RunID != "anchor.run" {
		t.Fatalf("text after the leading page break = %+v; want it adjacent to the inline fallback on page 2", result.Pages[1].Text)
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.SourceID == "float" && diagnostic.Code == "floating-image-inline-order-fallback" && diagnostic.Severity == DiagnosticWarning {
			return
		}
	}
	t.Fatalf("diagnostics = %+v; want a warning that source order requires inline fallback", result.Diagnostics)
}

func TestFloatBearingIncrementalLayoutMatchesFullLayout(t *testing.T) {
	anchor := document.WordParagraph{ID: "anchor", Inlines: []document.WordInline{
		testTightFloat("float", 60, 0, 50, 55),
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "anchor.run", Text: "A", Style: document.WordTextStyle{FontSize: 10}}},
	}}
	blocks := []document.WordBlock{
		{ID: "anchor", Kind: document.WordParagraphBlock, Paragraph: &anchor},
		paragraphBlock("padding", "P", document.WordTextStyle{FontSize: 10}),
		paragraphBlock("flow", "abcdefghijklmnopqrstuvwxyzabcdefghijklmn", document.WordTextStyle{FontSize: 10}),
		paragraphBlock("tail", "tail", document.WordTextStyle{FontSize: 10}),
	}
	input := layoutTestInput(150, 160, 10, blocks...)
	input.DocumentID, input.Revision = "float-incremental", 1
	engine := NewEngine(layoutFakeShaper{})
	previous, err := engine.Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	input.Blocks[2].Paragraph.Inlines[0].Text.Text = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmn"
	input.Revision = 2
	input.Previous = &previous
	incremental, err := engine.Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	input.Previous = nil
	full, err := engine.Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(incremental.Pages, full.Pages) || !reflect.DeepEqual(incremental.Locations, full.Locations) {
		t.Fatal("float-bearing incremental result differs from a full reflow")
	}
	var float Rect
	for _, placement := range full.Pages[0].Images {
		if placement.ImageID == "float" {
			float = placement.Bounds
		}
	}
	if float.WidthPT == 0 {
		t.Fatal("full layout lost the anchored float")
	}
	for _, page := range full.Pages {
		for _, fragment := range page.Text {
			if fragment.RunID == "flow.run" && fragment.BaselineYPT-float.HeightPT/2 >= float.YPT && fragment.BaselineYPT-float.HeightPT/2 < float.YPT+float.HeightPT && fragment.XPT >= float.XPT-0.01 && fragment.XPT < float.XPT+float.WidthPT {
				t.Fatalf("flow text overlaps the active float: %+v against %+v", fragment, float)
			}
		}
	}
}

func TestFloatingImageFallbackInTableKeepsVisibleImageAndWarns(t *testing.T) {
	paragraph := document.WordParagraph{ID: "cell-p", Inlines: []document.WordInline{testTightFloat("float", 0, 0, 10, 10)}}
	table := document.WordTable{Rows: []document.WordTableRow{{Cells: []document.WordTableCell{{ID: "cell", Paragraphs: []document.WordParagraph{paragraph}}}}}}
	input := layoutTestInput(120, 100, 5, document.WordBlock{ID: "table", Kind: document.WordTableBlock, Table: &table})
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pages) == 0 || len(result.Pages[0].Images) != 1 || result.Pages[0].Images[0].ImageID != "float" {
		t.Fatalf("table float fallback pages = %+v; want the image kept inline", result.Pages)
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.SourceID == "float" && diagnostic.Code == "floating-image-table-fallback" && diagnostic.Severity == DiagnosticWarning {
			return
		}
	}
	t.Fatalf("diagnostics = %+v; want a located warning for table-cell fallback", result.Diagnostics)
}
