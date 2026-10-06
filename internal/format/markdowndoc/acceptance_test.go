package markdowndoc

import (
	"os"
	"strings"
	"testing"
)

func TestMarkdownLiteralCodeAndEscapedText(t *testing.T) {
	literal := "[link](https://example.com)\n![image](https://example.com/a.png)\n\\*literal\\* $x^2$\n"
	p, err := Parse("```\n" + literal + "```\n\n`[link](https://example.com) ![image](a.png)`\n\n\\*literal\\* &amp; &#124;\n")
	if err != nil {
		t.Fatal(err)
	}
	if p.Blocks[0].Kind != "code" || p.Blocks[0].Text != literal || len(p.Blocks[0].Spans) != 0 {
		t.Fatalf("fenced literal altered: %+v", p.Blocks[0])
	}
	if len(p.Blocks[1].Spans) != 1 || p.Blocks[1].Spans[0].Kind != "code" || p.Blocks[1].Spans[0].Text != "[link](https://example.com) ![image](a.png)" {
		t.Fatalf("inline literal: %+v", p.Blocks[1])
	}
	if p.Blocks[2].Text != "*literal* & |" {
		t.Fatalf("escaped paragraph: %q", p.Blocks[2].Text)
	}
	images, diagnostics := PreviewResources(p, "")
	if len(images)+len(diagnostics) != 0 {
		t.Fatal("code triggered resources")
	}
}

func TestMarkdownAcceptanceSemantics(t *testing.T) {
	source := "~~removed~~ [![video](https://example.com/image.png)](https://example.com/video)\n\n|left|center|right|\n|:---|:---:|---:|\n|\\||x|y|\n\n1. first\n\n   continuation\n\n   - child\n     - grandchild\n\nFootnote[^first].\n\n[^first]: note **bold**\n"
	p, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	strike, linked, aligned, continuation, nested, footnote := false, false, false, false, false, false
	for _, b := range p.Blocks {
		if b.Kind == "table" {
			aligned = strings.Join(b.TableAlignments, ",") == "left,center,right" && b.Rows[1][0] == "|"
		}
		if b.Text == "continuation" {
			continuation = b.Prefix == "" && b.Level == 1
		}
		if b.Text == "grandchild" {
			nested = b.Level == 3
		}
		if b.Anchor == "footnote-1" {
			footnote = true
		}
		for _, s := range b.Spans {
			strike = strike || s.Strike
			linked = linked || (s.Kind == "image" && s.Destination == "https://example.com/image.png" && s.LinkDestination == "https://example.com/video")
		}
	}
	if !strike || !linked || !aligned || !continuation || !nested || !footnote {
		t.Fatalf("strike=%v linked=%v align=%v continuation=%v nested=%v footnote=%v blocks=%+v", strike, linked, aligned, continuation, nested, footnote, p.Blocks)
	}
	if p.Source != source {
		t.Fatal("derived rendering rewrote source")
	}
}

func TestMarkdownExternalAcceptanceFile(t *testing.T) {
	path := os.Getenv("GLANCE_MARKDOWN_ACCEPTANCE_SOURCE")
	if path == "" {
		t.Skip("optional user acceptance file")
	}
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := Parse(string(source))
	if err != nil {
		t.Fatal(err)
	}
	codes, images, formulas := 0, 0, 0
	for _, b := range p.Blocks {
		if b.Kind == "code" {
			codes++
			if len(b.Spans) != 0 {
				t.Fatal("fenced/HTML code became rich spans")
			}
		}
		if b.Kind == "math" {
			formulas++
		}
		for _, s := range b.Spans {
			if s.Kind == "image" {
				images++
			}
		}
	}
	if codes < 15 || images != 6 || formulas != 2 {
		t.Fatalf("acceptance counts: code=%d images=%d formulas=%d", codes, images, formulas)
	}
	if p.Source != string(source) {
		t.Fatal("acceptance source modified")
	}
	t.Logf("acceptance: %d blocks, %d headings, %d literal code blocks, %d images, %d display formulas", len(p.Blocks), len(p.Headings), codes, images, formulas)
}
