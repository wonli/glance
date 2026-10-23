package desktopui

import (
	"image"

	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

const (
	markdownSplitHandleWidth unit.Dp = 10
	markdownSplitMinColumn   unit.Dp = 280
)

func (a *App) updateMarkdownSplitDrag(gtx layout.Context, v *markdownViewState) {
	if v.mode != 2 {
		if v.splitDragging {
			v.splitRatio = v.splitDragStartRatio
		}
		v.splitDragging = false
		v.splitDrag = gesture.Drag{}
		return
	}
	for _, ok := gtx.Event(pointer.Filter{Target: &v.splitDrag, Kinds: pointer.Cancel}); ok; _, ok = gtx.Event(pointer.Filter{Target: &v.splitDrag, Kinds: pointer.Cancel}) {
		if v.splitDragging {
			v.splitRatio = v.splitDragStartRatio
		}
		v.splitDragging = false
		v.splitDrag = gesture.Drag{}
		return
	}
	for e, ok := v.splitDrag.Update(gtx.Metric, gtx.Source, gesture.Horizontal); ok; e, ok = v.splitDrag.Update(gtx.Metric, gtx.Source, gesture.Horizontal) {
		switch e.Kind {
		case pointer.Press:
			// Pointer positions are local to the moving handle. Convert them to
			// split-container coordinates so the origin stays fixed as columns move.
			v.splitDragStartX = e.Position.X + float32(v.splitHandleOriginPx)
			v.splitDragStartRatio = markdownConstrainedSplitRatio(gtx, v.splitRatio, v.splitAvailableWidthPx)
			v.splitRatio = v.splitDragStartRatio
			v.splitDragging = true
		case pointer.Drag:
			if v.splitDragging && v.splitAvailableWidthPx > 0 {
				pointerX := e.Position.X + float32(v.splitHandleOriginPx)
				delta := pointerX - v.splitDragStartX
				v.splitRatio = markdownConstrainedSplitRatio(gtx, v.splitDragStartRatio+delta/float32(v.splitAvailableWidthPx), v.splitAvailableWidthPx)
			}
		case pointer.Release:
			v.splitDragging = false
		case pointer.Cancel:
			if v.splitDragging {
				v.splitRatio = v.splitDragStartRatio
			}
			v.splitDragging = false
		}
	}
}

func markdownConstrainedSplitRatio(gtx layout.Context, ratio float32, availablePx int) float32 {
	minColumnPx := gtx.Dp(markdownSplitMinColumn)
	if availablePx <= 0 || availablePx < minColumnPx*2 {
		return .5
	}
	minimumRatio := float32(minColumnPx) / float32(availablePx)
	maximumRatio := 1 - minimumRatio
	if ratio < minimumRatio {
		return minimumRatio
	}
	if ratio > maximumRatio {
		return maximumRatio
	}
	return ratio
}

func (a *App) layoutMarkdownSplitHandle(gtx layout.Context, v *markdownViewState, width int) layout.Dimensions {
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	pointer.CursorColResize.Add(gtx.Ops)
	v.splitDrag.Add(gtx.Ops)
	area.Pop()

	lineWidth := max(1, gtx.Dp(unit.Dp(1)))
	left := (width - lineWidth) / 2
	line := image.Rect(left, 0, left+lineWidth, gtx.Constraints.Max.Y)
	paint.FillShape(gtx.Ops, agentSidebarBorder, clip.Rect(line).Op())
	return layout.Dimensions{Size: gtx.Constraints.Max}
}
