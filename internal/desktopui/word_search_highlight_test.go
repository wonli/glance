package desktopui

import (
	"reflect"
	"testing"

	"glance/internal/document"
)

func TestWordSearchHighlightsCoverEveryMatchAcrossEditableRuns(t *testing.T) {
	paragraph := document.WordParagraph{ID: "p1", Inlines: []document.WordInline{
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "first", Text: "alpha be", Editable: true}},
		{Kind: document.WordTextInline, Text: &document.WordTextRun{ID: "second", Text: "ta beta", Editable: true}},
	}}
	blocks := []document.WordBlock{{Kind: document.WordParagraphBlock, Paragraph: &paragraph}}

	got := wordSearchHighlightRanges(blocks, "beta", nil)
	want := map[string][]wordSearchTextRange{
		"first":  {{Start: 6, End: 8}},
		"second": {{Start: 0, End: 2}, {Start: 3, End: 7}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("search highlight ranges = %#v; want %#v", got, want)
	}
}
