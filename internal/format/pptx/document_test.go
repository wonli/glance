package pptx

import (
	"os"
	"path/filepath"
	"testing"

	"glance/internal/document"
)

func TestNewBlankPresentation(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatalf("NewBlank failed: %v", err)
	}
	defer doc.Close()

	if doc.SlideCount() < 1 {
		t.Fatalf("expected at least 1 slide in blank presentation, got %d", doc.SlideCount())
	}

	snap := doc.Snapshot()
	if snap.SlideCount != doc.SlideCount() {
		t.Fatalf("snapshot slide count mismatch: %d vs %d", snap.SlideCount, doc.SlideCount())
	}
	if snap.WidthPT != 960 || snap.HeightPT != 540 {
		t.Fatalf("expected 960x540 pt dimensions, got %fx%f", snap.WidthPT, snap.HeightPT)
	}
}

func TestSlideAddAndDelete(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatalf("NewBlank failed: %v", err)
	}
	defer doc.Close()

	initialCount := doc.SlideCount()
	if err := doc.AddSlide(); err != nil {
		t.Fatalf("AddSlide failed: %v", err)
	}
	if doc.SlideCount() != initialCount+1 {
		t.Fatalf("expected %d slides, got %d", initialCount+1, doc.SlideCount())
	}

	// Delete newly added slide
	if err := doc.DeleteSlide(initialCount); err != nil {
		t.Fatalf("DeleteSlide failed: %v", err)
	}
	if doc.SlideCount() != initialCount {
		t.Fatalf("expected back to %d slides, got %d", initialCount, doc.SlideCount())
	}

	// Cannot delete last slide
	for doc.SlideCount() > 1 {
		_ = doc.DeleteSlide(0)
	}
	if err := doc.DeleteSlide(0); err == nil {
		t.Fatalf("expected error deleting the last slide, but succeeded")
	}
}

func TestUpdateSlideTextAndNotes(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatalf("NewBlank failed: %v", err)
	}
	defer doc.Close()

	snap := doc.Snapshot()
	if len(snap.Slides) == 0 || len(snap.Slides[0].Elements) == 0 {
		t.Fatalf("expected elements on slide 0")
	}

	elemID := snap.Slides[0].Elements[0].ID
	newText := "更新后的标题文字"
	if err := doc.UpdateSlideText(0, elemID, newText); err != nil {
		t.Fatalf("UpdateSlideText failed: %v", err)
	}

	if err := doc.UpdateSlideNotes(0, "这是演讲者备注"); err != nil {
		t.Fatalf("UpdateSlideNotes failed: %v", err)
	}

	snap2 := doc.Snapshot()
	if snap2.Slides[0].Notes != "这是演讲者备注" {
		t.Fatalf("expected updated notes, got %q", snap2.Slides[0].Notes)
	}

	var foundText bool
	for _, el := range snap2.Slides[0].Elements {
		if el.ID == elemID && el.PlainText() == newText {
			foundText = true
			break
		}
	}
	if !foundText {
		t.Fatalf("updated text %q not found on element %d", newText, elemID)
	}
}

func TestSaveAsAndReopenRoundTrip(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatalf("NewBlank failed: %v", err)
	}
	defer doc.Close()

	_ = doc.AddSlide()
	snap := doc.Snapshot()
	elemID := snap.Slides[0].Elements[0].ID
	_ = doc.UpdateSlideText(0, elemID, "持久化测试标题")
	_ = doc.UpdateSlideNotes(0, "持久化测试备注")

	tmpPath := filepath.Join(t.TempDir(), "roundtrip.pptx")
	if err := doc.SaveAs(tmpPath); err != nil {
		t.Fatalf("SaveAs failed: %v", err)
	}

	if doc.IsDirty() {
		t.Fatalf("document should not be dirty after successful save")
	}

	// Reopen
	doc2, err := Open(tmpPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer doc2.Close()

	if doc2.SlideCount() != doc.SlideCount() {
		t.Fatalf("slide count mismatch: got %d, want %d", doc2.SlideCount(), doc.SlideCount())
	}

	snap2 := doc2.Snapshot()
	if snap2.Slides[0].Notes != "持久化测试备注" {
		t.Fatalf("notes mismatch after reopen: got %q", snap2.Slides[0].Notes)
	}
}

