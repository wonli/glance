package desktopui

import (
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
)

type horizontalRibbonScrollState struct {
	tag           struct{}
	remainder     float32
	dragRemainder float32
	dragPointer   pointer.ID
	dragStartX    float32
	dragStartY    float32
	dragLastX     float32
	dragPressed   bool
	dragPanning   bool
	canScroll     bool
}

func layoutHorizontalRibbonList(gtx layout.Context, state *horizontalRibbonScrollState, list *widget.List, length int, element layout.ListElement) layout.Dimensions {
	if state == nil || list == nil {
		return layout.Dimensions{}
	}
	list.ScrollAnyAxis = false
	handleHorizontalRibbonScroll(gtx, state, list)

	dimensions := list.Layout(gtx, length, element)
	state.canScroll = dimensions.Size.X > 0 && list.Position.Length > dimensions.Size.X
	pass := pointer.PassOp{}.Push(gtx.Ops)
	area := clip.Rect{Max: dimensions.Size}.Push(gtx.Ops)
	event.Op(gtx.Ops, &state.tag)
	area.Pop()
	pass.Pop()
	return dimensions
}

func handleHorizontalRibbonScroll(gtx layout.Context, state *horizontalRibbonScrollState, list *widget.List) {
	const scrollLimit = 1 << 30
	filter := pointer.Filter{
		Target:  &state.tag,
		Kinds:   pointer.Scroll,
		ScrollX: pointer.ScrollRange{Min: -scrollLimit, Max: scrollLimit},
		ScrollY: pointer.ScrollRange{Min: -scrollLimit, Max: scrollLimit},
	}
	for eventValue, ok := gtx.Event(filter); ok; eventValue, ok = gtx.Event(filter) {
		pe, ok := eventValue.(pointer.Event)
		if !ok || pe.Kind != pointer.Scroll || pe.Scroll.X == 0 && pe.Scroll.Y == 0 {
			continue
		}
		delta := pe.Scroll.X
		if absFloat32(pe.Scroll.Y) >= absFloat32(pe.Scroll.X) {
			delta = pe.Scroll.Y
		}
		applyHorizontalRibbonScrollDelta(state, list, delta)
	}
}

func handleHorizontalRibbonDrag(gtx layout.Context, state *horizontalRibbonScrollState, list *widget.List) {
	if state == nil || list == nil {
		return
	}
	filter := pointer.Filter{
		Target: &state.tag,
		Kinds:  pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel,
	}
	for eventValue, ok := gtx.Event(filter); ok; eventValue, ok = gtx.Event(filter) {
		pe, ok := eventValue.(pointer.Event)
		if !ok || pe.Source != pointer.Mouse {
			continue
		}
		switch pe.Kind {
		case pointer.Press:
			if !state.canScroll || pe.Buttons&pointer.ButtonPrimary == 0 {
				continue
			}
			state.dragPointer = pe.PointerID
			state.dragStartX = pe.Position.X
			state.dragStartY = pe.Position.Y
			state.dragLastX = pe.Position.X
			state.dragPressed = true
			state.dragPanning = false
		case pointer.Drag:
			if !state.dragPressed || pe.PointerID != state.dragPointer {
				continue
			}
			previousX := state.dragLastX
			state.dragLastX = pe.Position.X
			if !state.dragPanning {
				deltaX := state.dragStartX - pe.Position.X
				deltaY := state.dragStartY - pe.Position.Y
				threshold := float32(gtx.Dp(unit.Dp(4)))
				if absFloat32(deltaX) < threshold || absFloat32(deltaX) < absFloat32(deltaY) {
					continue
				}
				state.dragPanning = true
				if pe.Priority < pointer.Grabbed {
					// Grab after the threshold so the originating button is
					// canceled and the Ribbon keeps receiving movement outside it.
					gtx.Execute(pointer.GrabCmd{Tag: &state.tag, ID: pe.PointerID})
				}
				applyHorizontalPanDelta(state, list, deltaX)
				continue
			}
			// Pointer positions are tag-local physical pixels; use their
			// relative delta so the pan stays in the List's coordinate space.
			applyHorizontalPanDelta(state, list, previousX-pe.Position.X)
		case pointer.Release, pointer.Cancel:
			if !state.dragPressed || pe.PointerID != state.dragPointer {
				continue
			}
			state.dragPressed = false
			state.dragPanning = false
			state.dragPointer = 0
		}
	}
}

func applyHorizontalRibbonScrollDelta(state *horizontalRibbonScrollState, list *widget.List, delta float32) {
	applyHorizontalRibbonDelta(&state.remainder, list, delta)
}

func applyHorizontalPanDelta(state *horizontalRibbonScrollState, list *widget.List, delta float32) {
	applyHorizontalRibbonDelta(&state.dragRemainder, list, delta)
}

func applyHorizontalRibbonDelta(remainder *float32, list *widget.List, delta float32) {
	*remainder += delta
	pixels := int(*remainder)
	*remainder -= float32(pixels)
	if pixels == 0 {
		return
	}
	list.Position.Offset += pixels
	list.Position.BeforeEnd = true
}
