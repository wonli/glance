package desktopui

import (
	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/unit"
	"image"
)

type wordPointerSample struct {
	nativeThread, windowThread uint32
	valid                      bool
	client                     f32.Point
	origin                     image.Point
}

type wordPointerState struct {
	hwnd            uintptr
	sample          wordPointerSample
	previousSession *documentSessionState
	previousSize    image.Point
	previousMetric  unit.Metric
}

// Only captured Windows mouse movement needs the native snapshot. A frame
// displays the latest cursor position even if multiple moves were coalesced.
// Press/release keep their event-time positions; other platforms have no sample.
func wordPointerViewportPoint(event pointer.Event, interaction wordVisiblePageInteraction, sample wordPointerSample) f32.Point {
	if sample.valid && event.Kind == pointer.Drag && event.Source == pointer.Mouse {
		return sample.client.Sub(f32.Pt(float32(sample.origin.X), float32(sample.origin.Y)))
	}
	return event.Position.Add(f32.Pt(float32(interaction.PageRect.Min.X), float32(interaction.PageRect.Min.Y)))
}

func wordPointerViewportBounds(nodes []input.SemanticNode) (image.Rectangle, bool) {
	for _, node := range nodes {
		if node.Desc.Description == nativePinchSurface && !node.Desc.Bounds.Empty() {
			return node.Desc.Bounds, true
		}
		if bounds, ok := wordPointerViewportBounds(node.Children); ok {
			return bounds, true
		}
	}
	return image.Rectangle{}, false
}

func (a *App) finishWordPointerFrame(frame app.FrameEvent) {
	a.wordPointer.previousSession = a.documentSessionState
	a.wordPointer.previousSize = frame.Size
	a.wordPointer.previousMetric = frame.Metric
}
