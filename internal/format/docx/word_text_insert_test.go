package docx

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInsertBodyParagraphBeforeSectionPropertiesAndRoundTrips(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	paragraphID, err := requireWordBodyEditor(t, doc).InsertBodyParagraph()
	if err != nil {
		t.Fatal(err)
	}
	blocks := doc.WordBlocks()
	if len(blocks) != 2 || blocks[1].Paragraph == nil || blocks[1].Paragraph.ID != paragraphID {
		t.Fatalf("inserted body blocks = %#v; returned paragraph ID %q", blocks, paragraphID)
	}
	if len(blocks[1].Paragraph.Inlines) != 1 || blocks[1].Paragraph.Inlines[0].Text == nil || !blocks[1].Paragraph.Inlines[0].Text.Editable {
		t.Fatalf("inserted paragraph lacks an editable empty text run: %#v", blocks[1].Paragraph.Inlines)
	}
	runID := blocks[1].Paragraph.Inlines[0].Text.ID
	if err := requireWordTextEditor(t, doc).ReplaceTextRun(runID, "typed at the end"); err != nil {
		t.Fatal(err)
	}
	data, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := OpenSnapshot(data)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	restoredBlocks := restored.WordBlocks()
	if len(restoredBlocks) != 2 || restoredBlocks[1].Paragraph.Inlines[0].Text.Text != "typed at the end" {
		t.Fatalf("reopened body paragraphs = %#v", restoredBlocks)
	}
	reopenedXML := string(readZipPart(t, data, "word/document.xml"))
	if strings.Index(reopenedXML, "typed at the end") > strings.Index(reopenedXML, "<w:sectPr") {
		t.Fatalf("inserted paragraph must remain before section properties: %s", reopenedXML)
	}

	history := requireWordHistory(t, doc)
	if applied, err := history.Undo(); err != nil || !applied {
		t.Fatalf("undo text edit = %v, %v", applied, err)
	}
	if applied, err := history.Undo(); err != nil || !applied {
		t.Fatalf("undo inserted paragraph = %v, %v", applied, err)
	}
	if len(doc.WordBlocks()) != 1 {
		t.Fatalf("undo left inserted body paragraph: %#v", doc.WordBlocks())
	}
	if applied, err := history.Redo(); err != nil || !applied {
		t.Fatalf("redo inserted paragraph = %v, %v", applied, err)
	}
}

func TestInsertParagraphAfterSpecificParagraphAndUndo(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	firstParaID := doc.WordBlocks()[0].Paragraph.ID
	secondParaID, err := requireWordBodyEditor(t, doc).InsertParagraphAfter(firstParaID)
	if err != nil {
		t.Fatalf("InsertParagraphAfter failed: %v", err)
	}
	blocks := doc.WordBlocks()
	if len(blocks) != 2 || blocks[1].Paragraph == nil || blocks[1].Paragraph.ID != secondParaID {
		t.Fatalf("blocks after InsertParagraphAfter = %#v; want 2 with second=%q", blocks, secondParaID)
	}
	thirdParaID, err := requireWordBodyEditor(t, doc).InsertParagraphAfter(firstParaID)
	if err != nil {
		t.Fatalf("InsertParagraphAfter middle failed: %v", err)
	}
	blocks = doc.WordBlocks()
	if len(blocks) != 3 || blocks[1].Paragraph == nil || blocks[1].Paragraph.ID != thirdParaID {
		t.Fatalf("blocks after inserting between = %#v; want 3 with middle=%q", blocks, thirdParaID)
	}
	history := requireWordHistory(t, doc)
	if applied, err := history.Undo(); err != nil || !applied {
		t.Fatalf("undo insert = %v, %v", applied, err)
	}
	if len(doc.WordBlocks()) != 2 {
		t.Fatalf("blocks after undo = %d; want 2", len(doc.WordBlocks()))
	}
}

func TestInsertParagraphAfterUnknownExplicitTargetDoesNotFallBackOrMutate(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	before := append([]byte(nil), doc.parts[doc.documentIdx].data...)
	dirtyBefore := doc.Dirty()
	if _, err := requireWordBodyEditor(t, doc).InsertParagraphAfter("missing-paragraph"); err == nil {
		t.Fatal("InsertParagraphAfter accepted an unknown explicit paragraph ID")
	}
	if after := doc.parts[doc.documentIdx].data; !bytes.Equal(after, before) {
		t.Fatalf("unknown paragraph target changed document XML: before=%s after=%s", before, after)
	}
	if dirtyAfter := doc.Dirty(); dirtyAfter != dirtyBefore {
		t.Fatalf("unknown paragraph target changed dirty state from %v to %v", dirtyBefore, dirtyAfter)
	}
	if history := requireWordHistory(t, doc); history.CanUndo() {
		t.Fatal("unknown paragraph target created an undo record")
	}
}

func TestReplaceTextRunEditsSelectedRunAndTableCellAndRoundTrips(t *testing.T) {
	doc := textEditingFixture(t)
	blocks := doc.WordBlocks()
	originalPackage, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	firstID, _ := findRunByText(t, blocks, "bold")
	cellID, _ := findRunByText(t, blocks, "cell text")
	editor := requireWordTextEditor(t, doc)
	if err := editor.ReplaceTextRun(firstID, "  你好 & <world>  "); err != nil {
		t.Fatal(err)
	}
	if err := editor.ReplaceTextRun(cellID, "单元格 & <内容>"); err != nil {
		t.Fatal(err)
	}
	updated := doc.WordBlocks()
	if got := updated[0].Paragraph.Inlines[0].Text.Text; got != "  你好 & <world>  " {
		t.Fatalf("edited first run = %q", got)
	}
	if got := updated[0].Paragraph.Inlines[1].Text.Text; got != " normal" {
		t.Fatalf("adjacent run changed to %q", got)
	}
	if got := updated[1].Table.Rows[0].Cells[0].Paragraphs[0].Inlines[0].Text.Text; got != "单元格 & <内容>" {
		t.Fatalf("edited table-cell run = %q", got)
	}
	if !doc.Dirty() {
		t.Fatal("text edits did not mark the document dirty")
	}

	output := filepath.Join(t.TempDir(), "edited.docx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatal(err)
	}
	if err := doc.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	textID, _ := findRunByText(t, reopened.WordBlocks(), "  你好 & <world>  ")
	if textID == "" {
		t.Fatal("edited run not found after reopen")
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	documentXML := readZipPart(t, data, "word/document.xml")
	if !bytes.Contains(documentXML, []byte(`xml:space="preserve"`)) || !bytes.Contains(documentXML, []byte("&amp; &lt;world&gt;")) {
		t.Fatalf("saved XML did not preserve boundary whitespace or escape text: %s", documentXML)
	}
	assertPackagePartsEqual(t, originalPackage, output)
}
