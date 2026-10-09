package docx

import (
	"bytes"
	"glance/internal/document"
	"image/color"
	"strings"
	"testing"
)

func TestSplitParagraphAtCaretPreservesStylesInlineImageAndHistory(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	paragraph := doc.WordBlocks()[0].Paragraph
	target := paragraph.Inlines[1].Text
	imageID, err := requireWordImageEditor(t, doc).InsertInlineImage(paragraph.ID, target.ID, "image/png", imageFixtureBytes(t, 2, 1, color.NRGBA{R: 10, G: 80, B: 200, A: 255}))
	if err != nil {
		t.Fatal(err)
	}
	newParagraphID, newRunID, err := requireWordParagraphSplitter(t, doc).SplitParagraphAt(target.ID, target.Text, 4, 4)
	if err != nil {
		t.Fatal(err)
	}
	blocks := doc.WordBlocks()
	if len(blocks) != 3 || blocks[0].Paragraph == nil || blocks[1].Paragraph == nil || blocks[2].Table == nil {
		t.Fatalf("body after split = %#v; want two paragraphs followed by the existing table", blocks)
	}
	left, right := blocks[0].Paragraph, blocks[1].Paragraph
	if left.Alignment != document.WordAlignLeft || right.Alignment != document.WordAlignLeft {
		t.Fatalf("split paragraph alignment = %q/%q; want both left-aligned", left.Alignment, right.Alignment)
	}
	if len(left.Inlines) != 2 || left.Inlines[0].Text == nil || left.Inlines[0].Text.Text != "bold" || left.Inlines[1].Text == nil || left.Inlines[1].Text.Text != " nor" || !left.Inlines[0].Text.Style.Bold || !left.Inlines[1].Text.Style.Italic {
		t.Fatalf("content or formatting before caret changed: %#v", left.Inlines)
	}
	if right.ID != newParagraphID || len(right.Inlines) != 2 || right.Inlines[0].Text == nil || right.Inlines[0].Text.ID != newRunID || right.Inlines[0].Text.Text != "mal" || !right.Inlines[0].Text.Style.Italic || right.Inlines[1].Image == nil || right.Inlines[1].Image.ID != imageID {
		t.Fatalf("content after caret did not move into the new paragraph intact: %#v", right.Inlines)
	}

	snapshot, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedBlocks := reopened.WordBlocks()
	if len(reopenedBlocks) != 3 || reopenedBlocks[0].Paragraph.Inlines[1].Text.Text != " nor" || reopenedBlocks[1].Paragraph.Inlines[0].Text.Text != "mal" || reopenedBlocks[1].Paragraph.Inlines[1].Image.ID != imageID {
		t.Fatalf("split paragraph did not survive save/reopen: %#v", reopenedBlocks)
	}

	history := requireWordHistory(t, doc)
	if applied, err := history.Undo(); err != nil || !applied {
		t.Fatalf("undo paragraph split = %v, %v", applied, err)
	}
	blocks = doc.WordBlocks()
	if len(blocks) != 2 || blocks[0].Paragraph == nil || len(blocks[0].Paragraph.Inlines) != 3 || blocks[0].Paragraph.Inlines[1].Text.Text != " normal" || blocks[0].Paragraph.Inlines[2].Image.ID != imageID {
		t.Fatalf("undo did not restore the original paragraph and image order: %#v", blocks)
	}
	if applied, err := history.Redo(); err != nil || !applied {
		t.Fatalf("redo paragraph split = %v, %v", applied, err)
	}
}

func TestSplitParagraphAtCaretKeepsTableCellBoundaries(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	cellParagraph := doc.WordBlocks()[1].Table.Rows[0].Cells[0].Paragraphs[0]
	target := cellParagraph.Inlines[0].Text
	newParagraphID, newRunID, err := requireWordParagraphSplitter(t, doc).SplitParagraphAt(target.ID, target.Text, 4, 4)
	if err != nil {
		t.Fatal(err)
	}
	blocks := doc.WordBlocks()
	if len(blocks) != 2 || blocks[0].Kind != document.WordParagraphBlock || blocks[1].Kind != document.WordTableBlock {
		t.Fatalf("table-cell split changed top-level block order: %#v", blocks)
	}
	cell := blocks[1].Table.Rows[0].Cells[0]
	if len(cell.Paragraphs) != 2 || cell.Paragraphs[0].Inlines[0].Text.Text != "cell" || cell.Paragraphs[1].ID != newParagraphID || cell.Paragraphs[1].Inlines[0].Text.ID != newRunID || cell.Paragraphs[1].Inlines[0].Text.Text != " text" {
		t.Fatalf("split did not stay within its table cell: %#v", cell.Paragraphs)
	}
}

