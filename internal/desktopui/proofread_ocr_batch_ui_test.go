package desktopui

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"glance/internal/i18n"
	"runtime"
	"strings"
)

func TestProofreadingOCRBatchDialogControlsAndShield(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		a, step := proofreadingBatchFixture(t)
		var router input.Router
		var ops op.Ops
		var underlying widget.Clickable
		clicks := 0
		frame := func() {
			ops.Reset()
			gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(int(900*scale), int(650*scale)))}
			a.processProofreadingOCRBatchControls(gtx)
			child := a.proofreadingOCRBatchContentContext(gtx)
			if underlying.Clicked(child) {
				clicks++
			}
			underlying.Layout(child, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} })
			a.layoutProofreadingOCRBatchDialog(gtx)
			router.Frame(&ops)
		}
		click := func(point f32.Point) {
			router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Position: point, Buttons: pointer.ButtonPrimary})
			frame()
			router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point})
			frame()
		}
		button := func(id string) f32.Point {
			var bounds image.Rectangle
			var visit func([]input.SemanticNode)
			visit = func(nodes []input.SemanticNode) {
				for _, node := range nodes {
					if node.Desc.Description == a.textID(id) {
						bounds = node.Desc.Bounds
					}
					visit(node.Children)
				}
			}
			visit(router.AppendSemantics(nil))
			if bounds.Empty() {
				t.Fatalf("missing dialog control %s", id)
			}
			if id == "proofread.ai.batch.pause" && (bounds.Min.Y < int(410*scale) || bounds.Max.X < int(681*scale)) {
				t.Fatalf("dialog action is not anchored bottom-right: %+v", bounds)
			}
			return f32.Pt(float32(bounds.Min.X+bounds.Dx()/2), float32(bounds.Min.Y+bounds.Dy()/2))
		}
		frame()
		click(f32.Pt(10*scale, 10*scale))
		router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
		frame()
		if clicks != 0 || !step.batch.open || !step.batch.running {
			t.Fatal("scrim/Escape dismissed dialog or clicked through")
		}
		click(button("proofread.ai.batch.pause"))
		if step.batch.running || !step.batch.open {
			t.Fatal("pause did not retain shield")
		}
		click(button("proofread.ai.batch.resume"))
		if !step.batch.running || !step.batch.open {
			t.Fatal("resume failed")
		}
		click(button("proofread.ai.batch.pause"))
		click(button("proofread.ai.batch.inspect"))
		if step.batch.open || step.batch.running {
			t.Fatal("inspect should release shield and remain paused")
		}
		click(f32.Pt(10*scale, 10*scale))
		if clicks != 1 {
			t.Fatal("inspect did not restore normal page interaction")
		}
	}
}

func TestProofreadingOCRBatchProgressMessagesSubstituteNumbers(t *testing.T) {
	a, _ := proofreadingBatchFixture(t)
	for _, id := range []string{"proofread.ai.batch.processing", "proofread.ai.batch.paused", "proofread.ai.batch.summary"} {
		text := a.textArgs(id, i18n.Args{"Page": "37", "Total": "892", "Done": "36", "Diff": "4"})
		if strings.ContainsAny(text, "{}") {
			t.Fatalf("progress still contains placeholders: %s", text)
		}
	}
}

func TestProofreadingOCRBatchDialogVisual(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		for _, state := range []string{"running", "paused", "complete"} {
			a, step := proofreadingBatchFixture(t)
			step.batch.running = state == "running"
			step.batch.checkpoint.Finished = state == "complete"
			var ops op.Ops
			gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(int(900*scale), int(650*scale)))}
			shot := renderProofreadingHistory(t, gtx, func(gtx layout.Context) { a.layoutProofreadingOCRBatchDialog(gtx) })
			// The card is centered at (190,175), 520x300dp, above a visible scrim.
			outside := shot.RGBAAt(int(10*scale), int(10*scale))
			inside := shot.RGBAAt(int(200*scale), int(185*scale))
			if outside.R > 200 || inside.R < 245 {
				t.Fatalf("scale=%v state=%s missing scrim or centered white card", scale, state)
			}
			blue := 0
			for y := int(203 * scale); y < int(227*scale); y++ {
				for x := int(218 * scale); x < int(242*scale); x++ {
					p := shot.RGBAAt(x, y)
					if int(p.B) > int(p.R)+50 {
						blue++
					}
				}
			}
			if blue < int(15*scale*scale) {
				t.Fatalf("scale=%v state=%s stack/scan icon has no visible strokes", scale, state)
			}
		}
	}
}

func TestProofreadingOCRBatchDialogBlocksFocusedEditorAndCanvasWheel(t *testing.T) {
	a, step := proofreadingBatchFixture(t)
	step.batch.open = false
	a.pdfView.scale = 1
	a.pdfView.canvas.Axis = layout.Vertical
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(600, 700))}
		child := a.proofreadingOCRBatchContentContext(gtx)
		a.processProofreadingEditorEvents(child)
		a.layoutProofreadingXNavigatorCanvas(child, a.pdfView, 3, "", false)
		a.layoutProofreadingOCRBatchDialog(gtx)
		router.Frame(&ops)
	}
	frame()
	router.Source().Execute(key.FocusCmd{Tag: &a.proofreading.xPageEditors[0]})
	frame()
	text := a.proofreading.xPageEditors[0].Text()
	revision := a.proofreading.controller.Model.Revision()
	step.batch.open = true
	frame()
	position := a.pdfView.canvas.Position
	router.Queue(key.EditEvent{Text: "误触"}, pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(5, 100), Scroll: f32.Pt(0, 500)})
	frame()
	frame()
	if a.proofreading.xPageEditors[0].Text() != text || a.proofreading.controller.Model.Revision() != revision {
		t.Fatal("modal let keyboard edit the manuscript")
	}
	if a.pdfView.canvas.Position.First != position.First || a.pdfView.canvas.Position.Offset != position.Offset {
		t.Fatal("modal let wheel scroll underlying canvas")
	}
}
