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
	if got, _ := v.Rows.Boundary(1); got != 40 {
		t.Fatalf("first row boundary=%v; want 40 device pixels", got)
	}
	if got, _ := v.Columns.Boundary(1); got != 128 {
		t.Fatalf("first column boundary=%v; want 128 device pixels", got)
	}
	v.setScale(2)
	if got, _ := v.Rows.Boundary(2); got != 80 {
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
