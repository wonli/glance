package desktopui

import (
	"image"
	"image/color"
	"runtime"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/op/paint"
)

func proofreadingSingleBadgeFixture(scale float32) (*App, []image.Rectangle) {
	a, gtx, comparison := proofreadingConnectorFixture(scale)
	frame := comparison.Frames[9]
	frame.Kind = proofreadingOCRDiffReplace
	comparison.Frames = []proofreadingOCRDiffFrame{frame}
	pair := proofreadingOCRStepPair{APage: 3, BPage: 3}
	a.proofreading.ocrStep.comparisons[pair] = comparison
	a.proofreading.ocrStep.diffCursor = 1
	a.drawProofreadingOCRStepConnector(gtx)
	var rects []image.Rectangle
	for _, badge := range a.proofreading.ocrStep.currentBadges {
		rects = append(rects, badge.rect)
	}
	return a, rects
}

func TestProofreadingBadgeFirstPlacementIgnoresTextScroll(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		_, want := proofreadingSingleBadgeFixture(scale)
		a, gtx, comparison := proofreadingConnectorFixture(scale)
		frame := comparison.Frames[9]
		frame.Kind = proofreadingOCRDiffReplace
		comparison.Frames = []proofreadingOCRDiffFrame{frame}
		pair := proofreadingOCRStepPair{APage: 3, BPage: 3}
		a.proofreading.ocrStep.comparisons[pair] = comparison
		a.proofreading.ocrStep.diffCursor = 1
		a.proofreading.xPageScrollOffsets[3] = 25
		a.drawProofreadingOCRStepConnector(gtx)
		got := a.proofreading.ocrStep.currentBadges
		if len(want) != 1 || len(got) != 1 || got[0].rect != want[0] {
			t.Fatalf("%gx: text scrollbar changed first badge placement: want %v, got %v", scale, want, got)
		}
		// Text browsing must not move the badge inside its paper frame.
		a.proofreading.xPageScrollOffsets[3] = 50
		gtx.Ops.Reset()
		a.drawProofreadingOCRStepConnector(gtx)
		got = a.proofreading.ocrStep.currentBadges
		if len(got) != 1 || got[0].rect != want[0] {
			t.Fatalf("%gx: scrolling moved the saved badge: want %v, got %v", scale, want, got)
		}
		// Moving both papers moves their badge by the same amount. A previous
		// page's badge must not stay behind over the next page.
		a.pdfView.canvas.Position.Offset = 25
		a.proofreading.xCanvas.Position.Offset = 25
		gtx.Ops.Reset()
		a.drawProofreadingOCRStepConnector(gtx)
		got = a.proofreading.ocrStep.currentBadges
		if len(got) != 1 || got[0].rect != want[0].Sub(image.Pt(0, 25)) {
			t.Fatalf("%gx: badge detached from its paper while canvas moved: want %v, got %v", scale, want[0].Sub(image.Pt(0, 25)), got)
		}
		a.pdfView.canvas.Position.Offset = 400
		a.proofreading.xCanvas.Position.Offset = 400
		gtx.Ops.Reset()
		a.drawProofreadingOCRStepConnector(gtx)
		if len(a.proofreading.ocrStep.currentBadges) != 0 {
			t.Fatal("offscreen page left its badge over another page")
		}
	}
}

func TestProofreadingBadgeExistsOutsideVirtualTextWindow(t *testing.T) {
	a, gtx, comparison := proofreadingConnectorFixture(1)
	frame := comparison.Frames[9]
	frame.Kind = proofreadingOCRDiffReplace
	comparison.Frames = []proofreadingOCRDiffFrame{frame}
	pair := proofreadingOCRStepPair{APage: 3, BPage: 3}
	a.proofreading.ocrStep.comparisons[pair] = comparison
	a.proofreading.xEditorWindows[3].Start = 20
	a.proofreading.xPageEditors[3].SetText(string([]rune(comparison.BText)[20:]))
	a.drawProofreadingOCRStepConnector(gtx)
	if len(a.proofreading.ocrStep.currentBadges) != 1 {
		t.Fatal("complete comparison lost its badge because its character is outside the virtual text window")
	}
}

func TestProofreadingBadgeSourceRowIgnoresGlyphHeight(t *testing.T) {
	a, gtx, comparison := proofreadingConnectorFixture(1)
	comparison.Frames = comparison.Frames[1:3]
	for index := range comparison.Frames {
		comparison.Frames[index].Kind = proofreadingOCRDiffReplace
	}
	page := a.proofreading.ocrStep.pages[3]
	page.layout.Characters[2].Bounds[1] += 1
	a.proofreading.ocrStep.pages[3] = page
	connections := proofreadingOCRStepBadgeConnections(gtx, a.pdfView, a.proofreading, a.proofreading.ocrStep, comparison)
	if len(connections) != 1 || len(connections[0].allFrames) != 2 || !proofreadingOCRConnectionContainsFrame(connections[0], comparison.Frames[1]) {
		t.Fatalf("one source difference row must keep one badge and both click targets: %v", connections)
	}
}

