package docx

import (
	"glance/internal/document"
	"testing"
)

func TestWordLayoutProjectsDocumentGridAndSnapOverride(t *testing.T) {
	doc := wordLayoutProjectionFixture(t, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:pPr><w:snapToGrid w:val="0"/></w:pPr><w:r><w:t>body</w:t></w:r></w:p><w:sectPr><w:docGrid w:type="lines" w:linePitch="312"/></w:sectPr></w:body></w:document>`, "")
	grid := doc.WordLayoutData().Sections[0].Grid
	if grid.Type != "lines" || grid.LinePitchPT != 15.6 {
		t.Fatalf("grid=%+v; want lines/15.6pt", grid)
	}
	p := doc.WordBlocks()[0].Paragraph.Layout
	if p.Set&document.WordParagraphSnapToGridSet == 0 || p.SnapToGrid {
		t.Fatalf("snap override=%+v; want explicit false", p)
	}
}
