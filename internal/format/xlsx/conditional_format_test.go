package xlsx

import (
	"testing"

	"glance/internal/document"
)

func TestConditionalFormat_SetAndUndo(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	cellRange := document.CellRange{StartColumn: 1, StartRow: 1, EndColumn: 2, EndRow: 10}

	// 1. Set Data Bar conditional formatting
	err := wb.SetConditionalFormat(document.SpreadsheetConditionalFormatSpec{
		Sheet: sheet,
		Range: cellRange,
		Type:  document.ConditionalFormatDataBar,
	})
	if err != nil {
		t.Fatalf("SetConditionalFormat data bar failed: %v", err)
	}

	cfMap, err := wb.file.GetConditionalFormats(sheet)
	if err != nil {
		t.Fatalf("GetConditionalFormats failed: %v", err)
	}
	if len(cfMap) == 0 {
		t.Fatal("expected non-empty conditional formats")
	}

	// 2. Test Undo
	if _, err := wb.Undo(); err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	cfMap, err = wb.file.GetConditionalFormats(sheet)
	if err != nil {
		t.Fatalf("GetConditionalFormats after undo failed: %v", err)
	}
	if len(cfMap) != 0 {
		t.Fatalf("expected 0 conditional formats after undo, got %d", len(cfMap))
	}

	// 3. Test Redo
	if _, err := wb.Redo(); err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	cfMap, err = wb.file.GetConditionalFormats(sheet)
	if err != nil {
		t.Fatalf("GetConditionalFormats after redo failed: %v", err)
	}
	if len(cfMap) == 0 {
		t.Fatal("expected conditional formats restored after redo")
	}

	// 4. Clear conditional format
	err = wb.ClearConditionalFormat(sheet, cellRange)
	if err != nil {
		t.Fatalf("ClearConditionalFormat failed: %v", err)
	}
	cfMap, err = wb.file.GetConditionalFormats(sheet)
	if err != nil {
		t.Fatalf("GetConditionalFormats after clear failed: %v", err)
	}
	if len(cfMap) != 0 {
		t.Fatalf("expected 0 conditional formats after clear, got %d", len(cfMap))
	}
}
