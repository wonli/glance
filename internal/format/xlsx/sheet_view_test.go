package xlsx

import (
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
