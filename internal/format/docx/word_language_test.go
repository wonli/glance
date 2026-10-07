package docx

import (
	"strings"
	"testing"

	"glance/internal/document"
)

func TestWordLanguage_SetAndUndo(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	editor, ok := any(doc).(document.WordLanguageEditor)
	if !ok {
		t.Fatal("doc does not implement WordLanguageEditor")
	}

	// 1. Set document language to en-US
	err = editor.SetLanguage("", "en-US")
	if err != nil {
		t.Fatalf("SetLanguage en-US failed: %v", err)
	}

	xml := string(doc.parts[doc.documentIdx].data)
	if !strings.Contains(xml, `w:val="en-US"`) {
		t.Fatalf("expected w:val=\"en-US\" in XML: %s", xml)
	}

	// 2. Undo
	history, ok := any(doc).(document.WordHistory)
	if !ok {
		t.Fatal("doc does not implement WordHistory")
	}
	if !history.CanUndo() {
		t.Fatal("expected CanUndo after SetLanguage")
	}
	if _, err := history.Undo(); err != nil {
		t.Fatalf("undo failed: %v", err)
	}

	xmlAfterUndo := string(doc.parts[doc.documentIdx].data)
	if strings.Contains(xmlAfterUndo, `w:val="en-US"`) {
		t.Fatalf("expected en-US reverted after undo: %s", xmlAfterUndo)
	}

	// 3. Redo
	if _, err := history.Redo(); err != nil {
		t.Fatalf("redo failed: %v", err)
	}
	xmlAfterRedo := string(doc.parts[doc.documentIdx].data)
	if !strings.Contains(xmlAfterRedo, `w:val="en-US"`) {
		t.Fatalf("expected en-US restored after redo: %s", xmlAfterRedo)
	}
}
