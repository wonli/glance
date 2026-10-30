package desktopui

import (
	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op/paint"
	"image"
	"image/color"
	"runtime"
	"testing"
)

func TestProofreadingUnlocatedCharacterHasNoConnectorSource(t *testing.T) {
	a, _, comparison := proofreadingConnectorFixture(1)
	frame := comparison.Frames[0]
	frame.AOffset = 9999
	if _, ok := proofreadingOCRStepConnectorSourceBounds(a.proofreading.ocrStep, comparison, frame, image.Pt(300, 120)); ok {
		t.Fatal("missing character was assigned a block as its location")
	}
}

func TestProofreadingConnectorCurveAndBadgeColors(t *testing.T) {
	from, to := image.Pt(0, 0), image.Pt(100, 100)
	if p := proofreadingConnectorCurvePoint(from, to, 0); p != from {
		t.Fatalf("start: %v", p)
	}
	if p := proofreadingConnectorCurvePoint(from, to, 1); p != to {
		t.Fatalf("end: %v", p)
	}
	if p := proofreadingConnectorCurvePoint(from, to, .25); p.Y == 25 || p.X == 25 {
		t.Fatalf("expected curved interpolation: %v", p)
	}
	for _, b := range []proofreadingCenterBadge{{}, {active: true}, {resolved: true}} {
		if got := proofreadingConnectorBadgeColor(b); got != proofreadingCenterBadgeStroke(b) {
			t.Fatalf("line and badge colors differ: %v %v", got, proofreadingCenterBadgeStroke(b))
		}
	}
}

func TestProofreadingUnlocatedCharacterDoesNotMarkPageOrCrop(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a, gtx, comparison := proofreadingConnectorFixture(scale)
		step := a.proofreading.ocrStep
		frame := comparison.Frames[0]
		frame.Kind = proofreadingOCRDiffReplace
		frame.AOffset = 9999
		comparison.Frames = []proofreadingOCRDiffFrame{frame}
		comparison.AOnlyBlockIDs = []int{7}
		page := step.pages[comparison.APage]
		page.layout.Characters = nil
		step.pages[comparison.APage] = page
		red := image.NewRGBA(image.Rect(0, 0, 300, 120))
		for y := 0; y < 120; y++ {
			for x := 0; x < 300; x++ {
				red.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
			}
		}
		a.pdfView.cache = map[int]pdfPageResult{comparison.APage: {imageOp: paint.NewImageOp(red)}}
		size := image.Pt(int(300*scale), int(120*scale))
		gtx.Constraints = layout.Exact(size)
		win, err := headless.NewWindow(size.X, size.Y)
		if err != nil {
			t.Skipf("headless GPU unavailable: %v", err)
		}
		for _, crop := range []bool{false, true} {
			gtx.Ops.Reset()
			paint.Fill(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			if crop {
				a.layoutInspectorPDFZoomedCrop(gtx, step, comparison, frame)
			} else {
				drawProofreadingOCRStepAOnlyBlockHighlights(gtx, size, page, step, comparison)
			}
			if err := win.Frame(gtx.Ops); err != nil {
				t.Fatal(err)
			}
			shot := image.NewRGBA(image.Rectangle{Max: size})
			if err := win.Screenshot(shot); err != nil {
				t.Fatal(err)
			}
			for y := 0; y < size.Y; y++ {
				for x := 0; x < size.X; x++ {
					p := shot.RGBAAt(x, y)
					if int(p.R) > int(p.G)+30 && int(p.R) > int(p.B)+30 {
						t.Fatalf("%gx crop=%v: unexpected red pixel at %d,%d", scale, crop, x, y)
					}
				}
			}
		}
		win.Release()
	}
}
