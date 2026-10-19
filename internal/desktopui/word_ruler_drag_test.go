package desktopui

import (
	"bytes"
	"image"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/format/docx"
	"glance/internal/wordlayout"
)

func TestWordRulerFourGesturesSaveAndCancel(t *testing.T) {
	for _, density := range []float32{1, 2} {
		for _, zoom := range []float64{0, .5, 1.5} {
			d, err := docx.NewBlank()
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			state := newDocumentSessionState("ruler.docx")
			state.word = d
			state.wordBlocks = cloneWordBlockSnapshot(d.WordBlocks())
			state.selectedWordParagraphID = state.wordBlocks[0].Paragraph.ID
			gestureScale := float32(1)
			if zoom > 0 {
				gestureScale = float32(zoom)
				state.wordLayoutZoom = zoom
				state.wordPageScrollX = 100 * float64(density)
				state.wordLayoutResult = &wordlayout.Result{CanPreview: true, Pages: []wordlayout.PageScene{{Clip: wordlayout.Rect{WidthPT: 600, HeightPT: 800}, Content: wordlayout.Rect{XPT: 72, WidthPT: 456}}}}
			}
			app := &App{documentSessionState: state, theme: material.NewTheme(), wordCommandResults: make(chan wordCommandResult, 1)}
			var ops op.Ops
			var router input.Router
			frame := func() {
				ops.Reset()
				app.layoutWordHorizontalRuler(layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: density, PxPerSp: density}, Constraints: layout.Exact(image.Pt(int(1100*density), int(28*density)))}, state)
				router.Frame(&ops)
			}
			target := func(name string) f32.Point {
				frame()
				var bounds image.Rectangle
				var visit func([]input.SemanticNode)
				visit = func(nodes []input.SemanticNode) {
					for _, n := range nodes {
						if n.Desc.Description == name {
							bounds = n.Desc.Bounds
						}
						visit(n.Children)
					}
				}
				visit(router.AppendSemantics(nil))
				if bounds.Empty() {
					t.Fatalf("missing %s", name)
				}
				return f32.Pt(float32(bounds.Min.X+bounds.Dx()/2), float32(bounds.Min.Y+bounds.Dy()/2))
			}
			gesture := func(name string, dx float32, cancel bool) {
				t.Logf("gesture %s dx=%v cancel=%v", name, dx, cancel)
				start := target(name)
				before, _ := d.Snapshot()
				router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 7, Buttons: pointer.ButtonPrimary, Position: start})
				frame()
				if state.wordRulerIndentDragSide == 0 {
					t.Fatalf("%s press not captured at density %v at %v geometry %+v", name, density, start, wordRulerGeometryForState(layout.Context{Metric: unit.Metric{PxPerDp: density, PxPerSp: density}, Constraints: layout.Exact(image.Pt(int(1100*density), int(28*density)))}, state))
				}
				end := start.Add(f32.Pt(dx*density*gestureScale, 0))
				router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 7, Buttons: pointer.ButtonPrimary, Position: end})
				frame()
				preview, _ := d.Snapshot()
				if !bytes.Equal(before, preview) {
					t.Fatal("drag preview changed document")
				}
				kind := pointer.Release
				if cancel {
					kind = pointer.Cancel
				}
				router.Queue(pointer.Event{Kind: kind, Source: pointer.Mouse, PointerID: 7, Position: end})
				frame()
				if cancel || dx == 0 {
					after, _ := d.Snapshot()
					if !bytes.Equal(before, after) || state.wordRulerIndentIntent != nil {
						t.Fatal("cancel/no-op changed document")
					}
					return
				}
				select {
				case r := <-app.wordCommandResults:
					if r.err != nil {
						t.Fatal(r.err)
					}
					app.wordCommandResults <- r
					app.processAsyncResults()
				case <-time.After(5 * time.Second):
					t.Fatal("ruler commit timed out")
				}
			}
			// Escape cancels the preview without a DOCX mutation, even before release.
			start := target("拖动调整段落首行缩进")
			before, _ := d.Snapshot()
			router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 8, Buttons: pointer.ButtonPrimary, Position: start})
			frame()
			router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 8, Buttons: pointer.ButtonPrimary, Position: start.Add(f32.Pt(24*density, 0))})
			frame()
			router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
			frame()
			if state.wordRulerIndentDragSide != 0 {
				t.Fatal("Escape left ruler drag active")
			}
			router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 8, Position: start})
			frame()
			after, _ := d.Snapshot()
			if !bytes.Equal(before, after) {
				t.Fatal("Escape committed ruler change")
			}
			gesture("拖动调整段落首行缩进", 24, true)
			gesture("拖动调整段落左缩进", 0, false)
			gesture("拖动调整段落首行缩进", 24, false)
			p := d.WordBlocks()[0].Paragraph
			if p.LeftIndentEMU != 0 || p.Layout.FirstLineIndentEMU != 24*wordEMUPerDP {
				t.Fatalf("first-line %+v", p)
			}
			gesture("拖动调整段落悬挂缩进", 24, false)
			p = d.WordBlocks()[0].Paragraph
			if p.LeftIndentEMU != 24*wordEMUPerDP || p.Layout.FirstLineIndentEMU-p.Layout.HangingIndentEMU != 0 {
				t.Fatalf("hanging %+v", p)
			}
			gesture("拖动调整段落左缩进", 24, false)
			gesture("拖动调整段落右缩进", -24, false)
			path := filepath.Join(t.TempDir(), "ruler.docx")
			if err := d.SaveAs(path); err != nil {
				t.Fatal(err)
			}
			reopened, err := docx.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			p = reopened.WordBlocks()[0].Paragraph
			reopened.Close()
			if p.LeftIndentEMU != 48*wordEMUPerDP || p.RightIndentEMU != 24*wordEMUPerDP || p.Layout.FirstLineIndentEMU-p.Layout.HangingIndentEMU != 0 {
				t.Fatalf("reopened indents: %+v", p)
			}
		}
	}
}
