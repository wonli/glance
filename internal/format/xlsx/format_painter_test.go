package xlsx

import (
	"testing"

	"github.com/xuri/excelize/v2"
	"glance/internal/document"
)

func TestPaintCellFormatCopiesCompleteSourceStyleAndUndoRedo(t *testing.T) {
	book := NewBlank()
	defer book.Close()
	sheet := book.SheetNames()[0]
	for cell, value := range map[string]string{"A1": "source", "B1": "target one", "B2": "target two"} {
		if err := book.file.SetCellValue(sheet, cell, value); err != nil {
			t.Fatalf("SetCellValue(%s): %v", cell, err)
		}
	}
	sourceStyle, err := book.file.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "Aptos", Size: 12, Italic: true, Color: "FF345678"},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border: []excelize.Border{
			{Type: "left", Color: "FF102030", Style: 2},
			{Type: "top", Color: "FF405060", Style: 3},
		},
		NumFmt: 4,
	})
	if err != nil {
		t.Fatalf("NewStyle(source): %v", err)
	}
	targetStyle, err := book.file.NewStyle(&excelize.Style{
		Font:   &excelize.Font{Family: "Arial", Size: 9, Bold: true, Color: "FFFF0000"},
		Fill:   excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"FFFFFF00"}},
		Border: []excelize.Border{{Type: "bottom", Color: "FF000000", Style: 1}},
	})
	if err != nil {
		t.Fatalf("NewStyle(target): %v", err)
	}
	if err := book.file.SetCellStyle(sheet, "A1", "A1", sourceStyle); err != nil {
		t.Fatalf("SetCellStyle(A1): %v", err)
	}
	if err := book.file.SetCellStyle(sheet, "B1", "B2", targetStyle); err != nil {
		t.Fatalf("SetCellStyle(B1:B2): %v", err)
	}
	book.RebaseHistory()

	painter, ok := any(book).(interface {
		PaintCellFormat(string, string, document.CellRange) (bool, error)
	})
	if !ok {
		t.Fatal("XLSX workbook does not expose full-style painting")
	}
	bounds := document.CellRange{StartRow: 1, StartColumn: 2, EndRow: 2, EndColumn: 2}
	changed, err := painter.PaintCellFormat(sheet, "A1", bounds)
	if err != nil || !changed {
		t.Fatalf("PaintCellFormat changed=%v error=%v; want one style change", changed, err)
	}
	assertSpreadsheetCellStyleID(t, book, sheet, "B1", sourceStyle)
	assertSpreadsheetCellStyleID(t, book, sheet, "B2", sourceStyle)
	for cell, want := range map[string]string{"A1": "source", "B1": "target one", "B2": "target two"} {
		got, err := book.file.GetCellValue(sheet, cell)
		if err != nil || got != want {
			t.Fatalf("cell %s value=%q error=%v; want unchanged %q", cell, got, err, want)
		}
	}
	if !book.Dirty() {
		t.Fatal("style paint should dirty the workbook")
	}
	if changed, err := painter.PaintCellFormat(sheet, "A1", bounds); err != nil || changed {
		t.Fatalf("painting the same style again changed=%v error=%v; want a no-op", changed, err)
	}

	if changed, err := book.Undo(); err != nil || !changed {
		t.Fatalf("Undo changed=%v error=%v; want one undoable style paint", changed, err)
	}
	assertSpreadsheetCellStyleID(t, book, sheet, "B1", targetStyle)
	assertSpreadsheetCellStyleID(t, book, sheet, "B2", targetStyle)
	if changed, err := book.Undo(); err != nil || changed {
		t.Fatalf("second Undo changed=%v error=%v; style paint must be one history step", changed, err)
	}
	if changed, err := book.Redo(); err != nil || !changed {
		t.Fatalf("Redo changed=%v error=%v; want the style paint restored", changed, err)
	}
	assertSpreadsheetCellStyleID(t, book, sheet, "B1", sourceStyle)
	assertSpreadsheetCellStyleID(t, book, sheet, "B2", sourceStyle)

	snapshot, err := book.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	reopened, err := OpenSnapshot(snapshot)
	if err != nil {
		t.Fatalf("OpenSnapshot: %v", err)
	}
	defer reopened.Close()
	assertSpreadsheetCellStyleID(t, reopened, sheet, "B1", sourceStyle)
	assertSpreadsheetCellStyleID(t, reopened, sheet, "B2", sourceStyle)
}

func assertSpreadsheetCellStyleID(t *testing.T, book *Workbook, sheet, cell string, want int) {
	t.Helper()
	got, err := book.file.GetCellStyle(sheet, cell)
	if err != nil || got != want {
		t.Fatalf("cell %s style ID=%d error=%v; want %d", cell, got, err, want)
	}
}
