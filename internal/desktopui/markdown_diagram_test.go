package desktopui

import (
	"context"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/SalvioniDigitalSolutions/gopdf"
	"glance/internal/diagramrender"
	"glance/internal/document"
	"glance/internal/format/markdowndoc"
	"image"
	"os"
	"path/filepath"
	"testing"
)

func TestMarkdownDiagramPreviewLogicalSize(t *testing.T) {
	a := markdownTestApp(t)
	b := document.MarkdownBlock{Kind: "diagram", Language: "mermaid", Text: "graph TD;A-->B"}
	s := markdowndoc.DiagramSpan(b)
	a.markdownView.images = map[string]image.Image{markdowndoc.ResourceKey(s): document.ResourceImage{Image: image.NewRGBA(image.Rect(0, 0, 200, 100)), Size: image.Pt(100, 50)}}
	for _, scale := range []float32{1, 2} {
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Constraints{Max: image.Pt(1000, 1000)}}
		d := a.layoutMarkdownBlock(gtx, b, 0)
		if d.Size != image.Pt(int(100*scale), int(50*scale)) {
			t.Fatalf("scale %g: %v", scale, d.Size)
		}
	}
}

func TestMarkdownDiagramPDFKeepsFrozenResources(t *testing.T) {
	path := os.Getenv("GLANCE_TEST_DIAGRAM_RUNTIME")
	if path == "" {
		t.Skip("set explicit offline runtime for actual PDF acceptance")
	}
	bundle, e := diagramrender.ResolveBundle(filepath.Join(path, "..", "glance-office"))
	if e != nil {
		t.Fatal(e)
	}
	renderer := diagramrender.New(bundle)
	defer renderer.Close()
	loader := markdowndoc.NewResourceLoader(func() (*diagramrender.Renderer, error) { return renderer, nil })
	dir := t.TempDir()
	source := filepath.Join(dir, "source.md")
	svg := filepath.Join(dir, "flow.svg")
	os.WriteFile(svg, []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="160" height="80"><rect width="160" height="80" fill="#e5edff"/><text x="10" y="40" font-size="20">离线图表</text></svg>`), 0600)
	text := "# 离线流程图\n\n```mermaid\nflowchart LR\n A[开始] --> B{检查}\n B -->|通过| C[完成]\n```\n\n![中文 SVG](flow.svg)\n"
	p, e := markdowndoc.Parse(text)
	if e != nil {
		t.Fatal(e)
	}
	resources, errors := loader.PreviewResources(context.Background(), p, source)
	if len(errors) != 0 || len(resources) != 2 {
		t.Fatalf("resources=%d errors=%v", len(resources), errors)
	}
	prepared, e := prepareMarkdownPDF(context.Background(), p, resources, errors)
	if e != nil {
		t.Fatal(e)
	}
	placements := 0
	for _, page := range prepared.scene.Pages {
		placements += len(page.Images)
	}
	if placements != 2 {
		t.Fatalf("PDF image placements=%d", placements)
	}
	renderer.Close()
	os.Remove(svg)
	out := os.Getenv("GLANCE_MARKDOWN_DIAGRAM_PDF")
	if out == "" {
		out = filepath.Join(dir, "diagram.pdf")
	}
	if e = exportPreparedMarkdownPDF(context.Background(), prepared, source, out, nil); e != nil {
		t.Fatal(e)
	}
	reader, e := gopdf.Open(out)
	if e != nil {
		t.Fatal(e)
	}
	if reader.NumPages() < 1 {
		t.Fatal("empty PDF")
	}
}
