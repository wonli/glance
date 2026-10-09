package docx

import (
	"bytes"
	"glance/internal/document"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestSetColorAndHighlightThenSplitParagraph(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	run := doc.WordBlocks()[0].Paragraph.Inlines[0].Text
	editor := requireWordTextEditor(t, doc)
	if err := editor.ReplaceTextRun(run.ID, "HelloWorld"); err != nil {
		t.Fatal(err)
	}

	color := "FF0000"
	highlight := "yellow"
	if err := editor.SetTextRunStyle(run.ID, document.WordTextFormatPatch{
		FontColor: &color,
		Highlight: &highlight,
	}); err != nil {
		t.Fatalf("SetTextRunStyle failed: %v", err)
	}

	snap, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	docXML := string(readZipPart(t, snap, "word/document.xml"))
	rPrIdx := strings.Index(docXML, "<w:rPr>")
	tIdx := strings.Index(docXML, "<w:t")
	if rPrIdx < 0 || tIdx < 0 || rPrIdx > tIdx {
		t.Fatalf("expected <w:rPr> to appear before <w:t>, got rPr at %d, t at %d in:\n%s", rPrIdx, tIdx, docXML)
	}

	splitter := requireWordParagraphSplitter(t, doc)
	newPID, newRunID, err := splitter.SplitParagraphAt(run.ID, "HelloWorld", 5, 5)
	if err != nil {
		t.Fatalf("SplitParagraphAt failed after setting color and highlight: %v", err)
	}
	if newPID == "" || newRunID == "" {
		t.Fatalf("SplitParagraphAt returned empty IDs: p=%q, run=%q", newPID, newRunID)
	}

	blocks := doc.WordBlocks()
	if len(blocks) != 2 || blocks[0].Paragraph == nil || blocks[1].Paragraph == nil {
		t.Fatalf("expected 2 paragraphs after split, got %d", len(blocks))
	}
	leftRun := blocks[0].Paragraph.Inlines[0].Text
	rightRun := blocks[1].Paragraph.Inlines[0].Text
	if leftRun.Text != "Hello" || rightRun.Text != "World" {
		t.Fatalf("unexpected text after split: left=%q, right=%q", leftRun.Text, rightRun.Text)
	}
	if leftRun.Style.FontColor != "FF0000" || leftRun.Style.Highlight != "yellow" {
		t.Fatalf("left run lost style: color=%q, highlight=%q", leftRun.Style.FontColor, leftRun.Style.Highlight)
	}
	if rightRun.Style.FontColor != "FF0000" || rightRun.Style.Highlight != "yellow" {
		t.Fatalf("right run lost style: color=%q, highlight=%q", rightRun.Style.FontColor, rightRun.Style.Highlight)
	}
}

func TestSplitRunXmlWithPropertiesAfterText(t *testing.T) {

	xmlData := []byte(`<w:r><w:t>HelloWorld</w:t><w:rPr><w:color w:val="FF0000"/><w:highlight w:val="yellow"/></w:rPr></w:r>`)
	runNode, err := parseWordXML(xmlData)
	if err != nil {
		t.Fatal(err)
	}
	targetText := firstChild(runNode, "t")
	if targetText == nil {
		t.Fatal("t node not found")
	}

	left, right, err := splitWordRunXML(xmlData, runNode, targetText, "Hello", "World")
	if err != nil {
		t.Fatalf("splitWordRunXML failed on run with rPr after t: %v", err)
	}

	leftStr := string(left)
	rightStr := string(right)

	for name, str := range map[string]string{"left": leftStr, "right": rightStr} {
		rPrIdx := strings.Index(str, "<w:rPr>")
		tIdx := strings.Index(str, "<w:t")
		if rPrIdx < 0 || tIdx < 0 || rPrIdx > tIdx {
			t.Fatalf("%s run did not place <w:rPr> before <w:t>: %s", name, str)
		}
	}
	if !strings.Contains(leftStr, "Hello") || !strings.Contains(rightStr, "World") {
		t.Fatalf("unexpected contents: left=%s, right=%s", leftStr, rightStr)
	}
}
