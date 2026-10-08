package wordlayout

import (
	"context"
	"math"
	"reflect"
	"testing"

	"glance/internal/document"
)

func TestParagraphSpacingDoesNotAddToLargerInterlineGap(t *testing.T) {
	for _, inTable := range []bool{false, true} {
		t.Run(map[bool]string{false: "body", true: "table"}[inTable], func(t *testing.T) {
			a := paragraphBlock("a", "A", document.WordTextStyle{FontSize: 10})
			b := paragraphBlock("b", "B", document.WordTextStyle{FontSize: 10})
			for _, p := range []*document.WordParagraph{a.Paragraph, b.Paragraph} {
				p.Layout = document.WordParagraphLayout{BeforePT: 6, AfterPT: 6, LineSpacingRule: "auto", LineSpacingMultiple: 2, Set: document.WordParagraphBeforeSet | document.WordParagraphAfterSet | document.WordParagraphLineSpacingSet}
			}
			blocks := []document.WordBlock{a, b}
			if inTable {
				blocks = []document.WordBlock{{ID: "t", Kind: document.WordTableBlock, Table: &document.WordTable{Rows: []document.WordTableRow{{Cells: []document.WordTableCell{{ID: "c", Paragraphs: []document.WordParagraph{*a.Paragraph, *b.Paragraph}}}}}}}}
			}
			r, e := NewEngine(layoutFakeShaper{}).Layout(context.Background(), layoutTestInput(100, 100, 5, blocks...))
			if e != nil {
				t.Fatal(e)
			}
			var first, second float64
			for _, p := range r.Pages {
				for _, f := range p.Text {
					if f.Text == "A" {
						first = f.BaselineYPT
					}
					if f.Text == "B" {
						second = f.BaselineYPT
					}
				}
			}
			if math.Abs(second-first-20) > 0.001 {
				t.Fatalf("baseline distance %.2f; want 20pt (10pt text plus max(10pt interline,6pt paragraph gap))", second-first)
			}
		})
	}
}

func TestEmptyParagraphHonorsAutomaticLineSpacing(t *testing.T) {
	empty := paragraphBlock("empty", "", document.WordTextStyle{})
	empty.Paragraph.Layout = document.WordParagraphLayout{LineSpacingRule: "auto", LineSpacingMultiple: 2, Set: document.WordParagraphLineSpacingSet}
	next := paragraphBlock("next", "X", document.WordTextStyle{FontSize: 10})
	input := layoutTestInput(100, 100, 5, empty, next)
	input.Layout.Defaults.Text = document.WordTextStyle{FontSize: 10}
	r, e := NewEngine(layoutFakeShaper{}).Layout(context.Background(), input)
	if e != nil {
		t.Fatal(e)
	}
	if got := r.Pages[0].Text[0].BaselineYPT; math.Abs(got-33) > 0.001 {
		t.Fatalf("next baseline %.2f; want 33 after a 20pt empty line", got)
	}
}

func TestParagraphSpacingCanReclaimMoreThanNextLineHeight(t *testing.T) {
	a := paragraphBlock("a", "A", document.WordTextStyle{FontSize: 10})
	a.Paragraph.Layout = document.WordParagraphLayout{AfterPT: 100, LineSpacingRule: "auto", LineSpacingMultiple: 10, Set: document.WordParagraphAfterSet | document.WordParagraphLineSpacingSet}
	b := paragraphBlock("b", "B", document.WordTextStyle{FontSize: 10})
	r, e := NewEngine(layoutFakeShaper{}).Layout(context.Background(), layoutTestInput(100, 400, 5, a, b))
	if e != nil {
		t.Fatal(e)
	}
	if !r.CanPreview || joinPageText(r.Pages[0]) != "AB" {
		t.Fatalf("spacing reclamation dropped text: %q, diagnostics %+v", joinPageText(r.Pages[0]), r.Diagnostics)
	}
	if got := r.Pages[0].Text[1].BaselineYPT; math.Abs(got-123) > 0.001 {
		t.Fatalf("baseline %.2f; want 123", got)
	}
}

func TestIncrementalLayoutRestoresParagraphSpacingState(t *testing.T) {
	var blocks []document.WordBlock
	for i := 0; i < 24; i++ {
		block := paragraphBlock("p"+itoa(i), "A", document.WordTextStyle{FontSize: 10})
		block.Paragraph.Layout = document.WordParagraphLayout{BeforePT: 6, AfterPT: 6, LineSpacingRule: "auto", LineSpacingMultiple: 2, Set: document.WordParagraphBeforeSet | document.WordParagraphAfterSet | document.WordParagraphLineSpacingSet}
		blocks = append(blocks, block)
	}
	input := layoutTestInput(100, 100, 5, blocks...)
	input.DocumentID = "spacing-reflow"
	engine := NewEngine(layoutFakeShaper{})
	previous, err := engine.Layout(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	input.Blocks[12].Paragraph.Inlines[0].Text.Text = "B"
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
		t.Fatal("incremental reflow differs from full spacing layout")
	}
}
