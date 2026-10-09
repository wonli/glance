package docx

import (
	"bytes"
	"glance/internal/document"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestWordParagraphSpacingRoundTripsAndPreservesOtherProperties(t *testing.T) {
	documentXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:pPr><w:keepNext/><w:contextualSpacing/><w:spacing w:before="120" w:after="360" w:beforeLines="20" w:afterAutospacing="1" w:line="240" w:lineRule="auto"/></w:pPr><w:r><w:t>spaced</w:t></w:r></w:p></w:body></w:document>`
	doc := projectionFixture(t, documentXML, "", nil)
	defer doc.Close()
	paragraph := doc.WordBlocks()[0].Paragraph
	editor, ok := any(doc).(document.WordParagraphSpacingEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose paragraph spacing edits")
	}
	before, after := int64(480), int64(0)
	if err := editor.SetParagraphSpacing(paragraph.ID, &before, &after); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "spaced.docx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got := reopened.WordBlocks()[0].Paragraph.Layout
	if got.BeforePT != 24 || got.AfterPT != 0 {
		t.Fatalf("reopened paragraph spacing = %g/%g pt; want 24/0 pt", got.BeforePT, got.AfterPT)
	}
	if got.Set&document.WordParagraphBeforeSet == 0 || got.Set&document.WordParagraphAfterSet == 0 {
		t.Fatalf("reopened spacing mask = %b; both direct values should be explicit", got.Set)
	}
	snapshot, err := reopened.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	savedXML := readZipPart(t, snapshot, "word/document.xml")
	for _, preserved := range [][]byte{[]byte(`<w:keepNext/>`), []byte(`<w:contextualSpacing/>`), []byte(`w:line="240"`), []byte(`w:lineRule="auto"`), []byte(`w:before="480"`), []byte(`w:after="0"`)} {
		if !bytes.Contains(savedXML, preserved) {
			t.Errorf("spacing edit did not preserve or write %s in document.xml: %s", preserved, savedXML)
		}
	}
	for _, removed := range [][]byte{[]byte(`w:beforeLines=`), []byte(`w:afterAutospacing=`)} {
		if bytes.Contains(savedXML, removed) {
			t.Errorf("spacing edit kept conflicting attribute %s: %s", removed, savedXML)
		}
	}
}

func TestWordParagraphSpacingPatchLeavesUnchangedSideAlone(t *testing.T) {
	documentXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:pPr><w:spacing w:beforeLines="20" w:after="360" w:afterAutospacing="1" w:line="240" w:lineRule="auto"/></w:pPr><w:r><w:t>spaced</w:t></w:r></w:p></w:body></w:document>`
	doc := projectionFixture(t, documentXML, "", nil)
	defer doc.Close()
	paragraph := doc.WordBlocks()[0].Paragraph
	editor, ok := any(doc).(wordParagraphSpacingPatcher)
	if !ok {
		t.Fatal("DOCX adapter cannot patch only one paragraph spacing value")
	}
	before := int64(480)
	if err := editor.SetParagraphSpacing(paragraph.ID, &before, nil); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "partial-spacing.docx")
	if err := doc.SaveAs(output); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got := reopened.WordBlocks()[0].Paragraph.Layout
	if got.BeforePT != 24 || got.AfterPT != 18 {
		t.Fatalf("reopened spacing = %g/%g pt; want 24/18 pt", got.BeforePT, got.AfterPT)
	}
	snapshot, err := reopened.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	savedXML := readZipPart(t, snapshot, "word/document.xml")
	for _, preserved := range [][]byte{[]byte(`w:before="480"`), []byte(`w:after="360"`), []byte(`w:afterAutospacing="1"`), []byte(`w:line="240"`), []byte(`w:lineRule="auto"`)} {
		if !bytes.Contains(savedXML, preserved) {
			t.Errorf("one-sided spacing edit did not preserve or write %s: %s", preserved, savedXML)
		}
	}
	if bytes.Contains(savedXML, []byte(`w:beforeLines=`)) {
		t.Errorf("one-sided spacing edit retained the overridden before-lines value: %s", savedXML)
	}
}

func TestSetAllParagraphsSpacingAndUndo(t *testing.T) {
	documentXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>p1</w:t></w:r></w:p><w:p><w:r><w:t>p2</w:t></w:r></w:p></w:body></w:document>`
	doc := projectionFixture(t, documentXML, "", nil)
	defer doc.Close()
	editor, ok := any(doc).(document.WordParagraphSpacingEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose paragraph spacing edits")
	}

	if err := editor.SetAllParagraphsSpacing(0, 80); err != nil {
		t.Fatal(err)
	}

	for i, block := range doc.WordBlocks() {
		layout := block.Paragraph.Layout
		if layout.BeforePT != 0 || layout.AfterPT != 4 {
			t.Fatalf("p%d spacing after compact = %g/%g pt; want 0/4 pt", i+1, layout.BeforePT, layout.AfterPT)
		}
	}

	history := requireWordHistory(t, doc)
	if ok, err := history.Undo(); err != nil || !ok {
		t.Fatalf("Undo failed: ok=%v, err=%v", ok, err)
	}
	for i, block := range doc.WordBlocks() {
		layout := block.Paragraph.Layout
		if layout.Set&(document.WordParagraphBeforeSet|document.WordParagraphAfterSet) != 0 {
			t.Fatalf("p%d spacing after undo still set: %b", i+1, layout.Set)
		}
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
