package docx

import (
	"strings"
	"testing"

	"glance/internal/document"
)

func TestWordCoverPage_InsertAndUndo(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	initialBlocks := doc.WordBlocks()

	editor, ok := any(doc).(document.WordCoverPageEditor)
	if !ok {
		t.Fatal("doc does not implement WordCoverPageEditor")
	}

	err = editor.InsertCoverPage("年度工作报告", "2026 业务规划与回顾", "项目组")
	if err != nil {
		t.Fatalf("InsertCoverPage failed: %v", err)
	}

	blocks := doc.WordBlocks()
	if len(blocks) <= len(initialBlocks) {
		t.Fatalf("expected more blocks after cover page, got %d <= %d", len(blocks), len(initialBlocks))
	}

	// Verify XML
	xml := string(doc.parts[doc.documentIdx].data)
	if !strings.Contains(xml, "年度工作报告") {
		t.Fatal("XML missing cover title")
	}
	if !strings.Contains(xml, "2026 业务规划与回顾") {
		t.Fatal("XML missing cover subtitle")
	}
	if !strings.Contains(xml, "项目组") {
		t.Fatal("XML missing cover author")
	}

	// Verify Undo
	history, ok := any(doc).(document.WordHistory)
	if !ok {
		t.Fatal("doc does not implement WordHistory")
	}
	if !history.CanUndo() {
		t.Fatal("expected CanUndo after InsertCoverPage")
	}
	if _, err := history.Undo(); err != nil {
		t.Fatalf("undo failed: %v", err)
	}

	xmlAfterUndo := string(doc.parts[doc.documentIdx].data)
	if strings.Contains(xmlAfterUndo, "年度工作报告") {
		t.Fatal("expected cover title removed after undo")
	}

	// Verify Redo
	if _, err := history.Redo(); err != nil {
		t.Fatalf("redo failed: %v", err)
	}
	xmlAfterRedo := string(doc.parts[doc.documentIdx].data)
	if !strings.Contains(xmlAfterRedo, "年度工作报告") {
		t.Fatal("expected cover title restored after redo")
	}
}
