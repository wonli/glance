package wordlayout

import (
	"context"
	"testing"

	"glance/internal/document"
)

func TestSectionReusesAutomaticallyOverflowedEmptyPage(t *testing.T) {
	for _, tc := range []struct {
		name, startType string
		explicitBreak   bool
		wantPages       int
	}{
		{"next page", "nextpage", false, 2},
		{"even page", "evenpage", false, 2},
		{"odd page", "oddpage", false, 3},
		{"explicit blank page", "nextpage", true, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cover := paragraphBlock("cover", "cover", document.WordTextStyle{FontSize: 20})
			if tc.explicitBreak {
				cover.Paragraph.Inlines = append(cover.Paragraph.Inlines, document.WordInline{Kind: document.WordPageBreakInline, PageBreak: &document.WordPageBreak{ID: "break"}})
			}
			empty := paragraphBlock("empty", "", document.WordTextStyle{})
			body := paragraphBlock("body", "body", document.WordTextStyle{FontSize: 10})
			input := layoutTestInput(150, 40, 5, cover, empty, body)
			input.Layout.Defaults.Text = document.WordTextStyle{FontSize: 20}
			input.Layout.Sections = []document.WordSection{{StartBlock: 0}, {StartBlock: 2, StartType: tc.startType}}
			r, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			if len(r.Pages) != tc.wantPages || joinPageText(r.Pages[tc.wantPages-1]) != "body" {
				t.Fatalf("got %d pages; want body on page %d", len(r.Pages), tc.wantPages)
			}
			if len(r.Pages[tc.wantPages-1].Text) == 0 || r.Pages[tc.wantPages-1].Text[0].BaselineYPT != 13 {
				t.Fatal("new section did not reset to page top")
			}
		})
	}
}

func TestKeepWithNextStopsAtNextPageSection(t *testing.T) {
	prefix := paragraphBlock("prefix", "X", document.WordTextStyle{FontSize: 70})
	last := paragraphBlock("last", "A", document.WordTextStyle{FontSize: 20})
	last.Paragraph.Layout = document.WordParagraphLayout{KeepWithNext: true, Set: document.WordParagraphKeepWithNextSet}
	next := paragraphBlock("next", "B", document.WordTextStyle{FontSize: 20})
	input := layoutTestInput(500, 110, 5, prefix, last, next)
	input.Layout.Sections = []document.WordSection{{StartBlock: 0}, {StartBlock: 2, StartType: "nextpage"}}
	result, err := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pages) != 2 || joinPageText(result.Pages[0]) != "XA" || joinPageText(result.Pages[1]) != "B" {
		t.Fatalf("keep-with-next crossed a section boundary: %+v", result.Pages)
	}
}
