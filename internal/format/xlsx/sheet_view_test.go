package xlsx

import (
	"path/filepath"
	"testing"
)

func TestWorkbookSheetViewAndUndo(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]

	// 1. Initial view defaults
	view, err := wb.GetSheetView(sheet)
	if err != nil {
		t.Fatalf("GetSheetView failed: %v", err)
	}
	if view.ViewMode != "normal" {
		t.Fatalf("expected initial normal view, got %+v", view)
	}
	initialRuler := view.ShowRuler

	// 2. Set to pageBreakPreview
	if err := wb.SetSheetViewMode(sheet, "pageBreakPreview"); err != nil {
		t.Fatalf("SetSheetViewMode failed: %v", err)
	}
	view, err = wb.GetSheetView(sheet)
	if err != nil || view.ViewMode != "pageBreakPreview" {
		t.Fatalf("expected pageBreakPreview mode, got %+v (err=%v)", view, err)
	}

	// 3. Set ruler to opposite of initial
	if err := wb.SetSheetRuler(sheet, !initialRuler); err != nil {
		t.Fatalf("SetSheetRuler failed: %v", err)
	}
	view, err = wb.GetSheetView(sheet)
	if err != nil || view.ShowRuler != !initialRuler {
		t.Fatalf("expected ruler toggled, got %+v (err=%v)", view, err)
	}

	// 4. Test Undo ruler
	applied, err := wb.Undo()
	if err != nil || !applied {
		t.Fatalf("Undo failed: applied=%v, err=%v", applied, err)
	}
	view, err = wb.GetSheetView(sheet)
	if err != nil || view.ShowRuler != initialRuler {
		t.Fatalf("expected ruler restored to %v after undo, got %+v", initialRuler, view)
	}

	// 5. Test Undo view mode
	applied, err = wb.Undo()
	if err != nil || !applied {
		t.Fatalf("Undo failed: applied=%v, err=%v", applied, err)
	}
	view, err = wb.GetSheetView(sheet)
	if err != nil || view.ViewMode != "normal" {
		t.Fatalf("expected normal view mode after second undo, got %+v", view)
	}
}

func TestWorkbookSheetZoomLimitsSaveAndReopenPerWorksheet(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	firstSheet := wb.SheetNames()[0]
	secondSheet, err := wb.file.NewSheet("Second")
	if err != nil || secondSheet < 0 {
		t.Fatalf("create second worksheet: index=%d err=%v", secondSheet, err)
	}
	for sheet, percent := range map[string]int{firstSheet: 10, "Second": 400} {
		if err := wb.SetSheetZoom(sheet, percent); err != nil {
			t.Fatalf("SetSheetZoom(%q, %d): %v", sheet, percent, err)
		}
	}

	path := filepath.Join(t.TempDir(), "sheet-zoom-limits.xlsx")
	if err := wb.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen workbook: %v", err)
	}
	defer reopened.Close()
	for sheet, want := range map[string]int{firstSheet: 10, "Second": 400} {
		view, err := reopened.GetSheetView(sheet)
		if err != nil || view.ZoomPercent != want {
			t.Errorf("reopened %q zoom = %d, err=%v; want %d", sheet, view.ZoomPercent, err, want)
		}
	}
}
