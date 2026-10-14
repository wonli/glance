package docx

import (
	"bytes"
	"glance/internal/document"
	"strings"
	"testing"
)

func TestLegacyImportRestoresSourceGridAndSeparatesFooterOnReopen(t *testing.T) {
	d := wordLayoutProjectionFixture(t, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:pPr><w:keepNext/><w:ind w:firstLine="480"/></w:pPr><w:r><w:t>正文</w:t></w:r></w:p><w:p><w:r><w:t>签章</w:t></w:r></w:p><w:p><w:r><w:t>PAGE 1</w:t></w:r></w:p><w:sectPr><w:pgSz w:w="12240" w:h="15840"/><w:pgMar w:top="1440" w:bottom="1440"/></w:sectPr></w:body></w:document>`, "")
	l := LegacyImportLayout{Page: document.WordPageSetup{WidthPT: 595.3, HeightPT: 841.9, Set: document.WordPageWidthSet | document.WordPageHeightSet}, Grid: document.WordDocumentGrid{Type: "lines", LinePitchPT: 15.6}, Paragraphs: []LegacyImportParagraph{{Text: "正文", Layout: document.WordParagraphLayout{LineSpacingRule: "auto", LineSpacingMultiple: 1.5, Set: document.WordParagraphLineSpacingSet}}, {Text: "签章"}}, Stories: []LegacyImportStory{{Kind: "footer", Type: "default", Text: "\x13PAGE \x141\x15\r"}}}
	restored, err := d.RestoreLegacyImportLayout(l)
	if err != nil || !restored {
		t.Fatalf("restore=%t err=%v", restored, err)
	}
	snapshot, err := d.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if len(reopened.WordBlocks()) != 2 {
		t.Fatalf("body blocks=%d; want only source main-story paragraphs", len(reopened.WordBlocks()))
	}
	sec := reopened.WordLayoutData().Sections[0]
	if sec.PageSetup.WidthPT != 595.3 || sec.Grid.LinePitchPT != 15.6 {
		t.Fatalf("restored section=%+v", sec)
	}
	p := reopened.WordBlocks()[0].Paragraph.Layout
	if p.LineSpacingMultiple != 1.5 || !p.KeepWithNext || p.FirstLineIndentEMU != 304800 {
		t.Fatalf("preserved paragraph layout=%+v", p)
	}
	found := false
	for _, part := range reopened.parts {
		if strings.HasPrefix(part.header.Name, "word/legacy-footer-") {
			found = true
			if !bytes.Contains(part.data, []byte("instrText")) || !bytes.Contains(part.data, []byte("fldChar")) {
				t.Fatal("footer field instruction/result was flattened")
			}
		}
	}
	if !found {
		t.Fatal("source footer was removed instead of retained in a footer part")
	}
}

func TestLegacyImportTextMismatchIsNoOp(t *testing.T) {
	d := wordLayoutProjectionFixture(t, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>converted text</w:t></w:r></w:p><w:sectPr/></w:body></w:document>`, "")
	before, err := d.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := d.RestoreLegacyImportLayout(LegacyImportLayout{Paragraphs: []LegacyImportParagraph{{Text: "different source text"}}})
	if err != nil || restored {
		t.Fatalf("mismatch restore=%t err=%v", restored, err)
	}
	after, _ := d.Snapshot()
	if !bytes.Equal(before, after) || d.Dirty() {
		t.Fatal("unverified source boundary changed the converted document")
	}
}

func TestLegacyStoryNestedFieldsPreserveOuterInstruction(t *testing.T) {
	xml := string(legacyStoryXML(LegacyImportStory{Kind: "footer", Text: "\x13IF \x13PAGE\x141\x15 = \x13NUMPAGES\x142\x15 yes no\x14no\x15\r"}))
	for _, text := range []string{"IF ", "PAGE", " = ", "NUMPAGES", " yes no"} {
		if !strings.Contains(xml, `<w:instrText xml:space="preserve">`+text+`</w:instrText>`) {
			t.Fatalf("lost instruction %q: %s", text, xml)
		}
	}
	for _, text := range []string{"1", "2", "no"} {
		if !strings.Contains(xml, `<w:t xml:space="preserve">`+text+`</w:t>`) {
			t.Fatalf("lost cached result %q: %s", text, xml)
		}
	}
}

