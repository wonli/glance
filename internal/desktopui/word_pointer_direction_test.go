package desktopui

import (
	"fmt"
	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"image"
	"testing"
	"time"
)

func TestWordPointerSelectionTracksBothDirections(t *testing.T) {
	for _, scale := range []float32{1, 1.25, 1.5, 2} {
		for _, tc := range []struct {
			name       string
			start, end int
			move       bool
		}{{"forward", 2, 8, true}, {"backward", 8, 2, true}, {"forward-coalesced", 2, 8, false}, {"backward-coalesced", 8, 2, false}} {
			t.Run(fmt.Sprintf("%s/scale%g", tc.name, scale), func(t *testing.T) {
				app, state, router, _ := newWordSelectionPageTest(t, `<w:p><w:r><w:t>中文abcdef尾部</w:t></w:r></w:p>`)
				var ops op.Ops
				frame := func() {
					ops.Reset()
					gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Constraints{Max: image.Pt(1200, 1000)}}
					offset := op.Offset(image.Pt(13, 117)).Push(&ops)
					app.layoutWordPages(gtx, state, wordPageContinuous)
					offset.Pop()
					router.Frame(&ops)
				}
				frame()
				transform := state.wordPageVisibleInteractions[0].Transform
				point := func(offset int) f32.Point {
					return transform.PageToScreen(f32.Pt(float32(10+offset*10), 30)).Add(f32.Pt(13, 117))
				}
				router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: point(tc.start), Time: time.Second})
				frame()
				if tc.move {
					router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: point(tc.end), Time: time.Second + time.Millisecond})
					frame()
					selection := state.wordPageTextSelection
					if selection == nil || selection.anchor.runeOffset != tc.start || selection.focus.runeOffset != tc.end {
						t.Fatalf("during drag selection=%+v; want %d:%d", selection, tc.start, tc.end)
					}
				}
				router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point(tc.end + 1), Time: time.Second + 2*time.Millisecond})
				frame()
				if state.wordPageSelectionDrag != nil {
					t.Fatal("release left selection drag active")
				}
				selection := state.wordPageTextSelection
				if selection == nil || selection.anchor.runeOffset != tc.start || selection.focus.runeOffset != tc.end+1 {
					t.Fatalf("selection=%+v; want %d:%d", selection, tc.start, tc.end+1)
				}
			})
		}
	}
}
