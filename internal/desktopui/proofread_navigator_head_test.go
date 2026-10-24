package desktopui

import (
	"context"
	"image"
	"reflect"
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

func TestProofreadingNavigatorTopMovesPrefixToPreviousAndKeepsTop(t *testing.T) {
	for _, long := range []bool{false, true} {
		for _, scale := range []float32{1, 2} {
			for _, reviewed := range []bool{false, true} {
				a, s := proofreadingModelFixture(t)
				current := "甲e\u0301👩‍💻短正文"
				if long {
					current += strings.Repeat("正文保持完整 mixed text\n", 150)
				}
				m, err := proofreading.New("前盒"+current+"后盒", []proofreading.PageID{1, 2, 3})
				if err != nil {
					t.Fatal(err)
				}
				if err = m.AssignInitial([]int{2, 2 + len([]rune(current)), 4 + len([]rune(current))}); err != nil {
					t.Fatal(err)
				}
				if reviewed {
					if err = m.Review(1, true); err != nil {
						t.Fatal(err)
					}
				}
				s.controller = application.NewProofreadingController(m)
				a.refreshProofreadingModelView()
				shaped, err := application.LayoutProofreadingOwnedPages(context.Background(), m.Snapshot(), []application.ProofreadingPageGeometry{{Page: 1, Width: 340, Height: 544}, {Page: 2, Width: 340, Height: 544}, {Page: 3, Width: 340, Height: 544}}, application.ProofreadingTextStyle{Size: 12, Family: string(a.theme.Face)}, application.ProofreadingLayoutSnapshot{}, []proofreading.PageID{1, 2, 3}, newProofreadingEditorLineShaper())
				if err != nil {
					t.Fatal(err)
				}
				s.controller.AcceptLayout(shaped)
				beforeIDs := append(append(m.IDs(1), m.IDs(2)...), m.IDs(3)...)
				var ops op.Ops
				var router input.Router
				view := &pdfViewState{info: document.PDFInfo{Pages: 3}, scale: float64(scale)}
				view.canvas.Axis = layout.Vertical
				view.canvas.Position.First = 1
				frame := func() {
					ops.Reset()
					gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(int(600*scale), int(700*scale)))}
					a.processProofreadingEditorEvents(gtx)
					a.layoutProofreadingXNavigatorCanvas(gtx, view, 3, "", false)
					router.Frame(&ops)
				}
				scroll := func(delta float32) {
					router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(575*scale, 200*scale), Scroll: f32.Pt(0, delta*scale)})
					frame()
					frame()
				}
				frame()
				if !reviewed {
					scroll(1)
					if m.Text(1) != "前" || m.Text(2) != "盒"+current || s.xPageScrollOffsets[1] != 0 {
						t.Fatal("initial down at top did not pull previous suffix", m.Text(1), s.xPageScrollOffsets[1])
					}
					scroll(-1)
					if m.Text(1) != "前盒" || m.Text(2) != current {
						t.Fatal("up at top did not restore previous suffix")
					}
				}
				scroll(-1)
				if reviewed {
					if m.Text(1) != "前盒" || m.Text(2) != current || s.xPageScrollOffsets[1] != 0 {
						t.Fatal("reviewed previous box changed or view jumped", m.Text(1), s.xPageScrollOffsets[1])
					}
					continue
				}
				if m.Text(1) != "前盒甲" || m.Text(2) != strings.TrimPrefix(current, "甲") || m.Text(3) != "后盒" {
					t.Fatal("top wheel changed the wrong boundary", m.Text(1), m.Text(2)[:min(40, len(m.Text(2)))], m.Text(3))
				}
				if s.xPageScrollOffsets[1] != 0 || s.xEditorWindows[1].Start != 0 || !strings.HasPrefix(s.xPageEditors[1].Text(), "e\u0301") {
					t.Fatal("top adjustment jumped to bottom", s.xPageScrollOffsets[1], s.xEditorWindows[1])
				}
				if !reflect.DeepEqual(beforeIDs, append(append(m.IDs(1), m.IDs(2)...), m.IDs(3)...)) {
					t.Fatal("character IDs changed")
				}
				scroll(1)
				if m.Text(1) != "前盒" || m.Text(2) != current || s.xPageScrollOffsets[1] != 0 {
					t.Fatal("reverse top adjustment did not restore prefix", m.Text(1), s.xPageScrollOffsets[1])
				}
				a.undoProofreadingPagination()
				if m.Text(1) != "前盒甲" || m.Text(2) != strings.TrimPrefix(current, "甲") {
					t.Fatal("top adjustment undo")
				}
			}
		}
	}
}
