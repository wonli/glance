package desktopui

import (
	"image"
	"reflect"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"glance/internal/document"
	"glance/internal/ocr"
	"glance/internal/textalign"
)

func TestProofreadingOCRBlockChoiceUsesOnlyPathChoices(t *testing.T) {
	page := proofreadingOCRPage{result: textalign.Result{
		Status:     "ambiguous",
		Candidates: []textalign.Candidate{{Evidence: []textalign.Evidence{{BlockID: 9, Range: textalign.Range{Start: 30, End: 35}}}}},
		PathChoices: []textalign.Candidate{
			{Evidence: []textalign.Evidence{
				{BlockID: 4, Range: textalign.Range{Start: 2, End: 8}},
				{BlockID: 9, Range: textalign.Range{Start: 20, End: 25}},
			}},
			{Evidence: []textalign.Evidence{{BlockID: 4, Range: textalign.Range{Start: 12, End: 18}}}},
			{Evidence: []textalign.Evidence{{BlockID: 7, Range: textalign.Range{Start: 40, End: 45}}}},
		},
	}}
	got := proofreadingOCRBlockChoice(page, 4)
	want := []proofreadingOCRBlockRange{
		{PathChoiceIndex: 0, Range: textalign.Range{Start: 2, End: 8}},
		{PathChoiceIndex: 1, Range: textalign.Range{Start: 12, End: 18}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("block choices = %+v, want %+v", got, want)
	}
}

func TestProofreadingOCRBlockChoiceRejectsUnmatchedOrLimitedPage(t *testing.T) {
	for _, status := range []string{"unmatched", "limited"} {
		t.Run(status, func(t *testing.T) {
			page := proofreadingOCRPage{result: textalign.Result{
				Status:      status,
				PathChoices: []textalign.Candidate{{Evidence: []textalign.Evidence{{BlockID: 4, Range: textalign.Range{Start: 1, End: 3}}}}},
			}}
			if got := proofreadingOCRBlockChoice(page, 4); len(got) != 0 {
				t.Fatalf("%s page exposed link choices: %+v", status, got)
			}
		})
	}
}

func TestProofreadingOCRChoiceIndexWraps(t *testing.T) {
	for _, test := range []struct {
		name             string
		at, delta, count int
		want             int
	}{
		{name: "next from selected", at: 0, delta: 1, count: 2, want: 1},
		{name: "next wraps", at: 1, delta: 1, count: 2, want: 0},
		{name: "previous wraps", at: 0, delta: -1, count: 2, want: 1},
		{name: "large step", at: 1, delta: 5, count: 3, want: 0},
		{name: "empty choices", at: 0, delta: 1, count: 0, want: -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := proofreadingOCRChoiceIndex(test.at, test.delta, test.count); got != test.want {
				t.Fatalf("choice index = %d, want %d", got, test.want)
			}
		})
	}
}

func TestProofreadingOCRLinkChoiceCyclesOnlyGlobalPaths(t *testing.T) {
	a, state, _ := proofreadingOCRLinkFixture(t)
	state.ocrScan.pages[0].result.Status = "ambiguous"
	state.ocrScan.pages[0].result.Candidates = []textalign.Candidate{{Evidence: []textalign.Evidence{{BlockID: 4, Range: textalign.Range{Start: 9, End: 12}}}}}
	state.ocrScan.pages[0].result.PathChoices = []textalign.Candidate{
		{Range: textalign.Range{Start: 5, End: 11}, Evidence: []textalign.Evidence{{BlockID: 4, Range: textalign.Range{Start: 5, End: 11}}}},
		{Range: textalign.Range{Start: 1, End: 3}, Evidence: []textalign.Evidence{{BlockID: 8, Range: textalign.Range{Start: 1, End: 3}}}},
		{Range: textalign.Range{Start: 9, End: 12}, Evidence: []textalign.Evidence{{BlockID: 4, Range: textalign.Range{Start: 9, End: 12}}}},
	}
	if !a.activateProofreadingOCRBlock(0, 4) {
		t.Fatal("ambiguous OCR block did not open its supported B choices")
	}
	if state.ocrScan.link.BRange != (textalign.Range{Start: 5, End: 11}) {
		t.Fatalf("initial path range = %+v, want first globally supported choice", state.ocrScan.link.BRange)
	}
	if !a.moveProofreadingOCRLinkChoice(1) {
		t.Fatal("next supported path choice was not selectable")
	}
	if state.ocrScan.link.PathChoiceIndex != 2 || state.ocrScan.link.BRange != (textalign.Range{Start: 9, End: 12}) {
		t.Fatalf("cycled link = %+v; local decoy or another block leaked into choices", state.ocrScan.link)
	}
	if !a.proofreadingOCRLinkValid() {
		t.Fatal("selected supported alternative was not current")
	}
}

