package wordlayout

import (
	"context"
	"glance/internal/document"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestExplicitTextNewlinesCreateLinesWithoutControlGlyphs(t *testing.T) {
	paragraph := &document.WordParagraph{ID: "p", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "r", Text: "first\n\nsecond\r\nthird"}}}}
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), Input{Blocks: []document.WordBlock{{ID: "p", Kind: document.WordParagraphBlock, Paragraph: paragraph}}})
	if err != nil {
		t.Fatal(err)
	}
	var fragments []TextFragment
	for _, page := range result.Pages {
		fragments = append(fragments, page.Text...)
	}
	var first, second, third float64
	for _, f := range fragments {
		if strings.ContainsAny(f.Text, "\r\n") {
			t.Fatalf("control glyph emitted: %q", f.Text)
		}
		switch f.RuneStart {
		case 0:
			first = f.BaselineYPT
		case 7:
			second = f.BaselineYPT
		case 15:
			third = f.BaselineYPT
		}
	}
	if first == 0 || second <= first || third <= second || second-first < 1.5*(third-second) {
		t.Fatalf("newlines or blank line lost: %v", fragments)
	}
}

func TestLatentBuiltInHeadingStylesRemainPreviewable(t *testing.T) {
	for _, test := range []struct {
		styleID string
		fontPT  float64
	}{
		{styleID: "Heading1", fontPT: 15},
		{styleID: "Heading2", fontPT: 13.5},
	} {
		t.Run(test.styleID, func(t *testing.T) {
			paragraph := document.WordParagraph{
				ID: "p0", Style: test.styleID,
				Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "r0", Text: "Heading text", Editable: true}}},
			}
			input := layoutTestInput(180, 120, 10, document.WordBlock{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &paragraph})
			input.Layout = document.WordLayoutData{Styles: []document.WordStyle{{ID: "Normal", DefaultParagraph: true}}}

			result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			if !result.CanPreview || len(result.Pages) == 0 {
				t.Fatalf("latent built-in style blocked layout: diagnostics=%+v", result.Diagnostics)
			}
			for _, diagnostic := range result.Diagnostics {
				if diagnostic.Code == "missing-style" {
					t.Fatalf("latent built-in style reported as missing: %+v", diagnostic)
				}
			}
			if len(result.Pages[0].Text) == 0 || !result.Pages[0].Text[0].Style.Bold || result.Pages[0].Text[0].Style.FontSize != test.fontPT {
				t.Fatalf("resolved %s text style = %+v; want bold at %.1fpt", test.styleID, result.Pages[0].Text, test.fontPT)
			}
		})
	}
}

func TestUnknownMissingParagraphStyleStillBlocksPreview(t *testing.T) {
	paragraph := document.WordParagraph{ID: "p0", Style: "ProjectCustomHeading", Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "r0", Text: "Text", Editable: true}}}}
	input := layoutTestInput(180, 120, 10, document.WordBlock{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &paragraph})
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.CanPreview {
		t.Fatal("unknown missing paragraph style must continue to block preview")
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.SourceID == "p0" && diagnostic.Code == "missing-style" {
			return
		}
	}
	t.Fatalf("diagnostics = %+v; want a located missing-style error", result.Diagnostics)
}

func TestLayoutPageSetupAndMargins(t *testing.T) {
	input := layoutTestInput(160, 120, 10, paragraphBlock("p0", "A", document.WordTextStyle{FontSize: 10}))
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pages) != 1 {
		t.Fatalf("page count = %d; want 1", len(result.Pages))
	}
	page := result.Pages[0]
	if page.PageSetup.WidthPT != 160 || page.PageSetup.HeightPT != 120 || page.Content.XPT != 10 || page.Content.YPT != 10 || page.Content.WidthPT != 140 || page.Content.HeightPT != 100 {
		t.Fatalf("page geometry = setup %+v content %+v", page.PageSetup, page.Content)
	}
	if page.Clip != (Rect{WidthPT: 160, HeightPT: 120}) {
		t.Fatalf("page clip = %+v; want the full physical page rectangle", page.Clip)
	}
	if len(page.Text) != 1 || page.Text[0].XPT != 10 || page.Text[0].BaselineYPT < 18 || page.Text[0].BaselineYPT > 21 {
		t.Fatalf("page text placement = %+v", page.Text)
	}
	if len(page.Text[0].Glyphs) != 1 || len(page.Text[0].Glyphs[0].Outline) != 4 || page.Text[0].Glyphs[0].Outline[2].Points[0].YPT != -7 {
		t.Fatalf("page scene glyph path = %+v; want scaled, baseline-relative vector outline", page.Text[0].Glyphs)
	}
}

