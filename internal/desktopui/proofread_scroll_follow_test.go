package desktopui

import (
	"context"
	"fmt"
	"image"
	"runtime"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/proofreading"
)

func TestProofreadingPageWheelShowsNumberedLinesWhileNeighborPending(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a, s := proofreadingModelFixture(t)
		var text strings.Builder
		for i := 2342; i < 2642; i++ {
			fmt.Fprintf(&text, "%d 中文校对 line\n", i)
		}
		if err := s.controller.Model.ReplacePage(1, text.String()); err != nil {
			t.Fatal(err)
		}
		a.refreshProofreadingModelView()
		shaped, err := application.LayoutProofreadingOwnedPages(context.Background(), s.controller.Model.Snapshot(), []application.ProofreadingPageGeometry{{Page: 1, Width: 340, Height: 544}, {Page: 2, Width: 340, Height: 544}, {Page: 3, Width: 340, Height: 544}}, application.ProofreadingTextStyle{Size: 12, Family: string(a.theme.Face)}, application.ProofreadingLayoutSnapshot{}, []proofreading.PageID{1, 2, 3}, newProofreadingEditorLineShaper(a.currentFontSnapshot()))
		if err != nil {
			t.Fatal(err)
		}
		s.controller.AcceptLayout(shaped)
		var ops op.Ops
		var router input.Router
		view := &pdfViewState{info: document.PDFInfo{Pages: 3}, scale: float64(scale)}
		view.canvas.Axis = layout.Vertical
		s.xCanvas.Axis = layout.Vertical
		frame := func() {
			ops.Reset()
			gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(int(500*scale), int(700*scale)))}
			a.processProofreadingEditorEvents(gtx)
			a.layoutProofreadingXPageList(gtx, view, 3, "", false)
			router.Frame(&ops)
		}
		frame()
		if !strings.HasPrefix(s.xPageEditors[0].Text(), "2342") {
			t.Fatal("wrong initial line")
		}
		beforePixels := renderPresentationTestOps(t, &ops, image.Pt(int(500*scale), int(700*scale)))
		if err := s.controller.Model.ReplacePage(2, "邻页更新，排版尚未回来"); err != nil {
			t.Fatal(err)
		}
		a.proofreadingBoundaryChanged(1)
		before := s.controller.Model.Text(1)
		router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(100*scale, 100*scale), Scroll: f32.Pt(0, 400*scale)})
		frame()
		frame()
		if s.xPageScrollOffsets[0] <= 0 || s.xEditorWindows[0].Start == 0 || strings.HasPrefix(s.xPageEditors[0].Text(), "2342") {
			t.Fatalf("%gx: wheel/scrollbar advanced without advancing numbered text: top=%v window=%+v text=%q", scale, s.xPageScrollOffsets[0], s.xEditorWindows[0], s.xPageEditors[0].Text()[:min(60, len(s.xPageEditors[0].Text()))])
		}
		if s.controller.Model.Text(1) != before {
			t.Fatal("view wheel edited content")
		}
		afterPixels := renderPresentationTestOps(t, &ops, image.Pt(int(500*scale), int(700*scale)))
		changedPixels := 0
		for y := int(50 * scale); y < int(220*scale); y++ {
			for x := int(90 * scale); x < int(400*scale); x++ {
				if beforePixels.RGBAAt(x, y) != afterPixels.RGBAAt(x, y) {
					changedPixels++
				}
			}
		}
		if changedPixels < 40 {
			t.Fatal("scrollbar moved but actual page pixels stayed unchanged", changedPixels)
		}

	}
}

func TestProofreadingNavigatorBurstDoesNotDripFourCharactersPerFrame(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	if err := s.controller.Model.ReplacePage(2, strings.Repeat("豆", 500)); err != nil {
		t.Fatal(err)
	}
	a.refreshProofreadingModelView()
	s.navigator = proofreadingNavigator{initialized: true, page: 0, pending: []proofreadingNavigatorIntent{{steps: 64}}}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(600, 700))}
	a.processProofreadingNavigatorEvents(gtx)
	if s.controller.Model.RuneCount(1) != 68 || len(s.navigator.pending) != 0 {
		t.Fatal("burst was throttled or compressed input was lost", s.controller.Model.RuneCount(1), s.navigator.pending)
	}
	for range 64 {
		if !s.controller.Model.Undo() {
			t.Fatal("per-character history lost")
		}
	}
	if s.controller.Model.Text(1) != "abcd" {
		t.Fatal("burst undo")
	}
}

func TestProofreadingRefillPrioritizesVisibleGeometry(t *testing.T) {
	a, s, _ := largeProofreadingFixture(t)
	s.visibleEditorPages = []int{0}
	s.navigator = proofreadingNavigator{initialized: true, page: 0}
	count := s.controller.Model.RuneCount(2)
	if err := s.controller.Model.AdjustBoundary(1, count); err != nil {
		t.Fatal(err)
	}
	a.proofreadingBoundaryChanged(0)
	if len(s.modelLayoutActivePages) > 2 {
		t.Fatal("refill blocks visible response on offscreen book layout", len(s.modelLayoutActivePages))
	}
	if !s.modelDirtyPages[3] {
		t.Fatal("offscreen invalidation was lost")
	}
	s.modelLayoutResults = make(chan proofreadingModelLayoutResult, 1)
	s.modelLayoutResults <- proofreadingModelLayoutResult{generation: s.modelLayoutGeneration, layout: application.ProofreadingLayoutSnapshot{Revision: s.controller.Model.Revision(), StyleRevision: s.xTextSizeStep, Pages: map[proofreading.PageID]application.ProofreadingPageLayout{}}}
	a.acceptProofreadingModelLayout()
	if !s.modelDirtyPages[3] || s.modelDirtyPages[1] || s.modelDirtyPages[2] {
		t.Fatal("partial result incorrectly cleared deferred dirty pages", s.modelDirtyPages)
	}
	s.visibleEditorPages = []int{2}
	s.modelRequestedPages = []proofreading.PageID{3}
	a.processProofreadingLayoutRequests()
	found := false
	for _, id := range s.modelLayoutActivePages {
		if id == 3 {
			found = true
		}
	}
	if !found {
		t.Fatal("deferred page never scheduled on visibility", s.modelLayoutActivePages)
	}
}

func TestProofreadingRefillDoesNotScheduleEmptyOffscreenSuffix(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	if err := s.controller.Model.ReplacePage(3, ""); err != nil {
		t.Fatal(err)
	}
	a.refreshProofreadingModelView()
	s.modelDirtyPages = nil
	s.visibleEditorPages = []int{0}
	s.navigator = proofreadingNavigator{initialized: true, page: 0}
	if err := s.controller.Model.AdjustBoundary(1, 4); err != nil {
		t.Fatal(err)
	}
	a.proofreadingBoundaryChanged(0)
	if len(s.modelLayoutActivePages) != 2 {
		t.Fatal("empty offscreen suffix delayed current box", s.modelLayoutActivePages)
	}
}
