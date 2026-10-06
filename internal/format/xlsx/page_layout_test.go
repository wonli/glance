package xlsx

import (
	"math"
	"testing"

	"glance/internal/document"
)

func TestWorkbookPageLayoutOrientationAndUndo(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	sheets := wb.SheetNames()
	if len(sheets) == 0 {
		t.Fatal("expected at least one sheet")
	}
	sheet := sheets[0]

	editor, ok := any(wb).(document.SpreadsheetPageLayoutEditor)
	if !ok {
		t.Fatal("workbook does not implement SpreadsheetPageLayoutEditor")
	}

	layout, err := editor.GetSheetPageLayout(sheet)
	if err != nil {
		t.Fatalf("GetSheetPageLayout failed: %v", err)
	}
	if layout.Orientation != "portrait" {
		t.Fatalf("expected default portrait, got %q", layout.Orientation)
	}

	// Change to landscape
	if err := editor.SetSheetPageOrientation(sheet, "landscape"); err != nil {
		t.Fatalf("SetSheetPageOrientation failed: %v", err)
	}
	layout, err = editor.GetSheetPageLayout(sheet)
	if err != nil {
		t.Fatalf("GetSheetPageLayout failed: %v", err)
	}
	if layout.Orientation != "landscape" {
		t.Fatalf("expected landscape, got %q", layout.Orientation)
	}

	// Test Undo
	history := any(wb).(document.SpreadsheetHistory)
	undone, err := history.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo failed: %v", err)
	}
	layout, err = editor.GetSheetPageLayout(sheet)
	if err != nil {
		t.Fatalf("GetSheetPageLayout failed: %v", err)
	}
	if layout.Orientation != "portrait" {
		t.Fatalf("expected restored portrait after undo, got %q", layout.Orientation)
	}

	// Test Redo
	redone, err := history.Redo()
	if err != nil || !redone {
		t.Fatalf("Redo failed: %v", err)
	}
	layout, err = editor.GetSheetPageLayout(sheet)
	if err != nil {
		t.Fatalf("GetSheetPageLayout failed: %v", err)
	}
	if layout.Orientation != "landscape" {
		t.Fatalf("expected landscape after redo, got %q", layout.Orientation)
	}

	// Error case
	if err := editor.SetSheetPageOrientation(sheet, "slant"); err == nil {
		t.Fatal("expected error for invalid orientation")
	}
}

func TestWorkbookPageLayoutMarginsAndUndo(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	editor := any(wb).(document.SpreadsheetPageLayoutEditor)

	// Set wide margins (1.0 inch all around)
	if err := editor.SetSheetPageMargins(sheet, 1.0, 1.0, 1.0, 1.0); err != nil {
		t.Fatalf("SetSheetPageMargins failed: %v", err)
	}

	layout, err := editor.GetSheetPageLayout(sheet)
	if err != nil {
		t.Fatalf("GetSheetPageLayout failed: %v", err)
	}
	if math.Abs(layout.MarginTop-1.0) > 0.01 || math.Abs(layout.MarginLeft-1.0) > 0.01 {
		t.Fatalf("unexpected margins: top=%f left=%f", layout.MarginTop, layout.MarginLeft)
	}

	// Undo
	history := any(wb).(document.SpreadsheetHistory)
	undone, err := history.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo failed: %v", err)
	}
	layout, err = editor.GetSheetPageLayout(sheet)
	if err != nil {
		t.Fatalf("GetSheetPageLayout failed: %v", err)
	}
	if math.Abs(layout.MarginTop-0.75) > 0.01 {
		t.Fatalf("expected restored top margin ~0.75, got %f", layout.MarginTop)
	}

	// Error case: negative
	if err := editor.SetSheetPageMargins(sheet, -1.0, 1.0, 1.0, 1.0); err == nil {
		t.Fatal("expected error for negative margin")
	}
}

func TestWorkbookPageLayoutSizeAndUndo(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	editor := any(wb).(document.SpreadsheetPageLayoutEditor)

	// Set Letter paper size (1)
	if err := editor.SetSheetPageSize(sheet, 1); err != nil {
		t.Fatalf("SetSheetPageSize failed: %v", err)
	}

	layout, err := editor.GetSheetPageLayout(sheet)
	if err != nil {
		t.Fatalf("GetSheetPageLayout failed: %v", err)
	}
	if layout.PaperSize != 1 {
		t.Fatalf("expected paper size 1, got %d", layout.PaperSize)
	}

	// Undo
	history := any(wb).(document.SpreadsheetHistory)
	undone, err := history.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo failed: %v", err)
	}
	layout, err = editor.GetSheetPageLayout(sheet)
	if err != nil {
		t.Fatalf("GetSheetPageLayout failed: %v", err)
	}
	if layout.PaperSize != 9 { // Default A4
		t.Fatalf("expected restored paper size 9, got %d", layout.PaperSize)
	}

	// Error case: zero size
	if err := editor.SetSheetPageSize(sheet, 0); err == nil {
		t.Fatal("expected error for zero paper size")
	}
}