func TestSnapshotBytesAndOpenBytes(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatalf("NewBlank failed: %v", err)
	}
	defer doc.Close()

	_ = doc.AddSlide()
	snap := doc.Snapshot()
	elemID := snap.Slides[0].Elements[0].ID
	_ = doc.UpdateSlideText(0, elemID, "快照恢复标题")

	data, err := doc.SnapshotBytes()
	if err != nil {
		t.Fatalf("SnapshotBytes failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("SnapshotBytes returned empty slice")
	}

	docRestored, err := OpenBytes(data)
	if err != nil {
		t.Fatalf("OpenBytes failed: %v", err)
	}
	defer docRestored.Close()

	if docRestored.SlideCount() != doc.SlideCount() {
		t.Fatalf("restored slide count mismatch: got %d, want %d", docRestored.SlideCount(), doc.SlideCount())
	}
	restoredSnap := docRestored.Snapshot()
	if len(restoredSnap.Slides[0].Elements) == 0 || restoredSnap.Slides[0].Elements[0].PlainText() != "快照恢复标题" {
		t.Fatalf("restored title mismatch: got %+v", restoredSnap.Slides[0].Elements)
	}
}

func TestInspectUserSample1(t *testing.T) {
	doc, err := Open("/Users/mima1234/Downloads/1.pptx")
	if err != nil {
		t.Skipf("cannot open sample 1.pptx: %v", err)
	}
	defer doc.Close()

	snap := doc.Snapshot()
	if snap.SlideCount != 30 {
		t.Fatalf("expected 30 slides, got %d", snap.SlideCount)
	}
	if snap.WidthPT != 631.5 || snap.HeightPT != 355.5 {
		t.Fatalf("expected 631.5x355.5 dimensions, got %.1fx%.1f", snap.WidthPT, snap.HeightPT)
	}

	// Slide 1 checks: Title (TextBox 2)
	slide1 := snap.Slides[0]
	var foundTitle bool
	for _, el := range slide1.Elements {
		if el.PlainText() == "Title" {
			foundTitle = true
			if el.WrapText {
				t.Fatalf("expected Title to have WrapText=false, got true")
			}
			if !el.AutoFit {
				t.Fatalf("expected Title to have AutoFit=true, got false")
			}
			if len(el.Paragraphs) == 0 || el.Paragraphs[0].Align != "center" {
				t.Fatalf("expected Title to have center alignment, got %q", el.Paragraphs[0].Align)
			}
			if len(el.Paragraphs[0].Runs) == 0 || el.Paragraphs[0].Runs[0].SizePT != 37.2 {
				t.Fatalf("expected Title font size 37.2pt, got %.1f", el.Paragraphs[0].Runs[0].SizePT)
			}
		}
	}
	if !foundTitle {
		t.Fatalf("Title element not found on slide 1")
	}

	// Slide 2 checks: Right-aligned "1 / 16" (TextBox 124)
	slide2 := snap.Slides[1]
	var foundFooter bool
	for _, el := range slide2.Elements {
		if el.PlainText() == "1 / 16" {
			foundFooter = true
			if len(el.Paragraphs) == 0 || el.Paragraphs[0].Align != "right" {
				t.Fatalf("expected '1 / 16' to have right alignment, got %q", el.Paragraphs[0].Align)
			}
		}
	}
	if !foundFooter {
		t.Fatalf("'1 / 16' footer element not found on slide 2")
	}
}

