package desktopui

import (
	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"glance/internal/document"
	"image"
	"image/color"
	"testing"
	"time"
)

func headerSelectionHarness(t *testing.T) *gridTextHarness {
	h := newFullGridTextHarness(t, false, "value")
	h.app.cellEdit.Editing = false
	h.size = image.Pt(600, 240)
	h.frame()
	return h
}
func TestGridHeaderClicksSelectRowsColumnsAndSheet(t *testing.T) {
	for _, tc := range []struct {
		name string
		x, y float32
		want document.CellRange
	}{
		{"row", 20, 108, document.CellRange{StartRow: 2, EndRow: 2, StartColumn: 1, EndColumn: xlsxColumns}},
		{"column", 437, 15, document.CellRange{StartRow: 1, EndRow: xlsxRows, StartColumn: 2, EndColumn: 2}},
		{"sheet", 20, 15, document.CellRange{StartRow: 1, EndRow: xlsxRows, StartColumn: 1, EndColumn: xlsxColumns}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := headerSelectionHarness(t)
			h.point(pointer.Press, tc.x, tc.y)
			h.point(pointer.Release, tc.x, tc.y)
			if got := h.app.selection.Bounds(); got != tc.want {
				t.Fatalf("header click selection=%+v; want %+v", got, tc.want)
			}
			if h.app.cellEdit.Editing {
				t.Fatal("header click entered cell text editing")
			}
		})
	}
}
func TestGridHeaderShiftAndDragExtendScope(t *testing.T) {
	h := headerSelectionHarness(t)
	h.point(pointer.Press, 20, 62)
	h.point(pointer.Release, 20, 62)
	h.router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(20, 140), Buttons: pointer.ButtonPrimary, Modifiers: key.ModShift, Time: h.now})
	h.frame()
	h.point(pointer.Release, 20, 140)
	want := document.CellRange{StartRow: 1, EndRow: 3, StartColumn: 1, EndColumn: xlsxColumns}
	if got := h.app.selection.Bounds(); got != want {
		t.Fatalf("Shift row header selection=%+v; want %+v", got, want)
	}
	h.point(pointer.Press, 200, 15)
	h.point(pointer.Move, 437, 100)
	h.point(pointer.Release, 437, 100)
	want = document.CellRange{StartRow: 1, EndRow: xlsxRows, StartColumn: 1, EndColumn: 2}
	if got := h.app.selection.Bounds(); got != want {
		t.Fatalf("drag column header selection=%+v; want %+v", got, want)
	}
	h.now += 1000000000
	h.point(pointer.Press, 80, 108)
	h.point(pointer.Release, 80, 108)
	if got := h.app.selection.Bounds(); got != (document.CellRange{StartRow: 2, EndRow: 2, StartColumn: 1, EndColumn: 1}) {
		t.Fatalf("cell click did not restore cell scope: %+v", got)
	}
}

func TestGridHeaderSelectionPaintsSelectedHeaders(t *testing.T) {
	for _, tc := range []struct {
		name  string
		x, y  float32
		pixel image.Point
	}{
		{"row", 20, 108, image.Pt(5, 108)},
		{"column", 437, 15, image.Pt(400, 5)},
		{"sheet", 20, 15, image.Pt(5, 5)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := headerSelectionHarness(t)
			h.point(pointer.Press, tc.x, tc.y)
			h.point(pointer.Release, tc.x, tc.y)
			img := renderPresentationTestOps(t, &h.ops, h.size)
			got := color.NRGBAModel.Convert(img.At(tc.pixel.X, tc.pixel.Y)).(color.NRGBA)
			if got != selectedFill {
				t.Fatalf("header fill=%v; want selected fill %v", got, selectedFill)
			}
		})
	}
}

func TestGridHeaderSelectionsEnableFontFormatting(t *testing.T) {
	for _, tc := range []struct {
		name string
		x, y float32
	}{{"row", 20, 108}, {"column", 437, 15}, {"sheet", 20, 15}} {
		t.Run(tc.name, func(t *testing.T) {
			h := headerSelectionHarness(t)
			h.point(pointer.Press, tc.x, tc.y)
			h.point(pointer.Release, tc.x, tc.y)
			for _, action := range []int{4, toolbarActionFontFamily, toolbarActionFontSize} {
				if !h.app.toolbarActionEnabled(action) {
					t.Fatalf("font action %d disabled for header selection", action)
				}
			}
		})
	}
}
func TestGridHeaderDeleteClearsContentsWithoutDeletingDimensions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		x, y    float32
		cleared map[string]bool
	}{
		{"row", 20, 108, map[string]bool{"B2": true}},
		{"column", 437, 15, map[string]bool{"B2": true}},
		{"sheet", 20, 15, map[string]bool{"A1": true, "B2": true, "C3": true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := headerSelectionHarness(t)
			a := h.app
			a.formatResults = make(chan spreadsheetFormatResult, 1)
			for _, cell := range []string{"A1", "B2", "C3"} {
				if err := a.book.SetCellText(a.sheet, cell, cell); err != nil {
					t.Fatal(err)
				}
			}
			h.point(pointer.Press, tc.x, tc.y)
			h.point(pointer.Release, tc.x, tc.y)
			if !h.router.Source().Focused(&a.virtualGrid.PointerTag) {
				t.Fatal("header selection did not focus grid")
			}
			h.router.Queue(key.Event{Name: key.NameDeleteBackward, State: key.Press})
			h.frame()
			select {
			case result := <-a.formatResults:
				if result.err != nil {
					t.Fatal(result.err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("Delete did not complete a content clear operation")
			}
			for _, cell := range []struct {
				name     string
				row, col int
			}{{"A1", 1, 1}, {"B2", 2, 2}, {"C3", 3, 3}} {
				window, err := a.book.(document.GridReader).Window(a.sheet, cell.row, cell.col, 1, 1)
				if err != nil {
					t.Fatal(err)
				}
				want := cell.name
				if tc.cleared[cell.name] {
					want = ""
				}
				if got := window.Cells[0][0].Value; got != want {
					t.Fatalf("%s=%q; want %q", cell.name, got, want)
				}
			}
			if a.virtualGrid.Rows.sizeAt(0) != 60 || a.virtualGrid.Columns.sizeAt(0) != 350 {
				t.Fatal("Delete changed row or column geometry")
			}
		})
	}
}

func TestGridHeaderClearRejectedDispatchReleasesOperation(t *testing.T) {
	h := headerSelectionHarness(t)
	h.app.managedBackgroundClosed = true
	h.app.clearSpreadsheetSelectionContents("清除内容")
	if h.app.formatRunning.Load() {
		t.Fatal("rejected clear dispatch left mutation operation held")
	}
}
