package docx

import (
	"archive/zip"
	"bytes"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"glance/internal/document"
)

func textEditingFixture(t *testing.T) *Document {
	t.Helper()
	documentXML := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:pPr><w:keepNext/><w:jc w:val="left"/></w:pPr><w:r><w:rPr><w:b/><w:color w:val="AABBCC"/><w:vanish/><w:rFonts w:ascii="Aptos"/></w:rPr><w:t>bold</w:t></w:r><w:r><w:rPr><w:i/></w:rPr><w:t> normal</w:t></w:r></w:p>
<w:tbl><w:tr><w:tc><w:p><w:r><w:t>cell text</w:t></w:r></w:p></w:tc></w:tr></w:tbl>
</w:body></w:document>`
	return projectionFixture(t, documentXML, "", nil)
}

func requireWordTextEditor(t *testing.T, doc *Document) document.WordTextEditor {
	t.Helper()
	editor, ok := any(doc).(document.WordTextEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose safe Word text editing")
	}
	return editor
}

func requireWordHistory(t *testing.T, doc *Document) document.WordHistory {
	t.Helper()
	history, ok := any(doc).(document.WordHistory)
	if !ok {
		t.Fatal("DOCX adapter does not expose Word undo/redo")
	}
	return history
}

func requireWordBodyEditor(t *testing.T, doc *Document) document.WordBodyEditor {
	t.Helper()
	editor, ok := any(doc).(document.WordBodyEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose body paragraph insertion")
	}
	return editor
}

type wordParagraphSplitCapability interface {
	SplitParagraphAt(runID, value string, selectionStart, selectionEnd int) (paragraphID, newRunID string, err error)
}

func requireWordParagraphSplitter(t *testing.T, doc *Document) wordParagraphSplitCapability {
	t.Helper()
	splitter, ok := any(doc).(wordParagraphSplitCapability)
	if !ok {
		t.Fatal("DOCX adapter does not expose paragraph splitting at a text caret")
	}
	return splitter
}

func requireWordParagraphJoiner(t *testing.T, doc *Document) document.WordParagraphJoiner {
	t.Helper()
	joiner, ok := any(doc).(document.WordParagraphJoiner)
	if !ok {
		t.Fatal("DOCX adapter does not expose paragraph joining at text boundaries")
	}
	return joiner
}

func TestJoinParagraphPreviousPreservesFormattingImageAndHistory(t *testing.T) {
	imageBytes := projectionPNG(t, color.NRGBA{R: 40, G: 120, B: 220, A: 255})
	documentXML := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><w:body>
<w:p><w:pPr><w:jc w:val="left"/><w:keepNext/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t>甲🌺</w:t></w:r><w:r><w:drawing><wp:inline><wp:extent cx="914400" cy="457200"/><wp:docPr id="1" name="Seam"/><a:graphic><a:graphicData><pic:pic><pic:blipFill><a:blip r:embed="rIdPicture1"/></pic:blipFill></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>
<w:p><w:pPr><w:jc w:val="right"/></w:pPr><w:r><w:rPr><w:i/></w:rPr><w:t>乙</w:t></w:r></w:p>
</w:body></w:document>`
	doc := projectionFixture(t, documentXML, projectionRelationships(), map[string][]byte{"word/media/one.png": imageBytes})
	blocks := doc.WordBlocks()
	leftParagraphID := blocks[0].Paragraph.ID
	rightRunID := blocks[1].Paragraph.Inlines[0].Text.ID

	result, err := requireWordParagraphJoiner(t, doc).JoinParagraphAtBoundary(rightRunID, document.WordJoinPrevious)
	if err != nil {
		t.Fatal(err)
	}
	blocks = doc.WordBlocks()
	if len(blocks) != 1 || blocks[0].Paragraph == nil {
		t.Fatalf("joined body = %#v; want one paragraph", blocks)
	}
	paragraph := blocks[0].Paragraph
	if paragraph.ID != leftParagraphID || paragraph.Alignment != document.WordAlignLeft || len(paragraph.Inlines) != 4 {
		t.Fatalf("merged paragraph identity, left properties, or inline count changed: %#v", paragraph)
	}
	if paragraph.Inlines[0].Text == nil || paragraph.Inlines[0].Text.Text != "甲🌺" || !paragraph.Inlines[0].Text.Style.Bold || paragraph.Inlines[1].Image == nil || paragraph.Inlines[2].Text == nil || paragraph.Inlines[2].Text.Text != "" || paragraph.Inlines[3].Text == nil || paragraph.Inlines[3].Text.Text != "乙" || !paragraph.Inlines[3].Text.Style.Italic {
		t.Fatalf("merged inline order, formatting, or seam caret run changed: %#v", paragraph.Inlines)
	}
	if result.ParagraphID != leftParagraphID || result.RunID != paragraph.Inlines[2].Text.ID || result.Caret != 0 {
		t.Fatalf("join caret target = %#v; want empty seam run at offset 0", result)
	}
	gotImage, err := requireWordContentReader(t, doc).WordImageBytes(paragraph.Inlines[1].Image.ID)
	if err != nil || !bytes.Equal(gotImage, imageBytes) {
		t.Fatalf("joined image bytes changed: %v", err)
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
	if len(reopenedBlocks) != 1 || len(reopenedBlocks[0].Paragraph.Inlines) != 4 || reopenedBlocks[0].Paragraph.Inlines[1].Image == nil || reopenedBlocks[0].Paragraph.Inlines[3].Text.Text != "乙" {
		t.Fatalf("joined paragraph did not survive save/reopen: %#v", reopenedBlocks)
	}

	history := requireWordHistory(t, doc)
	if applied, err := history.Undo(); err != nil || !applied {
		t.Fatalf("undo paragraph join = %v, %v", applied, err)
	}
	blocks = doc.WordBlocks()
	if len(blocks) != 2 || blocks[0].Paragraph.Alignment != document.WordAlignLeft || blocks[1].Paragraph.Alignment != document.WordAlignRight {
		t.Fatalf("undo did not restore both original paragraphs: %#v", blocks)
	}
	if applied, err := history.Redo(); err != nil || !applied {
		t.Fatalf("redo paragraph join = %v, %v", applied, err)
	}
}

func TestJoinParagraphNextAndSameTableCell(t *testing.T) {
	documentXML := `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:tbl><w:tr><w:tc><w:p><w:r><w:t>🌺</w:t></w:r></w:p><w:p><w:r><w:t>乙</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>丙</w:t></w:r></w:p></w:tc></w:tr></w:tbl></w:body></w:document>`
	doc := projectionFixture(t, documentXML, "", nil)
	cell := doc.WordBlocks()[0].Table.Rows[0].Cells[0]
	leftRun := cell.Paragraphs[0].Inlines[0].Text

	result, err := requireWordParagraphJoiner(t, doc).JoinParagraphAtBoundary(leftRun.ID, document.WordJoinNext)
	if err != nil {
		t.Fatal(err)
	}
	cells := doc.WordBlocks()[0].Table.Rows[0].Cells
	if len(cells[0].Paragraphs) != 1 || len(cells[0].Paragraphs[0].Inlines) != 2 || cells[0].Paragraphs[0].Inlines[0].Text.Text != "🌺" || cells[0].Paragraphs[0].Inlines[1].Text.Text != "乙" {
		t.Fatalf("next paragraph join did not merge the same table cell: %#v", cells[0].Paragraphs)
	}
	if len(cells[1].Paragraphs) != 1 || cells[1].Paragraphs[0].Inlines[0].Text.Text != "丙" {
		t.Fatalf("join crossed into a neighboring table cell: %#v", cells[1].Paragraphs)
	}
	if result.ParagraphID != cells[0].Paragraphs[0].ID || result.RunID != leftRun.ID || result.Caret != 1 {
		t.Fatalf("next-join caret target = %#v; want left run at rune offset 1", result)
	}
	if !doc.dirty {
		t.Fatal("paragraph join did not mark the DOCX dirty")
	}
	before := append([]byte(nil), doc.parts[doc.documentIdx].data...)
	neighborRunID := cells[1].Paragraphs[0].Inlines[0].Text.ID
	if _, err := requireWordParagraphJoiner(t, doc).JoinParagraphAtBoundary(neighborRunID, document.WordJoinPrevious); err == nil {
		t.Fatal("paragraph join crossed into the neighboring table cell")
	}
	if !bytes.Equal(before, doc.parts[doc.documentIdx].data) {
		t.Fatal("rejected neighboring-cell join mutated word/document.xml")
	}
}

func TestJoinParagraphExpandsSelfClosingEmptyLeftParagraph(t *testing.T) {
	documentXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p/><w:p><w:r><w:t>乙</w:t></w:r></w:p></w:body></w:document>`
	doc := projectionFixture(t, documentXML, "", nil)
	defer doc.Close()
	blocks := doc.WordBlocks()
	if len(blocks) != 2 || blocks[1].Paragraph == nil || len(blocks[1].Paragraph.Inlines) == 0 || blocks[1].Paragraph.Inlines[0].Text == nil {
		t.Fatalf("fixture did not project editable right paragraph: %#v", blocks)
	}
	rightRun := blocks[1].Paragraph.Inlines[0].Text
	result, err := requireWordParagraphJoiner(t, doc).JoinParagraphAtBoundary(rightRun.ID, document.WordJoinPrevious)
	if err != nil {
		t.Fatalf("join self-closing empty paragraph: %v", err)
	}
	blocks = doc.WordBlocks()
	if len(blocks) != 1 || blocks[0].Paragraph == nil || len(blocks[0].Paragraph.Inlines) != 2 || blocks[0].Paragraph.Inlines[0].Text == nil || blocks[0].Paragraph.Inlines[0].Text.Text != "" || blocks[0].Paragraph.Inlines[1].Text == nil || blocks[0].Paragraph.Inlines[1].Text.Text != "乙" {
		t.Fatalf("expanded empty paragraph join = %#v", blocks)
	}
	if result.RunID != blocks[0].Paragraph.Inlines[0].Text.ID || result.Caret != 0 {
		t.Fatalf("caret seam = %#v; want empty first run at offset 0", result)
	}
	snapshot, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSnapshot(snapshot)
	if err != nil {
		t.Fatalf("reopen joined empty paragraph: %v", err)
	}
	defer reopened.Close()
	if reopenedBlocks := reopened.WordBlocks(); len(reopenedBlocks) != 1 || reopenedBlocks[0].Paragraph.Inlines[1].Text.Text != "乙" {
		t.Fatalf("joined empty paragraph did not survive reopen: %#v", reopenedBlocks)
	}
}

func TestJoinParagraphKeepsImageOnlyLeftParagraph(t *testing.T) {
	imageBytes := projectionPNG(t, color.NRGBA{R: 30, G: 110, B: 190, A: 255})
	documentXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><w:body><w:p><w:r><w:drawing><wp:inline><wp:extent cx="914400" cy="457200"/><wp:docPr id="1" name="Only image"/><a:graphic><a:graphicData><pic:pic><pic:blipFill><a:blip r:embed="rIdPicture1"/></pic:blipFill></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p><w:p><w:r><w:t>正文</w:t></w:r></w:p></w:body></w:document>`
	doc := projectionFixture(t, documentXML, projectionRelationships(), map[string][]byte{"word/media/one.png": imageBytes})
	defer doc.Close()
	blocks := doc.WordBlocks()
	if len(blocks) != 2 || blocks[0].Paragraph == nil || len(blocks[0].Paragraph.Inlines) != 1 || blocks[0].Paragraph.Inlines[0].Image == nil || blocks[1].Paragraph == nil || blocks[1].Paragraph.Inlines[0].Text == nil {
		t.Fatalf("fixture did not project image-only and editable paragraphs: %#v", blocks)
	}
	imageID := blocks[0].Paragraph.Inlines[0].Image.ID
	rightRun := blocks[1].Paragraph.Inlines[0].Text
	result, err := requireWordParagraphJoiner(t, doc).JoinParagraphAtBoundary(rightRun.ID, document.WordJoinPrevious)
	if err != nil {
		t.Fatalf("join after image-only paragraph: %v", err)
	}
	joined := doc.WordBlocks()
	if len(joined) != 1 || joined[0].Paragraph == nil || len(joined[0].Paragraph.Inlines) != 3 || joined[0].Paragraph.Inlines[0].Image == nil || joined[0].Paragraph.Inlines[0].Image.ID != imageID || joined[0].Paragraph.Inlines[1].Text == nil || joined[0].Paragraph.Inlines[1].Text.Text != "" || joined[0].Paragraph.Inlines[2].Text == nil || joined[0].Paragraph.Inlines[2].Text.Text != "正文" {
		t.Fatalf("image-only paragraph join = %#v", joined)
	}
	if result.RunID != joined[0].Paragraph.Inlines[1].Text.ID || result.Caret != 0 {
		t.Fatalf("image-only seam caret = %#v; want the editable empty run after the image", result)
	}
	gotImage, err := requireWordContentReader(t, doc).WordImageBytes(imageID)
	if err != nil || !bytes.Equal(gotImage, imageBytes) {
		t.Fatalf("image bytes changed after joining image-only paragraph: %v", err)
	}
}

func TestJoinParagraphRejectsInheritedNamespaceScopeChangeWithoutMutation(t *testing.T) {
	documentXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:x="urn:right"><w:body><w:p xmlns:x="urn:left"><w:r><w:t>甲</w:t></w:r></w:p><w:p><w:r><w:t>乙</w:t></w:r><x:payload/></w:p></w:body></w:document>`
	doc := projectionFixture(t, documentXML, "", nil)
	defer doc.Close()
	blocks := doc.WordBlocks()
	rightRun := blocks[1].Paragraph.Inlines[0].Text
	before := append([]byte(nil), doc.parts[doc.documentIdx].data...)
	if _, err := requireWordParagraphJoiner(t, doc).JoinParagraphAtBoundary(rightRun.ID, document.WordJoinPrevious); err == nil {
		t.Fatal("join accepted inherited namespace bindings that would change the right inline XML meaning")
	}
	if !bytes.Equal(before, doc.parts[doc.documentIdx].data) || doc.Dirty() {
		t.Fatal("rejected namespace-scope join mutated the DOCX package")
	}
}

func TestJoinParagraphRejectsContainerBoundariesWithoutMutation(t *testing.T) {
	tests := []struct {
		name string
		xml  string
	}{
		{
			name: "body table boundary",
			xml:  `<w:body><w:p><w:r><w:t>甲</w:t></w:r></w:p><w:tbl><w:tr><w:tc><w:p><w:r><w:t>表</w:t></w:r></w:p></w:tc></w:tr></w:tbl><w:p><w:r><w:t>乙</w:t></w:r></w:p></w:body>`,
		},
		{
			name: "unsupported body block",
			xml:  `<w:body><w:p><w:r><w:t>甲</w:t></w:r></w:p><w:customXml><w:p><w:r><w:t>中间</w:t></w:r></w:p></w:customXml><w:p><w:r><w:t>乙</w:t></w:r></w:p></w:body>`,
		},
		{
			name: "section properties paragraph",
			xml:  `<w:body><w:p><w:pPr><w:sectPr/></w:pPr><w:r><w:t>甲</w:t></w:r></w:p><w:p><w:r><w:t>乙</w:t></w:r></w:p></w:body>`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := projectionFixture(t, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">`+test.xml+`</w:document>`, "", nil)
			var rightRunID string
			for _, block := range doc.WordBlocks() {
				if block.Paragraph != nil && len(block.Paragraph.Inlines) != 0 && block.Paragraph.Inlines[0].Text != nil && block.Paragraph.Inlines[0].Text.Text == "乙" {
					rightRunID = block.Paragraph.Inlines[0].Text.ID
				}
			}
			if rightRunID == "" {
				t.Fatal("fixture did not project the right-hand paragraph")
			}
			before := append([]byte(nil), doc.parts[doc.documentIdx].data...)
			if _, err := requireWordParagraphJoiner(t, doc).JoinParagraphAtBoundary(rightRunID, document.WordJoinPrevious); err == nil {
				t.Fatal("join succeeded across an unsupported paragraph boundary")
			}
			if !bytes.Equal(before, doc.parts[doc.documentIdx].data) {
				t.Fatal("rejected paragraph join mutated word/document.xml")
			}
		})
	}
}

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