func TestLayoutPageSetupUsesExplicitZeroMargin(t *testing.T) {
	input := layoutTestInput(160, 200, 10, paragraphBlock("p0", "A", document.WordTextStyle{FontSize: 10}))
	input.Layout.Sections = []document.WordSection{{StartBlock: 0, PageSetup: document.WordPageSetup{
		WidthPT: 100, HeightPT: 120, Orientation: "portrait", MarginTopPT: 0, MarginLeftPT: 12,
		Set: document.WordPageWidthSet | document.WordPageHeightSet | document.WordPageOrientationSet | document.WordPageMarginTopSet | document.WordPageMarginLeftSet,
	}}}
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	page := result.Pages[0]
	if page.PageSetup.WidthPT != 100 || page.PageSetup.HeightPT != 120 || page.Content.YPT != 0 || page.Content.XPT != 12 || page.Content.WidthPT != 78 || page.Content.HeightPT != 110 {
		t.Fatalf("explicit-zero page setup was replaced by defaults: setup=%+v content=%+v", page.PageSetup, page.Content)
	}
}

func TestLayoutReportsProductDefaultPageSetup(t *testing.T) {
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), Input{
		Blocks: []document.WordBlock{paragraphBlock("p0", "A", document.WordTextStyle{FontSize: 10})},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Pages[0].PageSetup.WidthPT != 595.28 || result.Pages[0].PageSetup.HeightPT != 841.89 || result.Pages[0].PageSetup.MarginTopPT != 72 {
		t.Fatalf("product-default page setup = %+v", result.Pages[0].PageSetup)
	}
	found := false
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Severity == DiagnosticWarning && diagnostic.SourceID == "section-0" && diagnostic.Code == "page-setup-defaulted" {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %+v; want a warning that product page defaults were used", result.Diagnostics)
	}
}

func TestLayoutParagraphRunFormattingAndMixedScripts(t *testing.T) {
	paragraph := document.WordParagraph{ID: "p0", Style: "Body", Alignment: document.WordAlignLeft, Layout: document.WordParagraphLayout{Set: document.WordParagraphAlignmentSet}}
	paragraph.Inlines = []document.WordInline{
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "r0", Text: "A中", Style: document.WordTextStyle{Bold: true, FontSize: 12}, StyleSet: document.WordTextBoldSet | document.WordTextFontSizeSet, Editable: true}},
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "r1", Text: "BC", Style: document.WordTextStyle{Italic: true}, StyleSet: document.WordTextItalicSet, Editable: true}},
	}
	input := layoutTestInput(40, 120, 5, document.WordBlock{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &paragraph})
	input.Layout = document.WordLayoutData{
		Defaults: document.WordStyleDefaults{Text: document.WordTextStyle{FontFamily: "Default Sans", FontSize: 10}, TextSet: document.WordTextFontFamilySet | document.WordTextFontSizeSet},
		Styles: []document.WordStyle{
			{ID: "Base", Text: document.WordTextStyle{FontColor: "224466"}, TextSet: document.WordTextFontColorSet},
			{ID: "Body", BasedOn: "Base", Paragraph: document.WordParagraphLayout{AfterPT: 2, Set: document.WordParagraphAfterSet}},
		},
	}
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	var got strings.Builder
	styles := map[string]document.WordTextStyle{}
	for _, fragment := range result.Pages[0].Text {
		if fragment.Synthetic {
			continue
		}
		got.WriteString(fragment.Text)
		styles[fragment.RunID] = fragment.Style
	}
	if got.String() != "A中BC" {
		t.Fatalf("mixed-script text fragments = %q; want original order", got.String())
	}
	if style := styles["r0"]; !style.Bold || style.FontSize != 12 || style.FontFamily != "Default Sans" || style.FontColor != "224466" {
		t.Fatalf("inherited/direct style for r0 = %+v", style)
	}
	if style := styles["r1"]; !style.Italic || style.Bold || style.FontSize != 10 || style.FontColor != "224466" {
		t.Fatalf("inherited/direct style for r1 = %+v", style)
	}
	if len(result.Pages[0].Text) < 2 {
		t.Fatalf("text fragments = %+v; want formatting split by run", result.Pages[0].Text)
	}
}

