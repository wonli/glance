package xlsx

import (
	"testing"
)

func TestWorkbookHyperlinkAndUndo(t *testing.T) {
	book := NewBlank()
	defer book.Close()

	sheet := "Sheet1"
	cell := "B2"

	// Initial state: no hyperlink
	link, ok, err := book.GetCellHyperlink(sheet, cell)
	if err != nil {
		t.Fatalf("GetCellHyperlink error: %v", err)
	}
	if ok || link.Target != "" {
		t.Fatalf("expected no initial hyperlink, got ok=%v target=%q", ok, link.Target)
	}

	// Set external hyperlink
	url := "https://example.com"
	if err := book.SetCellHyperlink(sheet, cell, url, "Example Site"); err != nil {
		t.Fatalf("SetCellHyperlink error: %v", err)
	}
	if !book.Dirty() {
		t.Fatal("expected book to be dirty after SetCellHyperlink")
	}

	link, ok, err = book.GetCellHyperlink(sheet, cell)
	if err != nil || !ok || link.Target != url {
		t.Fatalf("expected hyperlink %q, got ok=%v target=%q err=%v", url, ok, link.Target, err)
	}

	// Undo
	undone, err := book.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo failed: undone=%v err=%v", undone, err)
	}
	link, ok, err = book.GetCellHyperlink(sheet, cell)
	if err != nil {
		t.Fatalf("GetCellHyperlink after undo error: %v", err)
	}
	if ok && link.Target != "" {
		t.Fatalf("expected hyperlink gone after undo, got target=%q", link.Target)
	}

	// Redo
	redone, err := book.Redo()
	if err != nil || !redone {
		t.Fatalf("Redo failed: redone=%v err=%v", redone, err)
	}
	link, ok, err = book.GetCellHyperlink(sheet, cell)
	if err != nil || !ok || link.Target != url {
		t.Fatalf("expected hyperlink restored after redo, got ok=%v target=%q err=%v", ok, link.Target, err)
	}

	// Remove hyperlink
	if err := book.RemoveCellHyperlink(sheet, cell); err != nil {
		t.Fatalf("RemoveCellHyperlink error: %v", err)
	}
	link, ok, err = book.GetCellHyperlink(sheet, cell)
	if err != nil {
		t.Fatalf("GetCellHyperlink after remove error: %v", err)
	}
	if ok && link.Target != "" {
		t.Fatalf("expected hyperlink gone after remove, got target=%q", link.Target)
	}

	// Undo remove
	undone, err = book.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo remove failed: undone=%v err=%v", undone, err)
	}
	link, ok, err = book.GetCellHyperlink(sheet, cell)
	if err != nil || !ok || link.Target != url {
		t.Fatalf("expected hyperlink restored after undo remove, got ok=%v target=%q err=%v", ok, link.Target, err)
	}
}
