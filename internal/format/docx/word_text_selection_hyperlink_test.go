package docx

import (
	"bytes"
	"glance/internal/document"
	"strings"
	"testing"
)

type wordTextSelectionHyperlinkEditor interface {
	InsertHyperlinkForTextSelection(startRunID string, startOffset int, endRunID string, endOffset int, displayText, targetURL string) error
}

func insertHyperlinkForSelection(t *testing.T, d *Document, startID string, startOffset int, endID string, endOffset int, displayText, targetURL string) {
	t.Helper()
	editor, ok := any(d).(wordTextSelectionHyperlinkEditor)
	if !ok {
		t.Fatal("DOCX lacks text selection hyperlink editor")
	}
	if err := editor.InsertHyperlinkForTextSelection(startID, startOffset, endID, endOffset, displayText, targetURL); err != nil {
		t.Fatal(err)
	}
}

func TestInsertHyperlinkForPartialWordTextSelectionPreservesUnselectedTextAndUndo(t *testing.T) {
	d := selectionDeleteFixture(t, `<w:p><w:r><w:rPr><w:i/></w:rPr><w:t>甲A🌱B乙</w:t></w:r></w:p>`)
	defer d.Close()
	run := d.WordBlocks()[0].Paragraph.Inlines[0].Text
	before := append([]byte(nil), d.parts[d.documentIdx].data...)

	insertHyperlinkForSelection(t, d, run.ID, 1, run.ID, 4, "选中的链接", "https://example.com")

	xml := string(d.parts[d.documentIdx].data)
	prefix, link, suffix := strings.Index(xml, ">甲</w:t>"), strings.Index(xml, ">选中的链接</w:t>"), strings.LastIndex(xml, ">乙</w:t>")
	if prefix < 0 || link < 0 || suffix < 0 || !(prefix < link && link < suffix) {
		t.Fatalf("hyperlink did not preserve prefix/selection/suffix order: %s", xml)
	}
	if !strings.Contains(xml, `<w:hyperlink xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" r:id=`) || !strings.Contains(xml, `<w:i`) || !strings.Contains(xml, `<w:color w:val="0563C1"`) || !strings.Contains(xml, `<w:u w:val="single"`) {
		t.Fatalf("selected link is missing hyperlink or inherited/standard link formatting: %s", xml)
	}
	rels, ok := findPackagePart(d.parts, "word/_rels/document.xml.rels")
	if !ok || !strings.Contains(string(rels.data), `Target="https://example.com"`) {
		t.Fatalf("hyperlink relationship was not added: %s", rels.data)
	}
	snapshot, err := d.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSnapshot(snapshot)
	if err != nil {
		t.Fatalf("reopen linked DOCX snapshot: %v", err)
	}
	_ = reopened.Close()
	history := any(d).(document.WordHistory)
	if !history.CanUndo() {
		t.Fatal("hyperlink insertion did not create an undo record")
	}
	if ok, err := history.Undo(); err != nil || !ok {
		t.Fatalf("undo hyperlink: %v %v", ok, err)
	}
	if !bytes.Equal(before, d.parts[d.documentIdx].data) {
		t.Fatal("undo did not restore original Word XML")
	}
}

func TestInsertHyperlinkForWordTextSelectionAcrossRuns(t *testing.T) {
	d := selectionDeleteFixture(t, `<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>甲AB</w:t></w:r><w:r><w:rPr><w:i/></w:rPr><w:t>CD乙</w:t></w:r></w:p>`)
	defer d.Close()
	p := d.WordBlocks()[0].Paragraph
	first, last := p.Inlines[0].Text, p.Inlines[1].Text

	insertHyperlinkForSelection(t, d, first.ID, 1, last.ID, 2, "ABCD", "#Section_One")

	xml := string(d.parts[d.documentIdx].data)
	if !strings.Contains(xml, `<w:hyperlink w:anchor="Section_One">`) || !strings.Contains(xml, ">AB</w:t>") || !strings.Contains(xml, ">CD</w:t>") {
		t.Fatalf("cross-run selected text was not wrapped by one internal hyperlink: %s", xml)
	}
	if strings.Index(xml, ">甲</w:t>") > strings.Index(xml, `<w:hyperlink`) || strings.LastIndex(xml, ">乙</w:t>") < strings.Index(xml, ">CD</w:t>") {
		t.Fatalf("cross-run link changed unselected content order: %s", xml)
	}
	root, err := parseWordXML(d.parts[d.documentIdx].data)
	if err != nil {
		t.Fatal(err)
	}
	links := findNodesByLocalName(root, "hyperlink")
	if len(links) != 1 {
		t.Fatalf("internal hyperlink count = %d; want 1", len(links))
	}
	linkedRuns := make([]*wordXMLNode, 0)
	for _, child := range links[0].children {
		if child.name.Space == wordprocessingMLNamespace && child.name.Local == "r" {
			linkedRuns = append(linkedRuns, child)
		}
	}
	firstProps, secondProps := (*wordXMLNode)(nil), (*wordXMLNode)(nil)
	if len(linkedRuns) == 2 {
		firstProps, secondProps = firstChild(linkedRuns[0], "rPr"), firstChild(linkedRuns[1], "rPr")
	}
	if len(linkedRuns) != 2 || firstProps == nil || firstChild(firstProps, "b") == nil || secondProps == nil || firstChild(secondProps, "i") == nil {
		t.Fatalf("same-text hyperlink did not preserve both selected run styles: %s", xml)
	}
}

func TestInsertHyperlinkForWordTextSelectionRejectsUnsupportedContentAtomically(t *testing.T) {
	d := selectionDeleteFixture(t, `<w:p><w:r><w:t>甲A</w:t><w:tab/><w:t>B乙</w:t></w:r></w:p>`)
	defer d.Close()
	inlines := d.WordBlocks()[0].Paragraph.Inlines
	if len(inlines) < 3 || inlines[0].Text == nil || inlines[2].Text == nil {
		t.Fatalf("unexpected fixture inlines: %#v", inlines)
	}
	before := append([]byte(nil), d.parts[d.documentIdx].data...)
	editor, ok := any(d).(wordTextSelectionHyperlinkEditor)
	if !ok {
		t.Fatal("DOCX lacks text selection hyperlink editor")
	}
	if err := editor.InsertHyperlinkForTextSelection(inlines[0].Text.ID, 1, inlines[2].Text.ID, 1, "AB", "https://example.com"); err == nil {
		t.Fatal("hyperlink insertion accepted selected content containing a tab")
	}
	if !bytes.Equal(before, d.parts[d.documentIdx].data) {
		t.Fatal("rejected hyperlink insertion changed document XML")
	}
	if history := any(d).(document.WordHistory); history.CanUndo() {
		t.Fatal("rejected hyperlink insertion created an undo record")
	}
}

func findNodesByLocalName(root *wordXMLNode, name string) []*wordXMLNode {
	var result []*wordXMLNode
	var visit func(*wordXMLNode)
	visit = func(node *wordXMLNode) {
		if node.name.Space == wordprocessingMLNamespace && node.name.Local == name {
			result = append(result, node)
		}
		for _, child := range node.children {
			visit(child)
		}
	}
	visit(root)
	return result
}
