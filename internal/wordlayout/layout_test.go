package wordlayout

import (
	"context"
	"math"
	"reflect"
	"strings"
	"testing"

	"glance/internal/document"
)

type layoutFakeShaper struct{}

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

func TestLayoutOrdinaryTableSplitsBetweenRows(t *testing.T) {
	table := document.WordTable{Layout: document.WordTableLayout{GridColumnsEMU: []int64{635000, 635000}}}
	for rowIndex := 0; rowIndex < 3; rowIndex++ {
		for cellIndex := 0; cellIndex < 2; cellIndex++ {
			id := strings.Join([]string{"t0", "r", string(rune('0' + rowIndex)), "c", string(rune('0' + cellIndex))}, "")
			paragraph := document.WordParagraph{ID: id + ".p0", Alignment: document.WordAlignDefault, Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: id + ".run", Text: "row", Style: document.WordTextStyle{FontSize: 10}, Editable: true}}}}
			tableRow := document.WordTableRow{Cells: []document.WordTableCell{{ID: id, WidthEMU: 635000, Paragraphs: []document.WordParagraph{paragraph}}}}
			if cellIndex == 0 {
				table.Rows = append(table.Rows, tableRow)
			} else {
				table.Rows[rowIndex].Cells = append(table.Rows[rowIndex].Cells, tableRow.Cells[0])
			}
		}
	}
	input := layoutTestInput(120, 38, 4, document.WordBlock{ID: "t0", Kind: document.WordTableBlock, Table: &table})
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pages) != 2 {
		t.Fatalf("page count = %d; want 2", len(result.Pages))
	}
	if len(result.Pages[0].TableCells) != 4 || len(result.Pages[1].TableCells) != 2 {
		t.Fatalf("table cells per page = %d/%d; want two rows then one row split by cells", len(result.Pages[0].TableCells), len(result.Pages[1].TableCells))
	}
	if result.Pages[0].TableCells[2].RowIndex != 1 || result.Pages[1].TableCells[0].RowIndex != 2 {
		t.Fatalf("table row order across pages = %+v / %+v", result.Pages[0].TableCells, result.Pages[1].TableCells)
	}
}

func TestLayoutFillsMissingTableGridColumnWithoutExceedingPageWidth(t *testing.T) {
	table := document.WordTable{Layout: document.WordTableLayout{GridColumnsEMU: []int64{1270000, 0}}}
	row := document.WordTableRow{}
	for index := 0; index < 2; index++ {
		paragraph := document.WordParagraph{ID: "cell.p" + itoa(index), Alignment: document.WordAlignDefault}
		row.Cells = append(row.Cells, document.WordTableCell{ID: "cell" + itoa(index), Paragraphs: []document.WordParagraph{paragraph}})
	}
	table.Rows = []document.WordTableRow{row}
	input := layoutTestInput(100, 80, 5, document.WordBlock{ID: "t0", Kind: document.WordTableBlock, Table: &table})
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	cells := result.Pages[0].TableCells
	if len(cells) != 2 || cells[0].Bounds.XPT != 5 || cells[0].Bounds.WidthPT != 45 || cells[1].Bounds.XPT != 50 || cells[1].Bounds.WidthPT != 45 || cells[1].Bounds.XPT+cells[1].Bounds.WidthPT != 95 {
		t.Fatalf("table grid cell bounds = %+v; want adjacent 45pt columns contained in the 90pt page width", cells)
	}
	foundWarning := false
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Severity == DiagnosticWarning && diagnostic.SourceID == "t0" && diagnostic.Code == "table-grid-column-width-inferred" {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Fatalf("diagnostics = %+v; want a located warning for the inferred grid column", result.Diagnostics)
	}
}

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
	// Page height 100pt, margins 10pt -> printable height is 80pt
	// Image has height 200pt (2540000 EMU), width 100pt (1270000 EMU)
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

func TestLayoutKeepsParagraphWithNextParagraphOnFreshPage(t *testing.T) {
	prefix := paragraphBlock("prefix", "x", document.WordTextStyle{FontSize: 15})
	kept := paragraphBlock("kept", "a", document.WordTextStyle{FontSize: 10})
	kept.Paragraph.Layout = document.WordParagraphLayout{KeepWithNext: true, Set: document.WordParagraphKeepWithNextSet}
	next := paragraphBlock("next", "b", document.WordTextStyle{FontSize: 10})
	input := layoutTestInput(100, 40, 5, prefix, kept, next)
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pages) != 2 || joinPageText(result.Pages[0]) != "x" || joinPageText(result.Pages[1]) != "ab" {
		t.Fatalf("keep-with-next pages = %q / %q; want prefix then linked paragraphs together", joinPageText(result.Pages[0]), joinPageText(result.Pages[1]))
	}
}

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

