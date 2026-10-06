package docx

import (
	"testing"

	"glance/internal/document"
)

func TestWordDocumentProtectionAndUndo(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatalf("NewBlank failed: %v", err)
	}
	defer doc.Close()

	editor, ok := any(doc).(document.WordDocumentProtectionEditor)
	if !ok {
		t.Fatal("doc does not implement WordDocumentProtectionEditor")
	}

	// Initially unprotected
	prot, err := editor.GetWordDocumentProtection()
	if err != nil {
		t.Fatalf("GetWordDocumentProtection failed: %v", err)
	}
	if prot {
		t.Fatal("expected document to be initially unprotected")
	}

	// Enable protection
	if err := editor.SetWordDocumentProtection(true); err != nil {
		t.Fatalf("SetWordDocumentProtection(true) failed: %v", err)
	}
	prot, err = editor.GetWordDocumentProtection()
	if err != nil {
		t.Fatalf("GetWordDocumentProtection failed: %v", err)
	}
	if !prot {
		t.Fatal("expected document to be protected")
	}

	// Undo protection
	undone, err := doc.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo failed: undone=%v err=%v", undone, err)
	}
	prot, err = editor.GetWordDocumentProtection()
	if err != nil {
		t.Fatalf("GetWordDocumentProtection after Undo failed: %v", err)
	}
	if prot {
		t.Fatal("expected document to be unprotected after Undo")
	}

	// Redo protection
	redone, err := doc.Redo()
	if err != nil || !redone {
		t.Fatalf("Redo failed: redone=%v err=%v", redone, err)
	}
	prot, err = editor.GetWordDocumentProtection()
	if err != nil {
		t.Fatalf("GetWordDocumentProtection after Redo failed: %v", err)
	}
	if !prot {
		t.Fatal("expected document to be protected after Redo")
	}

	// Disable protection
	if err := editor.SetWordDocumentProtection(false); err != nil {
		t.Fatalf("SetWordDocumentProtection(false) failed: %v", err)
	}
	prot, err = editor.GetWordDocumentProtection()
	if err != nil {
		t.Fatalf("GetWordDocumentProtection failed: %v", err)
	}
	if prot {
		t.Fatal("expected document to be unprotected after disable")
	}

	// Undo disable
	undone, err = doc.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo failed: undone=%v err=%v", undone, err)
	}
	prot, err = editor.GetWordDocumentProtection()
	if err != nil {
		t.Fatalf("GetWordDocumentProtection failed: %v", err)
	}
	if !prot {
		t.Fatal("expected document to be protected after Undo disable")
	}
}
