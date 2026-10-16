package xlsx

import (
	"path/filepath"
	"testing"

	"glance/internal/document"
)

func TestWorkbookPanesAndUndo(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()

	sheet := wb.SheetNames()[0]
	editor, ok := any(wb).(document.SpreadsheetPanesEditor)
	if !ok {
		t.Fatal("workbook does not implement SpreadsheetPanesEditor")
	}

	// Initial: unfrozen
	panes, err := editor.GetSheetPanes(sheet)
	if err != nil {
		t.Fatalf("GetSheetPanes failed: %v", err)
	}
	if panes.Frozen {
		t.Fatal("expected initially unfrozen panes")
	}

	// 1. Freeze top row (YSplit=1, XSplit=0)
	err = editor.SetSheetPanes(sheet, document.SpreadsheetPaneFreeze{
		Frozen: true,
		XSplit: 0,
		YSplit: 1,
	})
	if err != nil {
		t.Fatalf("SetSheetPanes freeze top row failed: %v", err)
	}
	panes, err = editor.GetSheetPanes(sheet)
	if err != nil || !panes.Frozen || panes.YSplit != 1 || panes.XSplit != 0 {
		t.Fatalf("expected top row frozen, got %+v (err=%v)", panes, err)
	}

	// Undo
	history := any(wb).(document.SpreadsheetHistory)
	undone, err := history.Undo()
	if err != nil || !undone {
		t.Fatalf("Undo failed: undone=%v err=%v", undone, err)
	}
	panes, err = editor.GetSheetPanes(sheet)
	if err != nil || panes.Frozen {
		t.Fatalf("expected unfrozen panes after undo, got %+v", panes)
	}

	// Redo
	redone, err := history.Redo()
	if err != nil || !redone {
		t.Fatalf("Redo failed: redone=%v err=%v", redone, err)
	}
	panes, err = editor.GetSheetPanes(sheet)
	if err != nil || !panes.Frozen || panes.YSplit != 1 {
		t.Fatalf("expected top row frozen after redo, got %+v", panes)
	}

	// 2. Freeze first column (XSplit=1, YSplit=0)
	err = editor.SetSheetPanes(sheet, document.SpreadsheetPaneFreeze{
		Frozen: true,
		XSplit: 1,
		YSplit: 0,
	})
	if err != nil {
		t.Fatalf("SetSheetPanes freeze first column failed: %v", err)
	}
	panes, err = editor.GetSheetPanes(sheet)
	if err != nil || !panes.Frozen || panes.XSplit != 1 || panes.YSplit != 0 {
		t.Fatalf("expected first column frozen, got %+v (err=%v)", panes, err)
	}

	// 3. Unfreeze
	err = editor.SetSheetPanes(sheet, document.SpreadsheetPaneFreeze{Frozen: false})
	if err != nil {
		t.Fatalf("SetSheetPanes unfreeze failed: %v", err)
	}
	panes, err = editor.GetSheetPanes(sheet)
	if err != nil || panes.Frozen {
		t.Fatalf("expected unfrozen panes, got %+v", panes)
	}
}

func TestWorkbookPanesSaveReopenAndWindowMetadata(t *testing.T) {
	wb := NewBlank()
	defer wb.Close()
	sheet := wb.SheetNames()[0]
	want := document.SpreadsheetPaneFreeze{Frozen: true, XSplit: 2, YSplit: 3}
	if err := wb.SetSheetPanes(sheet, want); err != nil {
		t.Fatalf("SetSheetPanes failed: %v", err)
	}

	path := filepath.Join(t.TempDir(), "frozen.xlsx")
	if err := wb.SaveAs(path); err != nil {
		t.Fatalf("SaveAs failed: %v", err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen workbook: %v", err)
	}
	defer reopened.Close()

	got, err := reopened.GetSheetPanes(sheet)
	if err != nil || got != want {
		t.Fatalf("reopened pane metadata = %+v, %v; want %+v", got, err, want)
	}
	window, err := reopened.Window(sheet, 1, 1, 1, 1)
	if err != nil {
		t.Fatalf("read reopened worksheet window: %v", err)
	}
	if window.Panes != want {
		t.Fatalf("reopened worksheet window pane metadata = %+v; want %+v", window.Panes, want)
	}
}
