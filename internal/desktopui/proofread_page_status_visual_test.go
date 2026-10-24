package desktopui

import (
	"bytes"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"glance/internal/document"
	"image"
	"image/color"
	"runtime"
	"testing"
	"time"
)

func TestProofreadingReviewStatusDoesNotDecorateXPaper(t *testing.T) {
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
		s.controller.Model.Review(1, true)
		a.refreshProofreadingModelView()
		for _, status := range []string{"reviewed", "saving", "failed", "saved"} {
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
			}
			after := render()
			if !bytes.Equal(before.Pix, after.Pix) {
				t.Fatalf("%s added review decoration to X paper at %gx", status, scale)
			}
		}
	}
}
