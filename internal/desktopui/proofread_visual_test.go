package desktopui

import (
	"context"
	"fmt"
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"glance/internal/application"
	"glance/internal/document"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestProofreadingNavigatorVisual(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a, s := proofreadingModelFixture(t)
		view := &pdfViewState{info: document.PDFInfo{Pages: 3}, scale: float64(scale)}
		size := image.Pt(int(600*scale), int(700*scale))
		var ops op.Ops
		var router input.Router
		frame := func() {
			ops.Reset()
			paint.Fill(&ops, proofreadXCanvas)
			gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
			a.layoutProofreadingXNavigatorCanvas(gtx, view, 3, "", false)
			router.Frame(&ops)
		}
		frame()
		router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(575*scale, 200*scale), Scroll: f32.Pt(0, 1*scale)})
		frame()
		frame()
		if s.controller.Model.Text(1) != "abcde" {
			t.Fatal("visual fixture did not adjust")
		}
		result, err := reflowProofreadingForEditor(context.Background(), s.controller.Model.Snapshot(), []application.ProofreadingPageGeometry{{Page: 1, Width: 340, Height: 544}, {Page: 2, Width: 340, Height: 544}, {Page: 3, Width: 340, Height: 544}}, application.ProofreadingTextStyle{Size: 12, Family: string(a.theme.Face)}, false)
		if err != nil {
			t.Fatal(err)
		}
		s.controller.AcceptLayout(result)
		frame()
		if s.xPageEditors[0].Text() != "abcde" {
			t.Fatal("adjusted character absent from editor")
		}
		shot := renderPresentationTestOps(t, &ops, size)
		ink, blue := 0, 0
		for y := 0; y < size.Y; y++ {
			for x := int(546 * scale); x < size.X; x++ {
				p := shot.RGBAAt(x, y)
				if p.R < 160 {
					ink++
				}
				if int(p.B) > int(p.R)+35 && int(p.G) > int(p.R)+10 {
					blue++
				}
			}
		}
		if ink < 100 || blue < 40 {
			t.Fatalf("navigator labels/ownership marker missing at %gx: ink=%d blue=%d", scale, ink, blue)
		}
		if dir := os.Getenv("GLANCE_PROOFREAD_CAPTURE"); dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			f, err := os.Create(filepath.Join(dir, fmt.Sprintf("navigator-%gx.png", scale)))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, shot)
			_ = f.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}