func TestLayoutRightTabStopAlignsFollowingTextWithoutBlockingPreview(t *testing.T) {
	paragraph := document.WordParagraph{
		ID: "p0", Alignment: document.WordAlignDefault,
		Layout: document.WordParagraphLayout{
			TabStops: []document.WordTabStop{{PositionPT: 50, Alignment: "right", Leader: "dot"}},
			Set:      document.WordParagraphTabStopsSet,
		},
		Inlines: []document.WordInline{
			{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "r0", Text: "A", Style: document.WordTextStyle{FontSize: 10}}},
			{Kind: document.WordTabInline, Tab: &document.WordTab{ID: "tab0"}},
			{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "r1", Text: "7", Style: document.WordTextStyle{FontSize: 10}}},
		},
	}
	input := layoutTestInput(120, 80, 5, document.WordBlock{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &paragraph})
	input.Layout.DefaultTabStopPT = 24
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.CanPreview || len(result.Pages) != 1 {
		t.Fatalf("tabbed paragraph blocked preview: %+v", result.Diagnostics)
	}
	if len(result.Pages[0].Text) != 2 || result.Pages[0].Text[1].Text != "7" || math.Abs(result.Pages[0].Text[1].XPT-50) > 0.01 {
		t.Fatalf("right tab placement = %+v; want page number at x=50pt", result.Pages[0].Text)
	}
	warningFound := false
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "tab-leader-omitted" && diagnostic.Severity == DiagnosticWarning {
			warningFound = true
		}
	}
	if !warningFound {
		t.Fatal("dotted tab leader should be reported as a non-blocking fidelity warning")
	}
}

func TestLayoutPositionedGlyphRuneOffsetsStayRelativeToTheirRun(t *testing.T) {
	input := layoutTestInput(240, 120, 10, paragraphBlock("offset", "abcdef", document.WordTextStyle{FontSize: 10}))
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pages) != 1 {
		t.Fatalf("layout page count = %d; want one text page", len(result.Pages))
	}
	if len(result.Pages[0].Text) == 0 {
		t.Fatal("layout page has no text fragments")
	}
	var glyphs []PositionedGlyph
	for _, fragment := range result.Pages[0].Text {
		glyphs = append(glyphs, fragment.Glyphs...)
	}
	if len(glyphs) != 6 {
		t.Fatalf("positioned glyph count = %d; want 6", len(glyphs))
	}
	for index, glyph := range glyphs {
		if glyph.RuneStart != index || glyph.RuneEnd != index+1 {
			t.Errorf("glyph %d rune range = [%d,%d); want [%d,%d)", index, glyph.RuneStart, glyph.RuneEnd, index, index+1)
		}
	}
}

