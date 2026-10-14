package docx

import (
	"bytes"
	"glance/internal/document"
	"path/filepath"
	"strings"
	"testing"
)

func TestNumberedListsCorpusProjectsDecimalAndAlphabeticMarkers(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "office-corpus", "docx", "numbered-lists.docx")
	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	want := []string{"1.", "2.", "a.", "b.", "c.", "3.", "4.", "1.", "2.", "3.", "10.", "11.", "12.", "13."}
	var markers []string
	for _, block := range doc.WordBlocks() {
		if block.Paragraph == nil || block.Paragraph.ListKind == document.WordListNone {
			continue
		}
		paragraph := block.Paragraph
		if paragraph.ListKind == document.WordListUnsupported {
			t.Errorf("paragraph %q has unsupported numbering", paragraph.ID)
		}
		markers = append(markers, paragraph.ListMarker)
	}
	if len(markers) != len(want) {
		t.Fatalf("list markers = %#v; want %#v", markers, want)
	}
	for index := range want {
		if markers[index] != want[index] {
			t.Errorf("list marker %d = %q; want %q", index, markers[index], want[index])
		}
	}
}

func TestUnsupportedNumberingCorpusRetainsParagraphTextAndMarksListUnsupported(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "office-corpus", "docx", "unsupported-numbering.docx")
	doc, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	var unsupportedCount, supportedCount int
	for _, block := range doc.WordBlocks() {
		if block.Paragraph == nil {
			continue
		}
		if block.Paragraph.ListKind == document.WordListBullet || block.Paragraph.ListKind == document.WordListNumbered {
			supportedCount++
			continue
		}
		if block.Paragraph.ListKind != document.WordListUnsupported {
			continue
		}
		unsupportedCount++
		if got := paragraphText(*block.Paragraph); got == "" {
			t.Errorf("unsupported list paragraph %q lost its text", block.Paragraph.ID)
		}
	}
	if unsupportedCount == 0 {
		t.Fatal("corpus fixture did not project an unsupported numbering definition")
	}
	if supportedCount == 0 {
		t.Fatal("corpus fixture lost all supported list definitions; no false-positive control remains")
	}
}

func paragraphText(paragraph document.WordParagraph) string {
	var text strings.Builder
	for _, inline := range paragraph.Inlines {
		if inline.Text != nil {
			text.WriteString(inline.Text.Text)
		}
	}
	return text.String()
}

