package docx

import (
	"glance/internal/document"
	"testing"
)

func TestWordLayoutProjectionReadsParagraphBottomBorder(t *testing.T) {
	documentXML := `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:pPr><w:pBdr><w:bottom w:val="single" w:sz="8" w:space="5" w:color="4F81BD"/></w:pBdr></w:pPr><w:r><w:t>Title</w:t></w:r></w:p><w:sectPr><w:pgSz w:w="12240" w:h="15840"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/></w:sectPr></w:body></w:document>`
	doc := wordLayoutProjectionFixture(t, documentXML, "")
	blocks := doc.WordBlocks()
	if len(blocks) != 1 || blocks[0].Paragraph == nil {
		t.Fatalf("projected blocks = %#v; want one paragraph", blocks)
	}
	layout := blocks[0].Paragraph.Layout
	if layout.Set&document.WordParagraphBottomBorderSet == 0 || layout.BottomBorder != (document.WordParagraphBorder{Style: "single", SizePT: 1, SpacePT: 5, Color: "4F81BD"}) {
		t.Fatalf("paragraph bottom border = %+v mask=%b; want single 1pt blue line with 5pt spacing", layout.BottomBorder, layout.Set)
	}
}

func TestWordLayoutProjectionIgnoresZeroCharacterIndent(t *testing.T) {
	documentXML := `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:pPr><w:ind w:firstLine="0" w:firstLineChars="0"/></w:pPr><w:r><w:t>text</w:t></w:r></w:p><w:sectPr/></w:body></w:document>`
	doc := wordLayoutProjectionFixture(t, documentXML, "")
	data := requireWordLayoutReader(t, doc).WordLayoutData()
	for _, item := range data.Unsupported {
		if item.Code == "character-indent" {
			t.Fatalf("zero character indent reported as unsupported: %+v", item)
		}
	}
	paragraph := doc.WordBlocks()[0].Paragraph
	if paragraph.Layout.FirstLineIndentEMU != 0 || paragraph.Layout.Set&document.WordParagraphFirstLineIndentSet == 0 {
		t.Fatalf("zero first-line indent = %+v; want an explicitly set zero indent", paragraph.Layout)
	}
}

func TestWordLayoutProjectionReportsMalformedStylesPart(t *testing.T) {
	documentXML := `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>text</w:t></w:r></w:p><w:sectPr/></w:body></w:document>`
	stylesXML := `<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:docDefaults><w:rPrDefault></w:styles>`
	doc := wordLayoutProjectionFixture(t, documentXML, stylesXML)
	data := requireWordLayoutReader(t, doc).WordLayoutData()
	for _, item := range data.Unsupported {
		if item.Code == "invalid-styles" && item.SourceID == "word/styles.xml" {
			return
		}
	}
	t.Fatalf("malformed styles part has no located layout diagnostic: %+v", data.Unsupported)
}
