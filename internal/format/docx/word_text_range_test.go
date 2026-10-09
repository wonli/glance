package docx

import (
	"bytes"
	"glance/internal/document"
	"image/color"
	"testing"
)

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
