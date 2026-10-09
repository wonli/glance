package docx

import (
	"bytes"
	"glance/internal/document"
	"path/filepath"
	"strings"
	"testing"
)

func TestTextFormatPatchKeepsUnknownRunProperties(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	originalPackage, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	runID, _ := findRunByText(t, doc.WordBlocks(), "bold")
	editor := requireWordTextEditor(t, doc)
	no := false
	yes := true
	color := "#112233"
	if err := editor.SetTextRunStyle(runID, document.WordTextFormatPatch{Bold: &no, Italic: &yes, Underline: &yes, FontColor: &color}); err != nil {
		t.Fatal(err)
	}
	style := doc.WordBlocks()[0].Paragraph.Inlines[0].Text.Style
	if style.Bold || !style.Italic || !style.Underline || style.FontColor != "112233" {
		t.Fatalf("updated style = %#v", style)
	}
	data, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	documentXML := readZipPart(t, data, "word/document.xml")
	for _, untouched := range [][]byte{[]byte("w:vanish"), []byte(`w:ascii="Aptos"`), []byte("w:keepNext")} {
		if !bytes.Contains(documentXML, untouched) {
			t.Errorf("format patch removed unrelated property %s: %s", untouched, documentXML)
		}
	}
	if !bytes.Contains(documentXML, []byte(`w:val="112233"`)) {
		t.Fatalf("font color patch was not written: %s", documentXML)
	}
	output := filepath.Join(t.TempDir(), "formatted.docx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedStyle := reopened.WordBlocks()[0].Paragraph.Inlines[0].Text.Style
	if reopenedStyle.Bold || !reopenedStyle.Italic || !reopenedStyle.Underline || reopenedStyle.FontColor != "112233" {
		t.Fatalf("saved run style = %#v", reopenedStyle)
	}
	assertPackagePartsEqual(t, originalPackage, output)
}

func TestSetTextRunHighlightWritesStandardColorAndCanClearIt(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	originalPackage, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	runID, _ := findRunByText(t, doc.WordBlocks(), "bold")
	editor := requireWordTextEditor(t, doc)
	yellow := "yellow"
	if err := editor.SetTextRunStyle(runID, document.WordTextFormatPatch{Highlight: &yellow}); err != nil {
		t.Fatal(err)
	}
	style := doc.WordBlocks()[0].Paragraph.Inlines[0].Text.Style
	if style.Highlight != "yellow" || !style.Bold || style.FontColor != "AABBCC" {
		t.Fatalf("highlight update = %#v; want yellow highlight and preserved bold/font color", style)
	}
	snapshot, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	documentXML := readZipPart(t, snapshot, "word/document.xml")
	if !bytes.Contains(documentXML, []byte(`<w:highlight w:val="yellow"/>`)) {
		t.Fatalf("highlight color was not written: %s", documentXML)
	}
	output := filepath.Join(t.TempDir(), "highlight.docx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedStyle := reopened.WordBlocks()[0].Paragraph.Inlines[0].Text.Style
	if reopenedStyle.Highlight != "yellow" || !reopenedStyle.Bold || reopenedStyle.FontColor != "AABBCC" {
		t.Fatalf("saved run style = %#v; want yellow highlight with existing properties", reopenedStyle)
	}
	assertPackagePartsEqual(t, originalPackage, output)
	none := ""
	if err := requireWordTextEditor(t, reopened).SetTextRunStyle(runID, document.WordTextFormatPatch{Highlight: &none}); err != nil {
		t.Fatal(err)
	}
	style = reopened.WordBlocks()[0].Paragraph.Inlines[0].Text.Style
	if style.Highlight != "" || !style.Bold || style.FontColor != "AABBCC" {
		t.Fatalf("clearing highlight changed unrelated style: %#v", style)
	}
	snapshot, err = reopened.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	documentXML = readZipPart(t, snapshot, "word/document.xml")
	if bytes.Contains(documentXML, []byte("<w:highlight")) {
		t.Fatalf("clearing highlight left a w:highlight property: %s", documentXML)
	}
}

func TestSetTextRunHighlightRejectsUnsupportedValues(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	runID, _ := findRunByText(t, doc.WordBlocks(), "bold")
	for _, value := range []string{"#FFFF00", "themeAccent1", "transparent"} {
		if err := requireWordTextEditor(t, doc).SetTextRunStyle(runID, document.WordTextFormatPatch{Highlight: &value}); err == nil {
			t.Errorf("unsupported Word highlight value %q was accepted", value)
		}
		if doc.Dirty() {
			t.Errorf("unsupported highlight value %q dirtied the document", value)
		}
	}
}

func TestSetTextRunFontSizeRoundTripsAndPreservesOtherRunProperties(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	originalPackage, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	runID, _ := findRunByText(t, doc.WordBlocks(), "bold")
	if err := requireWordTextFontSizeEditor(t, doc).SetTextRunFontSize(runID, 14); err != nil {
		t.Fatal(err)
	}
	style := doc.WordBlocks()[0].Paragraph.Inlines[0].Text.Style
	if style.FontSize != 14 || !style.Bold || style.FontColor != "AABBCC" {
		t.Fatalf("updated run style = %#v; want 14pt while preserving bold and font color", style)
	}
	if got := doc.WordBlocks()[0].Paragraph.Inlines[1].Text.Style.FontSize; got != 0 {
		t.Fatalf("untouched run font size = %g; want no explicit size", got)
	}
	snapshot, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	documentXML := readZipPart(t, snapshot, "word/document.xml")
	for _, preserved := range [][]byte{[]byte(`w:val="28"`), []byte("w:vanish"), []byte(`w:ascii="Aptos"`), []byte("w:keepNext")} {
		if !bytes.Contains(documentXML, preserved) {
			t.Errorf("font-size edit did not write or preserve %s: %s", preserved, documentXML)
		}
	}
	output := filepath.Join(t.TempDir(), "font-size.docx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedStyle := reopened.WordBlocks()[0].Paragraph.Inlines[0].Text.Style
	if reopenedStyle.FontSize != 14 || !reopenedStyle.Bold || reopenedStyle.FontColor != "AABBCC" {
		t.Fatalf("saved run style = %#v; want 14pt with preserved existing style", reopenedStyle)
	}
	assertPackagePartsEqual(t, originalPackage, output)
}

func TestSetTextRunFontFamilyRoundTripsAndPreservesOtherRunProperties(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	originalPackage, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	runID, _ := findRunByText(t, doc.WordBlocks(), "bold")
	const family = "Noto Sans CJK SC"
	if err := requireWordTextFontFamilyEditor(t, doc).SetTextRunFontFamily(runID, family); err != nil {
		t.Fatal(err)
	}
	style := doc.WordBlocks()[0].Paragraph.Inlines[0].Text.Style
	if style.FontFamily != family || !style.Bold || style.FontColor != "AABBCC" {
		t.Fatalf("updated run style = %#v; want font family %q while preserving bold and font color", style, family)
	}
	snapshot, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	documentXML := readZipPart(t, snapshot, "word/document.xml")
	for _, preserved := range [][]byte{
		[]byte(`w:ascii="Noto Sans CJK SC"`),
		[]byte(`w:hAnsi="Noto Sans CJK SC"`),
		[]byte(`w:eastAsia="Noto Sans CJK SC"`),
		[]byte(`w:cs="Noto Sans CJK SC"`),
		[]byte("w:vanish"),
		[]byte("w:keepNext"),
	} {
		if !bytes.Contains(documentXML, preserved) {
			t.Errorf("font-family edit did not write or preserve %s: %s", preserved, documentXML)
		}
	}
	output := filepath.Join(t.TempDir(), "font-family.docx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedStyle := reopened.WordBlocks()[0].Paragraph.Inlines[0].Text.Style
	if reopenedStyle.FontFamily != family || !reopenedStyle.Bold || reopenedStyle.FontColor != "AABBCC" {
		t.Fatalf("saved run style = %#v; want font family %q with existing style", reopenedStyle, family)
	}
	assertPackagePartsEqual(t, originalPackage, output)
}

func TestSetTextRunFontFamilyAddsFontPropertiesAndEscapesValues(t *testing.T) {
	documentXML := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:rPr><w:i/><w:rFonts w:asciiTheme="minorHAnsi" w:eastAsiaTheme="eastAsia" w:hint="eastAsia"/></w:rPr><w:t>sample</w:t></w:r><w:r><w:rPr><w:b/></w:rPr><w:t> no fonts</w:t></w:r></w:p></w:body></w:document>`
	doc := projectionFixture(t, documentXML, "", nil)
	defer doc.Close()
	blocks := doc.WordBlocks()
	firstRunID := blocks[0].Paragraph.Inlines[0].Text.ID
	secondRunID := blocks[0].Paragraph.Inlines[1].Text.ID
	const family = `A&B "Quoted"`
	editor := requireWordTextFontFamilyEditor(t, doc)
	if err := editor.SetTextRunFontFamily(firstRunID, family); err != nil {
		t.Fatal(err)
	}
	if err := editor.SetTextRunFontFamily(secondRunID, "Noto Sans CJK SC"); err != nil {
		t.Fatal(err)
	}
	updated := doc.WordBlocks()[0].Paragraph.Inlines
	if got := updated[0].Text.Style.FontFamily; got != family || !updated[0].Text.Style.Italic {
		t.Fatalf("font-property update = %#v; want escaped family and preserved italic", updated[0].Text.Style)
	}
	if got := updated[1].Text.Style.FontFamily; got != "Noto Sans CJK SC" || !updated[1].Text.Style.Bold {
		t.Fatalf("missing-font-property update = %#v; want family and preserved bold", updated[1].Text.Style)
	}
	snapshot, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	outputXML := readZipPart(t, snapshot, "word/document.xml")
	for _, want := range [][]byte{
		[]byte(`w:ascii="A&amp;B &quot;Quoted&quot;"`),
		[]byte(`w:eastAsia="A&amp;B &quot;Quoted&quot;"`),
		[]byte(`w:hint="eastAsia"`),
		[]byte(`w:ascii="Noto Sans CJK SC"`),
		[]byte("w:i"),
		[]byte("w:b"),
	} {
		if !bytes.Contains(outputXML, want) {
			t.Errorf("font-family XML missing %s: %s", want, outputXML)
		}
	}
	for _, removed := range [][]byte{[]byte("w:asciiTheme"), []byte("w:eastAsiaTheme")} {
		if bytes.Contains(outputXML, removed) {
			t.Errorf("font-family override retained conflicting theme property %s: %s", removed, outputXML)
		}
	}
}

func TestSetTextRunFontFamilyRejectsInvalidNamesWithoutDirtyingDocument(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	runID, _ := findRunByText(t, doc.WordBlocks(), "bold")
	if doc.Dirty() {
		t.Fatal("fixture should begin clean")
	}
	for _, family := range []string{"", " \t ", strings.Repeat("x", 129), "bad\nfont"} {
		if err := requireWordTextFontFamilyEditor(t, doc).SetTextRunFontFamily(runID, family); err == nil {
			t.Errorf("font family %q was accepted", family)
		}
		if doc.Dirty() {
			t.Errorf("invalid font family %q dirtied the document", family)
		}
	}
}
