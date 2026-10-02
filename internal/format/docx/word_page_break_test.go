package docx

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

type wordPageBreakTestCapability interface {
	InsertPageBreakAtRange(runID, value string, start, end int) (paragraphID, caretRunID string, err error)
}

func requireWordPageBreakEditor(t *testing.T, doc *Document) wordPageBreakTestCapability {
	t.Helper()
	editor, ok := any(doc).(wordPageBreakTestCapability)
	if !ok {
		t.Fatal("DOCX adapter does not expose page-break insertion at a text range")
	}
	return editor
}

func TestInsertWordPageBreakAtCaretPreservesRunFormattingAndRoundTrips(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	initial, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	blocks := doc.WordBlocks()
	runID := blocks[0].Paragraph.Inlines[0].Text.ID
	paragraphID, caretRunID, err := requireWordPageBreakEditor(t, doc).InsertPageBreakAtRange(runID, "bold", 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if paragraphID != blocks[0].Paragraph.ID || caretRunID == "" {
		t.Fatalf("page-break result paragraph/run=%q/%q; want %q/nonempty", paragraphID, caretRunID, blocks[0].Paragraph.ID)
	}

	paragraph := doc.WordBlocks()[0].Paragraph
	if len(paragraph.Inlines) != 4 {
		t.Fatalf("inserted paragraph inlines = %#v; want prefix, page break, suffix, following run", paragraph.Inlines)
	}
	if got := paragraph.Inlines[0].Text; got == nil || got.Text != "bo" || !got.Style.Bold {
		t.Fatalf("prefix after page break = %#v; want bold text %q", got, "bo")
	}
	if !isWordPageBreakInline(t, paragraph.Inlines[1]) {
		t.Fatalf("middle inline %#v is not projected as a page break", paragraph.Inlines[1])
	}
	if got := paragraph.Inlines[2].Text; got == nil || got.Text != "ld" || !got.Style.Bold || got.ID != caretRunID {
		t.Fatalf("suffix after page break = %#v; returned caret run %q", got, caretRunID)
	}
	if got := paragraph.Inlines[3].Text; got == nil || got.Text != " normal" || !got.Style.Italic {
		t.Fatalf("following run changed during page-break insertion: %#v", got)
	}

	data, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(readZipPart(t, data, "word/document.xml"), []byte(`<w:br w:type="page"/>`)) {
		t.Fatalf("DOCX XML does not contain a native page break: %s", readZipPart(t, data, "word/document.xml"))
	}
	reopened, err := OpenSnapshot(data)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedParagraph := reopened.WordBlocks()[0].Paragraph
	if len(reopenedParagraph.Inlines) != 4 || !isWordPageBreakInline(t, reopenedParagraph.Inlines[1]) {
		t.Fatalf("reopened page-break projection = %#v", reopenedParagraph.Inlines)
	}

	history := requireWordHistory(t, doc)
	if changed, err := history.Undo(); err != nil || !changed {
		t.Fatalf("Undo() = %v, %v; want page-break edit undone", changed, err)
	}
	if got := doc.WordBlocks()[0].Paragraph.Inlines[0].Text.Text; got != "bold" {
		t.Fatalf("undo restored text %q; want %q", got, "bold")
	}
	if changed, err := history.Redo(); err != nil || !changed {
		t.Fatalf("Redo() = %v, %v; want page-break edit restored", changed, err)
	}
	if restored, err := doc.Snapshot(); err != nil || !bytes.Equal(restored, data) {
		t.Fatalf("redo did not restore exact package snapshot: err=%v", err)
	}
	if bytes.Equal(initial, data) {
		t.Fatal("page-break insertion did not change the DOCX package")
	}
}

func TestInsertWordPageBreakReplacesSelectionAndEmptyRunStaysEditable(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	initialRun := doc.WordBlocks()[0].Paragraph.Inlines[0].Text
	if err := requireWordTextEditor(t, doc).ReplaceTextRun(initialRun.ID, "左删词右"); err != nil {
		t.Fatal(err)
	}
	paragraphID, caretRunID, err := requireWordPageBreakEditor(t, doc).InsertPageBreakAtRange(initialRun.ID, "左删词右", 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	paragraph := doc.WordBlocks()[0].Paragraph
	if paragraph.ID != paragraphID || len(paragraph.Inlines) != 3 {
		t.Fatalf("selection replacement inlines = %#v", paragraph.Inlines)
	}
	if paragraph.Inlines[0].Text == nil || paragraph.Inlines[0].Text.Text != "左" || !isWordPageBreakInline(t, paragraph.Inlines[1]) {
		t.Fatalf("page break did not replace the selected text: %#v", paragraph.Inlines)
	}
	if suffix := paragraph.Inlines[2].Text; suffix == nil || suffix.Text != "右" || suffix.ID != caretRunID || !suffix.Editable {
		t.Fatalf("page-break caret run is not the editable suffix: %#v", suffix)
	}
}

func TestInsertWordPageBreakRejectsInvalidRangeAndTableParagraph(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	before, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	blocks := doc.WordBlocks()
	bodyRunID := blocks[0].Paragraph.Inlines[0].Text.ID
	if _, _, err := requireWordPageBreakEditor(t, doc).InsertPageBreakAtRange(bodyRunID, "bold", -1, 1); err == nil {
		t.Fatal("negative page-break range should be rejected")
	}
	tableRunID, _ := findRunByText(t, blocks, "cell text")
	if _, _, err := requireWordPageBreakEditor(t, doc).InsertPageBreakAtRange(tableRunID, "cell text", 4, 4); err == nil {
		t.Fatal("page-break insertion into a table cell should be rejected")
	}
	after, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if doc.Dirty() || !bytes.Equal(before, after) {
		t.Fatal("rejected page-break insertion changed the DOCX package or dirty state")
	}
}

func isWordPageBreakInline(t *testing.T, inline any) bool {
	t.Helper()
	value := reflect.ValueOf(inline)
	if value.Kind() != reflect.Struct {
		t.Fatalf("page-break inline has kind %s, want struct", value.Kind())
	}
	field := value.FieldByName("PageBreak")
	return field.IsValid() && ((field.Kind() == reflect.Pointer && !field.IsNil()) || (field.Kind() == reflect.Struct && !field.IsZero()))
}

func TestWordPageBreakXMLContainsOnlyOneNativeMarker(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	blocks := doc.WordBlocks()
	runID := blocks[0].Paragraph.Inlines[0].Text.ID
	if _, _, err := requireWordPageBreakEditor(t, doc).InsertPageBreakAtRange(runID, "bold", 0, 4); err != nil {
		t.Fatal(err)
	}
	data, err := doc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	xml := string(readZipPart(t, data, "word/document.xml"))
	if count := strings.Count(xml, `w:type="page"`); count != 1 {
		t.Fatalf("serialized DOCX has %d native page-break markers: %s", count, xml)
	}
}
