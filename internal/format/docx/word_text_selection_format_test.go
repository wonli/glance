package docx

import (
	"bytes"
	"glance/internal/document"
	"path/filepath"
	"testing"
)

type wordTextSelectionFormatter interface {
	SetTextSelectionStyle(startRunID string, startOffset int, endRunID string, endOffset int, patch document.WordTextFormatPatch) (document.WordTextSelectionFormatResult, error)
}

func formatTextSelection(t *testing.T, d *Document, startID string, startOffset int, endID string, endOffset int, patch document.WordTextFormatPatch) document.WordTextSelectionFormatResult {
	t.Helper()
	formatter, ok := any(d).(wordTextSelectionFormatter)
	if !ok {
		t.Fatal("DOCX lacks selection formatter")
	}
	result, err := formatter.SetTextSelectionStyle(startID, startOffset, endID, endOffset, patch)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestFormatPartialWordTextSelectionPreservesUnselectedText(t *testing.T) {
	d := selectionDeleteFixture(t, `<w:p><w:r><w:rPr><w:vanish/><w:rStyle w:val="custom"/></w:rPr><w:t>甲A🌱B乙</w:t></w:r></w:p>`)
	defer d.Close()
	run := d.WordBlocks()[0].Paragraph.Inlines[0].Text
	yes := true
	result := formatTextSelection(t, d, run.ID, 1, run.ID, 4, document.WordTextFormatPatch{Bold: &yes})
	inlines := d.WordBlocks()[0].Paragraph.Inlines
	if len(inlines) != 3 {
		t.Fatalf("formatted inlines = %#v; want prefix, selection, suffix", inlines)
	}
	for i, inline := range inlines {
		if inline.Text == nil {
			t.Fatalf("formatted inline %d is not text: %#v; XML: %s", i, inlines, d.parts[d.documentIdx].data)
		}
	}
	got := []struct {
		text string
		bold bool
	}{
		{inlines[0].Text.Text, inlines[0].Text.Style.Bold},
		{inlines[1].Text.Text, inlines[1].Text.Style.Bold},
		{inlines[2].Text.Text, inlines[2].Text.Style.Bold},
	}
	want := []struct {
		text string
		bold bool
	}{{"甲", false}, {"A🌱B", true}, {"乙", false}}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("inline %d = %#v; want %#v", i, got[i], want[i])
		}
	}
	if result.StartRunID != inlines[1].Text.ID || result.StartOffset != 0 || result.EndRunID != inlines[1].Text.ID || result.EndOffset != 3 {
		t.Fatalf("formatted selection = %#v; want selected fragment offsets [0,3)", result)
	}
	if count := bytes.Count(d.parts[d.documentIdx].data, []byte("w:vanish")); count != 3 {
		t.Fatalf("split run retained %d copies of the unknown vanish property; want 3", count)
	}
}

func TestFormatWordTextSelectionAcrossRunsAndUndoRoundTrip(t *testing.T) {
	d := selectionDeleteFixture(t, `<w:p><w:r><w:rPr><w:b w:val="0"/></w:rPr><w:t>甲A</w:t></w:r><w:r><w:rPr><w:i/></w:rPr><w:t>B🌱乙</w:t></w:r></w:p>`)
	defer d.Close()
	p := d.WordBlocks()[0].Paragraph
	startID, endID := p.Inlines[0].Text.ID, p.Inlines[1].Text.ID
	before := append([]byte(nil), d.parts[d.documentIdx].data...)
	yes := true
	formatTextSelection(t, d, startID, 1, endID, 2, document.WordTextFormatPatch{Bold: &yes})
	p = d.WordBlocks()[0].Paragraph
	want := []struct {
		text   string
		bold   bool
		italic bool
	}{
		{"甲", false, false}, {"A", true, false}, {"B🌱", true, true}, {"乙", false, true},
	}
	if len(p.Inlines) != len(want) {
		t.Fatalf("formatted runs = %#v; want %d text fragments", p.Inlines, len(want))
	}
	for i, expected := range want {
		actual := p.Inlines[i].Text
		if actual == nil || actual.Text != expected.text || actual.Style.Bold != expected.bold || actual.Style.Italic != expected.italic {
			t.Fatalf("inline %d = %#v; want text=%q bold=%v italic=%v", i, actual, expected.text, expected.bold, expected.italic)
		}
	}
	snapshot, err := d.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "selection-format.docx")
	if err := d.SaveAs(output); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got := reopened.WordBlocks()[0].Paragraph.Inlines[2].Text; got.Text != "B🌱" || !got.Style.Bold || !got.Style.Italic {
		t.Fatalf("saved selection style = %#v", got)
	}
	if ok, err := d.Undo(); err != nil || !ok {
		t.Fatalf("undo: %v %v", ok, err)
	}
	if !bytes.Equal(before, d.parts[d.documentIdx].data) || d.CanUndo() {
		t.Fatal("selection formatting was not a single undo step")
	}
	if len(snapshot) == 0 {
		t.Fatal("selection formatting produced an empty snapshot")
	}
}

