package desktopui

import (
	"errors"
	"glance/internal/document"
	"math"
	"testing"
)

type resizeTestReader struct {
	rows, cols int
	fail       bool
}

func (*resizeTestReader) Window(string, int, int, int, int) (document.SheetWindow, error) {
	return document.SheetWindow{}, nil
}
func (r *resizeTestReader) SetRowHeight(_ string, _ int, _ float64) error {
	r.rows++
	if r.fail {
		return errors.New("write failed")
	}
	return nil
}
func (r *resizeTestReader) SetColumnWidth(_ string, _ int, _ float64) error {
	r.cols++
	if r.fail {
		return errors.New("write failed")
	}
	return nil
}

func TestResizePreviewTracksPointerAcrossCoordinateRebase(t *testing.T) {
	r := newResizeState(resizeColumns, 2, 100, 290, 90, 12.14)
	r.update(130, minGridColumnPx, maxGridColumnPx)
	if r.PreviewPx != 120 {
		t.Fatalf("preview=%v", r.PreviewPx)
	}
	if r.previewEdge() != 320 {
		t.Fatalf("preview edge=%v", r.previewEdge())
	}
	r.rebase(130, 270)
	r.update(140, minGridColumnPx, maxGridColumnPx)
	if r.PreviewPx != 130 || r.StartEdge != 270 {
		t.Fatalf("after rebase=%+v", r)
	}
	if r.previewEdge() != 280 {
		t.Fatalf("rebased preview edge=%v", r.previewEdge())
	}
}
func TestResizePreviewExtendsPastGridEdge(t *testing.T) {
	if got := resizePreviewExtent(400, 435, resizePreviewLinePx); got != 437 {
		t.Fatalf("extent=%v", got)
	}
	if got := resizePreviewExtent(400, 390, resizePreviewLinePx); got != 400 {
		t.Fatalf("extent=%v", got)
	}
}
func TestResizeCancelKeepsOriginalSize(t *testing.T) {
	r := newResizeState(resizeRows, 0, 50, 30, 20, 15)
	r.update(-10, minGridRowPx, maxGridRowPx)
	r.cancel()
	if r.PreviewPx != 20 {
		t.Fatalf("cancel preview=%v", r.PreviewPx)
	}
}
func TestResizeCommitOccursOnlyOnRelease(t *testing.T) {
	r := newResizeState(resizeColumns, 0, 50, 100, 60, 8)
	r.update(80, minGridColumnPx, maxGridColumnPx)
	calls := 0
	commit := func(float64) error { calls++; return nil }
	if calls != 0 {
		t.Fatal("committed during preview")
	}
	if err := commit(gridPixelsToColumnChars(r.PreviewPx)); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}
func TestExcelDimensionConversions(t *testing.T) {
	for _, p := range []float64{0, 10, 15, 30, 409.5} {
		if got := gridPixelsToPoints(pointsToGridPixels(p)); math.Abs(got-p) > 1e-8 {
			t.Fatalf("points %v => %v", p, got)
		}
	}
	for _, w := range []float64{0, 8.43, 12, 255} {
		if got := gridPixelsToColumnChars(columnCharsToGridPixels(w)); math.Abs(got-w) > 1e-8 {
			t.Fatalf("chars %v => %v", w, got)
		}
	}
}

func TestDevicePixelDimensionConversionsAtRetinaScale(t *testing.T) {
	for _, p := range []float64{10, 15, 30, 409.5} {
		if got := devicePixelsToPoints(pointsToDevicePixels(p, 2), 2); math.Abs(got-p) > 1e-8 {
			t.Fatalf("scaled points %v => %v", p, got)
		}
	}
	for _, w := range []float64{8.43, 12, 255} {
		if got := devicePixelsToChars(charsToDevicePixels(w, 2), 2); math.Abs(got-w) > 1e-8 {
			t.Fatalf("scaled chars %v => %v", w, got)
		}
	}
}

func TestGridResizePreviewIsTransientAndReleaseCommits(t *testing.T) {
	v := newGridViewport()
	before := v.Columns.TotalSize()
	v.beginResize(resizeColumns, 1, 100, 160)
	v.updateResize(130)
	if v.Columns.TotalSize() == before {
		t.Fatal("preview geometry was not updated")
	}
	r := new(resizeTestReader)
	if r.cols != 0 {
		t.Fatal("write happened before release")
	}
	if err := v.completeResize(r, "Sheet1"); err != nil {
		t.Fatal(err)
	}
	if r.cols != 1 || r.rows != 0 || v.Resize != nil {
		t.Fatalf("reader=%+v resize=%v", r, v.Resize)
	}
}

func TestGridResizeCancelAndFailureRestoreGeometry(t *testing.T) {
	for _, fail := range []bool{false, true} {
		v := newGridViewport()
		before := v.Rows.TotalSize()
		v.beginResize(resizeRows, 4, 100, 128)
		v.updateResize(150)
		if fail {
			r := &resizeTestReader{fail: true}
			if err := v.completeResize(r, "Sheet1"); err == nil {
				t.Fatal("expected failure")
			}
		} else {
			v.cancelResize()
		}
		if v.Rows.TotalSize() != before || v.Resize != nil {
			t.Fatalf("failed/cancelled resize left preview geometry, fail=%v", fail)
		}
	}
}

func TestGridResizeHandleUsesFixedHeaderCoordinates(t *testing.T) {
	v := newGridViewport()
	axis, index, _, _, ok := gridResizeHandleAt(v, 48+int(gridDefaultColumn), 10, 48, 28)
	if !ok || axis != resizeColumns || index != 0 {
		t.Fatalf("column handle: %v %d %v", axis, index, ok)
	}
	axis, index, _, _, ok = gridResizeHandleAt(v, 10, 28+int(gridDefaultRow), 48, 28)
	if !ok || axis != resizeRows || index != 0 {
		t.Fatalf("row handle: %v %d %v", axis, index, ok)
	}
}
