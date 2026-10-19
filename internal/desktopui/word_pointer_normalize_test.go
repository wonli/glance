package desktopui

import (
	"gioui.org/f32"
	"gioui.org/io/pointer"
	"image"
	"testing"
)

func TestWordPointerWindowsDragUsesClientSnapshot(t *testing.T) {
	interaction := wordVisiblePageInteraction{PageRect: image.Rect(79, 0, 1000, 1000)}
	// Captured Windows drag from the trace was displaced up/left, while
	// press/release positions remained in the correct page-tag coordinates.
	sample := wordPointerSample{valid: true, client: f32.Pt(454, 788), origin: image.Pt(0, 256)}
	for _, tc := range []struct {
		name   string
		kind   pointer.Kind
		source pointer.Source
		want   f32.Point
	}{
		{"mouse-drag", pointer.Drag, pointer.Mouse, f32.Pt(454, 532)},
		{"mouse-release", pointer.Release, pointer.Mouse, f32.Pt(219, 239)},
		{"mouse-press", pointer.Press, pointer.Mouse, f32.Pt(219, 239)},
		{"touch-drag", pointer.Drag, pointer.Touch, f32.Pt(219, 239)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := wordPointerViewportPoint(pointer.Event{Kind: tc.kind, Source: tc.source, Position: f32.Pt(140, 239)}, interaction, sample)
			if got != tc.want {
				t.Fatalf("position=%v want %v", got, tc.want)
			}
		})
	}
	sample.valid = false
	got := wordPointerViewportPoint(pointer.Event{Kind: pointer.Drag, Source: pointer.Mouse, Position: f32.Pt(140, 239)}, interaction, sample)
	if got != f32.Pt(219, 239) {
		t.Fatalf("unavailable native sample changed fallback: %v", got)
	}
}
