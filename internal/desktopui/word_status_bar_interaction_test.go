package desktopui

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/format/docx"
)

func TestWordStatusZoomSliderDragUsesSessionZoomAndNormalizedCoordinates(t *testing.T) {
	for _, test := range []struct {
		name      string
		pxPerDP   float32
		widthPX   int
		trackLeft int
		trackMid  int
		trackEnd  int
	}{
		{name: "one-x", pxPerDP: 1, widthPX: 1440, trackLeft: 1257, trackMid: 1304, trackEnd: 1338},
		{name: "two-x-narrow-viewport", pxPerDP: 2, widthPX: 2560, trackLeft: 2194, trackMid: 2288, trackEnd: 2356},
	} {
		t.Run(test.name, func(t *testing.T) {
			word, err := docx.NewBlank()
			if err != nil {
				t.Fatal(err)
			}
			defer word.Close()
			state := newDocumentSessionState("zoom.docx")
			state.word = word
			state.wordLayoutZoom = 1
			app := &App{documentSessionState: state, theme: material.NewTheme()}
			router := new(input.Router)
			var ops op.Ops
			metric := unit.Metric{PxPerDp: test.pxPerDP, PxPerSp: test.pxPerDP}
			frame := func() {
				t.Helper()
				ops.Reset()
				gtx := layout.Context{
					Source:      router.Source(),
					Ops:         &ops,
					Now:         time.Now(),
					Metric:      metric,
					Constraints: layout.Constraints{Max: image.Pt(test.widthPX, int(float32(wordStatusBarHeightDP)*test.pxPerDP))},
				}
				app.layoutWordStatusBar(gtx, state)
				router.Frame(&ops)
			}
			point := func(x int) f32.Point { return f32.Pt(float32(x), float32(17*test.pxPerDP)) }
			zoomMatches := func(want float64) {
				t.Helper()
				if state.wordLayoutZoom != want {
					t.Fatalf("Word session zoom = %.3f; want %.3f", state.wordLayoutZoom, want)
				}
			}

			frame()
			center := point(test.trackMid)
			router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 41, Buttons: pointer.ButtonPrimary, Position: center})
			frame()
			zoomMatches(1)
			router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 41, Position: center})
			frame()

			left := point(test.trackLeft)
			router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 42, Buttons: pointer.ButtonPrimary, Position: left})
			frame()
			zoomMatches(0.5)
			router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 42, Buttons: pointer.ButtonPrimary, Position: point(test.trackEnd)})
			frame()
			zoomMatches(1.5)
			router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 42, Position: point(test.trackEnd)})
			frame()
			zoomMatches(1.5)
		})
	}
}