// Identical lines must retain their pixel positions after changing the bounded
// native-editor window; a second layout engine or private editor scroll makes
// this crop jump or turn blank despite correct model ownership.
func TestProofreadingPageScrollKeepsTextGeometryVisual(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a, s := proofreadingModelFixture(t)
		value := strings.Repeat("中文校对 mixed English，文字位置应稳定。\n", 150)
		if err := s.controller.Model.ReplacePage(1, value); err != nil {
			t.Fatal(err)
		}
		if err := s.controller.Model.Lock(1, true); err != nil {
			t.Fatal(err)
		}
		a.refreshProofreadingModelView()
		result, err := reflowProofreadingForEditor(context.Background(), s.controller.Model.Snapshot(), []application.ProofreadingPageGeometry{{Page: 1, Width: 340, Height: 544}, {Page: 2, Width: 340, Height: 544}, {Page: 3, Width: 340, Height: 544}}, application.ProofreadingTextStyle{Size: 12, Family: string(a.theme.Face)}, false)
		if err != nil {
			t.Fatal(err)
		}
		s.controller.AcceptLayout(result)
		size := image.Pt(int(400*scale), int(600*scale))
		render := func(top float64) *image.RGBA {
			s.xPageScrollOffsets[0] = top
			a.syncProofreadingEditors()
			var ops op.Ops
			paint.Fill(&ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
			a.drawProofreadingXContent(gtx, s.xPages[0], size)
			return renderPresentationTestOps(t, &ops, size)
		}
		before := render(0)
		after := render(result.Pages[1].Lines[50].Top)
		ink := 0
		for y := int(28 * scale); y < int(220*scale); y++ {
			for x := int(30 * scale); x < int(370*scale); x++ {
				b, n := before.RGBAAt(x, y), after.RGBAAt(x, y)
				if b.R < 150 {
					ink++
				}
				// Metal may differ by one channel level between separate targets.
				if math.Abs(float64(b.R)-float64(n.R)) > 2 || math.Abs(float64(b.G)-float64(n.G)) > 2 || math.Abs(float64(b.B)-float64(n.B)) > 2 || b.A != n.A {
					t.Fatalf("%gx: page text jumped after scrolling at (%d,%d): before=%v after=%v", scale, x, y, b, n)
				}
			}
		}
		if ink < 100 {
			t.Fatal("no visible text in scroll comparison")
		}
		if dir := os.Getenv("GLANCE_PROOFREAD_CAPTURE"); dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			f, err := os.Create(filepath.Join(dir, fmt.Sprintf("scrolled-%gx.png", scale)))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, after)
			_ = f.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestProofreadingLockedPageVisual(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a, s := proofreadingModelFixture(t)
		_ = s.controller.Model.Lock(1, true)
		a.refreshProofreadingModelView()
		a.syncProofreadingEditors()
		size := image.Pt(int(500*scale), int(700*scale))
		var ops op.Ops
		paint.Fill(&ops, color.NRGBA{R: 243, G: 245, B: 248, A: 255})
		gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
		view := &pdfViewState{info: document.PDFInfo{Pages: 3}, scale: float64(scale)}
		a.layoutProofreadingProjectionPage(gtx, view, 0, &s.xPages[0], "", false)
		shot := renderPresentationTestOps(t, &ops, size)
		blue := func(p color.RGBA) bool { return int(p.B) > int(p.R)+35 && int(p.G) > int(p.R)+10 }
		edges := []image.Point{{int(50 * scale), int(150 * scale)}, {int(449 * scale), int(150 * scale)}, {int(200 * scale), int(50 * scale)}, {int(200 * scale), int(649 * scale)}}
		for _, edge := range edges {
			found := false
			for dy := -3; dy <= 3; dy++ {
				for dx := -3; dx <= 3; dx++ {
					if blue(shot.RGBAAt(edge.X+dx, edge.Y+dy)) {
						found = true
					}
				}
			}
			if !found {
				t.Errorf("missing whole-page lock border around %v at %gx", edge, scale)
			}
		}
		if dir := os.Getenv("GLANCE_PROOFREAD_CAPTURE"); dir != "" {
			_ = os.MkdirAll(dir, 0o755)
			name := "locked-1x.png"
			if scale == 2 {
				name = "locked-2x.png"
			}
			f, err := os.Create(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, shot)
			_ = f.Close()
			if err != nil {
				t.Fatal(err)
			}
			_ = os.WriteFile(filepath.Join(dir, "source.pdf"), pdfTestBytes(t), 0o600)
			_ = os.WriteFile(filepath.Join(dir, "copy.txt"), []byte("Synthetic proofreading content.\n"+strings.Repeat("Line of text for page comparison and manual pagination.\n", 160)), 0o600)
		}
	}
}

func TestProofreadingLongPageViewportGPU(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a, s := proofreadingModelFixture(t)
	long := strings.Repeat("长文本不应整篇提交给显卡；完整内容留在校对模型中。\n", 2500)
	if err := s.controller.Model.ReplacePage(1, long); err != nil {
		t.Fatal(err)
	}
	_ = s.controller.Model.Lock(1, true)
	a.refreshProofreadingModelView()
	a.syncProofreadingEditors()
	size := image.Pt(500, 700)
	view := &pdfViewState{info: document.PDFInfo{Pages: 3}, scale: 1}
	for frame := 0; frame < 8; frame++ {
		var ops op.Ops
		paint.Fill(&ops, color.NRGBA{R: 243, G: 245, B: 248, A: 255})
		gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(size)}
		a.layoutProofreadingProjectionPage(gtx, view, 0, &s.xPages[0], "", false)
		_ = renderPresentationTestOps(t, &ops, size)
	}
	if s.controller.Model.Text(1) != long {
		t.Fatal("viewport rendering changed model content")
	}
	if len([]rune(s.xPageEditors[0].Text())) > 4096 {
		t.Fatal("unbounded glyph input")
	}
}
