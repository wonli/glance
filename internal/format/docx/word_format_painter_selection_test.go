package docx

import (
	"bytes"
	"strings"
	"testing"

	"glance/internal/document"
)

type wordTextSelectionFormatPainter interface {
	ApplyWordFormattingToTextSelection(sourceRunID, startRunID string, startOffset int, endRunID string, endOffset int) (bool, error)
}

func TestWordFormatPainterAppliesCharacterStyleOnlyToSelectedText(t *testing.T) {
	documentXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:pPr><w:jc w:val="center"/><w:ind w:left="720"/></w:pPr><w:r><w:rPr><w:rFonts w:ascii="Aptos"/><w:b/><w:color w:val="123456"/><w:sz w:val="30"/></w:rPr><w:t>来源</w:t></w:r></w:p>
<w:p><w:pPr><w:jc w:val="right"/><w:spacing w:before="120"/></w:pPr><w:r><w:rPr><w:i/><w:vanish/></w:rPr><w:t>前目标后</w:t></w:r></w:p>
</w:body></w:document>`
	doc := projectionFixture(t, documentXML, "", nil)
	defer doc.Close()
	before, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	blocks := doc.WordBlocks()
	sourceRun := blocks[0].Paragraph.Inlines[0].Text
	targetRun := blocks[1].Paragraph.Inlines[0].Text
	painter, ok := any(doc).(wordTextSelectionFormatPainter)
	if !ok {
		t.Fatal("DOCX adapter does not expose selection-aware format painting")
	}
	changed, err := painter.ApplyWordFormattingToTextSelection(sourceRun.ID, targetRun.ID, 1, targetRun.ID, 3)
	if err != nil || !changed {
		t.Fatalf("paint selected target text: changed=%v error=%v", changed, err)
	}

	blocks = doc.WordBlocks()
	paragraph := blocks[1].Paragraph
	if len(paragraph.Inlines) != 3 {
		t.Fatalf("selected formatting produced %d target runs; want prefix/selection/suffix", len(paragraph.Inlines))
	}
	prefix, selected, suffix := paragraph.Inlines[0].Text, paragraph.Inlines[1].Text, paragraph.Inlines[2].Text
	if prefix == nil || selected == nil || suffix == nil || prefix.Text != "前" || selected.Text != "目标" || suffix.Text != "后" {
		t.Fatalf("target text fragments = %#v, %#v, %#v", prefix, selected, suffix)
	}
	if !prefix.Style.Italic || prefix.Style.Bold || !suffix.Style.Italic || suffix.Style.Bold {
		t.Fatalf("unselected fragments lost target formatting: prefix=%#v suffix=%#v", prefix.Style, suffix.Style)
	}
	if selected.Style != sourceRun.Style {
		t.Fatalf("selected text style = %#v; want source style %#v", selected.Style, sourceRun.Style)
	}
	if paragraph.Alignment != blocks[0].Paragraph.Alignment {
		t.Fatalf("target paragraph alignment = %q; want source alignment %q", paragraph.Alignment, blocks[0].Paragraph.Alignment)
	}
	data := doc.parts[doc.documentIdx].data
	selectedTarget := doc.wordTextTargets[selected.ID]
	if !strings.Contains(string(data[selectedTarget.run.start:selectedTarget.run.end]), "w:vanish") {
		t.Fatal("format painting removed an unsupported target run property")
	}
	paragraphTarget := doc.wordParagraphTargets[paragraph.ID]
	if !strings.Contains(string(data[paragraphTarget.start:paragraphTarget.end]), `w:spacing w:before="120"`) {
		t.Fatal("format painting removed an unsupported target paragraph property")
	}
	saved, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSnapshot(saved)
	if err != nil {
		t.Fatalf("reopen painted DOCX: %v", err)
	}
	reopenedBlocks := reopened.WordBlocks()
	reopenedParagraph := reopenedBlocks[1].Paragraph
	if len(reopenedParagraph.Inlines) != 3 || reopenedParagraph.Inlines[1].Text.Text != "目标" || reopenedParagraph.Inlines[1].Text.Style != reopenedBlocks[0].Paragraph.Inlines[0].Text.Style {
		t.Fatalf("painted selection did not survive reopen: %#v", reopenedParagraph.Inlines)
	}
	_ = reopened.Close()

	history := requireWordHistory(t, doc)
	if ok, err := history.Undo(); err != nil || !ok {
		t.Fatalf("Undo selected format paint: ok=%v error=%v", ok, err)
	}
	undone, err := doc.Snapshot()
	if err != nil || !bytes.Equal(undone, before) {
		t.Fatalf("undo did not restore original DOCX package: equal=%v error=%v", bytes.Equal(undone, before), err)
	}
}

func TestWordFormatPainterAppliesSelectionAcrossAdjacentRuns(t *testing.T) {
	documentXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>Source</w:t></w:r></w:p>
<w:p><w:r><w:rPr><w:i/></w:rPr><w:t>甲AB</w:t></w:r><w:r><w:rPr><w:u/></w:rPr><w:t>CD乙</w:t></w:r></w:p>
</w:body></w:document>`
	doc := projectionFixture(t, documentXML, "", nil)
	defer doc.Close()
	blocks := doc.WordBlocks()
	sourceRun := blocks[0].Paragraph.Inlines[0].Text
	first, second := blocks[1].Paragraph.Inlines[0].Text, blocks[1].Paragraph.Inlines[1].Text
	painter, ok := any(doc).(wordTextSelectionFormatPainter)
	if !ok {
		t.Fatal("DOCX adapter does not expose selection-aware format painting")
	}
	if changed, err := painter.ApplyWordFormattingToTextSelection(sourceRun.ID, first.ID, 1, second.ID, 2); err != nil || !changed {
		t.Fatalf("paint cross-run selection: changed=%v error=%v", changed, err)
	}
	paragraph := doc.WordBlocks()[1].Paragraph
	var text strings.Builder
	for _, inline := range paragraph.Inlines {
		if inline.Text != nil {
			text.WriteString(inline.Text.Text)
		}
	}
	if text.String() != "甲ABCD乙" {
		t.Fatalf("format painting changed target text: %q", text.String())
	}
	var selectedText, beforeStyleRetained, afterStyleRetained bool
	for _, inline := range paragraph.Inlines {
		if inline.Text == nil {
			continue
		}
		switch inline.Text.Text {
		case "甲":
			beforeStyleRetained = inline.Text.Style.Italic && !inline.Text.Style.Bold
		case "AB":
			selectedText = inline.Text.Style.Bold
		case "CD":
			selectedText = selectedText && inline.Text.Style.Bold
		case "乙":
			afterStyleRetained = inline.Text.Style.Underline && !inline.Text.Style.Bold
		}
	}
	if !selectedText || !beforeStyleRetained || !afterStyleRetained {
		t.Fatalf("cross-run style result: selected=%v before=%v after=%v inlines=%#v", selectedText, beforeStyleRetained, afterStyleRetained, paragraph.Inlines)
	}
}

func TestWordFormatPainterRejectsUnsupportedSelectionAtomically(t *testing.T) {
	doc := selectionDeleteFixture(t, `<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>Source</w:t></w:r></w:p><w:p><w:r><w:t>左A</w:t><w:tab/><w:t>B右</w:t></w:r></w:p>`)
	defer doc.Close()
	blocks := doc.WordBlocks()
	source := blocks[0].Paragraph.Inlines[0].Text
	inlines := blocks[1].Paragraph.Inlines
	if len(inlines) < 3 || inlines[0].Text == nil || inlines[2].Text == nil {
		t.Fatalf("unexpected tab fixture inlines: %#v", inlines)
	}
	before, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	painter, ok := any(doc).(wordTextSelectionFormatPainter)
	if !ok {
		t.Fatal("DOCX adapter does not expose selection-aware format painting")
	}
	if _, err := painter.ApplyWordFormattingToTextSelection(source.ID, inlines[0].Text.ID, 1, inlines[2].Text.ID, 1); err == nil {
		t.Fatal("format painter accepted a selection containing a tab")
	}
	after, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || any(doc).(document.WordHistory).CanUndo() {
		t.Fatal("rejected format-painter selection changed the package or undo history")
	}
}
