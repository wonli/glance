package desktopui

import (
	"image"
	"os"
	"testing"

	"glance/internal/document"
	"glance/internal/format/xlsx"
)

func TestGridFrozenPaneGeometryStaysFixedDuringScroll(t *testing.T) {
	v := newGridViewport()
	v.setPanes(document.SpreadsheetPaneFreeze{Frozen: true, XSplit: 2, YSplit: 3})
	v.X, v.Y = 128, 60

	const rowHeader, columnHeader = 42, 32
	frozenCell := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}
	bodyCell := document.CellRange{StartRow: 7, StartColumn: 5, EndRow: 7, EndColumn: 5}
	wantFrozenRect := image.Rect(42, 32, 106, 52)
	wantBodyRect := image.Rect(170, 92, 234, 112)
	if got := v.screenCellRect(frozenCell, true, true, rowHeader, columnHeader); got != wantFrozenRect {
		t.Fatalf("frozen A1 screen rect = %v; want %v", got, wantFrozenRect)
	}
	if got := v.screenCellRect(bodyCell, false, false, rowHeader, columnHeader); got != wantBodyRect {
		t.Fatalf("scrolled E7 screen rect = %v; want %v", got, wantBodyRect)
	}
	if row, column, ok := v.cellPoint(image.Pt(74, 42), rowHeader, columnHeader); !ok || row != 1 || column != 1 {
		t.Fatalf("frozen A1 hit = (%d,%d), %v; want (1,1), true", row, column, ok)
	}
	if row, column, ok := v.cellPoint(image.Pt(202, 102), rowHeader, columnHeader); !ok || row != 7 || column != 5 {
		t.Fatalf("body E7 hit after scroll = (%d,%d), %v; want (7,5), true", row, column, ok)
	}
	wantInitialRanges := gridPaneRanges{
		Body:   document.CellRange{StartRow: 5, EndRow: 15, StartColumn: 3, EndColumn: 11},
		Top:    document.CellRange{StartRow: 1, EndRow: 3, StartColumn: 3, EndColumn: 11},
		Left:   document.CellRange{StartRow: 5, EndRow: 15, StartColumn: 1, EndColumn: 2},
		Corner: document.CellRange{StartRow: 1, EndRow: 3, StartColumn: 1, EndColumn: 2},
	}
	if got := v.visiblePaneRanges(400, 200); got != wantInitialRanges {
		t.Fatalf("visible pane windows at initial scroll = %+v; want %+v", got, wantInitialRanges)
	}

	// Advancing the body scroll moves the next body cell into the same screen
	// position while the frozen corner remains anchored to the viewport.
	v.X += 64
	v.Y += 20
	if got := v.screenCellRect(frozenCell, true, true, rowHeader, columnHeader); got != wantFrozenRect {
		t.Fatalf("frozen A1 moved after body scroll: got %v; want %v", got, wantFrozenRect)
	}
	if got := v.screenCellRect(document.CellRange{StartRow: 8, StartColumn: 6, EndRow: 8, EndColumn: 6}, false, false, rowHeader, columnHeader); got != wantBodyRect {
		t.Fatalf("body F8 did not follow scroll into E7's prior position: got %v; want %v", got, wantBodyRect)
	}
	if row, column, ok := v.cellPoint(image.Pt(202, 102), rowHeader, columnHeader); !ok || row != 8 || column != 6 {
		t.Fatalf("body F8 hit after additional scroll = (%d,%d), %v; want (8,6), true", row, column, ok)
	}

	wantScrolledRanges := gridPaneRanges{
		Body:   document.CellRange{StartRow: 6, EndRow: 16, StartColumn: 4, EndColumn: 12},
		Top:    document.CellRange{StartRow: 1, EndRow: 3, StartColumn: 4, EndColumn: 12},
		Left:   document.CellRange{StartRow: 6, EndRow: 16, StartColumn: 1, EndColumn: 2},
		Corner: document.CellRange{StartRow: 1, EndRow: 3, StartColumn: 1, EndColumn: 2},
	}
	if got := v.visiblePaneRanges(400, 200); got != wantScrolledRanges {
		t.Fatalf("visible pane windows after scroll = %+v; want %+v", got, wantScrolledRanges)
	}
}

