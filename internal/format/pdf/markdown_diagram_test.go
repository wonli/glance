package pdf

import (
	"glance/internal/document"
	"glance/internal/format/markdowndoc"
	"image"
	"strings"
	"testing"
)

func TestMarkdownDiagramPDFUsesLogicalGeometry(t *testing.T) {
	p, e := markdowndoc.Parse("```mermaid\ngraph TD; A-->B\n```\n")
	if e != nil {
		t.Fatal(e)
	}
	s := markdowndoc.DiagramSpan(p.Blocks[0])
	i := document.ResourceImage{Image: image.NewRGBA(image.Rect(0, 0, 200, 100)), Size: image.Pt(100, 50)}
	blocks, images, e := MarkdownWordBlocks(p, map[string]image.Image{markdowndoc.ResourceKey(s): i}, nil)
	if e != nil {
		t.Fatal(e)
	}
	if len(images) != 1 || len(blocks) != 1 || len(blocks[0].Paragraph.Inlines) != 1 {
		t.Fatalf("images=%d blocks=%+v", len(images), blocks)
	}
	img := blocks[0].Paragraph.Inlines[0].Image
	if img == nil || img.WidthEMU != 952500 || img.HeightEMU != 476250 {
		t.Fatalf("diagram geometry=%+v", img)
	}
}
func TestMarkdownDiagramPDFFailureKeepsPlaceholder(t *testing.T) {
	p, _ := markdowndoc.Parse("```mermaid\ngraph TD; A-->B\n```\n")
	s := markdowndoc.DiagramSpan(p.Blocks[0])
	blocks, _, e := MarkdownWordBlocks(p, nil, map[string]string{markdowndoc.ResourceKey(s): "synthetic syntax error"})
	if e != nil {
		t.Fatal(e)
	}
	text := blocks[0].Paragraph.Inlines[0].Text
	if text == nil || !strings.Contains(text.Text, "synthetic syntax error") {
		t.Fatalf("missing placeholder %+v", blocks)
	}
}