func TestWorkbookPageLayoutPrintAreaAndUndo(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	editor := any(wb).(document.SpreadsheetPageLayoutEditor)

	layout, err := editor.GetSheetPageLayout(sheet)
	if err != nil {
		t.Fatalf("GetSheetPageLayout failed: %v", err)
	}
	if layout.PrintArea != "" {
		t.Fatalf("expected empty print area initially, got %q", layout.PrintArea)
	}

	// Set print area
	if err := editor.SetSheetPrintArea(sheet, "$A$1:$D$10"); err != nil {
		t.Fatalf("SetSheetPrintArea failed: %v", err)
	}

	layout, err = editor.GetSheetPageLayout(sheet)
	if err != nil {
		t.Fatalf("GetSheetPageLayout failed: %v", err)
	}
	if layout.PrintArea != "$A$1:$D$10" {
		t.Fatalf("expected print area $A$1:$D$10, got %q", layout.PrintArea)
	}

	// Undo
	history := any(wb).(document.SpreadsheetHistory)
	undone, err := history.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo failed: %v", err)
	}
	layout, err = editor.GetSheetPageLayout(sheet)
	if err != nil {
		t.Fatalf("GetSheetPageLayout failed: %v", err)
	}
	if layout.PrintArea != "" {
		t.Fatalf("expected empty print area after undo, got %q", layout.PrintArea)
	}

	// Redo
	redone, err := history.Redo()
	if err != nil || !redone {
		t.Fatalf("Redo failed: %v", err)
	}
	layout, err = editor.GetSheetPageLayout(sheet)
	if err != nil {
		t.Fatalf("GetSheetPageLayout failed: %v", err)
	}
	if layout.PrintArea != "$A$1:$D$10" {
		t.Fatalf("expected print area after redo, got %q", layout.PrintArea)
	}

	// Clear print area
	if err := editor.SetSheetPrintArea(sheet, ""); err != nil {
		t.Fatalf("clear print area failed: %v", err)
	}
	layout, err = editor.GetSheetPageLayout(sheet)
	if err != nil {
		t.Fatalf("GetSheetPageLayout failed: %v", err)
	}
	if layout.PrintArea != "" {
		t.Fatalf("expected empty print area after clear, got %q", layout.PrintArea)
	}
}

func TestWorkbookGridlinesAndUndo(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	editor, ok := any(wb).(document.SpreadsheetGridlinesEditor)
	if !ok {
		t.Fatal("workbook does not implement SpreadsheetGridlinesEditor")
	}

	visible, err := editor.GetSheetGridlines(sheet)
	if err != nil {
		t.Fatalf("GetSheetGridlines failed: %v", err)
	}
	if !visible {
		t.Fatal("expected default gridlines to be visible")
	}

	// Turn off gridlines
	if err := editor.SetSheetGridlines(sheet, false); err != nil {
		t.Fatalf("SetSheetGridlines failed: %v", err)
	}
	visible, err = editor.GetSheetGridlines(sheet)
	if err != nil {
		t.Fatalf("GetSheetGridlines failed: %v", err)
	}
	if visible {
		t.Fatal("expected gridlines to be hidden")
	}

	// Undo
	history := any(wb).(document.SpreadsheetHistory)
	undone, err := history.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo failed: %v", err)
	}
	visible, err = editor.GetSheetGridlines(sheet)
	if err != nil {
		t.Fatalf("GetSheetGridlines failed: %v", err)
	}
	if !visible {
		t.Fatal("expected gridlines visible after undo")
	}
}

func TestWorkbookPageLayoutPrintTitlesAndUndo(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	editor, ok := any(wb).(document.SpreadsheetPageLayoutEditor)
	if !ok {
		t.Fatal("workbook does not implement SpreadsheetPageLayoutEditor")
	}

	layout, err := editor.GetSheetPageLayout(sheet)
	if err != nil {
		t.Fatalf("GetSheetPageLayout failed: %v", err)
	}
	if layout.PrintTitles != "" {
		t.Fatalf("expected empty initial print titles, got %q", layout.PrintTitles)
	}

	// Set print titles to repeated rows "$1:$1"
	if err := editor.SetSheetPrintTitles(sheet, "$1:$1"); err != nil {
		t.Fatalf("SetSheetPrintTitles failed: %v", err)
	}
	layout, err = editor.GetSheetPageLayout(sheet)
	if err != nil {
		t.Fatalf("GetSheetPageLayout failed: %v", err)
	}
	if layout.PrintTitles != "$1:$1" {
		t.Fatalf("expected print titles $1:$1, got %q", layout.PrintTitles)
	}

	// Undo
	history := any(wb).(document.SpreadsheetHistory)
	undone, err := history.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo failed: %v", err)
	}
	layout, err = editor.GetSheetPageLayout(sheet)
	if err != nil {
		t.Fatalf("GetSheetPageLayout failed: %v", err)
	}
	if layout.PrintTitles != "" {
		t.Fatalf("expected empty print titles after undo, got %q", layout.PrintTitles)
	}

	// Redo
	redone, err := history.Redo()
	if err != nil || !redone {
		t.Fatalf("Redo failed: %v", err)
	}
	layout, err = editor.GetSheetPageLayout(sheet)
	if err != nil {
		t.Fatalf("GetSheetPageLayout failed: %v", err)
	}
	if layout.PrintTitles != "$1:$1" {
		t.Fatalf("expected print titles after redo, got %q", layout.PrintTitles)
	}

	// Clear print titles
	if err := editor.SetSheetPrintTitles(sheet, ""); err != nil {
		t.Fatalf("clear print titles failed: %v", err)
	}
	layout, err = editor.GetSheetPageLayout(sheet)
	if err != nil {
		t.Fatalf("GetSheetPageLayout failed: %v", err)
	}
	if layout.PrintTitles != "" {
		t.Fatalf("expected empty print titles after clear, got %q", layout.PrintTitles)
	}
}