func TestGridUnfreezePanesPreservesZeroScrollAndHitTesting(t *testing.T) {
	v := newGridViewport()
	v.setPanes(document.SpreadsheetPaneFreeze{Frozen: true, YSplit: 3})
	const rowHeader, columnHeader = 42, 32

	// Initially unscrolled.
	if v.X != 0 || v.Y != 0 {
		t.Fatalf("expected initial scroll (0,0), got (%v,%v)", v.X, v.Y)
	}

	// Unfreeze panes while unscrolled: origin must remain at (0,0).
	v.setPanes(document.SpreadsheetPaneFreeze{Frozen: false})
	if v.X != 0 || v.Y != 0 {
		t.Fatalf("unfreezing while unscrolled should preserve (0,0), got (%v,%v)", v.X, v.Y)
	}

	// Verify hit testing on row 1 and row 4.
	cell1 := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}
	rect1 := v.screenCellRect(cell1, false, false, rowHeader, columnHeader)
	center1 := image.Pt((rect1.Min.X+rect1.Max.X)/2, (rect1.Min.Y+rect1.Max.Y)/2)
	row, col, ok := v.cellPoint(center1, rowHeader, columnHeader)
	if !ok || row != 1 || col != 1 {
		t.Fatalf("unfrozen A1 hit = (%d,%d), %v; want (1,1), true", row, col, ok)
	}
	if headerRow := v.headerRowAt(center1.Y - columnHeader); headerRow != 0 {
		t.Fatalf("headerRowAt(%d) = %d; want 0", center1.Y-columnHeader, headerRow)
	}

	cell4 := document.CellRange{StartRow: 4, StartColumn: 1, EndRow: 4, EndColumn: 1}
	rect4 := v.screenCellRect(cell4, false, false, rowHeader, columnHeader)
	center4 := image.Pt((rect4.Min.X+rect4.Max.X)/2, (rect4.Min.Y+rect4.Max.Y)/2)
	row, col, ok = v.cellPoint(center4, rowHeader, columnHeader)
	if !ok || row != 4 || col != 1 {
		t.Fatalf("unfrozen A4 hit = (%d,%d), %v; want (4,1), true", row, col, ok)
	}
	if headerRow := v.headerRowAt(center4.Y - columnHeader); headerRow != 3 {
		t.Fatalf("headerRowAt(%d) = %d; want 3", center4.Y-columnHeader, headerRow)
	}
}

func TestGridUnfrozenScrollHitTesting(t *testing.T) {
	v := newGridViewport()
	v.setPanes(document.SpreadsheetPaneFreeze{Frozen: false})
	v.X, v.Y = 128, 60
	const rowHeader, columnHeader = 42, 32

	// Hit test a scrolled body cell at row 7, column 5 (E7).
	cell := document.CellRange{StartRow: 7, StartColumn: 5, EndRow: 7, EndColumn: 5}
	rect := v.screenCellRect(cell, false, false, rowHeader, columnHeader)
	center := image.Pt((rect.Min.X+rect.Max.X)/2, (rect.Min.Y+rect.Max.Y)/2)

	row, col, ok := v.cellPoint(center, rowHeader, columnHeader)
	if !ok || row != 7 || col != 5 {
		t.Fatalf("scrolled unfrozen E7 hit = (%d,%d), %v; want (7,5), true", row, col, ok)
	}
	if headerRow := v.headerRowAt(center.Y - columnHeader); headerRow != 6 {
		t.Fatalf("scrolled unfrozen headerRowAt = %d; want 6", headerRow)
	}
	if headerCol := v.headerColumnAt(center.X - rowHeader); headerCol != 4 {
		t.Fatalf("scrolled unfrozen headerColumnAt = %d; want 4", headerCol)
	}
}