func TestProofreadingBadgeDistinctSourceRowsKeepLiveConnectors(t *testing.T) {
	for _, nextTop := range []float64{60, 39} {
		a, gtx, comparison := proofreadingConnectorFixture(1)
		comparison.Frames = comparison.Frames[1:3] // One X text row.
		for index := range comparison.Frames {
			comparison.Frames[index].Kind = proofreadingOCRDiffReplace
		}
		step := a.proofreading.ocrStep
		page := step.pages[3]
		page.layout.Characters[1].Bounds[1], page.layout.Characters[1].Bounds[3] = 20, 40
		page.layout.Characters[2].Bounds[1], page.layout.Characters[2].Bounds[3] = nextTop, nextTop+20
		step.pages[3] = page
		pair := proofreadingOCRStepPair{APage: 3, BPage: 3}
		step.comparisons[pair] = comparison
		step.diffCursor = len(comparison.Frames)
		a.drawProofreadingOCRStepConnector(gtx)
		if got := len(step.badgeConnections[pair]); got != 2 {
			t.Fatalf("source row at %v merged into the previous row: got %d badges", nextTop, got)
		}
		live := proofreadingOCRStepConnections(gtx, a.pdfView, a.proofreading, step, comparison)
		if len(live) != 2 {
			t.Fatalf("source row at %v lost its connector when X wrapped both rows together: %v", nextTop, live)
		}
	}
}

func TestProofreadingCompletedHistoryShowsEveryConnection(t *testing.T) {
	a, gtx, comparison := proofreadingConnectorFixture(1)
	step := a.proofreading.ocrStep
	step.diffPair = proofreadingOCRStepPair{APage: -1, BPage: -1}
	want := proofreadingOCRStepConnections(gtx, a.pdfView, a.proofreading, step, comparison)
	step.diffPair = proofreadingOCRStepPair{APage: 3, BPage: 3}
	step.diffCursor = 2 // Left over from another playback, with no animation active.
	got := proofreadingOCRStepConnections(gtx, a.pdfView, a.proofreading, step, comparison)
	if len(want) < 2 || len(got) != len(want) {
		t.Fatalf("completed history was truncated by an idle playback cursor: want %d visible connections, got %d", len(want), len(got))
	}
}

func TestProofreadingDrawingDoesNotChangePlaybackPair(t *testing.T) {
	a, gtx, _ := proofreadingConnectorFixture(1)
	want := a.proofreading.ocrStep.diffPair
	a.pdfView.page = 0
	a.drawProofreadingOCRStepConnector(gtx)
	if got := a.proofreading.ocrStep.diffPair; got != want {
		t.Fatalf("drawing a selected page changed the playback's pair: %v -> %v", want, got)
	}
}

func TestProofreadingBadgePixelsStayFixedWithinPaper(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a, gtx, comparison := proofreadingConnectorFixture(scale)
		frame := comparison.Frames[9]
		frame.Kind = proofreadingOCRDiffReplace
		comparison.Frames = []proofreadingOCRDiffFrame{frame}
		a.proofreading.ocrStep.comparisons[proofreadingOCRStepPair{APage: 3, BPage: 3}] = comparison
		a.proofreading.ocrStep.diffCursor = 1
		window, err := headless.NewWindow(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)
		if err != nil {
			t.Skipf("headless GPU unavailable: %v", err)
		}
		render := func() *image.RGBA {
			gtx.Ops.Reset()
			paint.Fill(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			a.drawProofreadingOCRStepConnector(gtx)
			if err := window.Frame(gtx.Ops); err != nil {
				t.Fatal(err)
			}
			shot := image.NewRGBA(image.Rectangle{Max: gtx.Constraints.Max})
			if err := window.Screenshot(shot); err != nil {
				t.Fatal(err)
			}
			return shot
		}
		before := render()
		rect := a.proofreading.ocrStep.currentBadges[0].rect.Inset(int(2 * scale))
		a.proofreading.xPageScrollOffsets[3] = 25
		a.proofreading.aiProofreading = false
		after := render()
		ink := 0
		for y := rect.Min.Y; y < rect.Max.Y; y++ {
			for x := rect.Min.X; x < rect.Max.X; x++ {
				pixel := before.RGBAAt(x, y)
				if int(pixel.R) > int(pixel.G)+20 {
					ink++
				}
				if pixel != after.RGBAAt(x, y) {
					t.Fatalf("%gx: rendered badge moved or changed after scrolling at (%d,%d)", scale, x, y)
				}
			}
		}
		if ink == 0 {
			t.Fatalf("%gx: badge number produced no visible pixels", scale)
		}
		pan := int(25 * scale)
		a.pdfView.canvas.Position.Offset = pan
		a.proofreading.xCanvas.Position.Offset = pan
		paperAfter := render()
		window.Release()
		for y := rect.Min.Y; y < rect.Max.Y; y++ {
			for x := rect.Min.X; x < rect.Max.X; x++ {
				if before.RGBAAt(x, y) != paperAfter.RGBAAt(x, y-pan) {
					t.Fatalf("%gx: rendered badge detached from its paper at (%d,%d)", scale, x, y)
				}
			}
		}
	}
}
