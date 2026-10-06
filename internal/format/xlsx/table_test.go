package xlsx

import (
	"testing"

	"glance/internal/document"
)

func TestWorkbookAddTableAndUndo(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	_ = wb.SetCellText(sheet, "A1", "Name")
	_ = wb.SetCellText(sheet, "B1", "Score")
	_ = wb.SetCellText(sheet, "A2", "Alice")
	_ = wb.SetCellText(sheet, "B2", "100")
	_ = wb.SetCellText(sheet, "A3", "Bob")
	_ = wb.SetCellText(sheet, "B3", "90")

	editor, ok := any(wb).(document.SpreadsheetTableEditor)
	if !ok {
		t.Fatal("workbook does not implement SpreadsheetTableEditor")
	}

	spec := document.SpreadsheetTableSpec{
		Sheet: sheet,
		Range: document.CellRange{
			StartRow:    1,
			StartColumn: 1,
			EndRow:      3,
			EndColumn:   2,
		},
		Name:      "TestTable",
		StyleName: "TableStyleMedium2",
	}

	if err := editor.AddTable(spec); err != nil {
		t.Fatalf("AddTable failed: %v", err)
	}

	tables, err := wb.file.GetTables(sheet)
	if err != nil {
		t.Fatalf("GetTables failed: %v", err)
	}
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if tables[0].Name != "TestTable" {
		t.Fatalf("expected table name TestTable, got %s", tables[0].Name)
	}

	// Undo
	undone, err := wb.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo failed: undone=%v err=%v", undone, err)
	}
	tables, err = wb.file.GetTables(sheet)
	if err != nil {
		t.Fatalf("GetTables after undo failed: %v", err)
	}
	if len(tables) != 0 {
		t.Fatalf("expected 0 tables after undo, got %d", len(tables))
	}

	// Redo
	redone, err := wb.Redo()
	if err != nil || !redone {
		t.Fatalf("Redo failed: redone=%v err=%v", redone, err)
	}
	tables, err = wb.file.GetTables(sheet)
	if err != nil {
		t.Fatalf("GetTables after redo failed: %v", err)
	}
	if len(tables) != 1 {
		t.Fatalf("expected 1 table after redo, got %d", len(tables))
	}
}

func TestWorkbookAddTableAutoHeaders(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	// Row 1 empty, Row 2 has data
	_ = wb.SetCellText(sheet, "A2", "Val1")
	_ = wb.SetCellText(sheet, "B2", "Val2")

	editor, ok := any(wb).(document.SpreadsheetTableEditor)
	if !ok {
		t.Fatal("workbook does not implement SpreadsheetTableEditor")
	}

	spec := document.SpreadsheetTableSpec{
		Sheet: sheet,
		Range: document.CellRange{
			StartRow:    1,
			StartColumn: 1,
			EndRow:      2,
			EndColumn:   2,
		},
	}

	if err := editor.AddTable(spec); err != nil {
		t.Fatalf("AddTable failed: %v", err)
	}

	vA1, _ := wb.Value(sheet, "A1")
	vB1, _ := wb.Value(sheet, "B1")
	if vA1 != "列1" || vB1 != "列2" {
		t.Fatalf("expected auto-filled headers '列1', '列2', got %q, %q", vA1, vB1)
	}
}

func TestWorkbookAddTableInvalidBounds(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	editor := any(wb).(document.SpreadsheetTableEditor)

	// Single row table
	err := editor.AddTable(document.SpreadsheetTableSpec{
		Sheet: sheet,
		Range: document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 2},
	})
	if err == nil {
		t.Fatal("expected error for single row table")
	}
}
