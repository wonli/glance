package docx

import (
	"glance/internal/document"
	"strings"
	"testing"
)

func TestMissingImageRelationshipRemainsVisibleAndCannotBeMutated(t *testing.T) {
	doc := projectionFixtureWithContentTypes(t,
		imageDocRoot(`<w:p>`+wordImageDrawing(1, "rIdMissing", 914400, 457200)+`</w:p>`),
		imageRelationshipRoot(), imageContentTypes(), nil)
	defer doc.Close()
	paragraph := doc.WordBlocks()[0].Paragraph
	block := paragraph.Inlines[0]
	if block.Kind != document.WordTextInline || block.Text == nil || block.Text.Editable || !strings.Contains(block.Text.Text, "preview unavailable") {
		t.Fatalf("missing relationship projection = %#v; want visible read-only fallback text", block)
	}
	if !strings.Contains(paragraph.PreviewWarning, "rIdMissing") {
		t.Fatalf("missing relationship warning = %q; want relationship detail", paragraph.PreviewWarning)
	}
	if err := requireWordImageEditor(t, doc).DeleteInlineImage(block.Text.ID); err == nil {
		t.Fatal("operation accepted an image with an unresolved relationship")
	}
	if doc.Dirty() {
		t.Fatal("missing relationship edit changed dirty state")
	}
}