func TestWordParagraphListsRoundTripContinuityAndPreserveOtherPackageParts(t *testing.T) {
	documentXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:pPr><w:keepNext/></w:pPr><w:r><w:t>first</w:t></w:r></w:p><w:p><w:r><w:t>second</w:t></w:r></w:p><w:p><w:r><w:t>between</w:t></w:r></w:p><w:p><w:r><w:t>bullet</w:t></w:r></w:p><w:p><w:r><w:t>restart</w:t></w:r></w:p></w:body></w:document>`
	preservedPart := []byte("keep me byte-for-byte")
	doc := projectionFixture(t, documentXML, "", map[string][]byte{"custom/data.xml": preservedPart})
	defer doc.Close()
	listEditor, ok := any(doc).(document.WordParagraphListEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose paragraph list editing")
	}
	paragraphs := make([]document.WordParagraph, 0, 5)
	for _, block := range doc.WordBlocks() {
		paragraphs = append(paragraphs, *block.Paragraph)
	}
	if err := listEditor.SetParagraphList(paragraphs[0].ID, document.WordListNumbered); err != nil {
		t.Fatal(err)
	}
	paragraphs = paragraphs[:0]
	for _, block := range doc.WordBlocks() {
		paragraphs = append(paragraphs, *block.Paragraph)
	}
	if err := listEditor.SetParagraphList(paragraphs[1].ID, document.WordListNumbered); err != nil {
		t.Fatal(err)
	}
	paragraphs = paragraphs[:0]
	for _, block := range doc.WordBlocks() {
		paragraphs = append(paragraphs, *block.Paragraph)
	}
	if err := listEditor.SetParagraphList(paragraphs[3].ID, document.WordListBullet); err != nil {
		t.Fatal(err)
	}
	paragraphs = paragraphs[:0]
	for _, block := range doc.WordBlocks() {
		paragraphs = append(paragraphs, *block.Paragraph)
	}
	if err := listEditor.SetParagraphList(paragraphs[4].ID, document.WordListNumbered); err != nil {
		t.Fatal(err)
	}
	assertMarkers := func(blocks []document.WordBlock) {
		t.Helper()
		want := []struct {
			kind   document.WordListKind
			marker string
		}{{document.WordListNumbered, "1."}, {document.WordListNumbered, "2."}, {document.WordListNone, ""}, {document.WordListBullet, "•"}, {document.WordListNumbered, "1."}}
		for index, expected := range want {
			got := blocks[index].Paragraph
			if got.ListKind != expected.kind || got.ListMarker != expected.marker || got.ListLevel != 0 {
				t.Errorf("paragraph %d list = %q/%q level %d; want %q/%q level 0", index, got.ListKind, got.ListMarker, got.ListLevel, expected.kind, expected.marker)
			}
		}
	}
	assertMarkers(doc.WordBlocks())
	history, ok := any(doc).(document.WordHistory)
	if !ok {
		t.Fatal("DOCX adapter does not expose Word history")
	}
	if undone, err := history.Undo(); err != nil || !undone {
		t.Fatalf("undo paragraph list = %v, %v", undone, err)
	}
	if got := doc.WordBlocks()[4].Paragraph.ListKind; got != document.WordListNone {
		t.Fatalf("undo list kind = %q; want none", got)
	}
	if redone, err := history.Redo(); err != nil || !redone {
		t.Fatalf("redo paragraph list = %v, %v", redone, err)
	}
	assertMarkers(doc.WordBlocks())

	output := filepath.Join(t.TempDir(), "lists.docx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	assertMarkers(reopened.WordBlocks())
	snapshot, err := reopened.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(readZipPart(t, snapshot, "word/document.xml"), []byte(`<w:keepNext/>`)) {
		t.Fatal("list operation removed the untouched keepNext paragraph property")
	}
	if !bytes.Contains(readZipPart(t, snapshot, "word/_rels/document.xml.rels"), []byte("/numbering")) {
		t.Fatal("numbering relationship was not saved")
	}
	if !bytes.Contains(readZipPart(t, snapshot, "[Content_Types].xml"), []byte("word/numbering.xml")) {
		t.Fatal("numbering content type was not saved")
	}
	if got := readZipPart(t, snapshot, "custom/data.xml"); !bytes.Equal(got, preservedPart) {
		t.Fatalf("unrelated package part changed from %q to %q", preservedPart, got)
	}
}

func TestNestedWordNumberingCanChangeLevelAndRoundTrip(t *testing.T) {
	documentXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:pPr><w:numPr><w:ilvl w:val="1"/><w:numId w:val="7"/></w:numPr></w:pPr><w:r><w:t>nested</w:t></w:r></w:p></w:body></w:document>`
	relationships := `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId8" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering" Target="numbering.xml"/></Relationships>`
	numbering := []byte(`<w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:abstractNum w:abstractNumId="3"><w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1."/></w:lvl><w:lvl w:ilvl="1"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1.%2."/></w:lvl></w:abstractNum><w:num w:numId="7"><w:abstractNumId w:val="3"/></w:num></w:numbering>`)
	doc := projectionFixture(t, documentXML, relationships, map[string][]byte{"word/numbering.xml": numbering})
	defer doc.Close()
	paragraph := doc.WordBlocks()[0].Paragraph
	if paragraph.ListKind != document.WordListNumbered || paragraph.ListLevel != 1 || paragraph.ListMaxLevel != 1 || paragraph.ListMarker != "1.1." {
		t.Fatalf("nested list projection = %q level %d max %d marker %q; want editable level 1 of 1, marker 1.1.", paragraph.ListKind, paragraph.ListLevel, paragraph.ListMaxLevel, paragraph.ListMarker)
	}
	listEditor, ok := any(doc).(document.WordParagraphListEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose paragraph list level editing")
	}
	if err := listEditor.SetParagraphListLevel(paragraph.ID, 0); err != nil {
		t.Fatal(err)
	}
	if got := doc.WordBlocks()[0].Paragraph; got.ListLevel != 0 || got.ListMarker != "1." {
		t.Fatalf("outdented numbered list = level %d marker %q; want level 0 marker 1.", got.ListLevel, got.ListMarker)
	}
	if err := listEditor.SetParagraphListLevel(paragraph.ID, 2); err == nil {
		t.Fatal("level outside the existing abstract numbering definition should be rejected")
	}
	output := filepath.Join(t.TempDir(), "nested-list.docx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got := reopened.WordBlocks()[0].Paragraph; got.ListKind != document.WordListNumbered || got.ListLevel != 0 || got.ListMarker != "1." {
		t.Fatalf("saved/reopened list = %q level %d marker %q; want decimal level 0 marker 1.", got.ListKind, got.ListLevel, got.ListMarker)
	}
}

