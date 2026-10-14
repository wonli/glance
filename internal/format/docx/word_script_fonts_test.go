package docx

import (
	"glance/internal/document"
	"testing"
)

func TestWordProjectionPreservesScriptFontFamilies(t *testing.T) {
	d := wordLayoutProjectionFixture(t, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:rPr><w:rFonts w:ascii="Times New Roman" w:hAnsi="Arial" w:eastAsia="黑体" w:cs="Amiri"/></w:rPr><w:t>A中 </w:t></w:r></w:p><w:sectPr/></w:body></w:document>`, "")
	r := d.WordBlocks()[0].Paragraph.Inlines[0].Text
	if r.Style.FontFamilyASCII != "Times New Roman" || r.Style.FontFamilyHighANSI != "Arial" || r.Style.FontFamilyEastAsia != "黑体" || r.Style.FontFamilyComplexScript != "Amiri" || r.StyleSet&document.WordTextFontASCIISet == 0 {
		t.Fatalf("script fonts lost: %+v mask=%x", r.Style, r.StyleSet)
	}
}

func TestFontFamilyEditUpdatesDifferentLatinSlot(t *testing.T) {
	d := wordLayoutProjectionFixture(t, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:rPr><w:rFonts w:ascii="Times New Roman" w:eastAsia="黑体"/></w:rPr><w:t>A中</w:t></w:r></w:p><w:sectPr/></w:body></w:document>`, "")
	r := d.WordBlocks()[0].Paragraph.Inlines[0].Text
	if err := d.SetTextRunFontFamily(r.ID, "黑体"); err != nil {
		t.Fatal(err)
	}
	if style := d.WordBlocks()[0].Paragraph.Inlines[0].Text.Style; style.FontFamilyASCII != "黑体" || style.FontFamilyEastAsia != "黑体" {
		t.Fatalf("Latin slot was left unchanged: %+v", style)
	}
}
