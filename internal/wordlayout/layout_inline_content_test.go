package wordlayout

import (
	"context"
	"glance/internal/document"
	"strings"
	"testing"
)

func TestLayoutListsInlineImagesAndPageBreaks(t *testing.T) {
	paragraph := document.WordParagraph{ID: "p0", ListKind: document.WordListBullet, ListMarker: "•", Alignment: document.WordAlignDefault}
	paragraph.Inlines = []document.WordInline{
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "r0", Text: "one", Style: document.WordTextStyle{FontSize: 10}, Editable: true}},
		{Kind: document.WordImageInline, Image: &document.WordImage{ID: "img0", WidthEMU: 127000, HeightEMU: 63500, Resolved: true}},
		{Kind: document.WordPageBreakInline, PageBreak: &document.WordPageBreak{ID: "break0"}},
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "r1", Text: "two", Style: document.WordTextStyle{FontSize: 10}, Editable: true}},
	}
	input := layoutTestInput(100, 70, 5, document.WordBlock{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &paragraph})
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pages) != 2 {
		t.Fatalf("page count = %d; want 2", len(result.Pages))
	}
	if len(result.Pages[0].Images) != 1 || result.Pages[0].Images[0].Bounds.WidthPT != 10 || result.Pages[0].Images[0].Bounds.HeightPT != 5 {
		t.Fatalf("inline image placement = %+v", result.Pages[0].Images)
	}
	foundMarker := false
	for _, fragment := range result.Pages[0].Text {
		foundMarker = foundMarker || (fragment.Synthetic && fragment.Text == "•")
	}
	if !foundMarker {
		t.Fatalf("list marker missing from page scene: %+v", result.Pages[0].Text)
	}
	if !strings.Contains(joinPageText(result.Pages[1]), "two") {
		t.Fatalf("content after explicit break is missing from page 2: %+v", result.Pages[1].Text)
	}
}

func TestFloatingImageFallbackKeepsAdjacentTextAndWarnsWithoutBlocking(t *testing.T) {
	paragraph := document.WordParagraph{ID: "p", Inlines: []document.WordInline{
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "before", Text: "before", Editable: true}},
		{Kind: document.WordImageInline, Image: &document.WordImage{ID: "floating", WidthEMU: 254000, HeightEMU: 254000, Resolved: true, InlineFallback: true}},
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "after", Text: "after", Editable: true}},
	}}
	input := layoutTestInput(120, 100, 10, document.WordBlock{ID: "p", Kind: document.WordParagraphBlock, Paragraph: &paragraph})
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.CanPreview {
		t.Fatalf("inline image fallback blocked preview: %+v", result.Diagnostics)
	}
	if got := joinPageText(result.Pages[0]); !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Fatalf("text around image fallback = %q; want both adjacent runs", got)
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.SourceID == "floating" && diagnostic.Code == "floating-image-inline-fallback" && diagnostic.Severity == DiagnosticWarning {
			return
		}
	}
	t.Fatalf("diagnostics = %+v; want a located warning for inline fallback", result.Diagnostics)
}

func TestLayoutScalesInlineImageToIndentedTextWidth(t *testing.T) {
	paragraph := document.WordParagraph{
		ID:        "p0",
		Alignment: document.WordAlignDefault,
		Layout: document.WordParagraphLayout{
			LeftIndentEMU: 254000, RightIndentEMU: 254000,
			Set: document.WordParagraphLeftIndentSet | document.WordParagraphRightIndentSet,
		},
		Inlines: []document.WordInline{{Kind: document.WordImageInline, Image: &document.WordImage{
			ID: "img0", WidthEMU: 1016000, HeightEMU: 508000, Resolved: true,
		}}},
	}
	input := layoutTestInput(100, 80, 5, document.WordBlock{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &paragraph})
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pages[0].Images) != 1 {
		t.Fatalf("images = %+v; want one inline image", result.Pages[0].Images)
	}
	image := result.Pages[0].Images[0]
	if image.Bounds.WidthPT != 50 || image.Bounds.HeightPT != 25 || image.Bounds.XPT < 25 {
		t.Fatalf("indented inline image bounds = %+v; want 50x25pt inside the 50pt text area", image.Bounds)
	}
}