func TestNestedWordNumberingRestartsAfterParentLevel(t *testing.T) {
	documentXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="7"/></w:numPr></w:pPr><w:r><w:t>one</w:t></w:r></w:p><w:p><w:pPr><w:numPr><w:ilvl w:val="1"/><w:numId w:val="7"/></w:numPr></w:pPr><w:r><w:t>one one</w:t></w:r></w:p><w:p><w:pPr><w:numPr><w:ilvl w:val="1"/><w:numId w:val="7"/></w:numPr></w:pPr><w:r><w:t>one two</w:t></w:r></w:p><w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="7"/></w:numPr></w:pPr><w:r><w:t>two</w:t></w:r></w:p><w:p><w:pPr><w:numPr><w:ilvl w:val="1"/><w:numId w:val="7"/></w:numPr></w:pPr><w:r><w:t>two one</w:t></w:r></w:p></w:body></w:document>`
	relationships := `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId8" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering" Target="numbering.xml"/></Relationships>`
	numbering := []byte(`<w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:abstractNum w:abstractNumId="3"><w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1."/></w:lvl><w:lvl w:ilvl="1"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1.%2."/></w:lvl></w:abstractNum><w:num w:numId="7"><w:abstractNumId w:val="3"/></w:num></w:numbering>`)
	doc := projectionFixture(t, documentXML, relationships, map[string][]byte{"word/numbering.xml": numbering})
	defer doc.Close()
	want := []string{"1.", "1.1.", "1.2.", "2.", "2.1."}
	for index, block := range doc.WordBlocks() {
		if got := block.Paragraph.ListMarker; got != want[index] {
			t.Errorf("nested marker %d = %q; want %q", index, got, want[index])
		}
	}
}

func TestCustomWordNumberingRestartRemainsReadOnly(t *testing.T) {
	documentXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="7"/></w:numPr></w:pPr><w:r><w:t>custom restart</w:t></w:r></w:p></w:body></w:document>`
	relationships := `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId8" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering" Target="numbering.xml"/></Relationships>`
	numbering := []byte(`<w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:abstractNum w:abstractNumId="3"><w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1."/></w:lvl></w:abstractNum><w:num w:numId="7"><w:abstractNumId w:val="3"/><w:lvlOverride w:ilvl="0"><w:startOverride w:val="8"/></w:lvlOverride></w:num></w:numbering>`)
	doc := projectionFixture(t, documentXML, relationships, map[string][]byte{"word/numbering.xml": numbering})
	defer doc.Close()
	paragraph := doc.WordBlocks()[0].Paragraph
	if paragraph.ListKind != document.WordListUnsupported {
		t.Fatalf("custom restart list kind = %q; want read-only unsupported", paragraph.ListKind)
	}
	if err := doc.SetParagraphList(paragraph.ID, document.WordListBullet); err == nil {
		t.Fatal("custom numbering restart should not be overwritten")
	}
}

func TestWordParagraphListCanBeRemovedWithoutDroppingOtherProperties(t *testing.T) {
	documentXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:pPr><w:keepNext/></w:pPr><w:r><w:t>remove list</w:t></w:r></w:p></w:body></w:document>`
	doc := projectionFixture(t, documentXML, "", nil)
	defer doc.Close()
	listEditor, ok := any(doc).(document.WordParagraphListEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose paragraph list editing")
	}
	paragraph := doc.WordBlocks()[0].Paragraph
	if err := listEditor.SetParagraphList(paragraph.ID, document.WordListBullet); err != nil {
		t.Fatal(err)
	}
	paragraph = doc.WordBlocks()[0].Paragraph
	if err := listEditor.SetParagraphList(paragraph.ID, document.WordListNone); err != nil {
		t.Fatal(err)
	}
	got := doc.WordBlocks()[0].Paragraph
	if got.ListKind != document.WordListNone || got.ListMarker != "" {
		t.Fatalf("removed list remains projected as %q marker %q", got.ListKind, got.ListMarker)
	}
	snapshot, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	savedXML := readZipPart(t, snapshot, "word/document.xml")
	if !bytes.Contains(savedXML, []byte(`<w:keepNext/>`)) || bytes.Contains(savedXML, []byte(`<w:numPr`)) {
		t.Fatalf("remove list changed unrelated paragraph properties: %s", savedXML)
	}
}
