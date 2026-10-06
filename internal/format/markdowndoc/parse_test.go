package markdowndoc

import "testing"

func TestMarkdownOutline(t *testing.T) {
	source := "# 同名\n\n```md\n# 不是标题\n```\n\n## 同名\n"
	p, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	if p.Source != source || len(p.Headings) != 2 {
		t.Fatalf("outline: %+v", p.Headings)
	}
	if p.Headings[0].Title != "同名" || p.Headings[1].Level != 2 || p.Headings[1].Offset <= p.Headings[0].Offset {
		t.Fatal("lost distinct source location")
	}
}

func TestMarkdownStructuredPreview(t *testing.T) {
	p, err := Parse("# 标题\n\n**粗体**和*斜体* [链接](./a.md)\n\n- [x] 完成\n- 未完成\n\n| A | B |\n|---|---|\n| 甲 | 乙 |\n\n![图片](a.png)\n\n$$\n\\frac{a}{b}\n$$\n")
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, b := range p.Blocks {
		kinds[b.Kind] = true
	}
	for _, k := range []string{"heading", "paragraph", "list", "table", "math"} {
		if !kinds[k] {
			t.Errorf("missing %s: %+v", k, p.Blocks)
		}
	}
}

func TestMarkdownMathCorpus(t *testing.T) {
	for _, expr := range []string{`\alpha+\beta`, `x^2+y_1`, `\frac{a}{b}`, `\sqrt{x}`, `\sum_{i=1}^n i`} {
		img, err := RenderMath(expr, true)
		if err != nil {
			t.Errorf("%s: %v", expr, err)
			continue
		}
		if img.Bounds().Empty() {
			t.Errorf("empty %s", expr)
		}
	}
	if _, err := RenderMath(`\unknowncommand{x}`, false); err == nil {
		t.Fatal("unsupported math accepted")
	}
}

func TestMarkdownBOMHeading(t *testing.T) {
	p, err := Parse("\ufeff# 中文\r\n")
	if err != nil || len(p.Headings) != 1 || p.Headings[0].Title != "中文" {
		t.Fatalf("BOM outline %+v %v", p.Headings, err)
	}
}
func TestMarkdownTableResources(t *testing.T) {
	p, err := Parse("|图片|公式|\n|---|---|\n|![图](missing.png)|$\\unknowncommand{x}$|\n")
	if err != nil {
		t.Fatal(err)
	}
	_, diagnostics := PreviewResources(p, "source.md")
	if len(diagnostics) != 2 {
		t.Fatalf("table silently lost resources: %+v", diagnostics)
	}
}
func TestMarkdownImagePathWithSpaces(t *testing.T) {
	out, err := ApplyEdit("", "image", 0, 0, EditArgs{Label: "图", Destination: "./my image.png"})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := Parse(out.Text)
	found := false
	for _, b := range p.Blocks {
		for _, s := range b.Spans {
			if s.Kind == "image" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("image path did not parse: %q", out.Text)
	}
}
func TestMarkdownFencedCodeRetainsFenceInSelection(t *testing.T) {
	out, err := ApplyEdit("foo\n```\nbar", "code", 0, 11, EditArgs{})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := Parse(out.Text)
	if len(p.Blocks) != 1 || p.Blocks[0].Kind != "code" || p.Blocks[0].Text != "foo\n```\nbar\n" {
		t.Fatalf("bad fenced result %q %+v", out.Text, p.Blocks)
	}
}