func TestLayoutScalesInlineImageToPrintablePageHeight(t *testing.T) {

	paragraph := document.WordParagraph{
		ID:        "p0",
		Alignment: document.WordAlignDefault,
		Inlines: []document.WordInline{{Kind: document.WordImageInline, Image: &document.WordImage{
			ID: "img0", WidthEMU: 1270000, HeightEMU: 2540000, Resolved: true,
		}}},
	}
	input := layoutTestInput(120, 100, 10, document.WordBlock{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &paragraph})
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	for _, diag := range result.Diagnostics {
		if diag.Severity == DiagnosticError {
			t.Fatalf("unexpected layout error diagnostic: %+v", diag)
		}
	}
	if len(result.Pages) == 0 || len(result.Pages[0].Images) != 1 {
		t.Fatalf("pages/images = %+v; want one placed image", result.Pages)
	}
	image := result.Pages[0].Images[0]
	if image.Bounds.HeightPT > 80 {
		t.Fatalf("image height = %.2f; should not exceed printable page height 80pt", image.Bounds.HeightPT)
	}
}

func TestLayoutExactPageBoundaryThenExplicitBreakHasNoExtraPage(t *testing.T) {
	first := paragraphBlock("p0", "1234", document.WordTextStyle{FontSize: 10})
	second := document.WordParagraph{ID: "p1", Alignment: document.WordAlignDefault, Inlines: []document.WordInline{
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "r1", Text: "5678", Style: document.WordTextStyle{FontSize: 10}, Editable: true}},
		{Kind: document.WordPageBreakInline, PageBreak: &document.WordPageBreak{ID: "break0"}},
	}}
	third := paragraphBlock("p2", "next", document.WordTextStyle{FontSize: 10})
	input := layoutTestInput(100, 30, 5,
		first,
		document.WordBlock{ID: "p1", Kind: document.WordParagraphBlock, Paragraph: &second},
		third,
	)
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pages) != 2 {
		t.Fatalf("page count = %d; want 2 without an empty page after exact-boundary content", len(result.Pages))
	}
	if strings.Contains(joinPageText(result.Pages[0]), "next") || !strings.Contains(joinPageText(result.Pages[1]), "next") {
		t.Fatalf("explicit page break placement = page0 %q page1 %q", joinPageText(result.Pages[0]), joinPageText(result.Pages[1]))
	}
	if len(result.Pages[0].Text) == 0 || len(result.Pages[1].Text) == 0 {
		t.Fatalf("unexpected blank page: %+v", result.Pages)
	}
}

func TestMultiColumnLayout(t *testing.T) {

	input := layoutTestInput(200, 100, 10,
		paragraphBlock("p0", "ColZero", document.WordTextStyle{FontSize: 40}),
		paragraphBlock("p1", "ColOne", document.WordTextStyle{FontSize: 40}),
		paragraphBlock("p2", "PageOneColZero", document.WordTextStyle{FontSize: 40}),
	)
	input.Layout.Sections = []document.WordSection{{
		StartBlock: 0,
		PageSetup: document.WordPageSetup{
			WidthPT:         200,
			HeightPT:        100,
			MarginTopPT:     10,
			MarginBottomPT:  10,
			MarginLeftPT:    10,
			MarginRightPT:   10,
			Columns:         2,
			ColumnSpacingPT: 20,
			Set:             document.WordPageWidthSet | document.WordPageHeightSet | document.WordPageMarginTopSet | document.WordPageMarginBottomSet | document.WordPageMarginLeftSet | document.WordPageMarginRightSet | document.WordPageColumnsSet | document.WordPageColumnSpacingSet,
		},
	}}

	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.CanPreview {
		t.Fatalf("CanPreview = false, diagnostics: %+v", result.Diagnostics)
	}
	if len(result.Pages) != 2 {
		t.Fatalf("page count = %d, want 2", len(result.Pages))
	}

	page0 := result.Pages[0]
	if len(page0.Text) < 2 {
		t.Fatalf("page 0 text count = %d, want at least 2", len(page0.Text))
	}
	// Verify p0 is in col 0 (around X=10) and p1 is in col 1 (around X=110)
	var p0X, p1X float64
	p0Found, p1Found := false, false
	for _, f := range page0.Text {
		if f.RunID == "p0.run" && !p0Found {
			p0X = f.XPT
			p0Found = true
		}
		if f.RunID == "p1.run" && !p1Found {
			p1X = f.XPT
			p1Found = true
		}
	}
	if !p0Found || p0X < 10 || p0X > 30 {
		t.Errorf("p0 X = %f (found=%v), want ~10", p0X, p0Found)
	}
	if !p1Found || p1X < 100 || p1X > 130 {
		t.Errorf("p1 X = %f (found=%v), want ~110", p1X, p1Found)
	}

	page1 := result.Pages[1]
	var p2X float64
	p2Found := false
	for _, f := range page1.Text {
		if f.RunID == "p2.run" && !p2Found {
			p2X = f.XPT
			p2Found = true
		}
	}
	if !p2Found || p2X < 10 || p2X > 30 {
		t.Errorf("p2 X = %f (found=%v), want ~10 on page 1", p2X, p2Found)
	}
}