func TestFormatWordSelectionWithMultipleTextNodesInOneRun(t *testing.T) {
	d := selectionDeleteFixture(t, `<w:p><w:r><w:rPr><w:i/></w:rPr><w:t>甲A</w:t><w:t>B乙</w:t><w:tab/></w:r></w:p>`)
	defer d.Close()
	p := d.WordBlocks()[0].Paragraph
	startID, endID := p.Inlines[0].Text.ID, p.Inlines[1].Text.ID
	yes := true
	formatTextSelection(t, d, startID, 1, endID, 1, document.WordTextFormatPatch{Bold: &yes})
	p = d.WordBlocks()[0].Paragraph
	if len(p.Inlines) != 5 || p.Inlines[0].Text == nil || p.Inlines[1].Text == nil || p.Inlines[2].Text == nil || p.Inlines[3].Text == nil || p.Inlines[4].Tab == nil {
		t.Fatalf("formatted multiple text nodes = %#v", p.Inlines)
	}
	if p.Inlines[0].Text.Text != "甲" || p.Inlines[1].Text.Text != "A" || p.Inlines[2].Text.Text != "B" || p.Inlines[3].Text.Text != "乙" || p.Inlines[0].Text.Style.Bold || !p.Inlines[1].Text.Style.Bold || !p.Inlines[2].Text.Style.Bold || p.Inlines[3].Text.Style.Bold || !p.Inlines[3].Text.Style.Italic {
		t.Fatalf("multiple text node styles = %#v", p.Inlines)
	}
}

func TestFormatWordTextSelectionAcrossParagraphs(t *testing.T) {
	d := selectionDeleteFixture(t, `<w:p><w:r><w:t>甲AB</w:t></w:r></w:p><w:p><w:r><w:t>CD乙</w:t></w:r></w:p>`)
	defer d.Close()
	blocks := d.WordBlocks()
	start := blocks[0].Paragraph.Inlines[0].Text
	end := blocks[1].Paragraph.Inlines[0].Text
	yes := true
	result := formatTextSelection(t, d, start.ID, 1, end.ID, 1, document.WordTextFormatPatch{Bold: &yes})
	blocks = d.WordBlocks()
	first, second := blocks[0].Paragraph.Inlines, blocks[1].Paragraph.Inlines
	if len(first) != 2 || first[0].Text.Text != "甲" || first[1].Text.Text != "AB" || !first[1].Text.Style.Bold {
		t.Fatalf("first formatted paragraph = %#v", first)
	}
	if len(second) != 2 || second[0].Text.Text != "C" || !second[0].Text.Style.Bold || second[1].Text.Text != "D乙" || second[1].Text.Style.Bold {
		t.Fatalf("second formatted paragraph = %#v", second)
	}
	if result.StartRunID != first[1].Text.ID || result.StartOffset != 0 || result.EndRunID != second[0].Text.ID || result.EndOffset != 1 {
		t.Fatalf("mapped cross-paragraph selection = %#v", result)
	}
}

