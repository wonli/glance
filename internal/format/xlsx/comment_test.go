package xlsx

import (
	"testing"

	"glance/internal/document"
)

func TestComments_AddGetDelete(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]

	// 1. Add comment to A1
	err := wb.AddComment(sheet, document.SpreadsheetComment{
		Author: "Alice",
		Cell:   "A1",
		Text:   "Review required",
	})
	if err != nil {
		t.Fatalf("AddComment failed: %v", err)
	}

	// 2. Add comment to B2
	err = wb.AddComment(sheet, document.SpreadsheetComment{
		Author: "Bob",
		Cell:   "B2",
		Text:   "Approved",
	})
	if err != nil {
		t.Fatalf("AddComment 2 failed: %v", err)
	}

	comments, err := wb.GetComments(sheet)
	if err != nil {
		t.Fatalf("GetComments failed: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(comments))
	}

	// Check content
	foundA1 := false
	for _, c := range comments {
		if c.Cell == "A1" {
			foundA1 = true
			if c.Text != "Review required" {
				t.Errorf("expected 'Review required', got %q", c.Text)
			}
			if c.Author != "Alice" {
				t.Errorf("expected author 'Alice', got %q", c.Author)
			}
		}
	}
	if !foundA1 {
		t.Errorf("comment on A1 not found")
	}

	// 3. Test Undo
	if _, err := wb.Undo(); err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	comments, err = wb.GetComments(sheet)
	if err != nil {
		t.Fatalf("GetComments after undo failed: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment after undo, got %d", len(comments))
	}

	// 4. Test Redo
	if _, err := wb.Redo(); err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	comments, err = wb.GetComments(sheet)
	if err != nil {
		t.Fatalf("GetComments after redo failed: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments after redo, got %d", len(comments))
	}

	// 5. Delete comment on A1
	err = wb.DeleteComment(sheet, "A1")
	if err != nil {
		t.Fatalf("DeleteComment failed: %v", err)
	}
	comments, err = wb.GetComments(sheet)
	if err != nil {
		t.Fatalf("GetComments after delete failed: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment after delete, got %d", len(comments))
	}
	if comments[0].Cell != "B2" {
		t.Errorf("expected remaining comment on B2, got %s", comments[0].Cell)
	}
}
