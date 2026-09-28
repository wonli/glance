package desktopui

import (
	"image"
	"testing"

	"glance/internal/document"
)

func TestCellAtPointUsesVariableDimensions(t *testing.T) {
	r, _ := NewAxisGeometry(20, 20)
	c, _ := NewAxisGeometry(10, 50)
	_ = r.SetSize(0, 30)
	_ = c.SetSize(0, 90)
	row, col, ok := cellAtPoint(r, c, 40, 25, image.Pt(160, 70), 20, 20)
	if !ok || row != 4 || col != 3 {
		t.Fatalf("cell=(%d,%d,%v)", row, col, ok)
	}
}
func TestMergeAnchorAtInteriorAndEdge(t *testing.T) {
	ms := []document.MergeRegion{{Range: document.CellRange{StartRow: 2, StartColumn: 3, EndRow: 4, EndColumn: 5}}}
	for _, p := range [][2]int{{2, 3}, {3, 4}, {4, 5}} {
		r, c, ok := mergeAnchorAt(ms, p[0], p[1])
		if !ok || r != 2 || c != 3 {
			t.Fatalf("%v => %d,%d,%v", p, r, c, ok)
		}
	}
	if _, _, ok := mergeAnchorAt(ms, 1, 3); ok {
		t.Fatal("outside merge matched")
	}
}
func TestMergedCellRectSpansRowsAndColumns(t *testing.T) {
	r, _ := NewAxisGeometry(10, 20)
	c, _ := NewAxisGeometry(10, 50)
	_ = r.SetSize(1, 30)
	_ = c.SetSize(2, 70)
	got := cellRect(r, c, document.CellRange{StartRow: 2, StartColumn: 3, EndRow: 4, EndColumn: 5}, 10, 5)
	if got != image.Rect(90, 15, 260, 85) {
		t.Fatalf("rect=%v", got)
	}
}
func TestVisibleWindowAddsOverscanAndClamps(t *testing.T) {
	r, _ := NewAxisGeometry(10, 10)
	c, _ := NewAxisGeometry(8, 10)
	got := visibleCellWindow(r, c, 20, 30, 20, 20)
	if got != (document.CellRange{StartRow: 2, StartColumn: 1, EndRow: 7, EndColumn: 6}) {
		t.Fatalf("range=%+v", got)
	}
}
