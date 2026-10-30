package desktopui

import (
	"gioui.org/f32"
	"gioui.org/gpu/headless"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/op/paint"
	"image"
	"image/color"
	"runtime"
	"testing"
)

func TestProofreadingConnectorAppearsOnlyOnBadgeHover(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a, gtx, comparison := proofreadingConnectorFixture(scale)
		frame := comparison.Frames[9]
		frame.Kind = proofreadingOCRDiffReplace
		comparison.Frames = []proofreadingOCRDiffFrame{frame}
		pair := proofreadingOCRStepPair{APage: 3, BPage: 3}
		a.proofreading.ocrStep.comparisons[pair] = comparison
		var router input.Router
		gtx.Source = router.Source()
		win, err := headless.NewWindow(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)
		if err != nil {
			t.Skipf("headless GPU unavailable: %v", err)
		}
		draw := func() *image.RGBA {
			t.Helper()
			gtx.Ops.Reset()
			paint.Fill(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			a.drawProofreadingOCRStepConnector(gtx)
			router.Frame(gtx.Ops)
			if err := win.Frame(gtx.Ops); err != nil {
				t.Fatal(err)
			}
			shot := image.NewRGBA(image.Rectangle{Max: gtx.Constraints.Max})
			if err := win.Screenshot(shot); err != nil {
				t.Fatal(err)
			}
			return shot
		}
		count := func(shot *image.RGBA) int {
			n := 0
			for y := int(44 * scale); y < int(180*scale); y++ {
				for x := int(350 * scale); x < int(470*scale); x++ {
					p := shot.RGBAAt(x, y)
					if int(p.R) > int(p.G)+10 || int(p.B) > int(p.R)+10 {
						n++
					}
				}
			}
			return n
		}
		initial := count(draw())
		if initial != 0 {
			win.Release()
			t.Fatalf("%gx: line visible without hover: %d pixels", scale, initial)
		}
		badges := a.proofreading.ocrStep.currentBadges
		if len(badges) != 1 {
			win.Release()
			t.Fatalf("expected one badge, got %d", len(badges))
		}
		b := badges[0].rect
		router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(float32((b.Min.X+b.Max.X)/2), float32((b.Min.Y+b.Max.Y)/2))})
		hovered := count(draw())
		if hovered == 0 {
			win.Release()
			t.Fatalf("%gx: hovered badge has no line", scale)
		}
		step := a.proofreading.ocrStep
		savedPage := step.pages[comparison.APage]
		noCharacterPage := savedPage
		noCharacterPage.layout.Characters = nil
		step.pages[comparison.APage] = noCharacterPage
		unlocated := draw()
		leftInk, rightInk := 0, 0
		for y := int(44 * scale); y < unlocated.Bounds().Max.Y; y++ {
			for x := 0; x < unlocated.Bounds().Max.X; x++ {
				p := unlocated.RGBAAt(x, y)
				if int(p.R) <= int(p.G)+10 {
					continue
				}
				if x < b.Min.X-int(5*scale) {
					leftInk++
				}
				if x > b.Max.X+int(5*scale) {
					rightInk++
				}
			}
		}
		if leftInk == 0 || rightInk == 0 {
			win.Release()
			t.Fatalf("%gx: unlocated PDF glyph: left=%d right=%d; want both visible line segments", scale, leftInk, rightInk)
		}
		sourceOrigin, _, ok := proofreadingOCRStepConnectorPageOrigins(gtx, a.pdfView, a.proofreading, comparison.APage, comparison.BPage)
		if !ok {
			win.Release()
			t.Fatal("source page origin unavailable")
		}
		pageEdge := sourceOrigin.X + proofreadingOCRStepSourcePagePixelSize(a.pdfView, comparison.APage).X
		edgeInk := 0
		for y := badges[0].y - int(2*scale); y <= badges[0].y+int(2*scale); y++ {
			for x := pageEdge; x <= pageEdge+int(4*scale); x++ {
				if !image.Pt(x, y).In(unlocated.Bounds()) {
					continue
				}
				p := unlocated.RGBAAt(x, y)
				if int(p.R) > int(p.G)+10 {
					edgeInk++
				}
			}
		}
		if edgeInk == 0 {
			win.Release()
			t.Fatalf("%gx: line does not reach PDF page right edge at x=%d", scale, pageEdge)
		}
		for y := badges[0].y - int(3*scale); y <= badges[0].y+int(3*scale); y++ {
			x := pageEdge - int(3*scale)
			if image.Pt(x, y).In(unlocated.Bounds()) {
				p := unlocated.RGBAAt(x, y)
				if int(p.R) > int(p.G)+10 {
					win.Release()
					t.Fatalf("%gx: incorrect glyph marker inside PDF at %d,%d", scale, x, y)
				}
			}
		}
		step.pages[comparison.APage] = savedPage
		a.proofreading.xPageScrollOffsets[3] = 200
		offscreen := draw()
		connections := proofreadingOCRStepConnections(gtx, a.pdfView, a.proofreading, a.proofreading.ocrStep, comparison)
		if len(connections) != 1 || connections[0].toOutside != -1 {
			win.Release()
			t.Fatalf("%gx: missing upward edge marker: %+v", scale, connections)
		}
		point := connections[0].to
		coloredArrow := 0
		for y := point.Y + int(2*scale); y <= point.Y+int(6*scale); y++ {
			for x := point.X - int(5*scale); x <= point.X+int(5*scale); x++ {
				if x == point.X || !image.Pt(x, y).In(offscreen.Bounds()) {
					continue
				}
				p := offscreen.RGBAAt(x, y)
				if int(p.R) > int(p.G)+10 {
					coloredArrow++
				}
			}
		}
		if coloredArrow == 0 {
			win.Release()
			t.Fatalf("%gx: edge arrow has no visible pixels", scale)
		}
		router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(10, 10)})
		left := count(draw())
		if left != 0 {
			win.Release()
			t.Fatalf("%gx: line remained after pointer left: %d", scale, left)
		}
		clickPoint := f32.Pt(float32((b.Min.X+b.Max.X)/2), float32((b.Min.Y+b.Max.Y)/2))
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Position: clickPoint, Buttons: pointer.ButtonPrimary})
		draw()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: clickPoint})
		draw()
		if !a.proofreading.ocrStep.inspector.open || a.proofreading.ocrStep.inspector.diffIndex != 0 {
			win.Release()
			t.Fatalf("%gx: badge click did not open the corresponding inspector", scale)
		}
		win.Release()
	}
}
