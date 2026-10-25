package desktopui

import (
	"image"
	"strings"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"

	"glance/internal/textalign"
)

func TestProofreadingOCRMatchingProgressPublishesCurrentCandidatePage(t *testing.T) {
	a, state := proofreadingModelFixture(t)
	attachProofreadingOCRTestPDF(t, a)
	scan := newProofreadingOCRScan()
	scan.generation = 7
	scan.stage = proofreadingOCRMatching
	scan.current = -1
	scan.total = 6
	scan.follow = true
	scan.markdownSnapshot = state.bDraftText
	scan.markdownSnapshotSet = true
	scan.sourceAHash = state.aSourceHash
	scan.sourceBHash = state.sourceBHash
	state.ocrScan = scan
	scan.progress <- proofreadingOCRProgress{
		generation: 7, stage: proofreadingOCRMatching, current: 2, total: 6, extracted: 6,
		alignment: textalign.AlignmentProgress{Phase: textalign.AlignmentPhaseCandidates, Page: 3, CompletedPages: 2, TotalPages: 6},
	}
	var ops op.Ops
	a.processProofreadingOCRScan(layout.Context{Ops: &ops, Now: time.Now()})
	if scan.current != 2 || scan.alignment.Page != 3 || scan.alignment.CompletedPages != 2 || len(scan.pages) != 0 {
		t.Fatalf("candidate progress was not published without final results: current=%d progress=%+v pages=%d", scan.current, scan.alignment, len(scan.pages))
	}
	if a.pdfView == nil || a.pdfView.page != 2 || state.xCanvas.Position.First != a.pdfView.canvas.Position.First {
		t.Fatalf("scan-follow did not move A/X to the active candidate page: view=%+v X=%+v", a.pdfView, state.xCanvas.Position)
	}
	message, _, ok := a.proofreadingOCRStatusText()
	if !ok || !strings.Contains(message, "3") || !strings.Contains(message, "2") || !strings.Contains(message, "6") {
		t.Fatalf("status does not show current candidate and completed/total pages: %q", message)
	}
}

func TestProofreadingOCRMatchingFollowStopsOnManualSourceNavigation(t *testing.T) {
	a, state := proofreadingModelFixture(t)
	attachProofreadingOCRTestPDF(t, a)
	view := a.ensurePDFView()
	scan := newProofreadingOCRScan()
	scan.stage = proofreadingOCRMatching
	scan.current = 1
	scan.total = 6
	scan.follow = true
	state.ocrScan = scan
	if !a.setProofreadingCurrentPage(3) {
		t.Fatal("manual source navigation did not move to the requested page")
	}
	if scan.follow {
		t.Fatal("manual source navigation left scan-follow enabled")
	}
	if view.page != 3 {
		t.Fatalf("manual page did not remain selected: got %d", view.page)
	}
	scan.followButton.Click()
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Now: time.Now(), Constraints: layout.Exact(image.Pt(160, 28))}
	a.layoutProofreadingOCRFollowControl(gtx)
	if !scan.follow || view.page != 1 {
		t.Fatalf("follow control did not resume at the current candidate: enabled=%v page=%d", scan.follow, view.page)
	}
}

func TestProofreadingOCRStatusShowsFirstLastLocatedAnchorAndBoundary(t *testing.T) {
	a, state := proofreadingModelFixture(t)
	state.xPages = []proofreadingXPage{
		{PageIndex: 0, ContentStart: 0, ContentEnd: 4, Content: "甲乙丙丁"},
		{PageIndex: 1, ContentStart: 4, ContentEnd: 8, Content: "戊己庚辛"},
		{PageIndex: 2, ContentStart: 8, ContentEnd: 12, Content: "壬癸子丑"},
	}
	scan := newProofreadingOCRScan()
	scan.stage = proofreadingOCRComplete
	scan.pages = []proofreadingOCRPage{
		{result: textalign.Result{Page: 1, Status: "matched", Selected: &textalign.Candidate{Range: textalign.Range{Start: 0, End: 3}}, BoundaryAfter: &textalign.Boundary{Status: "point"}}},
		{result: textalign.Result{Page: 2, Status: "matched", Selected: &textalign.Candidate{Range: textalign.Range{Start: 4, End: 7}}, BoundaryAfter: &textalign.Boundary{Status: "range"}}},
		{result: textalign.Result{Page: 3, Status: "matched", Selected: &textalign.Candidate{Range: textalign.Range{Start: 8, End: 11}}, BoundaryAfter: &textalign.Boundary{Status: "unresolved"}}},
	}
	state.ocrScan = scan
	message := proofreadingOCRStatusEvidence(a, scan)
	summary := buildProofreadingOCRAlignmentSummary(scan.pages, state.xPages, 0)
	wantEndpoints := a.textArgs("proofread.ai.evidence.anchors", proofreadingOCRAnchorArgs(a, summary.First, summary.Last))
	if !strings.Contains(message, wantEndpoints) {
		t.Fatalf("status endpoint summary = %q; want %q", message, wantEndpoints)
	}
	for _, want := range []string{"A", "B", "1", "3", "点", "区间", "未确定"} {
		if !strings.Contains(message, want) {
			t.Fatalf("endpoint/boundary evidence %q missing from %q", want, message)
		}
	}
	scan.pages = []proofreadingOCRPage{
		{result: textalign.Result{Page: 1, Status: "ambiguous", PathChoices: []textalign.Candidate{{Range: textalign.Range{Start: 0, End: 2}}}}},
		{result: textalign.Result{Page: 2, Status: "unmatched"}},
	}
	message = proofreadingOCRStatusEvidence(a, scan)
	if !strings.Contains(message, "暂定首尾（未确认）") || strings.Contains(message, "首尾锚点：") {
		t.Fatalf("status invented confident anchors for ambiguous/unmatched pages: %q", message)
	}
}

func TestProofreadingOCRAnimationStopsForHiddenPageOrInactiveView(t *testing.T) {
	scan := newProofreadingOCRScan()
	scan.stage = proofreadingOCRMatching
	scan.follow = true
	scan.current = 1
	scan.alignment = textalign.AlignmentProgress{Phase: textalign.AlignmentPhaseCandidates, Page: 2, CompletedPages: 1, TotalPages: 4}
	if !proofreadingOCRAnimationShouldRun(scan, 1, true, true) {
		t.Fatal("visible active candidate page should animate")
	}
	for _, tc := range []struct {
		name        string
		page        int
		pageVisible bool
		viewActive  bool
	}{
		{name: "hidden page", page: 1, pageVisible: false, viewActive: true},
		{name: "inactive proofreading tab", page: 1, pageVisible: true, viewActive: false},
		{name: "different page", page: 2, pageVisible: true, viewActive: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if proofreadingOCRAnimationShouldRun(scan, tc.page, tc.pageVisible, tc.viewActive) {
				t.Fatal("inactive animation state scheduled another frame")
			}
		})
	}
	scan.stage = proofreadingOCRComplete
	if proofreadingOCRAnimationShouldRun(scan, 1, true, true) {
		t.Fatal("completed scan kept scheduling laser frames")
	}
	pulseAt := time.Now()
	if got := proofreadingOCRPulseAlpha(pulseAt, pulseAt, 48); got <= 48 {
		t.Fatalf("new link did not start with a visible highlight pulse: alpha=%d", got)
	}
	if got := proofreadingOCRPulseAlpha(pulseAt.Add(time.Second), pulseAt, 48); got != 48 {
		t.Fatalf("link highlight did not settle to persistent low alpha: alpha=%d", got)
	}
}