func findRunByText(t *testing.T, blocks []document.WordBlock, value string) (string, string) {
	t.Helper()
	for _, block := range blocks {
		if block.Paragraph != nil {
			for _, inline := range block.Paragraph.Inlines {
				if inline.Text != nil && inline.Text.Text == value {
					return inline.Text.ID, block.Paragraph.ID
				}
			}
		}
		if block.Table != nil {
			for _, row := range block.Table.Rows {
				for _, cell := range row.Cells {
					for _, paragraph := range cell.Paragraphs {
						for _, inline := range paragraph.Inlines {
							if inline.Text != nil && inline.Text.Text == value {
								return inline.Text.ID, paragraph.ID
							}
						}
					}
				}
			}
		}
	}
	t.Fatalf("text run %q not found", value)
	return "", ""
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

func TestParagraphAlignmentWritesSupportedValuesAndKeepsOtherProperties(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	_, paragraphID := findRunByText(t, doc.WordBlocks(), "bold")
	editor := requireWordTextEditor(t, doc)
	for _, test := range []struct {
		alignment document.WordAlignment
		projected document.WordAlignment
		xmlValue  string
	}{
		{alignment: document.WordAlignLeft, projected: document.WordAlignLeft, xmlValue: "left"},
		{alignment: document.WordAlignCenter, projected: document.WordAlignCenter, xmlValue: "center"},
		{alignment: document.WordAlignRight, projected: document.WordAlignRight, xmlValue: "right"},
		{alignment: document.WordAlignJustified, projected: document.WordAlignJustified, xmlValue: "both"},
	} {
		if err := editor.SetParagraphAlignment(paragraphID, test.alignment); err != nil {
			t.Fatalf("set %s alignment: %v", test.alignment, err)
		}
		if got := doc.WordBlocks()[0].Paragraph.Alignment; got != test.projected {
			t.Errorf("projected alignment = %q; want %q", got, test.projected)
		}
		data, err := doc.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		documentXML := readZipPart(t, data, "word/document.xml")
		if !bytes.Contains(documentXML, []byte(`w:jc w:val="`+test.xmlValue+`"`)) || !bytes.Contains(documentXML, []byte("w:keepNext")) {
			t.Fatalf("alignment patch %s lost pPr content: %s", test.alignment, documentXML)
		}
	}
	output := filepath.Join(t.TempDir(), "aligned.docx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got := reopened.WordBlocks()[0].Paragraph.Alignment; got != document.WordAlignJustified {
		t.Fatalf("saved alignment = %q; want justified", got)
	}
}

func TestFailedTextEditLeavesDocumentCleanAndHistoryEmpty(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	editor := requireWordTextEditor(t, doc)
	if err := editor.ReplaceTextRun("missing-run", "replacement"); err == nil {
		t.Fatal("expected missing text-run ID to be rejected")
	}
	runID, paragraphID := findRunByText(t, doc.WordBlocks(), "bold")
	invalidColor := "not-a-color"
	if err := editor.SetTextRunStyle(runID, document.WordTextFormatPatch{FontColor: &invalidColor}); err == nil {
		t.Fatal("expected invalid font color to be rejected")
	}
	if err := editor.SetParagraphAlignment(paragraphID, document.WordAlignDefault); err == nil {
		t.Fatal("expected default alignment to be rejected as an edit target")
	}
	if doc.Dirty() {
		t.Fatal("failed edit changed dirty state")
	}
	if applied, err := requireWordHistory(t, doc).Undo(); err != nil || applied {
		t.Fatalf("undo after failed edit = %v, %v; want false, nil", applied, err)
	}
}

func TestWordTextStyleAndAlignmentCanBeInsertedWhenPropertiesAreAbsent(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	blocks := doc.WordBlocks()
	cellRun, _ := findRunByText(t, blocks, "cell text")
	cellParagraph := blocks[1].Table.Rows[0].Cells[0].Paragraphs[0].ID
	yes := true
	editor := requireWordTextEditor(t, doc)
	if err := editor.SetTextRunStyle(cellRun, document.WordTextFormatPatch{Bold: &yes}); err != nil {
		t.Fatal(err)
	}
	if !doc.WordBlocks()[1].Table.Rows[0].Cells[0].Paragraphs[0].Inlines[0].Text.Style.Bold {
		t.Fatal("bold was not inserted for a run without w:rPr")
	}
	if err := editor.SetParagraphAlignment(cellParagraph, document.WordAlignRight); err != nil {
		t.Fatal(err)
	}
	if got := doc.WordBlocks()[1].Table.Rows[0].Cells[0].Paragraphs[0].Alignment; got != document.WordAlignRight {
		t.Fatalf("inserted paragraph alignment = %q", got)
	}
}

func TestTextEditUpgradesExplicitDefaultSpacePreservation(t *testing.T) {
	documentXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t xml:space="default">text</w:t></w:r></w:p></w:body></w:document>`
	doc := projectionFixture(t, documentXML, "", nil)
	defer doc.Close()
	runID, _ := findRunByText(t, doc.WordBlocks(), "text")
	if err := requireWordTextEditor(t, doc).ReplaceTextRun(runID, " spaced "); err != nil {
		t.Fatal(err)
	}
	snapshot, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	data := readZipPart(t, snapshot, "word/document.xml")
	if !bytes.Contains(data, []byte(`xml:space="preserve"`)) {
		t.Fatalf("boundary whitespace left with xml:space default: %s", data)
	}
}

func TestComplexFieldRunsRemainReadOnly(t *testing.T) {
	documentXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText> PAGE </w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r><w:r><w:t>7</w:t></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r></w:p></w:body></w:document>`
	doc := projectionFixture(t, documentXML, "", nil)
	defer doc.Close()
	runID, _ := findRunByText(t, doc.WordBlocks(), "7")
	if inline := doc.WordBlocks()[0].Paragraph.Inlines[3]; inline.Text == nil || inline.Text.Editable {
		t.Fatalf("field result inline = %#v; want read-only text", inline)
	}
	if err := requireWordTextEditor(t, doc).ReplaceTextRun(runID, "8"); err == nil {
		t.Fatal("field result text should not be editable")
	}
	if doc.Dirty() {
		t.Fatal("failed field edit changed dirty state")
	}
}

func TestEmptySelfClosingTextRunCanBeFilledAndSaved(t *testing.T) {
	documentXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t/></w:r></w:p></w:body></w:document>`
	doc := projectionFixture(t, documentXML, "", nil)
	defer doc.Close()
	blocks := doc.WordBlocks()
	if len(blocks) != 1 || len(blocks[0].Paragraph.Inlines) != 1 || blocks[0].Paragraph.Inlines[0].Text.Text != "" {
		t.Fatalf("empty text projection = %#v", blocks)
	}
	runID := blocks[0].Paragraph.Inlines[0].Text.ID
	if err := requireWordTextEditor(t, doc).ReplaceTextRun(runID, "filled"); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "filled.docx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got := reopened.WordBlocks()[0].Paragraph.Inlines[0].Text.Text; got != "filled" {
		t.Fatalf("reopened self-closing text = %q", got)
	}
}

func TestNoOpWordTextAndFormattingEditsDoNotCreateHistory(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	blocks := doc.WordBlocks()
	runID, paragraphID := findRunByText(t, blocks, "bold")
	editor := requireWordTextEditor(t, doc)
	history := requireWordHistory(t, doc)
	yes := true
	if err := editor.ReplaceTextRun(runID, "bold"); err != nil {
		t.Fatal(err)
	}
	if err := editor.SetTextRunStyle(runID, document.WordTextFormatPatch{Bold: &yes}); err != nil {
		t.Fatal(err)
	}
	if err := editor.SetParagraphAlignment(paragraphID, document.WordAlignLeft); err != nil {
		t.Fatal(err)
	}
	if doc.Dirty() || history.CanUndo() {
		t.Fatalf("no-op edit changed document: dirty=%v canUndo=%v", doc.Dirty(), history.CanUndo())
	}
}

func assertPackagePartsEqual(t *testing.T, originalData []byte, savedPath string) {
	t.Helper()
	original := readZipParts(t, originalData)
	savedData, err := os.ReadFile(savedPath)
	if err != nil {
		t.Fatal(err)
	}
	saved := readZipParts(t, savedData)
	for name, originalPart := range original {
		if name == "word/document.xml" {
			continue
		}
		if !bytes.Equal(originalPart, saved[name]) {
			t.Errorf("untouched package part %q changed", name)
		}
	}
}

func readZipPart(t *testing.T, data []byte, name string) []byte {
	t.Helper()
	return readZipParts(t, data)[name]
}

func readZipParts(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	parts := make(map[string][]byte, len(reader.File))
	for _, file := range reader.File {
		stream, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		part, readErr := io.ReadAll(stream)
		closeErr := stream.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		parts[file.Name] = part
	}
	return parts
}

func TestWordTextEditDoesNotDropUnescapedMarkup(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	runID, _ := findRunByText(t, doc.WordBlocks(), "bold")
	if err := requireWordTextEditor(t, doc).ReplaceTextRun(runID, "<& \"text\""); err != nil {
		t.Fatal(err)
	}
	data, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	text := string(readZipPart(t, data, "word/document.xml"))
	if strings.Contains(text, "<& \"text\"") || !strings.Contains(text, `&lt;&amp; &#34;text&#34;`) {
		t.Fatalf("text was not safely XML escaped: %s", text)
	}
}