func TestProofreadingOCRLinkControlsCycleAndUnlink(t *testing.T) {
	a, state, _ := proofreadingOCRLinkFixture(t)
	state.ocrScan.pages[0].result.Status = "ambiguous"
	state.ocrScan.pages[0].result.PathChoices = []textalign.Candidate{
		{Range: textalign.Range{Start: 5, End: 11}, Evidence: []textalign.Evidence{{BlockID: 4, Range: textalign.Range{Start: 5, End: 11}}}},
		{Range: textalign.Range{Start: 9, End: 12}, Evidence: []textalign.Evidence{{BlockID: 4, Range: textalign.Range{Start: 9, End: 12}}}},
	}
	if !a.activateProofreadingOCRBlock(0, 4) {
		t.Fatal("ambiguous OCR block did not activate its B link")
	}
	var router input.Router
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(260, 34))}
	state.ocrScan.linkNext.Click()
	a.layoutProofreadingOCRLinkControls(gtx)
	router.Frame(&ops)
	if state.ocrScan.link.PathChoiceIndex != 1 || state.ocrScan.link.BRange != (textalign.Range{Start: 9, End: 12}) {
		t.Fatalf("Next control did not select the next global candidate: %+v", state.ocrScan.link)
	}
	ops.Reset()
	gtx.Ops = &ops
	state.ocrScan.linkUnlink.Click()
	a.layoutProofreadingOCRLinkControls(gtx)
	router.Frame(&ops)
	if state.ocrScan.link.Active || state.ocrScan.highlight.Active {
		t.Fatal("Unlink control left OCR link or highlight active")
	}
}

func TestProofreadingOCRLinkChoiceRevealsFilteredTargetPage(t *testing.T) {
	a, state, view := proofreadingOCRLinkFixture(t)
	state.pageOwnership[2].Reviewed = true
	state.pageFilter = proofreadingPageFilterUnreviewed
	state.ocrScan.pages[0].result.Status = "ambiguous"
	state.ocrScan.pages[0].result.PathChoices = []textalign.Candidate{
		{Range: textalign.Range{Start: 5, End: 7}, Evidence: []textalign.Evidence{{BlockID: 4, Range: textalign.Range{Start: 5, End: 7}}}},
		{Range: textalign.Range{Start: 9, End: 11}, Evidence: []textalign.Evidence{{BlockID: 4, Range: textalign.Range{Start: 9, End: 11}}}},
	}
	if !a.activateProofreadingOCRBlock(0, 4) {
		t.Fatal("first visible candidate did not activate")
	}
	if state.pageFilter != proofreadingPageFilterUnreviewed || state.xCanvas.Position.First != 1 {
		t.Fatalf("link activation changed the visible filter or chose the wrong X row: filter=%d X=%+v", state.pageFilter, state.xCanvas.Position)
	}
	if !a.moveProofreadingOCRLinkChoice(1) {
		t.Fatal("next supported path choice was not selectable")
	}
	if state.pageFilter != proofreadingPageFilterAll || view.page != 0 || view.canvas.Position.First != 0 {
		t.Fatalf("hidden alternative did not reveal its B page while keeping A pinned: filter=%d A=%+v page=%d", state.pageFilter, view.canvas.Position, view.page)
	}
	if state.xCanvas.Position.First != 2 || state.ocrScan.link.XFirstPage != 2 {
		t.Fatalf("X did not move to the newly revealed candidate: X=%+v link=%+v", state.xCanvas.Position, state.ocrScan.link)
	}
}