func TestLayoutUnsupportedSectionsBlockOutput(t *testing.T) {
	input := layoutTestInput(100, 70, 5, paragraphBlock("p0", "text", document.WordTextStyle{FontSize: 10}))
	input.Layout.Sections = []document.WordSection{
		{StartBlock: 0, PageSetup: document.WordPageSetup{WidthPT: 100, HeightPT: 70, MarginTopPT: 5, MarginRightPT: 5, MarginBottomPT: 5, MarginLeftPT: 5}},
		{StartBlock: 1, PageSetup: document.WordPageSetup{WidthPT: 120, HeightPT: 70, MarginTopPT: 5, MarginRightPT: 5, MarginBottomPT: 5, MarginLeftPT: 5}},
	}
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.CanPreview {
		t.Fatal("unsupported section layout must block preview/output")
	}
	found := false
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Severity == DiagnosticError && diagnostic.SourceID == "section-1" && diagnostic.Code == "multiple-sections" {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %+v; want located multiple-section error", result.Diagnostics)
	}
}

func TestLayoutEquivalentSectionsDoNotBlockOutput(t *testing.T) {
	pageSetup := document.WordPageSetup{
		WidthPT: 100, HeightPT: 70, Orientation: "portrait",
		MarginTopPT: 5, MarginRightPT: 5, MarginBottomPT: 5, MarginLeftPT: 5,
		Columns: 1, ColumnSpacingPT: 36,
		Set: document.WordPageWidthSet | document.WordPageHeightSet | document.WordPageOrientationSet | document.WordPageMarginTopSet | document.WordPageMarginRightSet | document.WordPageMarginBottomSet | document.WordPageMarginLeftSet | document.WordPageColumnsSet | document.WordPageColumnSpacingSet,
	}
	input := layoutTestInput(100, 70, 5,
		paragraphBlock("p0", "cover", document.WordTextStyle{FontSize: 10}),
		paragraphBlock("p1", "body", document.WordTextStyle{FontSize: 10}),
	)
	input.Layout.Sections = []document.WordSection{
		{StartBlock: 0, PageSetup: pageSetup},
		{StartBlock: 1, PageSetup: pageSetup},
	}
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.CanPreview {
		t.Fatalf("equivalent section page setups blocked preview: %+v", result.Diagnostics)
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

func TestLayoutOversizedTableRowReportsOverflowAndTerminates(t *testing.T) {
	paragraph := document.WordParagraph{ID: "cell.p0", Alignment: document.WordAlignDefault, Inlines: []document.WordInline{{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "r0", Text: "abcdefghijklmnopqrstuvwx", Style: document.WordTextStyle{FontSize: 10}, Editable: true}}}}
	table := document.WordTable{Rows: []document.WordTableRow{{Cells: []document.WordTableCell{{ID: "cell0", WidthEMU: 254000, Paragraphs: []document.WordParagraph{paragraph}}}}}}
	input := layoutTestInput(40, 30, 5, document.WordBlock{ID: "t0", Kind: document.WordTableBlock, Table: &table})
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.CanPreview {
		t.Fatal("oversized row must block output")
	}
	count := 0
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "table-row-overflow" && diagnostic.SourceID == "t0.r0" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("overflow diagnostics = %+v; want one located t0.r0 diagnostic", result.Diagnostics)
	}
	if len(result.Pages) != 1 {
		t.Fatalf("oversized row caused retry/blank pages: %d", len(result.Pages))
	}
	for _, page := range result.Pages {
		for _, cell := range page.TableCells {
			if cell.Bounds.XPT < 0 || cell.Bounds.YPT < 0 || cell.Bounds.WidthPT < 0 || cell.Bounds.HeightPT < 0 || math.IsNaN(cell.Bounds.HeightPT) || math.IsInf(cell.Bounds.HeightPT, 0) {
				t.Fatalf("invalid overflow geometry: %+v", cell.Bounds)
			}
		}
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

func joinPageText(page PageScene) string {
	var value strings.Builder
	for _, fragment := range page.Text {
		value.WriteString(fragment.Text)
	}
	return value.String()
}

func TestMultiColumnLayout(t *testing.T) {
	// Page: width 200, height 100, margin 10 -> content width 180, height 80
	// 2 columns with spacing 20 -> column width = (180 - 20) / 2 = 80 pt
	// Col 0: X = 10..90
	// Col 1: X = 110..190
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
	// Page 0 should have ColZero and ColOne
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

	// Page 1 should have PageOneColZero in Col 0 (around X=10)
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
