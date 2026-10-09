package wordlayout

import (
	"context"
	"glance/internal/document"
	"strings"
	"testing"
)

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
	roundedSetup := pageSetup
	roundedSetup.WidthPT += 0.05
	input := layoutTestInput(100, 70, 5,
		paragraphBlock("p0", "cover", document.WordTextStyle{FontSize: 10}),
		paragraphBlock("p1", "body", document.WordTextStyle{FontSize: 10}),
	)
	input.Layout.Sections = []document.WordSection{
		{StartBlock: 0, PageSetup: pageSetup},
		{StartBlock: 1, PageSetup: roundedSetup},
	}
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.CanPreview {
		t.Fatalf("equivalent section page setups blocked preview: %+v", result.Diagnostics)
	}
}

func TestSectionHeadersFollowSectionPagesAndRepeatOnOverflow(t *testing.T) {
	pageSetup := document.WordPageSetup{WidthPT: 100, HeightPT: 90, MarginTopPT: 20, MarginRightPT: 20, MarginBottomPT: 20, MarginLeftPT: 20}
	blocks := []document.WordBlock{
		paragraphBlock("cover", "cover", document.WordTextStyle{FontSize: 10}),
		paragraphBlock("body-a", "a", document.WordTextStyle{FontSize: 40}),
		paragraphBlock("body-b", "b", document.WordTextStyle{FontSize: 40}),
		paragraphBlock("body-c", "c", document.WordTextStyle{FontSize: 40}),
	}
	input := layoutTestInput(100, 90, 20, blocks...)
	input.Layout.Sections = []document.WordSection{
		{StartBlock: 0, PageSetup: pageSetup, HeaderText: "First", HeaderSet: true, HeaderDistancePT: 4},
		{StartBlock: 1, StartType: "nextpage", PageSetup: pageSetup, HeaderText: "Second", HeaderSet: true, HeaderDistancePT: 4},
	}
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pages) != 4 {
		t.Fatalf("page count = %d; want a cover and three overflow pages", len(result.Pages))
	}
	if got := joinPageText(result.Pages[0]); !strings.Contains(got, "First") || strings.Contains(got, "Second") {
		t.Fatalf("first section page text = %q; want only its own header", got)
	}
	for pageIndex := 1; pageIndex < len(result.Pages); pageIndex++ {
		page := result.Pages[pageIndex]
		if !strings.Contains(joinPageText(page), "Second") || strings.Contains(joinPageText(page), "First") {
			t.Errorf("section 2 page %d text = %q; want its repeated header only", pageIndex, joinPageText(page))
		}
		for _, fragment := range page.Text {
			if fragment.Text == "Second" && (!fragment.Synthetic || fragment.Editable || fragment.XPT != 20 || fragment.BaselineYPT >= page.Content.YPT) {
				t.Errorf("section header fragment = %+v; want read-only text in the header area", fragment)
			}
		}
	}
}

func TestContinuousSectionHeadersDoNotLeakBetweenPages(t *testing.T) {
	pageSetup := document.WordPageSetup{WidthPT: 100, HeightPT: 90, MarginTopPT: 20, MarginRightPT: 20, MarginBottomPT: 20, MarginLeftPT: 20}
	input := layoutTestInput(100, 90, 20,
		paragraphBlock("first-body", "first", document.WordTextStyle{FontSize: 10}),
		paragraphBlock("continuous-body", "middle", document.WordTextStyle{FontSize: 10}),
		paragraphBlock("continued-page", "n", document.WordTextStyle{FontSize: 40}),
	)
	input.Layout.Sections = []document.WordSection{
		{StartBlock: 0, PageSetup: pageSetup, HeaderText: "First", HeaderSet: true, HeaderDistancePT: 4},
		{StartBlock: 1, StartType: "continuous", PageSetup: pageSetup, HeaderText: "Second", HeaderSet: true, HeaderDistancePT: 4},
	}
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pages) != 2 {
		var texts []string
		for _, page := range result.Pages {
			texts = append(texts, joinPageText(page))
		}
		t.Fatalf("page count = %d with text %q; want two pages", len(result.Pages), texts)
	}
	if text := joinPageText(result.Pages[0]); !strings.Contains(text, "First") || strings.Contains(text, "Second") {
		t.Fatalf("shared continuous page headers = %q; want the initial page header only", text)
	}
	if text := joinPageText(result.Pages[1]); !strings.Contains(text, "Second") || strings.Contains(text, "First") {
		t.Fatalf("continued page headers = %q; want the active section header only", text)
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "continuous-section-header-fallback" && diagnostic.Severity == DiagnosticWarning {
			return
		}
	}
	t.Fatalf("diagnostics = %+v; want a warning for shared continuous-section header space", result.Diagnostics)
}
