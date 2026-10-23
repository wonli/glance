package markdowndoc

import (
	"context"
	"glance/internal/diagramrender"
	"glance/internal/document"
	"glance/internal/nativewebview"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func diagramLoader(t *testing.T) *ResourceLoader {
	t.Helper()
	path := os.Getenv("GLANCE_TEST_DIAGRAM_RUNTIME")
	if path == "" {
		t.Skip("set GLANCE_TEST_DIAGRAM_RUNTIME for actual renderer")
	}
	b, e := diagramrender.ResolveBundle(filepath.Join(path, "..", "glance-office"))
	if e != nil {
		t.Fatal(e)
	}
	r := diagramrender.New(b)
	t.Cleanup(func() { r.Close() })
	return NewResourceLoader(func() (*diagramrender.Renderer, error) { return r, nil })
}
func TestLoadLocalSVGWithBOMAndComments(t *testing.T) {
	l := diagramLoader(t)
	d := t.TempDir()
	p := filepath.Join(d, "中文.svg")
	os.WriteFile(p, []byte("\xef\xbb\xbf<!-- test -->"+`<svg xmlns="http://www.w3.org/2000/svg" width="80" height="40"><rect width="80" height="40" fill="red"/></svg>`), 0600)
	i, e := l.LoadImage(context.Background(), "中文.svg", filepath.Join(d, "source.md"))
	if e != nil {
		t.Fatal(e)
	}
	if document.ResourceLogicalSize(i).X != 80 || i.Bounds().Dx() != 160 {
		t.Fatal(i.Bounds())
	}
}
func TestHTTPImageSniffsSVGRegardlessOfSuffix(t *testing.T) {
	l := diagramLoader(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="10"><rect width="20" height="10" fill="blue"/></svg>`))
	}))
	defer server.Close()
	i, e := l.LoadImage(context.Background(), server.URL+"/diagram.png", "")
	if e != nil || i.Bounds().Dx() != 40 {
		t.Fatalf("%v %v", i, e)
	}
}
func TestMermaidFenceBecomesDiagram(t *testing.T) {
	source := "```MeRmAiD\nflowchart TD\n A[开始] --> B[结束]\n```\n"
	p, e := Parse(source)
	if e != nil {
		t.Fatal(e)
	}
	if p.Source != source || len(p.Blocks) != 1 || p.Blocks[0].Kind != "diagram" || p.Blocks[0].Text != "flowchart TD\n A[开始] --> B[结束]\n" {
		t.Fatalf("%+v", p.Blocks)
	}
}
func TestMermaidResourcesDeduplicateAndCancel(t *testing.T) {
	l := diagramLoader(t)
	p, _ := Parse("```mermaid\ngraph TD;A-->B\n```\n\n```mermaid\ngraph TD;A-->B\n```\n")
	images, errs := l.PreviewResources(context.Background(), p, "")
	if len(errs) != 0 || len(images) != 1 {
		t.Fatalf("images=%d errors=%v", len(images), errs)
	}
	ctx, c := context.WithCancel(context.Background())
	c()
	images, _ = l.PreviewResources(ctx, p, "")
	if len(images) != 0 {
		t.Fatal("canceled job published image")
	}
}
func TestRasterImagesDoNotStartDiagramRuntime(t *testing.T) {
	l := NewResourceLoader(func() (*diagramrender.Renderer, error) { t.Fatal("raster started browser"); return nil, nil })
	_, e := l.decode(context.Background(), []byte("not image"))
	if e == nil || !strings.Contains(e.Error(), "图片") {
		t.Fatal(e)
	}
}

func TestMermaidExamplesRemainLiteral(t *testing.T) {
	for _, source := range []string{"    graph TD; A-->B\n", "```markdown\n```mermaid\ngraph TD;A-->B\n```\n", "```text\ngraph TD; A-->B\n```\n"} {
		p, e := Parse(source)
		if e != nil {
			t.Fatal(e)
		}
		if p.Source != source {
			t.Fatal("source mutated")
		}
		for _, b := range p.Blocks {
			if b.Kind == "diagram" {
				t.Fatal("literal example became diagram")
			}
		}
	}
}

func TestVegaResourceKeysPreserveLanguage(t *testing.T) {
	source := `{"mark":"point"}`
	vega := DiagramSpan(document.MarkdownBlock{Kind: "diagram", Language: "vega", Text: source})
	vegaLite := DiagramSpan(document.MarkdownBlock{Kind: "diagram", Language: "vega-lite", Text: source})
	if ResourceKey(vega) == ResourceKey(vegaLite) {
		t.Fatal("identical source under Vega and Vega-Lite shared a resource key")
	}
}

func TestVegaResourceKeysDistinguishFenceOccurrences(t *testing.T) {
	spec := `{"mark":"point"}`
	source := "```vega\n" + spec + "\n```\n\n```vega\n" + spec + "\n```\n"
	p, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Blocks) != 2 {
		t.Fatalf("parsed %d Vega blocks, want 2", len(p.Blocks))
	}
	first := ResourceKey(DiagramSpan(p.Blocks[0]))
	second := ResourceKey(DiagramSpan(p.Blocks[1]))
	if first == second {
		t.Fatalf("identical chart fences shared resource key %q", first)
	}
}

func TestVegaLiteFenceRendersInitialResource(t *testing.T) {
	if !nativewebview.PlatformSupported() {
		t.Skip("native diagram renderer is unavailable on this platform")
	}
	source := "```vega-lite\n" + `{"$schema":"https://vega.github.io/schema/vega-lite/v6.json","width":100,"height":60,"data":{"values":[{"x":"A","y":2}]},"mark":"bar","encoding":{"x":{"field":"x","type":"nominal"},"y":{"field":"y","type":"quantitative"}}}` + "\n```\n"
	p, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	images, diagnostics := PreviewResourcesContext(context.Background(), p, "")
	key := ResourceKey(DiagramSpan(p.Blocks[0]))
	if len(diagnostics) != 0 || images[key] == nil || images[key].Bounds().Empty() {
		t.Fatalf("Vega-Lite initial resource failed: image=%v diagnostics=%v", images[key], diagnostics)
	}
}

func TestSuppliedSVGWithForeignObjectLabels(t *testing.T) {
	l := diagramLoader(t)
	i, e := l.LoadImage(context.Background(), "testdata/complex-flowchart.svg", filepath.Join("source.md"))
	if e != nil {
		t.Fatal(e)
	}
	if i.Bounds().Dx() < 100 || document.ResourceLogicalSize(i).Y < 100 {
		t.Fatal(i.Bounds())
	}
}
