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

type wordTextFontSizeEditor interface {
	SetTextRunFontSize(runID string, points float64) error
}

type wordTextFontFamilyEditor interface {
	SetTextRunFontFamily(runID, family string) error
}

type wordTextRangeEditor interface {
	ReplaceTextRange(runID string, start, end int, replacement string) error
}

func requireWordTextRangeEditor(t *testing.T, doc *Document) wordTextRangeEditor {
	t.Helper()
	editor, ok := any(doc).(wordTextRangeEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose safe Word text-range replacement")
	}
	return editor
}

func requireWordTextFontSizeEditor(t *testing.T, doc *Document) wordTextFontSizeEditor {
	t.Helper()
	editor, ok := any(doc).(wordTextFontSizeEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose safe Word font-size editing")
	}
	return editor
}

func requireWordTextFontFamilyEditor(t *testing.T, doc *Document) wordTextFontFamilyEditor {
	t.Helper()
	editor, ok := any(doc).(wordTextFontFamilyEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose safe Word font-family editing")
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

func TestReplaceWordTextRangeUsesRuneOffsetsAndPreservesRunFormatting(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	paragraph := doc.WordBlocks()[0].Paragraph
	firstRun := paragraph.Inlines[0].Text
	secondRun := paragraph.Inlines[1].Text
	before := append([]byte(nil), doc.parts[doc.documentIdx].data...)

	if err := requireWordTextRangeEditor(t, doc).ReplaceTextRange(firstRun.ID, 1, 3, "🌺&"); err != nil {
		t.Fatal(err)
	}
	blocks := doc.WordBlocks()
	got := blocks[0].Paragraph.Inlines
	if got[0].Text.Text != "b🌺&d" || !got[0].Text.Style.Bold || got[1].Text.ID != secondRun.ID || got[1].Text.Text != " normal" {
		t.Fatalf("range replacement changed run contents or formatting unexpectedly: %#v", got)
	}
	if bytes.Equal(before, doc.parts[doc.documentIdx].data) {
		t.Fatal("range replacement did not update word/document.xml")
	}
	changed := append([]byte(nil), doc.parts[doc.documentIdx].data...)
	if err := requireWordTextRangeEditor(t, doc).ReplaceTextRange(firstRun.ID, 0, 99, "invalid"); err == nil {
		t.Fatal("out-of-range replacement unexpectedly succeeded")
	}
	if !bytes.Equal(changed, doc.parts[doc.documentIdx].data) {
		t.Fatal("rejected out-of-range replacement mutated word/document.xml")
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
	reopenedRuns := reopened.WordBlocks()[0].Paragraph.Inlines
	if reopenedRuns[0].Text.Text != "b🌺&d" || !reopenedRuns[0].Text.Style.Bold || reopenedRuns[1].Text.Text != " normal" {
		t.Fatalf("range replacement did not survive save/reopen: %#v", reopenedRuns)
	}
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

func TestWordParagraphIndentsRoundTripAndPreserveOtherParagraphProperties(t *testing.T) {
	documentXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:pPr><w:keepNext/><w:ind w:firstLine="360" w:hanging="120" w:left="720" w:right="180"/></w:pPr><w:r><w:t>adjusted</w:t></w:r></w:p><w:p><w:r><w:t>untouched</w:t></w:r></w:p></w:body></w:document>`
	doc := projectionFixture(t, documentXML, "", nil)
	defer doc.Close()
	paragraph := doc.WordBlocks()[0].Paragraph
	const leftIndentEMU, rightIndentEMU int64 = 914400, 457200
	if err := requireWordTextEditor(t, doc).SetParagraphIndents(paragraph.ID, leftIndentEMU, rightIndentEMU); err != nil {
		t.Fatal(err)
	}
	if got := doc.WordBlocks()[0].Paragraph; got.LeftIndentEMU != leftIndentEMU || got.RightIndentEMU != rightIndentEMU {
		t.Fatalf("projected paragraph indents = %d/%d EMU; want %d/%d", got.LeftIndentEMU, got.RightIndentEMU, leftIndentEMU, rightIndentEMU)
	}
	output := filepath.Join(t.TempDir(), "indented.docx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	blocks := reopened.WordBlocks()
	got := blocks[0].Paragraph
	if got.LeftIndentEMU != leftIndentEMU || got.RightIndentEMU != rightIndentEMU {
		t.Fatalf("reopened paragraph indents = %d/%d EMU; want %d/%d", got.LeftIndentEMU, got.RightIndentEMU, leftIndentEMU, rightIndentEMU)
	}
	if got := blocks[0].Paragraph.Inlines[0].Text.Text; got != "adjusted" {
		t.Fatalf("edited paragraph text changed to %q", got)
	}
	if got := blocks[1].Paragraph.Inlines[0].Text.Text; got != "untouched" {
		t.Fatalf("untouched paragraph text changed to %q", got)
	}
	if blocks[1].Paragraph.LeftIndentEMU != 0 || blocks[1].Paragraph.RightIndentEMU != 0 {
		t.Fatalf("untouched paragraph gained indents %d/%d EMU", blocks[1].Paragraph.LeftIndentEMU, blocks[1].Paragraph.RightIndentEMU)
	}
	snapshot, err := reopened.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	savedXML := readZipPart(t, snapshot, "word/document.xml")
	for _, preserved := range [][]byte{[]byte(`<w:keepNext/>`), []byte(`w:firstLine="360"`), []byte(`w:hanging="120"`)} {
		if !bytes.Contains(savedXML, preserved) {
			t.Errorf("indent edit did not preserve %s in document.xml: %s", preserved, savedXML)
		}
	}
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

func TestWordFormatPainterCopiesSupportedTextAndParagraphFormatting(t *testing.T) {
	documentXML := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:pPr><w:pStyle w:val="Title"/><w:jc w:val="center"/><w:ind w:left="720" w:right="360"/><w:keepNext/></w:pPr><w:r><w:rPr><w:rStyle w:val="Emphasis"/><w:rFonts w:ascii="Aptos" w:eastAsia="Noto Sans CJK SC"/><w:b/><w:i w:val="0"/><w:u w:val="single"/><w:color w:val="123456"/><w:sz w:val="30"/><w:highlight w:val="yellow"/></w:rPr><w:t>Source</w:t></w:r></w:p>
<w:p><w:pPr><w:pStyle w:val="Normal"/><w:jc w:val="right"/><w:ind w:left="120" w:right="60"/><w:keepNext/><w:spacing w:before="120"/></w:pPr><w:r><w:rPr><w:i/><w:color w:val="ABCDEF"/><w:vanish/></w:rPr><w:t>Target</w:t></w:r></w:p>
</w:body></w:document>`
	doc := projectionFixture(t, documentXML, "", nil)
	defer doc.Close()
	originalPackage, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	blocks := doc.WordBlocks()
	sourceParagraph, targetParagraph := blocks[0].Paragraph, blocks[1].Paragraph
	sourceRun := sourceParagraph.Inlines[0].Text
	targetRun := targetParagraph.Inlines[0].Text
	targetBefore := *targetRun
	targetParagraphBefore := *targetParagraph
	painter, ok := any(doc).(interface {
		ApplyWordFormatting(sourceRunID, targetRunID string) error
	})
	if !ok {
		t.Fatal("DOCX adapter does not expose format painting")
	}
	if err := painter.ApplyWordFormatting(sourceRun.ID, targetRun.ID); err != nil {
		t.Fatal(err)
	}

	blocks = doc.WordBlocks()
	paintedParagraph := blocks[1].Paragraph
	paintedRun := paintedParagraph.Inlines[0].Text
	if paintedRun.Text != "Target" || paintedRun.Style != sourceRun.Style {
		t.Fatalf("painted target text/style = %q/%#v; want unchanged text and source style %#v", paintedRun.Text, paintedRun.Style, sourceRun.Style)
	}
	if paintedParagraph.Alignment != sourceParagraph.Alignment || paintedParagraph.LeftIndentEMU != sourceParagraph.LeftIndentEMU || paintedParagraph.RightIndentEMU != sourceParagraph.RightIndentEMU || paintedParagraph.Style != sourceParagraph.Style {
		t.Fatalf("painted paragraph style = %#v; want source paragraph alignment, style, and indents %#v", paintedParagraph, sourceParagraph)
	}
	data := doc.parts[doc.documentIdx].data
	if !strings.Contains(string(data[doc.wordTextTargets[targetRun.ID].run.start:doc.wordTextTargets[targetRun.ID].run.end]), "w:vanish") {
		t.Fatal("format painting removed an unsupported target run property")
	}
	if !strings.Contains(string(data[doc.wordParagraphTargets[targetParagraph.ID].start:doc.wordParagraphTargets[targetParagraph.ID].end]), `w:spacing w:before="120"`) {
		t.Fatal("format painting removed an unsupported target paragraph property")
	}

	history := requireWordHistory(t, doc)
	if undone, err := history.Undo(); err != nil || !undone {
		t.Fatalf("undo painted formatting: undone=%v err=%v", undone, err)
	}
	blocks = doc.WordBlocks()
	if got := blocks[1].Paragraph.Inlines[0].Text.Style; got != targetBefore.Style || blocks[1].Paragraph.Alignment != targetParagraphBefore.Alignment {
		t.Fatalf("undo restored style/alignment = %#v/%q; want %#v/%q", got, blocks[1].Paragraph.Alignment, targetBefore.Style, targetParagraphBefore.Alignment)
	}
	if redone, err := history.Redo(); err != nil || !redone {
		t.Fatalf("redo painted formatting: redone=%v err=%v", redone, err)
	}

	output := filepath.Join(t.TempDir(), "painted.docx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got := reopened.WordBlocks()[1].Paragraph.Inlines[0].Text.Style; got != sourceRun.Style {
		t.Fatalf("reopened painted style = %#v; want %#v", got, sourceRun.Style)
	}
	assertPackagePartsEqual(t, originalPackage, output)
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
