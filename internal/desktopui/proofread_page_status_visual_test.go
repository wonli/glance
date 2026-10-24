package desktopui

import (
	"bytes"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"glance/internal/document"
	"glance/internal/proofreading"
	"image"
	"image/color"
	"runtime"
	"testing"
	"time"
)

func TestProofreadingReviewStatusDecoratesOnlyUnselectedXPaper(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a, s := proofreadingModelFixture(t)
		size := image.Pt(int(500*scale), int(700*scale))
		var ops op.Ops
		g := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
		view := &pdfViewState{info: document.PDFInfo{Pages: 3}, scale: float64(scale)}
		render := func() *image.RGBA {
			ops.Reset()
			paint.Fill(&ops, color.NRGBA{R: 243, G: 245, B: 248, A: 255})
			a.layoutProofreadingProjectionPage(g, view, 0, &s.xPages[0], "", false)
			return renderPresentationTestOps(t, &ops, size)
		}
		before := render()
		s.xPageAdjustmentActive = true
		s.xPageAdjustmentPage = 0
		selectedBefore := render()
		s.xPageAdjustmentActive = false
		s.controller.Model.Review(1, true)
		a.refreshProofreadingModelView()
		for _, status := range []string{"reviewed", "saving", "failed", "saved", "modified"} {
			s.history.pending = status == "saving"
			s.history.pendingPage = 0
			s.history.err = ""
			if status == "failed" {
				s.history.err = "synthetic failure"
				s.history.errorPage = 0
			}
			if status == "saved" {
				s.history.confirmed = map[int]string{0: "hash"}
				s.history.snapshots = map[int]string{0: "hash"}
				s.history.tokens = map[int]proofreading.ContentToken{0: s.controller.Model.ContentToken(1)}
			}
			if status == "modified" {
				s.history.tokens[0] = proofreading.ContentToken{}
			}
			after := render()
			want := color.RGBA{R: 111, G: 160, B: 136, A: 255}
			if status == "saving" || status == "modified" {
				want = color.RGBA{R: 216, G: 162, B: 69, A: 255}
			} else if status == "failed" {
				want = color.RGBA{R: 198, G: 83, B: 72, A: 255}
			}
			foundBorder := false
			for y := int(30 * scale); y < size.Y && !foundBorder; y++ {
				for x := 0; x < size.X; x++ {
					pixel := after.RGBAAt(x, y)
					// Subpixel strokes blend with the paper; inspect the
					// visible hue rather than requiring an opaque RGB value.
					r, g, b := int(pixel.R), int(pixel.G), int(pixel.B)
					matches := g > r+10 && g > b+3
					if status == "saving" || status == "modified" {
						matches = r > g+8 && g > b+15
					} else if status == "failed" {
						matches = r > g+20 && proofreadingTestAbs(g-b) < 25
					}
					if matches {
						foundBorder = true
						break
					}
				}
			}
			if !foundBorder {
				t.Fatalf("%s missing expected border color %v at %gx", status+"/"+a.proofreadingPageStatusKey(0), want, scale)
			}
			if bytes.Equal(before.Pix, after.Pix) {
				t.Fatalf("%s missing review border on X paper at %gx", status, scale)
			}
			s.xPageAdjustmentActive = true
			selectedAfter := render()
			if !bytes.Equal(selectedBefore.Pix, selectedAfter.Pix) {
				t.Fatalf("%s changed selected X paper at %gx", status, scale)
			}
			s.xPageAdjustmentActive = false
		}
	}
}

func proofreadingTestAbs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
