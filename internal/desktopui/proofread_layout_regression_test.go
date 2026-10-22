package desktopui

import (
	"context"
	"image"
	"math"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/proofreading"
)

func TestProofreadingPageListCentersPaperAndMatchesWheelHitBounds(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	s.xCanvas.Axis = layout.Vertical
	var ops op.Ops
	var router input.Router
	view := &pdfViewState{info: document.PDFInfo{Pages: 3}, scale: 1}
	gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1000, 700))}
	gtx.Constraints.Min.X = 0 // The pane may offer width without a cross-axis minimum.
	a.layoutProofreadingXPageList(gtx, view, 3, "", false)
	router.Frame(&ops)
	var bounds []image.Rectangle
	var visit func([]input.SemanticNode)
	visit = func(nodes []input.SemanticNode) {
		for _, n := range nodes {
			if n.Desc.Description == a.textID("proofread.x_page.click_for_break") {
				bounds = append(bounds, n.Desc.Bounds)
			}
			visit(n.Children)
		}
	}
	visit(router.AppendSemantics(nil))
	if len(bounds) == 0 {
		t.Fatal("paper hit region not registered")
	}
	want := image.Rect(300, 12, 700, 612)
	if bounds[0] != want {
		t.Fatalf("paper drawn/hit at %v, want centered %v", bounds[0], want)
	}
	i, hit := proofreadingXPageAt(gtx, view, s, image.Pt(650, 100))
	if i != 0 || hit != want {
		t.Fatalf("wheel hit geometry differs from paper: index=%d bounds=%v", i, hit)
	}
}

func TestProofreadingScaledScrolledEditorAcceptsChineseAtVisibleLine(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		a, s := proofreadingModelFixture(t)
		line := "中文校对 mixed English，文字位置应稳定。\n"
		value := strings.Repeat(line, 150)
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
		s.xPageScrollOffsets[0] = result.Pages[1].Lines[50].Top
		var router input.Router
		var ops op.Ops
		size := image.Pt(int(400*scale), int(600*scale))
		frame := func() {
			ops.Reset()
			gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
			a.processProofreadingEditorEvents(gtx)
			a.syncProofreadingEditors()
			a.drawProofreadingXContent(gtx, s.xPages[0], size)
			router.Frame(&ops)
		}
		frame()
		point := f32.Pt(35*scale, 41*scale)
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Position: point, Buttons: pointer.ButtonPrimary})
		frame()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point})
		frame()
		if !router.Source().Focused(&s.xPageEditors[0]) {
			t.Fatalf("%gx: cannot focus scaled scrolled text", scale)
		}
		start, end := s.xPageEditors[0].Selection()
		router.Queue(key.EditEvent{Range: key.Range{Start: start, End: end}, Text: "新增汉字"})
		frame()
		got := s.controller.Model.Text(1)
		at := strings.Index(got, "新增汉字")
		if at < len(line)*50 || at >= len(line)*51 {
			t.Fatalf("%gx: click edited wrong line at byte %d, want line 50", scale, at)
		}
		if strings.Replace(got, "新增汉字", "", 1) != value {
			t.Fatalf("%gx: edit lost or duplicated owned text", scale)
		}
	}
}

func TestProofreadingLayoutMatchesVisibleEditorLines(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	value := strings.Repeat("中文校对 mixed English words，空格和换行需要一致。 ", 8) + "\n\n末尾"
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
	a.syncProofreadingEditors()
	for _, scale := range []float32{1, 2, 1.25} {
		var ops op.Ops
		size := image.Pt(int(400*scale), int(600*scale))
		gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
		a.drawProofreadingXContent(gtx, s.xPages[0], size)
		editor := &s.xPageEditors[0]
		editor.SetCaret(0, 0)
		base := editor.CaretCoords().Y
		for i, line := range result.Pages[1].Lines {
			if line.Start >= len([]rune(value)) {
				break
			}
			editor.SetCaret(line.Start, line.Start)
			row, _ := editor.CaretPos()
			y := editor.CaretCoords().Y - base
			if row != i || math.Abs(float64(y)-line.Top) > 1 {
				t.Errorf("%gx: layout line %d at rune %d y=%.2f, editor line=%d y=%.2f", scale, i, line.Start, line.Top, row, y)
			}
		}
	}
}

func TestProofreadingLongOwnedPageLayoutIncludesTail(t *testing.T) {
	a, s := proofreadingModelFixture(t)
	value := strings.Repeat("一行内容\n", 2000) + "TAIL"
	if err := s.controller.Model.ReplacePage(1, value); err != nil {
		t.Fatal(err)
	}
	if err := s.controller.Model.Review(1, true); err != nil {
		t.Fatal(err)
	}
	a.refreshProofreadingModelView()
	result, err := reflowProofreadingForEditor(context.Background(), s.controller.Model.Snapshot(), []application.ProofreadingPageGeometry{{Page: 1, Width: 340, Height: 544}, {Page: 2, Width: 340, Height: 544}, {Page: 3, Width: 340, Height: 544}}, application.ProofreadingTextStyle{Size: 12}, false)
	if err != nil {
		t.Fatal(err)
	}
	s.controller.AcceptLayout(result)
	s.xPageScrollOffsets[0] = 1e9
	a.syncProofreadingEditors()
	if !strings.HasSuffix(s.xPageEditors[0].Text(), "TAIL") {
		t.Fatalf("scrolling to bottom cannot reach owned tail: layout runes=%d owned=%d editor=%q", len([]rune(result.Pages[proofreading.PageID(1)].Text)), len([]rune(value)), s.xPageEditors[0].Text())
	}
}