func TestProofreadingOCRLinkedBlockClickOpensEvidenceRange(t *testing.T) {
	a, state, _ := proofreadingOCRLinkFixture(t)
	state.ocrScan.pages[0].layout = ocr.Layout{
		Width: 100, Height: 100,
		Blocks: []ocr.LayoutBlock{{ID: 4, Bounds: [4]float64{10, 10, 90, 40}}},
	}
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(100, 100))}
		a.layoutProofreadingOCRBlockTargets(gtx, image.Pt(100, 100), 0, &state.ocrScan.pages[0])
		router.Frame(&ops)
	}
	frame()
	point := f32.Pt(20, 20)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: point})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point})
	frame()
	if !state.ocrScan.link.Active || state.ocrScan.link.BlockID != 4 || state.ocrScan.link.BRange != (textalign.Range{Start: 5, End: 11}) {
		t.Fatalf("OCR block click did not open its evidence link: %+v", state.ocrScan.link)
	}
}

func TestProofreadingOCRManualSourceNavigationExitsLink(t *testing.T) {
	a, state, view := proofreadingOCRLinkFixture(t)
	if !a.activateProofreadingOCRBlock(0, 4) {
		t.Fatal("matched OCR block did not activate its B link")
	}
	if !a.setProofreadingCurrentPage(1) {
		t.Fatal("manual source page navigation did not move A")
	}
	if state.ocrScan.link.Active || state.ocrScan.highlight.Active || view.page != 1 {
		t.Fatalf("manual A navigation failed to exit link: page=%d link=%+v", view.page, state.ocrScan.link)
	}
}

func TestProofreadingOCRLinkedCompareKeepsSourcePageAndMovesXToBPage(t *testing.T) {
	a, state, view := proofreadingOCRLinkFixture(t)
	if !a.activateProofreadingOCRBlock(0, 4) {
		t.Fatal("matched OCR block did not activate its B link")
	}
	if view.page != 0 || state.ocrScan.link.APage != 0 {
		t.Fatalf("A source page moved during link activation: page=%d link=%+v", view.page, state.ocrScan.link)
	}
	if state.xCanvas.Position.First != 1 || state.ocrScan.link.XFirstPage != 1 || state.ocrScan.link.XLastPage != 2 {
		t.Fatalf("X did not move to the linked B span: pos=%+v link=%+v", state.xCanvas.Position, state.ocrScan.link)
	}
	state.xCanvas.Position.First = 2
	state.xCanvas.Position.Offset = 31
	var router input.Router
	var ops op.Ops
	state.xCanvas.Axis = layout.Vertical
	gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(500, 220))}
	a.layoutProofreadingXPageList(gtx, view, 3, "", false)
	router.Frame(&ops)
	if view.page != 0 || view.canvas.Position.First != 0 {
		t.Fatalf("A moved while X was independently scrolled: page=%d position=%+v", view.page, view.canvas.Position)
	}
	if state.xCanvas.Position.First < 2 {
		t.Fatalf("X page list snapped back to A during link mode: X=%+v A=%+v", state.xCanvas.Position, view.canvas.Position)
	}
}

func TestProofreadingOCRUnlinkRestoresSynchronizedScroll(t *testing.T) {
	a, state, view := proofreadingOCRLinkFixture(t)
	if !a.activateProofreadingOCRBlock(0, 4) {
		t.Fatal("matched OCR block did not activate its B link")
	}
	state.xCanvas.Position.First = 2
	state.xCanvas.Position.Offset = 35
	var router input.Router
	var ops op.Ops
	frame := func() bool {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(200, 100))}
		handled := a.handleOverlayEscape(gtx)
		router.Frame(&ops)
		return handled
	}
	frame()
	router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	if !frame() {
		t.Fatal("Escape did not handle the active OCR link")
	}
	if state.ocrScan.link.Active || state.ocrScan.highlight.Active {
		t.Fatal("Escape left OCR link or highlight active")
	}
	if state.xCanvas.Position.First != view.canvas.Position.First || state.xCanvas.Position.Offset != view.canvas.Position.Offset {
		t.Fatalf("Escape did not restore X to A: A=%+v X=%+v", view.canvas.Position, state.xCanvas.Position)
	}
}

