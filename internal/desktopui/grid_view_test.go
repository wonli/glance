package desktopui

import (
	"errors"
	"glance/internal/document"
	"testing"
)

func TestGridScrollDeltaFollowsPointerScrollDirection(t *testing.T) {
	if dx, dy := gridScrollDelta(0, 120, false); dx != 0 || dy != 120 {
		t.Fatalf("downward scroll delta=(%v,%v); want (0,120)", dx, dy)
	}
	if dx, dy := gridScrollDelta(8, 12, true); dx != 20 || dy != 0 {
		t.Fatalf("shift scroll delta=(%v,%v); want (20,0)", dx, dy)
	}
}

func TestValueFilterHidesRowsAndClearsToOriginalHeights(t *testing.T) {
	v := newGridViewport()
	v.WindowSheet = "Sheet1"
	v.WindowRange = document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 3, EndColumn: 1}
	base := v.Rows.sizeAt(2)
	state := newDocumentSessionState("")
	state.valueFilter = &ValueFilter{Sheet: "Sheet1", Bounds: v.WindowRange, VisibleRows: map[int]bool{2: true}, OriginalHeights: map[int]float64{0: v.Rows.sizeAt(0), 1: v.Rows.sizeAt(1), 2: base}}
	state.virtualGrid = v
	app := &App{documentSessionState: state}
	app.applyValueFilterWindow(state, v)
	if got := v.Rows.sizeAt(2); got != 0 {
		t.Fatalf("filtered row size=%v; want 0", got)
	}
	app.clearValueFilter(state)
	if got := v.Rows.sizeAt(2); got != base {
		t.Fatalf("cleared row size=%v; want %v", got, base)
	}
}

func TestFilterMatchingRowsSkipsHeaderAndMatchesBlankExactly(t *testing.T) {
	cells := [][]document.Cell{{{Value: "类型"}}, {{Value: ""}}, {{Value: "A"}}, {{Value: ""}}}
	if got := filterMatchingRows(cells, 10, 0, ""); len(got) != 2 || !got[11] || !got[13] {
		t.Fatalf("blank filter visible rows=%v; want 11 and 13", got)
	}
	if got := filterMatchingRows(cells, 10, 0, "A"); len(got) != 1 || !got[12] {
		t.Fatalf("exact filter visible rows=%v; want 12", got)
	}
}

type memoryGridReader struct {
	calls int
	fail  bool
}

func (m *memoryGridReader) Window(sheet string, r, c, nr, nc int) (document.SheetWindow, error) {
	m.calls++
	if m.fail {
		return document.SheetWindow{}, errors.New("read error")
	}
	return document.SheetWindow{StartRow: r, StartColumn: c, Cells: make([][]document.Cell, nr)}, nil
}
func (*memoryGridReader) SetRowHeight(string, int, float64) error   { return nil }
func (*memoryGridReader) SetColumnWidth(string, int, float64) error { return nil }

func TestGridViewCachesAndInvalidatesWindow(t *testing.T) {
	v := newGridViewport()
	reader := new(memoryGridReader)
	one := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 10, EndColumn: 10}
	if err := v.ensureWindow(reader, "Sheet1", one); err != nil {
		t.Fatal(err)
	}
	if err := v.ensureWindow(reader, "Sheet1", document.CellRange{StartRow: 2, StartColumn: 2, EndRow: 8, EndColumn: 8}); err != nil {
		t.Fatal(err)
	}
	if reader.calls != 1 {
		t.Fatalf("calls=%d", reader.calls)
	}
	v.invalidateWindow()
	_ = v.ensureWindow(reader, "Sheet1", one)
	if reader.calls != 2 {
		t.Fatalf("after invalidation calls=%d", reader.calls)
	}
	_ = v.ensureWindow(reader, "Sheet2", one)
	if reader.calls != 3 {
		t.Fatalf("after sheet switch calls=%d", reader.calls)
	}
}
func TestGridViewRetainsPreviousWindowOnReadError(t *testing.T) {
	v := newGridViewport()
	reader := new(memoryGridReader)
	one := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 4, EndColumn: 4}
	if err := v.ensureWindow(reader, "Sheet1", one); err != nil {
		t.Fatal(err)
	}
	before := v.Window
	reader.fail = true
	next := document.CellRange{StartRow: 20, StartColumn: 20, EndRow: 24, EndColumn: 24}
	if err := v.ensureWindow(reader, "Sheet1", next); err == nil {
		t.Fatal("expected read error")
	}
	if v.WindowRange != one || v.Window.StartRow != before.StartRow || v.WindowSheet != "Sheet1" {
		t.Fatal("previous window was discarded")
	}
}

func TestGridViewportScalesGeometryToDevicePixels(t *testing.T) {
	v := newGridViewport()
	v.setScale(2)
	if got, _ := v.Rows.Boundary(1); got != gridDefaultRow*2 {
		t.Fatalf("first row boundary=%v; want %v device pixels", got, gridDefaultRow*2)
	}
	if got, _ := v.Columns.Boundary(1); got != gridDefaultColumn*2 {
		t.Fatalf("first column boundary=%v; want %v device pixels", got, gridDefaultColumn*2)
	}
	v.setScale(2)
	if got, _ := v.Rows.Boundary(2); got != gridDefaultRow*4 {
		t.Fatalf("repeated scale changed row geometry: boundary=%v", got)
	}
}

func TestGridViewportScalesWorkbookDimensionsToDevicePixels(t *testing.T) {
	v := newGridViewport()
	v.setScale(2)
	window := document.SheetWindow{
		StartRow: 1, StartColumn: 1,
		Cells:             [][]document.Cell{{{Value: "x"}}},
		RowHeightsPoints:  []float64{18},
		ColumnWidthsChars: []float64{10},
	}
	if err := v.applyWindow("Sheet1", document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1}, window); err != nil {
		t.Fatal(err)
	}
	if got, _ := v.Rows.Boundary(1); got != 48 {
		t.Fatalf("row boundary=%v; want 48 device pixels", got)
	}
	if got, _ := v.Columns.Boundary(1); got != 150 {
		t.Fatalf("column boundary=%v; want 150 device pixels", got)
	}
}
