package markdowndoc

import (
	"os"
	"strings"
	"testing"
)

func TestNormalizeYuqueDiagram(t *testing.T) {
	input := `## 📈 图表与技术图示
### Mermaid 流程图
<!-- 这是一个文本绘图，源码为：graph TD
    A[开始] --> B{条件判断}
    B -- 是 --> C[执行操作1]
    B -- 否 --> D[执行操作2]
    C --> E[结束]
    D --> E -->
![](https://cdn.nlark.com/yuque/__mermaid_v3/ecd9b9e67824f36c6ab0266adc41a8ae.svg)

### 下一节
`

	normalized := normalizeYuque(input)
	if !strings.Contains(normalized, "```mermaid") {
		t.Fatalf("expected ```mermaid, got:\n%s", normalized)
	}
	if strings.Contains(normalized, "<!-- 这是一个文本绘图") {
		t.Fatalf("yuque comment not removed:\n%s", normalized)
	}
	if strings.Contains(normalized, "https://cdn.nlark.com") {
		t.Fatalf("yuque fallback image not replaced:\n%s", normalized)
	}

	p, err := Parse(normalized)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	foundDiagram := false
	for _, b := range p.Blocks {
		if b.Kind == "diagram" {
			foundDiagram = true
			if !strings.Contains(b.Text, "A[开始] --> B{条件判断}") {
				t.Errorf("diagram content missing expected node: %s", b.Text)
			}
			if strings.HasSuffix(strings.TrimSpace(b.Text), "-->") {
				t.Errorf("diagram has trailing comment terminator: %s", b.Text)
			}
		}
	}
	if !foundDiagram {
		t.Fatal("no diagram block parsed after normalization")
	}
}

func TestNormalizeYuqueMathBlock(t *testing.T) {
	input := `### 块级公式
$ \begin{bmatrix}
a & b \\
c & d 
\end{bmatrix}^{-1} = \frac{1}{ad-bc} \begin{bmatrix}
d & -b \\
-c & a 
\end{bmatrix} $
`

	normalized := normalizeYuque(input)
	if !strings.Contains(normalized, "$$\n\\begin{bmatrix}") {
		t.Fatalf("expected $$, got:\n%s", normalized)
	}

	p, err := Parse(normalized)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	foundMath := false
	for _, b := range p.Blocks {
		if b.Kind == "math" {
			foundMath = true
			if !strings.Contains(b.Text, "\\begin{bmatrix}") {
				t.Errorf("math content missing matrix: %s", b.Text)
			}
		}
	}
	if !foundMath {
		t.Fatal("no math block parsed after normalization")
	}
}

func TestComplexMarkdownEndToEnd(t *testing.T) {
	content, err := os.ReadFile("/Users/mima1234/Downloads/complex_markdown.md")
	if err != nil {
		t.Skip("complex_markdown.md not found, skip")
	}

	p, err := Parse(string(content))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	foundDiagram := false
	foundVega := false
	for _, b := range p.Blocks {
		if b.Kind == "diagram" {
			if b.Language == "vega-lite" {
				foundVega = strings.Contains(b.Text, "https://vega.github.io/schema/vega-lite/v5.json")
				continue
			}
			foundDiagram = true
			if !strings.Contains(b.Text, "条件判断") {
				t.Fatalf("diagram text corrupted: %s", b.Text)
			}
			t.Logf("Found normalized diagram block: %q", b.Text)
		}
	}
	if !foundDiagram {
		t.Fatal("complex_markdown.md did not produce a diagram block")
	}

	if !foundVega {
		t.Fatal("plain official Vega-Lite schema was not recognized")
	}
	mathCount := 0
	for _, b := range p.Blocks {
		if b.Kind == "math" {
			mathCount++
			t.Logf("Found math block: %q", b.Text)
		}
		for _, s := range b.Spans {
			if s.Kind == "math" {
				t.Logf("Found math span: %q", s.Text)
			}
		}
	}
	t.Logf("Total math blocks: %d", mathCount)

	imgs, errs := PreviewResources(p, "/Users/mima1234/Downloads/complex_markdown.md")
	t.Logf("Decoded images: %d, errors: %d", len(imgs), len(errs))
	for k := range errs {
		t.Errorf("Resource decoding error for %s: %s", k, errs[k])
	}
	if len(imgs) != 5 {
		t.Fatalf("expected 3 formulas, Mermaid and Vega-Lite, got %d resources", len(imgs))
	}
}
