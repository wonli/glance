package desktopui

import (
	"image"
	"runtime"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/unit"
)

func TestProofreadingInspectorFinishTextCentered(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a, gtx, _ := proofreadingConnectorFixture(scale)
		a.theme = New(nil, "").theme
		gtx.Metric = unit.Metric{PxPerDp: scale, PxPerSp: scale}
		gtx.Constraints = layout.Exact(image.Pt(int(640*scale), int(400*scale)))
		shot := renderProofreadingHistory(t, gtx, func(gtx layout.Context) { a.layoutProofreadingInspectorCompletion(gtx, a.proofreading.ocrStep) })
		var button image.Rectangle
		for y := 0; y < shot.Bounds().Max.Y; y++ {
			for x := 0; x < shot.Bounds().Max.X; x++ {
				p := shot.RGBAAt(x, y)
				if p.R == 26 && p.G == 115 && p.B == 232 {
					button = button.Union(image.Rect(x, y, x+1, y+1))
				}
			}
		}
		if button.Empty() {
			t.Fatal("finish button not rendered")
		}
		var ink image.Rectangle
		region := button.Inset(int(4 * scale))
		for y := region.Min.Y; y < region.Max.Y; y++ {
			for x := region.Min.X; x < region.Max.X; x++ {
				p := shot.RGBAAt(x, y)
				if p.R > 220 && p.G > 220 && p.B > 220 {
					ink = ink.Union(image.Rect(x, y, x+1, y+1))
				}
			}
		}
		if ink.Empty() {
			t.Fatal("finish label not rendered")
		}
		dx := ink.Min.X + ink.Max.X - button.Min.X - button.Max.X
		dy := ink.Min.Y + ink.Max.Y - button.Min.Y - button.Max.Y
		if max(dx, -dx) > int(4*scale) || max(dy, -dy) > int(4*scale) {
			t.Fatalf("scale %.0f: finish text off center: button=%v ink=%v delta=(%d,%d)", scale, button, ink, dx, dy)
		}
	}
}

func TestProofreadingInspectorCompletionHorizontallyCenteredWithLooseConstraints(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a, gtx, _ := proofreadingConnectorFixture(scale)
		a.theme = New(nil, "").theme
		gtx.Metric = unit.Metric{PxPerDp: scale, PxPerSp: scale}
		cardW := int(820 * scale)
		cardH := int(340 * scale)
		// Provide loose constraints where Min.X is 0, matching layout.Flexed(1, ...)
		gtx.Constraints = layout.Constraints{
			Min: image.Pt(0, cardH),
			Max: image.Pt(cardW, cardH),
		}
		shot := renderProofreadingHistory(t, gtx, func(gtx layout.Context) {
			a.layoutProofreadingInspectorCompletion(gtx, a.proofreading.ocrStep)
		})
		var button image.Rectangle
		for y := 0; y < shot.Bounds().Max.Y; y++ {
			for x := 0; x < shot.Bounds().Max.X; x++ {
				p := shot.RGBAAt(x, y)
				if p.R == 26 && p.G == 115 && p.B == 232 {
					button = button.Union(image.Rect(x, y, x+1, y+1))
				}
			}
		}
		if button.Empty() {
			t.Fatal("finish button not rendered")
		}
		buttonCenterX := (button.Min.X + button.Max.X) / 2
		expectedCenterX := cardW / 2
		delta := buttonCenterX - expectedCenterX
		if max(delta, -delta) > int(4*scale) {
			t.Fatalf("scale %.0f: finish button is not centered in card: button center X=%d, expected %d, delta=%d",
				scale, buttonCenterX, expectedCenterX, delta)
		}
	}
}

func TestProofreadingFreshComparisonStartsScanAnimation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a, gtx, comparison := proofreadingConnectorFixture(1)
	step := a.proofreading.ocrStep
	comparison.HasEvidence = true
	a.presentProofreadingOCRStepComparison(comparison)
	if !step.diffRunning || step.diffCursor != 0 || step.diffNextAt.IsZero() {
		t.Fatalf("fresh comparison skipped animation: running=%v cursor=%d next=%v", step.diffRunning, step.diffCursor, step.diffNextAt)
	}
	gtx.Now = step.diffNextAt
	a.advanceProofreadingOCRDiffPlayback(gtx, step)
	if step.diffCursor != 1 {
		t.Fatalf("first animation frame not advanced: %d", step.diffCursor)
	}
	for step.diffRunning {
		gtx.Now = step.diffNextAt.Add(time.Millisecond)
		a.advanceProofreadingOCRDiffPlayback(gtx, step)
	}
	if step.diffCursor != len(comparison.Frames) {
		t.Fatal("animation stopped before scanning all characters")
	}
	if got := proofreadingOCRStepConnections(gtx, a.pdfView, a.proofreading, step, comparison); len(got) != 4 {
		t.Fatalf("completion lost history lines: %d", len(got))
	}
}

func TestProofreadingScanSpeedDoesNotDependOnRefreshRate(t *testing.T) {
	a, gtx, comparison := proofreadingConnectorFixture(1)
	comparison.HasEvidence = true
	comparison.Frames = make([]proofreadingOCRDiffFrame, 40)
	for i := range comparison.Frames {
		comparison.Frames[i] = proofreadingOCRDiffFrame{Kind: proofreadingOCRDiffEqual, BPosition: i}
	}
	step := a.proofreading.ocrStep
	step.comparisons[proofreadingOCRStepPair{APage: comparison.APage, BPage: comparison.BPage}] = comparison
	startProofreadingOCRDiffPlayback(step, comparison, gtx.Now)
	gtx.Now = step.diffNextAt.Add(80 * time.Millisecond)
	a.advanceProofreadingOCRDiffPlayback(gtx, step)
	if step.diffCursor < 16 || step.diffCursor > 24 {
		t.Fatalf("80ms at fast scan speed advanced %d characters; refresh rate must not limit playback to one character", step.diffCursor)
	}
	if step.diffMatches != step.diffCursor {
		t.Fatal("batched scan lost match counts")
	}
	cursor := step.diffCursor
	a.advanceProofreadingOCRDiffPlayback(gtx, step)
	if step.diffCursor != cursor {
		t.Fatal("same timestamp advanced scan twice")
	}
}

func TestProofreadingDiffPlaybackUsesImmediateInvalidation(t *testing.T) {
	a, gtx, comparison := proofreadingConnectorFixture(1)
	comparison.HasEvidence = true
	comparison.Frames = make([]proofreadingOCRDiffFrame, 10)
	for i := range comparison.Frames {
		comparison.Frames[i] = proofreadingOCRDiffFrame{Kind: proofreadingOCRDiffEqual, BPosition: i}
	}
	step := a.proofreading.ocrStep
	step.comparisons[proofreadingOCRStepPair{APage: comparison.APage, BPage: comparison.BPage}] = comparison

	var r input.Router
	gtx.Source = r.Source()
	startProofreadingOCRDiffPlayback(step, comparison, gtx.Now)

	// During active playback, InvalidateCmd must request immediate redraw (zero At)
	// so Gio's CVDisplayLink stays running locked to hardware VSync without OS timer jitter.
	gtx.Now = step.diffNextAt
	a.advanceProofreadingOCRDiffPlayback(gtx, step)
	wakeup, ok := r.WakeupTime()
	if !ok || !wakeup.IsZero() {
		t.Fatalf("expected immediate VSync-locked invalidation (zero WakeupTime), got ok=%v at=%v", ok, wakeup)
	}
}
