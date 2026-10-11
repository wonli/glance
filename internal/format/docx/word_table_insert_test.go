package docx

import (
	"bytes"
	"strings"
	"testing"

	"glance/internal/document"
)

func TestWordInsertTableAndUndo(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	editor, ok := any(doc).(document.WordTableInsertEditor)
	if !ok {
		t.Fatal("doc does not implement WordTableInsertEditor")
	}

	initialBlocks := doc.WordBlocks()

	// Insert 3x4 table
	tableID, err := editor.InsertTable("", 3, 4)
	if err != nil {
		t.Fatalf("InsertTable failed: %v", err)
	}
	if tableID == "" {
		t.Fatal("expected non-empty tableID")
	}

	blocks := doc.WordBlocks()
	if len(blocks) <= len(initialBlocks) {
		t.Fatalf("expected more blocks after table insertion, got %d <= %d", len(blocks), len(initialBlocks))
	}

	foundTable := false
	for _, block := range blocks {
		if block.Kind == document.WordTableBlock && block.Table != nil {
			foundTable = true
			if len(block.Table.Rows) != 3 {
				t.Fatalf("expected 3 rows, got %d", len(block.Table.Rows))
			}
			for r, row := range block.Table.Rows {
				if len(row.Cells) != 4 {
					t.Fatalf("row %d: expected 4 cells, got %d", r, len(row.Cells))
				}
			}
		}
	}
	if !foundTable {
		t.Fatal("inserted table block not found in WordBlocks")
	}

	// Verify XML contains tbl
	xml := string(doc.parts[doc.documentIdx].data)
	if !strings.Contains(xml, "<w:tbl>") || !strings.Contains(xml, "</w:tbl>") {
		t.Fatal("XML missing <w:tbl> tags")
	}

	// Test Undo
	history, ok := any(doc).(document.WordHistory)
	if !ok {
		t.Fatal("doc does not implement WordHistory")
	}
	if !history.CanUndo() {
		t.Fatal("expected CanUndo after table insertion")
	}
	if _, err := history.Undo(); err != nil {
		t.Fatalf("undo failed: %v", err)
	}

	blocksAfterUndo := doc.WordBlocks()
	if len(blocksAfterUndo) != len(initialBlocks) {
		t.Fatalf("expected blocks count %d after undo, got %d", len(initialBlocks), len(blocksAfterUndo))
	}

	// Test Redo
	if !history.CanRedo() {
		t.Fatal("expected CanRedo after undo")
	}
	if _, err := history.Redo(); err != nil {
		t.Fatalf("redo failed: %v", err)
	}

	blocksAfterRedo := doc.WordBlocks()
	if len(blocksAfterRedo) <= len(initialBlocks) {
		t.Fatalf("expected table restored after redo, got %d blocks", len(blocksAfterRedo))
	}
}

func TestWordInsertTable_UnknownExplicitParagraphDoesNotFallBackOrMutate(t *testing.T) {
	doc, err := NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	editor, ok := any(doc).(document.WordTableInsertEditor)
	if !ok {
		t.Fatal("doc does not implement WordTableInsertEditor")
	}
	before := append([]byte(nil), doc.parts[doc.documentIdx].data...)
	dirtyBefore := doc.Dirty()
	if _, err := editor.InsertTable("missing-paragraph", 1, 1); err == nil {
		t.Fatal("InsertTable accepted an unknown explicit paragraph ID")
	}
	if after := doc.parts[doc.documentIdx].data; !bytes.Equal(after, before) {
		t.Fatalf("unknown table target changed document XML: before=%s after=%s", before, after)
	}
	history, ok := any(doc).(document.WordHistory)
	if !ok {
		t.Fatal("doc does not implement WordHistory")
	}
	if history.CanUndo() {
		t.Fatal("unknown table target created an undo record")
	}
	if dirtyAfter := doc.Dirty(); dirtyAfter != dirtyBefore {
		t.Fatalf("unknown table target changed dirty state from %v to %v", dirtyBefore, dirtyAfter)
	}
}
