package docx

import (
	"strings"
	"testing"

	"glance/internal/document"
)

func TestWordPageBorders_SetClearAndUndo(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	editor, ok := any(doc).(document.WordPageSetupEditor)
	if !ok {
		t.Fatal("doc does not implement WordPageSetupEditor")
	}

	// 1. Set box border
	if err := editor.SetPageBorders("box"); err != nil {
		t.Fatalf("SetPageBorders box failed: %v", err)
	}

	xml := string(doc.parts[doc.documentIdx].data)
	if !strings.Contains(xml, "w:pgBorders") {
		t.Fatal("expected w:pgBorders in XML")
	}
	if !strings.Contains(xml, `w:val="single"`) {
		t.Fatalf("expected w:val=\"single\" in pgBorders: %s", xml)
	}

	// 2. Set double border
	if err := editor.SetPageBorders("double"); err != nil {
		t.Fatalf("SetPageBorders double failed: %v", err)
	}
	xml = string(doc.parts[doc.documentIdx].data)
	if !strings.Contains(xml, `w:val="double"`) {
		t.Fatalf("expected w:val=\"double\" in pgBorders: %s", xml)
	}

	// 3. Undo
	history, ok := any(doc).(document.WordHistory)
	if !ok {
		t.Fatal("doc does not implement WordHistory")
	}
	if !history.CanUndo() {
		t.Fatal("expected CanUndo after SetPageBorders")
	}
	if _, err := history.Undo(); err != nil {
		t.Fatalf("undo failed: %v", err)
	}
	xml = string(doc.parts[doc.documentIdx].data)
	if !strings.Contains(xml, `w:val="single"`) {
		t.Fatalf("expected reverted to single after undo: %s", xml)
	}

	// 4. Redo
	if _, err := history.Redo(); err != nil {
		t.Fatalf("redo failed: %v", err)
	}
	xml = string(doc.parts[doc.documentIdx].data)
	if !strings.Contains(xml, `w:val="double"`) {
		t.Fatalf("expected double after redo: %s", xml)
	}

	// 5. Clear border
	if err := editor.SetPageBorders("none"); err != nil {
		t.Fatalf("SetPageBorders none failed: %v", err)
	}
	xml = string(doc.parts[doc.documentIdx].data)
	if strings.Contains(xml, "w:pgBorders") {
		t.Fatalf("expected w:pgBorders removed after none: %s", xml)
	}
}
