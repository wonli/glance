package docx

import (
	"strings"
	"testing"

	"glance/internal/document"
)

func TestWordWatermark_SetClearAndUndo(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	editor, ok := any(doc).(document.WordWatermarkEditor)
	if !ok {
		t.Fatal("doc does not implement WordWatermarkEditor")
	}

	// 1. Initial watermark should be empty
	wm, err := editor.GetWatermark()
	if err != nil {
		t.Fatalf("GetWatermark failed: %v", err)
	}
	if wm != "" {
		t.Fatalf("expected empty watermark, got %q", wm)
	}

	// 2. Set watermark to "机密"
	if err := editor.SetWatermark("机密"); err != nil {
		t.Fatalf("SetWatermark failed: %v", err)
	}
	wm, err = editor.GetWatermark()
	if err != nil || wm != "机密" {
		t.Fatalf("expected watermark '机密', got %q (err: %v)", wm, err)
	}

	headerPart, found := findPackagePart(doc.parts, wordHeaderPath)
	if !found {
		t.Fatal("word/header1.xml not found")
	}
	if !strings.Contains(string(headerPart.data), "PowerPlusWaterMarkObject") {
		t.Fatal("header does not contain PowerPlusWaterMarkObject")
	}

	// 3. Undo should revert watermark
	history := any(doc).(document.WordHistory)
	if !history.CanUndo() {
		t.Fatal("expected CanUndo after SetWatermark")
	}
	if _, err := history.Undo(); err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	wm, err = editor.GetWatermark()
	if err != nil || wm != "" {
		t.Fatalf("expected empty watermark after undo, got %q", wm)
	}

	// 4. Redo should restore watermark
	if _, err := history.Redo(); err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	wm, err = editor.GetWatermark()
	if err != nil || wm != "机密" {
		t.Fatalf("expected watermark '机密' after redo, got %q", wm)
	}

	// 5. Clear watermark
	if err := editor.ClearWatermark(); err != nil {
		t.Fatalf("ClearWatermark failed: %v", err)
	}
	wm, err = editor.GetWatermark()
	if err != nil || wm != "" {
		t.Fatalf("expected empty watermark after ClearWatermark, got %q", wm)
	}
}
