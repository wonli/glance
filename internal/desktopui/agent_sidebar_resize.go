package desktopui

import (
	"image"

	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

func agentSidebarResizeBounds(gtx layout.Context) (int, int) {
	available := max(0, gtx.Constraints.Max.X)
	maximum := min(gtx.Dp(unit.Dp(600)), max(available/3, available-gtx.Dp(unit.Dp(320))))
	return min(gtx.Dp(unit.Dp(180)), maximum), maximum
}

func (a *App) updateAgentSidebarResize(gtx layout.Context) {
	s := &a.agentUI
	// Router cancellation may reset the pointer ID. Handle it before Drag's
	// ID filter so a lost capture never leaves a preview width committed.
	for _, ok := gtx.Event(pointer.Filter{Target: &s.sidebarResizeDrag, Kinds: pointer.Cancel}); ok; _, ok = gtx.Event(pointer.Filter{Target: &s.sidebarResizeDrag, Kinds: pointer.Cancel}) {
		s.sidebarResizing = false
		s.sidebarResizeDrag = gesture.Drag{}
	}
	for e, ok := s.sidebarResizeDrag.Update(gtx.Metric, gtx.Source, gesture.Horizontal); ok; e, ok = s.sidebarResizeDrag.Update(gtx.Metric, gtx.Source, gesture.Horizontal) {
		switch e.Kind {
		case pointer.Press:
			s.sidebarResizeStartX = e.Position.X
			s.sidebarResizeStartWidthDP = unit.Dp(float32(a.agentSidebarWidth(gtx)) / max32(.1, gtx.Metric.PxPerDp))
			s.sidebarResizePreviewDP = s.sidebarResizeStartWidthDP
			s.sidebarResizing = true
		case pointer.Drag:
			if s.sidebarResizing {
				// The recognizer is registered in workspace coordinates, even
				// when its narrow hit area moves with the sidebar boundary.
				s.sidebarResizePreviewDP = s.sidebarResizeStartWidthDP + unit.Dp((e.Position.X-s.sidebarResizeStartX)/max32(.1, gtx.Metric.PxPerDp))
			}
		case pointer.Release:
			if s.sidebarResizing {
				s.sidebarWidthDP = unit.Dp(float32(a.agentSidebarWidth(gtx)) / max32(.1, gtx.Metric.PxPerDp))
				a.saveAgentSidebarPreference()
			}
			s.sidebarResizing = false
		case pointer.Cancel:
			s.sidebarResizing = false
		}
	}
}

func (a *App) layoutAgentSidebarResizeHandle(gtx layout.Context, width int) {
	if width <= 0 {
		return
	}
	slop := gtx.Dp(unit.Dp(4))
	area := image.Rect(max(0, width-slop), 0, min(gtx.Constraints.Max.X, width+slop), gtx.Constraints.Max.Y)
	if a.agentUI.sidebarResizing {
		area = image.Rectangle{Max: gtx.Constraints.Max}
	}
	stack := clip.Rect(area).Push(gtx.Ops)
	semantic.DescriptionOp("拖动调整 AI 侧栏宽度").Add(gtx.Ops)
	pointer.CursorColResize.Add(gtx.Ops)
	a.agentUI.sidebarResizeDrag.Add(gtx.Ops)
	stack.Pop()
	// The design uses a full-height divider; retain a wider invisible drag target.
	line := image.Rect(width-gtx.Dp(unit.Dp(1)), 0, width, gtx.Constraints.Max.Y)
	paint.FillShape(gtx.Ops, agentSidebarBorder, clip.Rect(line).Op())
}
