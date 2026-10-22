package desktopui

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/proofreading"
	"image"
	"strings"
	"testing"
	"time"
)

func largeProofreadingFixture(t testing.TB) (*App, *proofreadingWorkspace, *pdfViewState) {
	t.Helper()
	const pages = 892
	value := strings.Repeat("中文校对正文 mixed English。\n", 100)
	count := len([]rune(value))
	snap := proofreading.Snapshot{Version: 2, Next: 1}
	frames := make([]proofreadingPageFrame, pages)
	for i := range pages {
		id := proofreading.PageID(i + 1)
		snap.Pages = append(snap.Pages, proofreading.Page{ID: id})
		snap.Runs = append(snap.Runs, proofreading.Run{First: snap.Next, Page: id, Text: value})
		snap.Next += proofreading.CharacterID(count)
		frames[i] = proofreadingPageFrame{PageIndex: i, Width: 400, Height: 600}
	}
	m, err := proofreading.Restore(snap)
	if err != nil {
		t.Fatal(err)
	}
	a := New(nil, "")
	a.managedBackgroundClosed = true
	s := &proofreadingWorkspace{controller: application.NewProofreadingController(m), modelInitialAssigned: true, bDocument: &proofreadingObject{}, bDraftInitialized: true, pageFrames: frames}
	s.controller.Layout = application.ProofreadingLayoutSnapshot{Revision: m.Snapshot().Revision, Pages: map[proofreading.PageID]application.ProofreadingPageLayout{}}
	a.proofreading = s
	a.refreshProofreadingModelView()
	view := &pdfViewState{info: document.PDFInfo{Pages: pages}, scale: 1}
	view.canvas.Axis = layout.Vertical
	return a, s, view
}

func TestProofreadingLargeDocumentScrollKeepsOffscreenEditorsCold(t *testing.T) {
	a, s, view := largeProofreadingFixture(t)
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(600, 700))}
	a.layoutProofreadingXNavigatorCanvas(gtx, view, 892, "", false)
	if s.xPageEditors[891].Len() != 0 || s.xEditorWindows[891].Initialized {
		t.Fatal("scroll initialized an offscreen editor in the 892-page book")
	}
	if s.xPageEditors[0].Len() == 0 {
		t.Fatal("visible page was not synchronized")
	}
}

func BenchmarkProofreadingLargeDocumentScrollFrame(b *testing.B) {
	a, _, view := largeProofreadingFixture(b)
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(600, 700))}
		a.processProofreadingEditorEvents(gtx)
		a.layoutProofreadingXNavigatorCanvas(gtx, view, 892, "", false)
	}
	frame()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		frame()
	}
}

func BenchmarkProofreadingLargeDocumentFineStep(b *testing.B) {
	_, s, _ := largeProofreadingFixture(b)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := s.controller.AdjustPageUnits(4, 1, false); err != nil {
			b.Fatal(err)
		}
	}
}