func TestUserSampleTextEditSaveAsPreservesRunStyleAndUntouchedContent(t *testing.T) {
	const sourcePath = "/Users/mima1234/Downloads/1.pptx"
	sourceBefore, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Skipf("cannot read sample 1.pptx: %v", err)
	}
	doc, err := Open(sourcePath)
	if err != nil {
		t.Skipf("cannot open sample 1.pptx: %v", err)
	}
	defer doc.Close()

	before := doc.Snapshot()
	var target document.PresentationElement
	var untouched document.PresentationElement
	for _, elem := range before.Slides[0].Elements {
		if elem.PlainText() == "Title" {
			target = elem
		}
	}
	if target.ID == 0 {
		t.Fatal("sample slide 1 title was not found")
	}
	for _, elem := range before.Slides[1].Elements {
		if elem.PlainText() != "" {
			untouched = elem
			break
		}
	}
	if untouched.ID == 0 {
		t.Fatal("sample slide 2 has no non-empty text element to check")
	}
	if len(target.Paragraphs) == 0 || len(target.Paragraphs[0].Runs) == 0 {
		t.Fatal("sample title has no projected text run")
	}
	wantStyle := target.Paragraphs[0].Runs[0]
	if err := doc.UpdateSlideText(0, target.ID, target.PlainText()+"\nsecond paragraph"); err == nil {
		t.Fatal("text edit that changes paragraph count should be rejected")
	}
	if doc.IsDirty() {
		t.Fatal("rejected paragraph-count edit changed the document dirty state")
	}
	var rejectedText string
	for _, elem := range doc.Snapshot().Slides[0].Elements {
		if elem.ID == target.ID {
			rejectedText = elem.PlainText()
			break
		}
	}
	if rejectedText != target.PlainText() {
		t.Fatalf("rejected paragraph-count edit changed source text: got %q, want %q", rejectedText, target.PlainText())
	}
	newText := target.PlainText() + " edited"
	if err := doc.UpdateSlideText(0, target.ID, newText); err != nil {
		t.Fatalf("UpdateSlideText failed: %v", err)
	}

	copyPath := filepath.Join(t.TempDir(), "edited-copy.pptx")
	if err := doc.SaveAs(copyPath); err != nil {
		t.Fatalf("SaveAs to disposable copy failed: %v", err)
	}
	reopened, err := Open(copyPath)
	if err != nil {
		t.Fatalf("reopen edited copy failed: %v", err)
	}
	defer reopened.Close()
	after := reopened.Snapshot()

	var edited document.PresentationElement
	for _, elem := range after.Slides[0].Elements {
		if elem.ID == target.ID {
			edited = elem
			break
		}
	}
	if edited.PlainText() != newText {
		t.Fatalf("reopened title = %q, want %q", edited.PlainText(), newText)
	}
	if len(edited.Paragraphs) == 0 || len(edited.Paragraphs[0].Runs) == 0 {
		t.Fatal("reopened edited title has no text run")
	}
	gotStyle := edited.Paragraphs[0].Runs[0]
	if gotStyle.Font != wantStyle.Font || gotStyle.SizePT != wantStyle.SizePT || gotStyle.Bold != wantStyle.Bold || gotStyle.Italic != wantStyle.Italic || gotStyle.Underline != wantStyle.Underline || gotStyle.ColorHex != wantStyle.ColorHex || gotStyle.Baseline != wantStyle.Baseline {
		t.Fatalf("edited title run style changed: got %+v, want %+v", gotStyle, wantStyle)
	}
	var unchangedText string
	for _, elem := range after.Slides[1].Elements {
		if elem.ID == untouched.ID {
			unchangedText = elem.PlainText()
			break
		}
	}
	if unchangedText != untouched.PlainText() {
		t.Fatalf("untouched slide 2 text changed: got %q, want %q", unchangedText, untouched.PlainText())
	}
	sourceAfter, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(sourceAfter) != string(sourceBefore) {
		t.Fatal("SaveAs modified the original sample file")
	}
}
