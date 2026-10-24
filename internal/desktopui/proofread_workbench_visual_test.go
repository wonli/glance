package desktopui

import (
	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"glance/internal/workspace"
	"image"
	"image/color"
	"image/png"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestProofreadingWorkbenchAndReviewedMarkersVisual(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a, s := proofreadingModelFixture(t)
		a.proofreadingModule = true
		var ops op.Ops
		var router input.Router
		size := image.Pt(int(96*scale), int(96*scale))
		view := &pdfViewState{page: 1, buttons: map[string]*widget.Clickable{}}
		gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
		paint.Fill(&ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		a.layoutProofreadingPageThumbnail(gtx, view, 0)
		unreviewed := renderPresentationTestOps(t, &ops, size)
		s.controller.Model.Review(1, true)
		a.refreshProofreadingModelView()
		ops.Reset()
		paint.Fill(&ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		a.layoutProofreadingPageThumbnail(gtx, view, 0)
		reviewed := renderPresentationTestOps(t, &ops, size)
		changed := 0
		green := 0
		for y := 0; y < size.Y; y++ {
			for x := 0; x < size.X; x++ {
				p := reviewed.RGBAAt(x, y)
				if p != unreviewed.RGBAAt(x, y) {
					changed++
				}
				if int(p.G) > int(p.R)+20 && p.G > p.B {
					green++
				}
			}
		}
		if changed < 50 || green < 15 {
			t.Fatalf("reviewed marker missing at %gx: changed=%d green=%d", scale, changed, green)
		}
		w := a.ensureProofreadingWorkbench()
		w.loaded = true
		w.projects = []workspace.ProofreadingProject{{ID: "test", Title: "校对测试书稿", PageCount: 500, CurrentPage: 6, ReviewedCount: 3, UpdatedAt: time.Date(2026, 10, 24, 14, 32, 0, 0, time.UTC)}}
		size = image.Pt(int(1440*scale), int(900*scale))
		gtx.Constraints = layout.Exact(size)
		ops.Reset()
		a.activeTabID = "recent"
		a.layoutDocument(gtx)
		shot := renderPresentationTestOps(t, &ops, size)
		ink := 0
		for y := int(120 * scale); y < int(280*scale); y++ {
			for x := int(226 * scale); x < int(900*scale); x++ {
				p := shot.RGBAAt(x, y)
				if p.R < 130 && p.G < 130 && p.B < 130 {
					ink++
				}
			}
		}
		if ink < 100 {
			t.Fatalf("project row content not visible: %d", ink)
		}
		if dir := os.Getenv("GLANCE_WORKBENCH_CAPTURE"); dir != "" && scale == 1 {
			f, err := os.Create(dir + "/workbench.png")
			if err != nil {
				t.Fatal(err)
			}
			png.Encode(f, shot)
			f.Close()
		}
		w.settings = true
		w.name.SetText("校对者")
		w.email.SetText("reader@example.com")
		ops.Reset()
		a.layoutProofreadingWorkbench(gtx)
		shot = renderPresentationTestOps(t, &ops, size)
		minX, minY, maxX, maxY := size.X, size.Y, 0, 0
		for y := int(630 * scale); y < int(660*scale); y++ {
			for x := int(250 * scale); x < int(279*scale); x++ {
				p := shot.RGBAAt(x, y)
				if p.B > p.R+20 && p.R < 150 {
					minX = min(minX, x)
					minY = min(minY, y)
					maxX = max(maxX, x)
					maxY = max(maxY, y)
				}
			}
		}
		if maxX-minX < int(10*scale) || maxY-minY < int(10*scale) {
			t.Fatalf("SSH icon has missing or collapsed visible pixels at %gx: (%d,%d)-(%d,%d)", scale, minX, minY, maxX, maxY)
		}

		if dir := os.Getenv("GLANCE_WORKBENCH_CAPTURE"); dir != "" && scale == 1 {
			f, err := os.Create(dir + "/identity.png")
			if err != nil {
				t.Fatal(err)
			}
			png.Encode(f, shot)
			f.Close()
		}
	}
}
