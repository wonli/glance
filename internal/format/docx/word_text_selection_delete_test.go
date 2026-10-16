package docx

import (
	"bytes"
	"glance/internal/document"
	"testing"
)

func selectionDeleteFixture(t *testing.T, body string) *Document {
	t.Helper()
	return projectionFixture(t, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`+body+`</w:body></w:document>`, "", nil)
}

func deleteSelection(t *testing.T, d *Document, a string, x int, b string, y int) document.WordTextSelectionDeleteResult {
	t.Helper()
	editor, ok := any(d).(document.WordTextSelectionEditor)
	if !ok {
		t.Fatal("DOCX lacks selection editor")
	}
	result, err := editor.DeleteTextSelection(a, x, b, y)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestDeleteWordTextSelectionFormattedRunsAtomic(t *testing.T) {
	d := selectionDeleteFixture(t, `<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>甲🌺AB</w:t></w:r><w:r><w:rPr><w:i/></w:rPr><w:t>中间</w:t></w:r><w:r><w:rPr><w:u w:val="single"/></w:rPr><w:t>CD乙</w:t></w:r></w:p>`)
	defer d.Close()
	p := d.WordBlocks()[0].Paragraph
	before := append([]byte(nil), d.parts[d.documentIdx].data...)
	result := deleteSelection(t, d, p.Inlines[0].Text.ID, 2, p.Inlines[2].Text.ID, 2)
	p = d.WordBlocks()[0].Paragraph
	if p.Inlines[0].Text.Text != "甲🌺" || !p.Inlines[0].Text.Style.Bold || p.Inlines[1].Text.Text != "" || !p.Inlines[1].Text.Style.Italic || p.Inlines[2].Text.Text != "乙" {
		t.Fatalf("unexpected runs: %#v", p.Inlines)
	}
	if result.ParagraphID != p.ID || result.RunID != p.Inlines[0].Text.ID || result.Caret != 2 {
		t.Fatalf("caret: %#v", result)
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
	if p := reopened.WordBlocks()[0].Paragraph; p.Inlines[0].Text.Text != "甲🌺" || p.Inlines[2].Text.Text != "乙" || !p.Inlines[0].Text.Style.Bold {
		t.Fatal("snapshot lost selection edit")
	}
	if ok, err := d.Undo(); err != nil || !ok {
		t.Fatalf("undo: %v %v", ok, err)
	}
	if !bytes.Equal(before, d.parts[d.documentIdx].data) || d.CanUndo() {
		t.Fatal("delete was not one undo step")
	}
	if ok, err := d.Redo(); err != nil || !ok {
		t.Fatalf("redo: %v %v", ok, err)
	}
}

func TestDeleteWordTextSelectionAcrossParagraphs(t *testing.T) {
	d := selectionDeleteFixture(t, `<w:p><w:pPr><w:jc w:val="left"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>ABC</w:t></w:r></w:p><w:p><w:r><w:t>middle</w:t></w:r></w:p><w:p><w:pPr><w:jc w:val="right"/></w:pPr><w:r><w:rPr><w:i/></w:rPr><w:t>DEF</w:t></w:r></w:p>`)
	defer d.Close()
	blocks := d.WordBlocks()
	result := deleteSelection(t, d, blocks[0].Paragraph.Inlines[0].Text.ID, 1, blocks[2].Paragraph.Inlines[0].Text.ID, 2)
	blocks = d.WordBlocks()
	if len(blocks) != 1 {
		t.Fatalf("paragraphs = %d", len(blocks))
	}
	p := blocks[0].Paragraph
	if p.Alignment != document.WordAlignLeft || len(p.Inlines) != 3 || p.Inlines[0].Text.Text != "A" || p.Inlines[1].Text.Text != "" || p.Inlines[2].Text.Text != "F" || !p.Inlines[2].Text.Style.Italic || result.RunID != p.Inlines[0].Text.ID || result.Caret != 1 {
		t.Fatalf("merged paragraph/caret: %#v %#v", p, result)
	}
}

func TestDeleteWordTextSelectionRejectsUnsafeContent(t *testing.T) {
	for _, body := range []string{
		`<w:p><w:r><w:t>ABC</w:t></w:r><w:r><w:tab/></w:r><w:r><w:t>DEF</w:t></w:r></w:p>`,
		` <w:p><w:r><w:t>ABC</w:t></w:r><w:r><w:drawing/></w:r><w:r><w:t>DEF</w:t></w:r></w:p>`,
		`<w:p><w:r><w:t>ABC</w:t></w:r></w:p><w:p xmlns:z="urn:one"><w:r><w:t>middle</w:t></w:r></w:p><w:p xmlns:z="urn:two"><w:r><w:t>DEF</w:t></w:r></w:p>`,
		`<w:p><w:r><w:t>ABC</w:t></w:r></w:p><w:tbl/><w:p><w:r><w:t>DEF</w:t></w:r></w:p>`,
		`<w:p><w:pPr><w:sectPr/></w:pPr><w:r><w:t>ABC</w:t></w:r></w:p><w:p><w:r><w:t>DEF</w:t></w:r></w:p>`,
		`<w:tbl><w:tr><w:tc><w:p><w:r><w:t>ABC</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>DEF</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`,
	} {
		d := selectionDeleteFixture(t, body)
		var a, b string
		for id, target := range d.wordTextTargets {
			if target.text.text == "ABC" {
				a = id
			}
			if target.text.text == "DEF" {
				b = id
			}
		}
		before := append([]byte(nil), d.parts[d.documentIdx].data...)
		if _, err := any(d).(document.WordTextSelectionEditor).DeleteTextSelection(a, 1, b, 2); err == nil {
			t.Fatalf("accepted unsafe selection: %s", body)
		}
		if !bytes.Equal(before, d.parts[d.documentIdx].data) || d.CanUndo() || d.Dirty() {
			t.Fatal("rejected edit mutated document")
		}
		d.Close()
	}
}

func TestDeleteWordTextSelectionNoOpAndInvalidRange(t *testing.T) {
	d := selectionDeleteFixture(t, `<w:p><w:r><w:t>甲🌺乙</w:t></w:r></w:p>`)
	defer d.Close()
	id := d.WordBlocks()[0].Paragraph.Inlines[0].Text.ID
	deleteSelection(t, d, id, 1, id, 1)
	if d.Dirty() || d.CanUndo() {
		t.Fatal("no-op changed document")
	}
	editor := any(d).(document.WordTextSelectionEditor)
	for _, r := range [][2]int{{-1, 1}, {2, 1}, {1, 9}} {
		if _, err := editor.DeleteTextSelection(id, r[0], id, r[1]); err == nil {
			t.Fatalf("accepted invalid range %v", r)
		}
	}
	deleteSelection(t, d, id, 1, id, 2)
	if d.WordBlocks()[0].Paragraph.Inlines[0].Text.Text != "甲乙" {
		t.Fatal("same run rune deletion failed")
	}
}

func TestDeleteWordTextSelectionPreservesUnselectedContentAndMultipleTextNodes(t *testing.T) {
	d := selectionDeleteFixture(t, `<w:p><w:r><w:tab/><w:rPr><w:b/></w:rPr><w:t>甲🌺</w:t><w:t>middle</w:t><w:t>乙&amp;</w:t><w:br/></w:r></w:p>`)
	defer d.Close()
	p := d.WordBlocks()[0].Paragraph
	var runs []*document.WordTextRun
	for _, i := range p.Inlines {
		if i.Text != nil && i.Text.Editable {
			runs = append(runs, i.Text)
		}
	}
	deleteSelection(t, d, runs[0].ID, 1, runs[2].ID, 1)
	p = d.WordBlocks()[0].Paragraph
	if p.Inlines[0].Tab == nil || p.Inlines[1].Text.Text != "甲" || p.Inlines[2].Text.Text != "" || p.Inlines[3].Text.Text != "&" || p.Inlines[4].Text.Text != "\n" {
		t.Fatalf("unselected content or multiple text nodes lost: %#v", p.Inlines)
	}
}

func TestDeleteWordTextSelectionPreservesIntermediateNamespaceDeclarations(t *testing.T) {
	d := selectionDeleteFixture(t, `<w:p><w:r><w:t>ABC</w:t></w:r></w:p><w:p xmlns:z="urn:middle"><w:r><w:rPr><z:opaque/></w:rPr><w:t>middle</w:t></w:r></w:p><w:p><w:r><w:t>DEF</w:t></w:r></w:p>`)
	defer d.Close()
	b := d.WordBlocks()
	deleteSelection(t, d, b[0].Paragraph.Inlines[0].Text.ID, 1, b[2].Paragraph.Inlines[0].Text.ID, 1)
	root, err := parseWordXML(d.parts[d.documentIdx].data)
	if err != nil {
		t.Fatal(err)
	}
	opaque := findDescendant(root, "opaque")
	if opaque == nil || opaque.name.Space != "urn:middle" {
		t.Fatalf("intermediate namespace lost: %#v", opaque)
	}
}

func TestDeleteWordTextSelectionEmptyRunBoundaryDoesNotChangePackage(t *testing.T) {
	d := selectionDeleteFixture(t, `<w:p><w:r><w:t> A </w:t></w:r><w:r><w:t> B </w:t></w:r></w:p>`)
	defer d.Close()
	p := d.WordBlocks()[0].Paragraph
	before := append([]byte(nil), d.parts[d.documentIdx].data...)
	deleteSelection(t, d, p.Inlines[0].Text.ID, 3, p.Inlines[1].Text.ID, 0)
	if !bytes.Equal(before, d.parts[d.documentIdx].data) || d.Dirty() || d.CanUndo() {
		t.Fatal("empty selection at run boundary changed package")
	}
}

func TestDeleteWordTextSelectionWithinTableCell(t *testing.T) {
	d := selectionDeleteFixture(t, `<w:tbl><w:tr><w:tc><w:p><w:r><w:t>ABC</w:t></w:r></w:p><w:p><w:r><w:t>DEF</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>neighbor</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`)
	defer d.Close()
	cell := d.WordBlocks()[0].Table.Rows[0].Cells[0]
	deleteSelection(t, d, cell.Paragraphs[0].Inlines[0].Text.ID, 1, cell.Paragraphs[1].Inlines[0].Text.ID, 2)
	cells := d.WordBlocks()[0].Table.Rows[0].Cells
	if len(cells[0].Paragraphs) != 1 || cells[0].Paragraphs[0].Inlines[0].Text.Text != "A" || cells[0].Paragraphs[0].Inlines[1].Text.Text != "F" || cells[1].Paragraphs[0].Inlines[0].Text.Text != "neighbor" {
		t.Fatal("selection deletion changed table cell boundary or neighboring content")
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
	if cells := reopened.WordBlocks()[0].Table.Rows[0].Cells; len(cells[0].Paragraphs) != 1 || cells[0].Paragraphs[0].Inlines[1].Text.Text != "F" || cells[1].Paragraphs[0].Inlines[0].Text.Text != "neighbor" {
		t.Fatal("table selection deletion did not survive snapshot/reopen")
	}
}
