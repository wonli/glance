package docx

import (
	"testing"

	"glance/internal/document"
)

func TestWordPageBackgroundAndUndo(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatalf("NewBlank failed: %v", err)
	}
	defer doc.Close()

	editor, ok := any(doc).(document.WordPageBackgroundEditor)
	if !ok {
		t.Fatal("doc does not implement WordPageBackgroundEditor")
	}

	// Initially no background
	bg, err := editor.GetPageBackgroundColor()
	if err != nil {
		t.Fatalf("GetPageBackgroundColor failed: %v", err)
	}
	if bg != "" {
		t.Fatalf("expected empty background color initially, got %q", bg)
	}

	// Set background to #FAF8F5 (parchment)
	if err := editor.SetPageBackgroundColor("FAF8F5"); err != nil {
		t.Fatalf("SetPageBackgroundColor failed: %v", err)
	}
	bg, err = editor.GetPageBackgroundColor()
	if err != nil {
		t.Fatalf("GetPageBackgroundColor failed: %v", err)
	}
	if bg != "FAF8F5" {
		t.Fatalf("expected background color FAF8F5, got %q", bg)
	}

	// Update to #E8F5E9 (green)
	if err := editor.SetPageBackgroundColor("#E8F5E9"); err != nil {
		t.Fatalf("SetPageBackgroundColor(#E8F5E9) failed: %v", err)
	}
	bg, _ = editor.GetPageBackgroundColor()
	if bg != "E8F5E9" {
		t.Fatalf("expected background color E8F5E9, got %q", bg)
	}

	// Undo
	undone, err := doc.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo failed: undone=%v err=%v", undone, err)
	}
	bgAfterUndo, _ := editor.GetPageBackgroundColor()
	if bgAfterUndo != "FAF8F5" {
		t.Fatalf("expected background color FAF8F5 after undo, got %q", bgAfterUndo)
	}

	// Undo again -> empty
	undone2, err := doc.Undo()
	if err != nil || !undone2 {
		t.Fatalf("Undo 2 failed: undone=%v err=%v", undone2, err)
	}
	bgInitial, _ := editor.GetPageBackgroundColor()
	if bgInitial != "" {
		t.Fatalf("expected empty background after second undo, got %q", bgInitial)
	}

	// Redo
	redone, err := doc.Redo()
	if err != nil || !redone {
		t.Fatalf("Redo failed: redone=%v err=%v", redone, err)
	}
	bgRedo, _ := editor.GetPageBackgroundColor()
	if bgRedo != "FAF8F5" {
		t.Fatalf("expected restored background color FAF8F5 after redo, got %q", bgRedo)
	}

	// Clear background
	if err := editor.SetPageBackgroundColor(""); err != nil {
		t.Fatalf("SetPageBackgroundColor('') failed: %v", err)
	}
	bgCleared, _ := editor.GetPageBackgroundColor()
	if bgCleared != "" {
		t.Fatalf("expected empty background after clear, got %q", bgCleared)
	}
}
