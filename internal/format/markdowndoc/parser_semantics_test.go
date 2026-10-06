package markdowndoc

import (
	"strings"
	"testing"
)

func TestMarkdownParserLiteralDollarsAndCurrency(t *testing.T) {
	p, err := Parse(`\$x\$ and $5 and $10; math $x^2$; &#36;y&#36;`)
	if err != nil {
		t.Fatal(err)
	}
	math := 0
	for _, s := range p.Blocks[0].Spans {
		if s.Kind == "math" {
			math++
			if s.Text != "x^2" {
				t.Fatalf("currency became math: %+v", s)
			}
		}
	}
	if math != 1 || !strings.Contains(p.Blocks[0].Text, "$x$ and $5 and $10") {
		t.Fatalf("literal dollars: %+v", p.Blocks[0])
	}
}

func TestMarkdownParserEntityEscapeOrdering(t *testing.T) {
	p, err := Parse(`\&amp; &amp; &copycat &copy; &#x4e2d;`)
	if err != nil {
		t.Fatal(err)
	}
	if p.Blocks[0].Text != "&amp; & &copycat © 中" {
		t.Fatalf("decoded text %q", p.Blocks[0].Text)
	}
}

func TestMarkdownParserFootnoteContentAndQuoteLevels(t *testing.T) {
	p, err := Parse("Reference[^n] and again[^n].\n\n[^n]: first **bold**\n\n    continuation\n\n> outer\n>> inner\n>>> deepest\n")
	if err != nil {
		t.Fatal(err)
	}
	links, anchored, continuation := 0, false, false
	levels := []int{}
	for _, b := range p.Blocks {
		if b.Anchor == "footnote-1" {
			anchored = strings.Contains(b.Text, "first bold")
		}
		if b.Text == "continuation" {
			continuation = true
		}
		if b.Kind == "quote" {
			levels = append(levels, b.Level)
		}
		for _, s := range b.Spans {
			if s.Destination == "#footnote-1" {
				links++
				if s.Text != "[1]" {
					t.Fatalf("footnote label %+v", s)
				}
			}
		}
	}
	if links != 2 || !anchored || !continuation || len(levels) != 3 || levels[0] != 1 || levels[1] != 2 || levels[2] != 3 {
		t.Fatalf("footnotes or quotes: %+v", p.Blocks)
	}
}