func TestLayoutReflowsChangedParagraphAndReusesAlignedSuffix(t *testing.T) {
	blocks := make([]document.WordBlock, 700)
	for index := range blocks {
		blocks[index] = paragraphBlock("p"+itoa(index), "x", document.WordTextStyle{FontSize: 10})
	}
	baseInput := layoutTestInput(100, 70, 5, blocks...)
	baseInput.DocumentID = "incremental"
	baseInput.Revision = 1
	engine := NewEngine(layoutFakeShaper{})
	previous, err := engine.Layout(context.Background(), baseInput)
	if err != nil {
		t.Fatal(err)
	}
	if len(previous.Pages) < 100 {
		t.Fatalf("baseline page count = %d; need at least 100 pages to exercise the visible-page window", len(previous.Pages))
	}
	dirtyBlock := -1
	for index, checkpoint := range previous.blockCheckpoints[:len(blocks)] {
		if checkpoint.PageIndex == 4 {
			dirtyBlock = index
			break
		}
	}
	if dirtyBlock < 0 {
		t.Fatal("baseline layout has no paragraph starting on page 5")
	}

	updatedInput := baseInput
	updatedInput.Blocks = append([]document.WordBlock(nil), baseInput.Blocks...)
	updatedParagraph := *updatedInput.Blocks[dirtyBlock].Paragraph
	updatedParagraph.Inlines = append([]document.WordInline(nil), updatedParagraph.Inlines...)
	updatedRun := *updatedParagraph.Inlines[0].Text
	updatedRun.Text = strings.Repeat("x", 100)
	updatedParagraph.Inlines[0].Text = &updatedRun
	updatedInput.Blocks[dirtyBlock].Paragraph = &updatedParagraph
	updatedInput.Revision = 2
	updatedInput.Previous = &previous

	incrementalShaper := &countingShapeCalls{}
	incremental, err := NewEngine(incrementalShaper).Layout(context.Background(), updatedInput)
	if err != nil {
		t.Fatal(err)
	}
	fullInput := updatedInput
	fullInput.Previous = nil
	fullShaper := &countingShapeCalls{}
	full, err := NewEngine(fullShaper).Layout(context.Background(), fullInput)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(incremental.Pages, full.Pages) {
		for index := 0; index < max(len(incremental.Pages), len(full.Pages)); index++ {
			incrementalText, fullText := "", ""
			if index < len(incremental.Pages) {
				incrementalText = joinPageText(incremental.Pages[index])
			}
			if index < len(full.Pages) {
				fullText = joinPageText(full.Pages[index])
			}
			t.Logf("page %d incremental=%q full=%q", index, incrementalText, fullText)
		}
		t.Fatalf("incremental page scenes differ from a full layout (%d vs %d pages)", len(incremental.Pages), len(full.Pages))
	}
	if !reflect.DeepEqual(incremental.Locations, full.Locations) {
		t.Fatal("incremental caret locations differ from a full layout")
	}
	if !reflect.DeepEqual(incremental.Diagnostics, full.Diagnostics) {
		t.Fatalf("incremental diagnostics = %+v; full layout diagnostics = %+v", incremental.Diagnostics, full.Diagnostics)
	}
	changedPage := previous.blockCheckpoints[dirtyBlock].PageIndex
	if changedPage != 4 {
		t.Fatalf("changed paragraph starts on page %d; want page 5", changedPage+1)
	}
	for pageIndex := 0; pageIndex < changedPage; pageIndex++ {
		if len(previous.Pages[pageIndex].Text) == 0 || len(incremental.Pages[pageIndex].Text) == 0 || &incremental.Pages[pageIndex].Text[0] != &previous.Pages[pageIndex].Text[0] {
			t.Fatalf("page %d before the edited page was not reused unchanged", pageIndex+1)
		}
	}
	if incrementalShaper.calls >= fullShaper.calls {
		t.Fatalf("incremental shaping calls = %d; full layout calls = %d; want the aligned suffix reused", incrementalShaper.calls, fullShaper.calls)
	}
}

func TestLayoutAppliesCharacterStyleInheritanceToRun(t *testing.T) {
	paragraph := document.WordParagraph{ID: "p0", Style: "Body", Alignment: document.WordAlignDefault, Inlines: []document.WordInline{{
		Kind: document.WordTextInline,
		Text: &document.WordTextRun{ID: "r0", Text: "styled", CharacterStyle: "Emphasis", Editable: true},
	}}}
	input := layoutTestInput(120, 80, 5, document.WordBlock{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &paragraph})
	input.Layout = document.WordLayoutData{
		Defaults: document.WordStyleDefaults{Text: document.WordTextStyle{FontFamily: "Default", FontSize: 10}, TextSet: document.WordTextFontFamilySet | document.WordTextFontSizeSet},
		Styles: []document.WordStyle{
			{ID: "Body", Text: document.WordTextStyle{FontColor: "224466"}, TextSet: document.WordTextFontColorSet},
			{ID: "BaseCharacter", Text: document.WordTextStyle{Bold: true, FontSize: 13}, TextSet: document.WordTextBoldSet | document.WordTextFontSizeSet},
			{ID: "Emphasis", BasedOn: "BaseCharacter", Text: document.WordTextStyle{Italic: true}, TextSet: document.WordTextItalicSet},
		},
	}
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pages[0].Text) == 0 || joinPageText(result.Pages[0]) != "styled" {
		t.Fatalf("page text = %+v; want the complete run", result.Pages[0].Text)
	}
	style := result.Pages[0].Text[0].Style
	if !style.Bold || !style.Italic || style.FontSize != 13 || style.FontFamily != "Default" || style.FontColor != "224466" {
		t.Fatalf("character/paragraph/default style merge = %+v", style)
	}
}