func TestSplitParagraphSelectionReplacesSelectedUnicodeRunes(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	run := doc.WordBlocks()[0].Paragraph.Inlines[0].Text
	if err := requireWordTextEditor(t, doc).ReplaceTextRun(run.ID, "甲BC乙"); err != nil {
		t.Fatal(err)
	}
	newParagraphID, _, err := requireWordParagraphSplitter(t, doc).SplitParagraphAt(run.ID, "甲BC乙", 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	blocks := doc.WordBlocks()
	if len(blocks) != 2 || blocks[0].Paragraph.Inlines[0].Text.Text != "甲" || blocks[1].Paragraph.ID != newParagraphID || blocks[1].Paragraph.Inlines[0].Text.Text != "乙" {
		t.Fatalf("selection split yielded %#v; want 甲 / 乙", blocks)
	}
}

func TestSplitParagraphRejectsInvalidSelectionWithoutMutation(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	target := doc.WordBlocks()[0].Paragraph.Inlines[0].Text
	before, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := requireWordParagraphSplitter(t, doc).SplitParagraphAt(target.ID, target.Text, 99, 99); err == nil {
		t.Fatal("paragraph split accepted a selection outside the text run")
	}
	after, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || doc.Dirty() {
		t.Fatal("rejected invalid selection changed the DOCX package")
	}
}

func TestSplitParagraphRejectsSectionPropertiesWithoutMutation(t *testing.T) {
	documentXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:pPr><w:sectPr/></w:pPr><w:r><w:t>section text</w:t></w:r></w:p></w:body></w:document>`
	doc := projectionFixture(t, documentXML, "", nil)
	defer doc.Close()
	target := doc.WordBlocks()[0].Paragraph.Inlines[0].Text
	before, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := requireWordParagraphSplitter(t, doc).SplitParagraphAt(target.ID, target.Text, 4, 4); err == nil {
		t.Fatal("paragraph containing section properties was split")
	}
	after, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || doc.Dirty() {
		t.Fatal("rejected paragraph split changed the DOCX package")
	}
}

func TestSplitParagraphRemovesRecognizedWordIdentityAttributes(t *testing.T) {
	documentXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:w14="http://schemas.microsoft.com/office/word/2010/wordml"><w:body><w:p w14:paraId="00112233" w14:textId="44556677"><w:r><w:t>identity</w:t></w:r></w:p></w:body></w:document>`
	doc := projectionFixture(t, documentXML, "", nil)
	defer doc.Close()
	target := doc.WordBlocks()[0].Paragraph.Inlines[0].Text
	if _, _, err := requireWordParagraphSplitter(t, doc).SplitParagraphAt(target.ID, target.Text, 4, 4); err != nil {
		t.Fatal(err)
	}
	snapshot, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	data := string(readZipPart(t, snapshot, "word/document.xml"))
	if strings.Count(data, `w14:paraId="`) != 1 || strings.Count(data, `w14:textId="`) != 1 {
		t.Fatalf("split did not remove duplicate Word identity attributes: %s", data)
	}
}

func TestSplitParagraphRejectsForeignIdentityLikeAttributesWithoutMutation(t *testing.T) {
	documentXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:x="urn:example:vendor"><w:body><w:p x:paraId="vendor-data"><w:r><w:t>identity</w:t></w:r></w:p></w:body></w:document>`
	doc := projectionFixture(t, documentXML, "", nil)
	defer doc.Close()
	target := doc.WordBlocks()[0].Paragraph.Inlines[0].Text
	before, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := requireWordParagraphSplitter(t, doc).SplitParagraphAt(target.ID, target.Text, 4, 4); err == nil {
		t.Fatal("paragraph with a foreign identity-like attribute was split")
	}
	after, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || doc.Dirty() {
		t.Fatal("rejected paragraph split changed the DOCX package")
	}
}

func TestSplitEmptyParagraphAtCaretKeepsBothRunsEditable(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	target := doc.WordBlocks()[0].Paragraph.Inlines[0].Text
	paragraphID, runID, err := requireWordParagraphSplitter(t, doc).SplitParagraphAt(target.ID, "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	blocks := doc.WordBlocks()
	if len(blocks) != 2 || blocks[0].Paragraph == nil || blocks[1].Paragraph == nil || blocks[1].Paragraph.ID != paragraphID {
		t.Fatalf("empty paragraph split produced blocks %#v", blocks)
	}
	for index, block := range blocks {
		if len(block.Paragraph.Inlines) != 1 || block.Paragraph.Inlines[0].Text == nil || !block.Paragraph.Inlines[0].Text.Editable || block.Paragraph.Inlines[0].Text.Text != "" {
			t.Fatalf("split paragraph %d is not an editable empty run: %#v", index, block.Paragraph.Inlines)
		}
	}
	if blocks[1].Paragraph.Inlines[0].Text.ID != runID {
		t.Fatalf("following empty run ID = %q; want %q", blocks[1].Paragraph.Inlines[0].Text.ID, runID)
	}
}
