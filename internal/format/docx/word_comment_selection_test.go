package docx

import (
	"bytes"
	"strings"
	"testing"

	"glance/internal/document"
)

type wordTextSelectionCommentEditor interface {
	AddWordCommentForTextSelection(startRunID string, startOffset int, endRunID string, endOffset int, author, text string) (string, bool, error)
}

func addWordCommentForSelection(t *testing.T, d *Document, startRunID string, startOffset int, endRunID string, endOffset int, author, text string) string {
	t.Helper()
	editor, ok := any(d).(wordTextSelectionCommentEditor)
	if !ok {
		t.Fatal("DOCX lacks text selection comment editor")
	}
	id, changed, err := editor.AddWordCommentForTextSelection(startRunID, startOffset, endRunID, endOffset, author, text)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || id == "" {
		t.Fatalf("add selected text comment = %q, changed=%v", id, changed)
	}
	return id
}

func TestAddWordCommentForPartialTextSelectionAnchorsOnlySelectedRunes(t *testing.T) {
	d := selectionDeleteFixture(t, `<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>甲豆豆乙</w:t></w:r></w:p>`)
	defer d.Close()
	run := d.WordBlocks()[0].Paragraph.Inlines[0].Text

	id := addWordCommentForSelection(t, d, run.ID, 1, run.ID, 3, "Review", "请确认")
	xml := string(d.parts[d.documentIdx].data)
	start := strings.Index(xml, `<w:commentRangeStart w:id="`+id+`"/>`)
	selected := strings.Index(xml, ">豆豆</w:t>")
	end := strings.Index(xml, `<w:commentRangeEnd w:id="`+id+`"/>`)
	prefix := strings.Index(xml, ">甲</w:t>")
	suffix := strings.Index(xml, ">乙</w:t>")
	if prefix < 0 || start < 0 || selected < 0 || end < 0 || suffix < 0 || !(prefix < start && start < selected && selected < end && end < suffix) {
		t.Fatalf("comment range does not cover only selected text: %s", xml)
	}
	if !strings.Contains(xml, `<w:commentReference w:id="`+id+`"/>`) || !strings.Contains(xml, "<w:b") {
		t.Fatalf("comment reference missing or run formatting lost: %s", xml)
	}
	comments, err := d.WordComments()
	if err != nil || len(comments) != 1 || comments[0].ID != id || comments[0].Text != "请确认" || comments[0].RunID == "" {
		t.Fatalf("comment projection = %+v, err=%v", comments, err)
	}
	snapshot, err := d.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSnapshot(snapshot)
	if err != nil {
		t.Fatalf("reopen commented DOCX: %v", err)
	}
	defer reopened.Close()
	reopenedComments, err := reopened.WordComments()
	if err != nil || len(reopenedComments) != 1 || reopenedComments[0].ID != id || reopenedComments[0].RunID == "" {
		t.Fatalf("reopened selected comment = %+v, err=%v", reopenedComments, err)
	}
}

func TestAddWordCommentForCrossRunTextSelection(t *testing.T) {
	d := selectionDeleteFixture(t, `<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>甲AB</w:t></w:r><w:r><w:rPr><w:i/></w:rPr><w:t>CD乙</w:t></w:r></w:p>`)
	defer d.Close()
	inlines := d.WordBlocks()[0].Paragraph.Inlines
	id := addWordCommentForSelection(t, d, inlines[0].Text.ID, 1, inlines[1].Text.ID, 2, "Review", "确认范围")
	xml := string(d.parts[d.documentIdx].data)
	start := strings.Index(xml, `<w:commentRangeStart w:id="`+id+`"/>`)
	firstSelected := strings.Index(xml, ">AB</w:t>")
	secondSelected := strings.Index(xml, ">CD</w:t>")
	end := strings.Index(xml, `<w:commentRangeEnd w:id="`+id+`"/>`)
	if start < 0 || firstSelected < 0 || secondSelected < 0 || end < 0 || !(start < firstSelected && firstSelected < secondSelected && secondSelected < end) {
		t.Fatalf("comment range does not cover the cross-run selection: %s", xml)
	}
	if !strings.Contains(xml, ">甲</w:t>") || !strings.Contains(xml, ">乙</w:t>") || !strings.Contains(xml, "<w:b") || !strings.Contains(xml, "<w:i") {
		t.Fatalf("cross-run comment did not preserve unselected text and run styling: %s", xml)
	}
}

func TestAddWordCommentForTextSelectionRejectsUnsupportedContentAtomically(t *testing.T) {
	d := selectionDeleteFixture(t, `<w:p><w:r><w:t>甲A</w:t><w:tab/><w:t>B乙</w:t></w:r></w:p>`)
	defer d.Close()
	inlines := d.WordBlocks()[0].Paragraph.Inlines
	if len(inlines) < 3 || inlines[0].Text == nil || inlines[2].Text == nil {
		t.Fatalf("unexpected fixture inlines: %#v", inlines)
	}
	before, err := d.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	editor, ok := any(d).(wordTextSelectionCommentEditor)
	if !ok {
		t.Fatal("DOCX lacks text selection comment editor")
	}
	if _, _, err := editor.AddWordCommentForTextSelection(inlines[0].Text.ID, 1, inlines[2].Text.ID, 1, "Review", "不应改动"); err == nil {
		t.Fatal("comment insertion accepted a selection containing a tab")
	}
	after, err := d.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("rejected comment insertion changed the DOCX package")
	}
	if history := any(d).(document.WordHistory); history.CanUndo() {
		t.Fatal("rejected comment insertion created an undo record")
	}
}
