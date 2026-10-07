package xlsx

import (
	"path/filepath"
	"testing"

	"glance/internal/document"
)

func TestWorkbookAddShapeAndUndo(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]

	// Add a rectangle shape
	err := wb.AddShape(document.SpreadsheetShapeSpec{
		Sheet:  sheet,
		Cell:   "C3",
		Type:   "roundRect",
		Width:  200,
		Height: 100,
		Text:   "测试形状",
	})
	if err != nil {
		t.Fatalf("AddShape failed: %v", err)
	}
	if !wb.Dirty() {
		t.Fatalf("expected workbook to be dirty after AddShape")
	}

	// Undo
	undone, err := wb.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo failed: %v", err)
	}

	// Redo
	redone, err := wb.Redo()
	if err != nil || !redone {
		t.Fatalf("Redo failed: %v", err)
	}

	// Save and verify reopen
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "shape_test.xlsx")
	if err := wb.SaveAs(path); err != nil {
		t.Fatalf("SaveAs failed: %v", err)
	}

	wb2, err := Open(path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer wb2.Close()
}
