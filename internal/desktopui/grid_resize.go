package desktopui

import "math"

type resizeAxis uint8

const (
	resizeColumns resizeAxis = iota + 1
	resizeRows
)

const (
	minGridColumnPx     = 20.0
	minGridRowPx        = 10.0
	maxGridColumnPx     = 1790.0
	maxGridRowPx        = 546.0
	resizeHandleSlop    = 5.0
	resizePreviewLinePx = 2.0
)

type resizeState struct {
	Axis         resizeAxis
	Index        int // zero-based axis index
	StartPointer float64
	LastPointer  float64
	StartEdge    float64
	OriginalPx   float64
	PreviewPx    float64
	OriginalFile float64
}

type gridResizeSession struct {
	State                     *resizeState
	OriginalAxis, PreviewAxis *AxisGeometry
}

func newResizeState(axis resizeAxis, index int, pointer, edge, sizePx, fileUnits float64) *resizeState {
	return &resizeState{Axis: axis, Index: index, StartPointer: pointer, LastPointer: pointer, StartEdge: edge, OriginalPx: sizePx, PreviewPx: sizePx, OriginalFile: fileUnits}
}
func (r *resizeState) update(pointer, minPx, maxPx float64) {
	if r == nil || !finite(pointer) {
		return
	}
	r.LastPointer = pointer
	r.PreviewPx = math.Max(minPx, math.Min(maxPx, r.OriginalPx+pointer-r.StartPointer))
}
func (r *resizeState) rebase(pointer, newEdge float64) {
	if r == nil || !finite(pointer) || !finite(newEdge) {
		return
	}
	r.StartPointer = pointer
	r.LastPointer = pointer
	r.StartEdge = newEdge
	r.OriginalPx = r.PreviewPx
}
func (r *resizeState) cancel() {
	if r != nil {
		r.PreviewPx = r.OriginalPx
	}
}

func (r *resizeState) previewEdge() float64 {
	if r == nil {
		return 0
	}
	return r.StartEdge + (r.PreviewPx - r.OriginalPx)
}

func resizePreviewExtent(oldExtent, previewEdge, lineWidth float64) float64 {
	if previewEdge+lineWidth > oldExtent {
		return previewEdge + lineWidth
	}
	return oldExtent
}
func pointsToGridPixels(points float64) float64     { return points * 96.0 / 72.0 }
func gridPixelsToPoints(pixels float64) float64     { return pixels * 72.0 / 96.0 }
func columnCharsToGridPixels(chars float64) float64 { return chars*7.0 + 5.0 }
func gridPixelsToColumnChars(pixels float64) float64 {
	return math.Round(math.Max(0, (pixels-5.0)/7.0)*100) / 100
}

func (v *gridViewport) beginResize(axis resizeAxis, index int, pointerPos, edge float64) {
	var original *AxisGeometry
	var fileUnits float64
	if axis == resizeColumns {
		original = v.Columns
		fileUnits = gridPixelsToColumnChars(original.sizeAt(index))
	} else {
		original = v.Rows
		fileUnits = gridPixelsToPoints(original.sizeAt(index))
	}
	preview := original.Clone()
	px := original.sizeAt(index)
	v.Resize = &gridResizeSession{State: newResizeState(axis, index, pointerPos, edge, px, fileUnits), OriginalAxis: original, PreviewAxis: preview}
	if axis == resizeColumns {
		v.Columns = preview
	} else {
		v.Rows = preview
	}
}

func (v *gridViewport) updateResize(pointerPos float64) {
	if v.Resize == nil {
		return
	}
	r := v.Resize.State
	minPx, maxPx := minGridRowPx, maxGridRowPx
	if r.Axis == resizeColumns {
		minPx, maxPx = minGridColumnPx, maxGridColumnPx
	}
	r.update(pointerPos, minPx, maxPx)
	_ = v.Resize.PreviewAxis.SetSize(r.Index, r.PreviewPx)
}

func (v *gridViewport) cancelResize() {
	if v.Resize == nil {
		return
	}
	r := v.Resize.State
	r.cancel()
	if r.Axis == resizeColumns {
		v.Columns = v.Resize.OriginalAxis
	} else {
		v.Rows = v.Resize.OriginalAxis
	}
	v.Resize = nil
}

func (v *gridViewport) completeResize(reader interface {
	SetRowHeight(string, int, float64) error
	SetColumnWidth(string, int, float64) error
}, sheet string) error {
	if v.Resize == nil {
		return nil
	}
	session := v.Resize
	r := session.State
	units := gridPixelsToPoints(r.PreviewPx)
	if r.Axis == resizeColumns {
		units = gridPixelsToColumnChars(r.PreviewPx)
	}
	var err error
	if r.Axis == resizeColumns {
		err = reader.SetColumnWidth(sheet, r.Index+1, units)
	} else {
		err = reader.SetRowHeight(sheet, r.Index+1, units)
	}
	if err != nil {
		v.cancelResize()
		return err
	}
	v.Resize = nil
	v.invalidateWindow()
	return nil
}

func gridResizeHandleAt(v *gridViewport, x, y, rowHead, colHead int) (resizeAxis, int, float64, float64, bool) {
	if y >= 0 && y <= colHead && x >= rowHead {
		position := float64(x-rowHead) + v.X
		idx := v.Columns.IndexAt(position)
		if idx < 0 {
			return 0, 0, 0, 0, false
		}
		for boundary := idx + 1; boundary >= idx-1 && boundary > 0; boundary-- {
			edge, _ := v.Columns.Boundary(boundary)
			screen := float64(rowHead) + edge - v.X
			if math.Abs(float64(x)-screen) <= resizeHandleSlop {
				return resizeColumns, boundary - 1, screen, v.Columns.sizeAt(boundary - 1), true
			}
		}
	}
	if x >= 0 && x <= rowHead && y >= colHead {
		position := float64(y-colHead) + v.Y
		idx := v.Rows.IndexAt(position)
		if idx < 0 {
			return 0, 0, 0, 0, false
		}
		for boundary := idx + 1; boundary >= idx-1 && boundary > 0; boundary-- {
			edge, _ := v.Rows.Boundary(boundary)
			screen := float64(colHead) + edge - v.Y
			if math.Abs(float64(y)-screen) <= resizeHandleSlop {
				return resizeRows, boundary - 1, screen, v.Rows.sizeAt(boundary - 1), true
			}
		}
	}
	return 0, 0, 0, 0, false
}

func resizeScreenEdge(v *gridViewport, r *resizeState, rowHead, colHead int) float64 {
	if r.Axis == resizeColumns {
		edge, _ := v.Columns.Boundary(r.Index + 1)
		return float64(rowHead) + edge - v.X
	}
	edge, _ := v.Rows.Boundary(r.Index + 1)
	return float64(colHead) + edge - v.Y
}