func TestProofreadingOCRLinkInvalidatesOnOwnershipRevision(t *testing.T) {
	a, state, _ := proofreadingOCRLinkFixture(t)
	draft := state.bDraftText
	if !a.activateProofreadingOCRBlock(0, 4) {
		t.Fatal("matched OCR block did not activate its B link")
	}
	if err := state.controller.Model.AdjustBoundary(1, 1); err != nil {
		t.Fatal(err)
	}
	if a.validateProofreadingOCRLink() {
		t.Fatal("link survived a B page-ownership revision")
	}
	if state.bDraftText != draft {
		t.Fatalf("ownership-only change altered B text snapshot: got %q want %q", state.bDraftText, draft)
	}
	if state.ocrScan.link.Active || state.ocrScan.highlight.Active {
		t.Fatal("stale ownership link/highlight was not cleared")
	}
}

func TestProofreadingOCRLinkInvalidatesOnDraftOrFilterChange(t *testing.T) {
	t.Run("draft changed", func(t *testing.T) {
		a, state, _ := proofreadingOCRLinkFixture(t)
		if !a.activateProofreadingOCRBlock(0, 4) {
			t.Fatal("matched OCR block did not activate its B link")
		}
		state.bDraftText += " 修改"
		if a.validateProofreadingOCRLink() || state.ocrScan.highlight.Active {
			t.Fatal("link/highlight survived a changed B draft")
		}
	})
	t.Run("filter changed", func(t *testing.T) {
		a, state, _ := proofreadingOCRLinkFixture(t)
		if !a.activateProofreadingOCRBlock(0, 4) {
			t.Fatal("matched OCR block did not activate its B link")
		}
		a.setProofreadingPageFilter(proofreadingPageFilterReviewed)
		if state.ocrScan.link.Active || state.ocrScan.highlight.Active {
			t.Fatal("page filter change left OCR link/highlight active")
		}
	})
}

func TestProofreadingOCRLinkClearsOnCancelOrGenerationChange(t *testing.T) {
	t.Run("cancel", func(t *testing.T) {
		a, state, _ := proofreadingOCRLinkFixture(t)
		if !a.activateProofreadingOCRBlock(0, 4) {
			t.Fatal("matched OCR block did not activate its B link")
		}
		state.ocrScan.cancelTask()
		if state.ocrScan.link.Active || state.ocrScan.highlight.Active {
			t.Fatal("cancel left OCR link or highlight active")
		}
	})
	t.Run("generation change", func(t *testing.T) {
		a, state, _ := proofreadingOCRLinkFixture(t)
		if !a.activateProofreadingOCRBlock(0, 4) {
			t.Fatal("matched OCR block did not activate its B link")
		}
		state.ocrScan.generation++
		if a.validateProofreadingOCRLink() {
			t.Fatal("link survived a newer OCR generation")
		}
		if state.ocrScan.link.Active || state.ocrScan.highlight.Active {
			t.Fatal("new generation left OCR link or highlight active")
		}
	})
}

func proofreadingOCRLinkFixture(t *testing.T) (*App, *proofreadingWorkspace, *pdfViewState) {
	t.Helper()
	a, state := proofreadingModelFixture(t)
	state.aSourceHash = "source-A"
	state.sourceBHash = "source-B"
	draft := state.bDraftText
	state.ocrScan = &proofreadingOCRScan{
		stage:               proofreadingOCRComplete,
		generation:          23,
		markdownSnapshot:    draft,
		markdownSnapshotSet: true,
		sourceAHash:         state.aSourceHash,
		sourceBHash:         state.sourceBHash,
		pages: []proofreadingOCRPage{{result: textalign.Result{
			Page: 1, Status: "matched",
			Selected:    &textalign.Candidate{Range: textalign.Range{Start: 5, End: 11}, Evidence: []textalign.Evidence{{BlockID: 4, Range: textalign.Range{Start: 5, End: 11}}}},
			PathChoices: []textalign.Candidate{{Range: textalign.Range{Start: 5, End: 11}, Evidence: []textalign.Evidence{{BlockID: 4, Range: textalign.Range{Start: 5, End: 11}}}}},
		}}},
	}
	view := &pdfViewState{info: document.PDFInfo{Pages: 3}, page: 0, scale: 1}
	view.canvas.Axis = layout.Vertical
	a.pdfView = view
	return a, state, view
}