func TestLegacyImportRestoresUniformSourceFontsWithoutChangingOtherRunProperties(t *testing.T) {
	d := wordLayoutProjectionFixture(t, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:rPr><w:b/><w:rFonts w:ascii="Times New Roman"/><w:sz w:val="24"/><w:color w:val="123456"/></w:rPr><w:t>地址：</w:t></w:r><w:r><w:t xml:space="preserve">  联系电话：</w:t></w:r></w:p><w:sectPr/></w:body></w:document>`, "")
	ok, err := d.RestoreLegacyImportLayout(LegacyImportLayout{Paragraphs: []LegacyImportParagraph{{Text: "地址：  联系电话：", Fonts: document.WordTextStyle{FontFamilyASCII: "黑体", FontFamilyEastAsia: "黑体"}}}})
	if err != nil || !ok {
		t.Fatalf("repair=%t err=%v", ok, err)
	}
	snapshot, err := d.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	runs := reopened.WordBlocks()[0].Paragraph.Inlines
	for _, r := range runs {
		if r.Text.Style.FontFamilyASCII != "黑体" || r.Text.Style.FontFamilyEastAsia != "黑体" {
			t.Fatalf("font recovery=%+v", r.Text.Style)
		}
	}
	style := runs[0].Text.Style
	if !style.Bold || style.FontSize != 12 || style.FontColor != "123456" {
		t.Fatalf("unrelated run formatting changed: %+v", style)
	}
}

func TestLegacyImportFontsExpandEmptyRunProperties(t *testing.T) {
	d := wordLayoutProjectionFixture(t, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:rPr/><w:t>文字</w:t></w:r></w:p><w:sectPr/></w:body></w:document>`, "")
	ok, err := d.RestoreLegacyImportLayout(LegacyImportLayout{Paragraphs: []LegacyImportParagraph{{Text: "文字", Fonts: document.WordTextStyle{FontFamilyASCII: "黑体", FontFamilyEastAsia: "黑体"}}}})
	if err != nil || !ok {
		t.Fatal(err)
	}
	runs := d.WordBlocks()[0].Paragraph.Inlines
	if len(runs) != 1 || runs[0].Text == nil {
		t.Fatalf("invalid repaired inline: %+v", runs)
	}
	style := runs[0].Text.Style
	if style.FontFamilyEastAsia != "黑体" {
		t.Fatalf("recovered font is outside run properties: %+v", style)
	}
}

func TestLegacyImportDirectComplexFontRemovesTheme(t *testing.T) {
	d := wordLayoutProjectionFixture(t, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:rPr><w:rFonts w:cs="Arial" w:cstheme="minorBidi"/></w:rPr><w:t>文字</w:t></w:r></w:p><w:sectPr/></w:body></w:document>`, "")
	ok, err := d.RestoreLegacyImportLayout(LegacyImportLayout{Paragraphs: []LegacyImportParagraph{{Text: "文字", Fonts: document.WordTextStyle{FontFamilyComplexScript: "Amiri"}}}})
	if err != nil || !ok {
		t.Fatal(err)
	}
	root, err := parseWordXML(d.parts[d.documentIdx].data)
	if err != nil {
		t.Fatal(err)
	}
	fonts := firstChild(firstChild(firstChild(firstChild(firstChild(root, "body"), "p"), "r"), "rPr"), "rFonts")
	if attrValue(fonts, "cstheme") != "" || attrValue(fonts, "cs") != "Amiri" {
		t.Fatal("complex theme overrides recovered font")
	}
}

func TestLegacyFontRecoveryPreservesUnrelatedFontAttributes(t *testing.T) {
	d := wordLayoutProjectionFixture(t, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:x="urn:glance:test"><w:body><w:p><w:r><w:rPr><w:rFonts w:ascii="Times New Roman" w:hint="eastAsia" x:keep="yes"/></w:rPr><w:t>文字</w:t></w:r></w:p><w:sectPr/></w:body></w:document>`, "")
	ok, err := d.RestoreLegacyImportLayout(LegacyImportLayout{Paragraphs: []LegacyImportParagraph{{Text: "文字", Fonts: document.WordTextStyle{FontFamilyASCII: "黑体"}}}})
	if err != nil || !ok {
		t.Fatal(err)
	}
	root, err := parseWordXML(d.parts[d.documentIdx].data)
	if err != nil {
		t.Fatal(err)
	}
	fonts := firstChild(firstChild(firstChild(firstChild(firstChild(root, "body"), "p"), "r"), "rPr"), "rFonts")
	if attrValue(fonts, "keep") != "yes" || attrValue(fonts, "hint") != "eastAsia" {
		t.Fatal("unrelated font attributes were removed")
	}
}