func TestDujiangyanWorkbookUnfreezePanesHitTesting(t *testing.T) {
	const path = "/Users/mima1234/Desktop/都江堰企业产业链分类与商机表.xlsx"
	if _, err := os.Stat(path); err != nil {
		t.Skip("sample workbook not available")
	}
	book, err := xlsx.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()

	names := book.SheetNames()
	if len(names) < 2 {
		t.Fatalf("expected at least 2 sheets, got %d", len(names))
	}
	sheet := names[1] // sheet 2 has YSplit: 3
	panes, err := book.GetSheetPanes(sheet)
	if err != nil {
		t.Fatal(err)
	}
	if !panes.Frozen || panes.YSplit != 3 {
		t.Fatalf("expected sheet %q to have YSplit: 3, got %+v", sheet, panes)
	}

	window, err := book.Window(sheet, 1, 1, 20, 10)
	if err != nil {
		t.Fatal(err)
	}

	v := newGridViewport()
	v.setPanes(panes)
	if err := v.applyPaneWindows(sheet, gridPaneRanges{Body: document.CellRange{StartRow: 4, EndRow: 20, StartColumn: 1, EndColumn: 10}}, window, document.SheetWindow{}, document.SheetWindow{}, document.SheetWindow{}); err != nil {
		t.Fatal(err)
	}

	const rowHeader, columnHeader = 48, 28

	// Before unfreeze: row 1 (frozen) and row 4 (body) hit testing.
	r1 := v.screenCellRect(document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}, true, false, rowHeader, columnHeader)
	center1 := image.Pt((r1.Min.X+r1.Max.X)/2, (r1.Min.Y+r1.Max.Y)/2)
	row, col, ok := v.cellPoint(center1, rowHeader, columnHeader)
	if !ok || row != 1 || col != 1 {
		t.Fatalf("frozen row 1 hit = (%d,%d), %v; want (1,1)", row, col, ok)
	}

	r4 := v.screenCellRect(document.CellRange{StartRow: 4, StartColumn: 1, EndRow: 4, EndColumn: 1}, false, false, rowHeader, columnHeader)
	center4 := image.Pt((r4.Min.X+r4.Max.X)/2, (r4.Min.Y+r4.Max.Y)/2)
	row, col, ok = v.cellPoint(center4, rowHeader, columnHeader)
	if !ok || row != 4 || col != 1 {
		t.Fatalf("body row 4 hit = (%d,%d), %v; want (4,1)", row, col, ok)
	}

	// Now unfreeze panes:
	v.setPanes(document.SpreadsheetPaneFreeze{Frozen: false})
	if v.X != 0 || v.Y != 0 {
		t.Fatalf("unfreezing unscrolled sheet must keep (0,0), got (%v,%v)", v.X, v.Y)
	}

	// After unfreeze, clicking on row 1, 2, 3, 4, 5... must accurately hit each cell.
	for expectedRow := 1; expectedRow <= 10; expectedRow++ {
		rect := v.screenCellRect(document.CellRange{StartRow: expectedRow, StartColumn: 2, EndRow: expectedRow, EndColumn: 2}, false, false, rowHeader, columnHeader)
		center := image.Pt((rect.Min.X+rect.Max.X)/2, (rect.Min.Y+rect.Max.Y)/2)
		hitRow, hitCol, ok := v.cellPoint(center, rowHeader, columnHeader)
		if !ok || hitRow != expectedRow || hitCol != 2 {
			t.Fatalf("unfrozen click on row %d = (%d,%d), %v; want (%d,2), true", expectedRow, hitRow, hitCol, ok, expectedRow)
		}
		if hdrRow := v.headerRowAt(center.Y - columnHeader); hdrRow != expectedRow-1 {
			t.Fatalf("unfrozen headerRowAt for row %d = %d; want %d", expectedRow, hdrRow, expectedRow-1)
		}
	}
}
