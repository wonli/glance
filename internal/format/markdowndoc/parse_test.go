package markdowndoc

import (
	"context"
	"errors"
	"testing"

	"glance/internal/diagramrender"
)

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
	loader := NewResourceLoader(func() (*diagramrender.Renderer, error) {
		return nil, errors.New("native renderer intentionally unavailable in resource validation test")
	})
	_, diagnostics := loader.PreviewResources(context.Background(), p, "source.md")
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

func TestVegaFencesBecomeLanguagePreservingDiagrams(t *testing.T) {
	source := "```VeGa\n{\"data\":{\"values\":[]}}\n```\n\n```VEGA-LITE\n{\"mark\":\"point\"}\n```\n"
	p, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	if p.Source != source || len(p.Blocks) != 2 {
		t.Fatalf("source or blocks changed: source=%q blocks=%+v", p.Source, p.Blocks)
	}
	want := []struct{ language, text string }{
		{"vega", "{\"data\":{\"values\":[]}}\n"},
		{"vega-lite", "{\"mark\":\"point\"}\n"},
	}
	for i, block := range p.Blocks {
		if block.Kind != "diagram" || block.Language != want[i].language || block.Text != want[i].text {
			t.Errorf("block %d = %+v, want diagram %q with exact source", i, block, want[i].language)
		}
	}
}

func TestOnlyVegaFencesBecomeDiagrams(t *testing.T) {
	for _, source := range []string{
		"```json\n{}\n```\n",
		"```javascript\nconst x = 1\n```\n",
		"```markdown\n```vega\n{}\n```\n",
		"    {\"mark\":\"point\"}\n",
	} {
		p, err := Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		for _, block := range p.Blocks {
			if block.Kind == "diagram" {
				t.Errorf("ordinary/nested code became a chart for %q: %+v", source, block)
			}
		}
	}
}

func TestPlainFenceWithOfficialVegaLiteSchemaBecomesDiagram(t *testing.T) {
	source := "```plain\n" + `{
  "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
  "data": {"values": [{"category": "A", "value": 28}]},
  "mark": "bar",
  "encoding": {
    "x": {"field": "category", "type": "nominal"},
    "y": {"field": "value", "type": "quantitative"}
  }
}` + "\n```\n"

	p, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Blocks) != 1 || p.Blocks[0].Kind != "diagram" || p.Blocks[0].Language != "vega-lite" {
		t.Fatalf("plain official Vega-Lite spec was not recognized: %+v", p.Blocks)
	}
	if p.Blocks[0].SourceStart <= 0 || p.Blocks[0].SourceEnd <= p.Blocks[0].SourceStart {
		t.Fatalf("Vega-Lite source range was not retained: %+v", p.Blocks[0])
	}
}

func TestPlainFenceRequiresOfficialVegaSchema(t *testing.T) {
	for _, schema := range []string{
		"https://example.com/schema/vega-lite/v5.json",
		"https://vega.github.io.evil.example/schema/vega-lite/v5.json",
		"https://vega.github.io/schema/vega/v5.json/extra",
	} {
		source := "```plain\n{\"$schema\":\"" + schema + "\",\"mark\":\"point\"}\n```\n"
		p, err := Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Blocks) != 1 || p.Blocks[0].Kind != "code" {
			t.Errorf("non-official schema %q was treated as a diagram: %+v", schema, p.Blocks)
		}
	}
}