func TestLayoutPlacesInheritedParagraphBottomBorder(t *testing.T) {
	paragraph := document.WordParagraph{ID: "p0", Style: "Title", Alignment: document.WordAlignDefault, Inlines: []document.WordInline{{
		Kind: document.WordTextInline,
		Text: &document.WordTextRun{ID: "r0", Text: "Title", Editable: true},
	}}}
	next := paragraphBlock("p1", "Next", document.WordTextStyle{FontSize: 10})
	following := paragraphBlock("p2", "Following", document.WordTextStyle{FontSize: 10})
	input := layoutTestInput(160, 120, 10,
		document.WordBlock{ID: "p0", Kind: document.WordParagraphBlock, Paragraph: &paragraph},
		next,
		following,
	)
	input.EditorParagraphInsets = true
	input.Layout.Defaults.Text = document.WordTextStyle{FontFamily: "Test Sans", FontSize: 10}
	input.Layout.Defaults.TextSet = document.WordTextFontFamilySet | document.WordTextFontSizeSet
	input.Layout.Styles = []document.WordStyle{{
		ID: "Title",
		Paragraph: document.WordParagraphLayout{
			BottomBorder: document.WordParagraphBorder{Style: "single", SizePT: 1, SpacePT: 5, Color: "4F81BD"},
			Set:          document.WordParagraphBottomBorderSet,
		},
	}}
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pages) != 1 || len(result.Pages[0].ParagraphBorders) != 1 {
		t.Fatalf("paragraph borders = %+v; want one border on one page", result.Pages)
	}
	border := result.Pages[0].ParagraphBorders[0]
	if border.SourceID != "p0" || border.Style != "single" || border.Color != "4F81BD" || border.Bounds != (Rect{XPT: 10, YPT: 47, WidthPT: 140, HeightPT: 1}) {
		t.Fatalf("paragraph border placement = %+v; want inherited blue 1pt bottom line after the text and Word border spacing at x=10 y=47", border)
	}
	var followingText *TextFragment
	for index := range result.Pages[0].Text {
		if result.Pages[0].Text[index].RunID == "p2.run" {
			followingText = &result.Pages[0].Text[index]
			break
		}
	}
	if followingText == nil || followingText.BaselineYPT <= border.Bounds.YPT+border.Bounds.HeightPT {
		t.Fatalf("following paragraph baseline = %+v; it must follow the bottom border", followingText)
	}
}

func TestSyntheticItalicSlantsNonItalicGlyphs(t *testing.T) {
	inputRegular := layoutTestInput(160, 200, 10, paragraphBlock("p0", "A", document.WordTextStyle{FontSize: 10, Italic: false}))
	resultRegular, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), inputRegular)
	if err != nil {
		t.Fatal(err)
	}
	inputItalic := layoutTestInput(160, 200, 10, paragraphBlock("p0", "A", document.WordTextStyle{FontSize: 10, Italic: true}))
	resultItalic, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), inputItalic)
	if err != nil {
		t.Fatal(err)
	}
	glyphReg := resultRegular.Pages[0].Text[0].Glyphs[0]
	glyphItal := resultItalic.Pages[0].Text[0].Glyphs[0]
	ptReg := glyphReg.Outline[2].Points[0]
	ptItal := glyphItal.Outline[2].Points[0]
	if ptItal.YPT != ptReg.YPT {
		t.Fatalf("YPT changed unexpectedly: regular=%g italic=%g", ptReg.YPT, ptItal.YPT)
	}
	// At YPT = -7 (above baseline), the italic glyph XPT must be shifted right:
	expectedShift := -ptReg.YPT * 0.21255656
	if math.Abs((ptItal.XPT-ptReg.XPT)-expectedShift) > 1e-4 {
		t.Fatalf("italic outline XPT = %g; want %g + %g = %g", ptItal.XPT, ptReg.XPT, expectedShift, ptReg.XPT+expectedShift)
	}
}