func TestFormatWordTextSelectionNoOpAndRejectsUnsafeSpan(t *testing.T) {
	d := selectionDeleteFixture(t, `<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>AB</w:t></w:r><w:r><w:tab/></w:r><w:r><w:t>CD</w:t></w:r></w:p>`)
	defer d.Close()
	p := d.WordBlocks()[0].Paragraph
	yes := true
	formatTextSelection(t, d, p.Inlines[0].Text.ID, 0, p.Inlines[0].Text.ID, 2, document.WordTextFormatPatch{Bold: &yes})
	if d.Dirty() || d.CanUndo() {
		t.Fatal("no-op selection formatting changed document history")
	}
	before := append([]byte(nil), d.parts[d.documentIdx].data...)
	formatter := any(d).(wordTextSelectionFormatter)
	if _, err := formatter.SetTextSelectionStyle(p.Inlines[0].Text.ID, 1, p.Inlines[2].Text.ID, 1, document.WordTextFormatPatch{Bold: &yes}); err == nil {
		t.Fatal("selection crossing a tab was accepted")
	}
	if !bytes.Equal(before, d.parts[d.documentIdx].data) || d.CanUndo() || d.Dirty() {
		t.Fatal("rejected selection formatting changed document")
	}
}

func TestFormatWordTextSelectionFontSize(t *testing.T) {
	d := selectionDeleteFixture(t, `<w:p><w:r><w:rPr><w:sz w:val="21"/></w:rPr><w:t>Hello World</w:t></w:r></w:p>`)
	defer d.Close()
	p := d.WordBlocks()[0].Paragraph
	run := p.Inlines[0].Text
	newSize := 16.0
	// Format "World" (offsets 6 to 11)
	result := formatTextSelection(t, d, run.ID, 6, run.ID, 11, document.WordTextFormatPatch{FontSize: &newSize})
	inlines := d.WordBlocks()[0].Paragraph.Inlines
	if len(inlines) != 2 {
		t.Fatalf("expected 2 runs after font size selection format, got %d", len(inlines))
	}
	if inlines[0].Text.Text != "Hello " || inlines[0].Text.Style.FontSize != 10.5 {
		t.Fatalf("prefix run = %q with size %g; want %q with size 10.5", inlines[0].Text.Text, inlines[0].Text.Style.FontSize, "Hello ")
	}
	if inlines[1].Text.Text != "World" || inlines[1].Text.Style.FontSize != 16.0 {
		t.Fatalf("selected run = %q with size %g; want %q with size 16.0", inlines[1].Text.Text, inlines[1].Text.Style.FontSize, "World")
	}
	if result.StartRunID != inlines[1].Text.ID || result.StartOffset != 0 || result.EndRunID != inlines[1].Text.ID || result.EndOffset != 5 {
		t.Fatalf("result = %#v; want [%s:0, %s:5)", result, inlines[1].Text.ID, inlines[1].Text.ID)
	}
}

func TestFormatWordSelectionPreservesBookmarksAndUndoRoundTrip(t *testing.T) {
	const startMarker = `<w:bookmarkStart w:id="7" w:name="section"/>`
	const endMarker = `<w:bookmarkEnd w:id="7"/>`
	d := selectionDeleteFixture(t, `<w:p><w:r><w:t>甲A</w:t></w:r>`+startMarker+`<w:r><w:t>B</w:t></w:r>`+endMarker+`<w:r><w:t>C乙</w:t></w:r></w:p>`)
	defer d.Close()
	before := append([]byte(nil), d.parts[d.documentIdx].data...)
	p := d.WordBlocks()[0].Paragraph
	size := 24.0
	formatTextSelection(t, d, p.Inlines[0].Text.ID, 1, p.Inlines[2].Text.ID, 1, document.WordTextFormatPatch{FontSize: &size})
	path := filepath.Join(t.TempDir(), "bookmarks.docx")
	if err := d.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	xml := reopened.parts[reopened.documentIdx].data
	for _, marker := range []string{startMarker, endMarker} {
		if bytes.Count(xml, []byte(marker)) != 1 {
			t.Fatalf("bookmark was changed: %s", marker)
		}
	}
	runs := reopened.WordBlocks()[0].Paragraph.Inlines
	if len(runs) != 5 {
		t.Fatalf("got %d fragments, want 5", len(runs))
	}
	for i, text := range []string{"甲", "A", "B", "C", "乙"} {
		run := runs[i].Text
		if run == nil || run.Text != text {
			t.Fatalf("fragment %d mismatch", i)
		}
		if (i >= 1 && i <= 3) != (run.Style.FontSize == 24) {
			t.Fatalf("fragment %d has incorrect size", i)
		}
	}
	if ok, err := d.Undo(); err != nil || !ok {
		t.Fatalf("undo: %v %v", ok, err)
	}
	if !bytes.Equal(before, d.parts[d.documentIdx].data) {
		t.Fatal("undo did not restore bookmark XML")
	}
}
