package docx

import (
	"path/filepath"
	"strings"
	"testing"

	"glance/internal/document"
)

func TestAppendBodyTextParagraphEscapesPreservesAndRoundTrips(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatalf("NewBlank: %v", err)
	}
	t.Cleanup(func() { _ = doc.Close() })
	want := " 本周 <总结 & 发薪> "
	paragraphID, runID, err := doc.AppendBodyTextParagraph(want)
	if err != nil {
		t.Fatalf("AppendBodyTextParagraph: %v", err)
	}
	if paragraphID == "" || runID == "" {
		t.Fatalf("append identities = paragraph %q, run %q", paragraphID, runID)
	}
	if got := findWordRunText(doc.WordBlocks(), runID); got != want {
		t.Fatalf("appended run text = %q, want %q", got, want)
	}
	if !strings.Contains(string(doc.parts[doc.documentIdx].data), "&lt;总结 &amp; 发薪&gt;") {
		t.Fatal("appended paragraph text was not XML escaped")
	}

	path := filepath.Join(t.TempDir(), "report.docx")
	if err := doc.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open after save: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if got := findWordRunText(reopened.WordBlocks(), runID); got != want {
		t.Fatalf("reopened run text = %q, want %q", got, want)
	}
}

func TestAppendBodyTextParagraphRejectsLineBreakWithoutMutation(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatalf("NewBlank: %v", err)
	}
	t.Cleanup(func() { _ = doc.Close() })
	before := doc.WordBlocks()
	wasDirty := doc.Dirty()
	if _, _, err := doc.AppendBodyTextParagraph("line one\nline two"); err == nil {
		t.Fatal("AppendBodyTextParagraph accepted a line break")
	}
	if doc.Dirty() != wasDirty {
		t.Fatalf("rejected append changed dirty state from %v to %v", wasDirty, doc.Dirty())
	}
	if got := doc.WordBlocks(); len(got) != len(before) {
		t.Fatalf("rejected append changed block count from %d to %d", len(before), len(got))
	}
}

func findWordRunText(blocks []document.WordBlock, runID string) string {
	for _, block := range blocks {
		if block.Paragraph == nil {
			continue
		}
		for _, inline := range block.Paragraph.Inlines {
			if inline.Text != nil && inline.Text.ID == runID {
				return inline.Text.Text
			}
		}
	}
	return ""
}
