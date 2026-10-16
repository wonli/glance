package docx

import (
	"bytes"
	"testing"

	"glance/internal/document"
)

func TestReplaceWordTextRunsBatchPreservesFormattingAndUndo(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	paragraph := doc.WordBlocks()[0].Paragraph
	first, second := paragraph.Inlines[0].Text, paragraph.Inlines[1].Text
	editor, ok := any(doc).(document.WordTextBatchEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose atomic batch replacement")
	}

	changed, err := editor.ReplaceTextRuns([]document.WordTextRunReplacement{
		{RunID: first.ID, ExpectedText: first.Text, Replacement: "b🌺ld"},
		{RunID: second.ID, ExpectedText: second.Text, Replacement: " plain"},
	})
	if err != nil || !changed {
		t.Fatalf("ReplaceTextRuns = %v, %v; want one change", changed, err)
	}
	updated := doc.WordBlocks()[0].Paragraph.Inlines
	if updated[0].Text.Text != "b🌺ld" || !updated[0].Text.Style.Bold || updated[1].Text.Text != " plain" || !updated[1].Text.Style.Italic {
		t.Fatalf("batch replacement changed run text or formatting: %#v", updated[:2])
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
	if reopenedRuns[0].Text.Text != "b🌺ld" || !reopenedRuns[0].Text.Style.Bold || reopenedRuns[1].Text.Text != " plain" || !reopenedRuns[1].Text.Style.Italic {
		t.Fatalf("batch replacement did not survive save/reopen: %#v", reopenedRuns[:2])
	}

	history := requireWordHistory(t, doc)
	if applied, err := history.Undo(); err != nil || !applied {
		t.Fatalf("single undo for batch = %v, %v", applied, err)
	}
	restored := doc.WordBlocks()[0].Paragraph.Inlines
	if restored[0].Text.Text != "bold" || restored[1].Text.Text != " normal" {
		t.Fatalf("one undo restored batch text = %q / %q", restored[0].Text.Text, restored[1].Text.Text)
	}
	if applied, err := history.Undo(); err != nil || applied {
		t.Fatalf("second undo after one batch = %v, %v; want no earlier edit", applied, err)
	}
	if applied, err := history.Redo(); err != nil || !applied {
		t.Fatalf("redo batch = %v, %v", applied, err)
	}
}

func TestReplaceWordTextRunsRejectsStaleBatchAtomically(t *testing.T) {
	doc := textEditingFixture(t)
	defer doc.Close()
	paragraph := doc.WordBlocks()[0].Paragraph
	first, second := paragraph.Inlines[0].Text, paragraph.Inlines[1].Text
	editor, ok := any(doc).(document.WordTextBatchEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose atomic batch replacement")
	}
	before := append([]byte(nil), doc.parts[doc.documentIdx].data...)
	changed, err := editor.ReplaceTextRuns([]document.WordTextRunReplacement{
		{RunID: first.ID, ExpectedText: "stale source", Replacement: "must not land"},
		{RunID: second.ID, ExpectedText: second.Text, Replacement: "must not partially land"},
	})
	if err == nil || changed {
		t.Fatalf("stale batch = %v, %v; want atomic rejection", changed, err)
	}
	if !bytes.Equal(before, doc.parts[doc.documentIdx].data) {
		t.Fatal("stale batch partially changed word/document.xml")
	}
	if applied, err := requireWordHistory(t, doc).Undo(); err != nil || applied {
		t.Fatalf("stale batch created undo history: %v, %v", applied, err)
	}
}
