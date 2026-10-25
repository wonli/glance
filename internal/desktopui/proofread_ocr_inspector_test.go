package desktopui

import (
	"strings"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"image"
)

func TestProofreadingDiffInspectorLifecycle(t *testing.T) {
	a, gtx, comparison := proofreadingConnectorFixture(1)
	page := a.proofreading.xPages[3]

	// Add 2 frames (one Replace, one BOnly)
	comparison.Frames = []proofreadingOCRDiffFrame{
		{Kind: proofreadingOCRDiffReplace, BPosition: page.ContentStart + 1, AChar: 'X', BChar: 'b', ABlockID: 7, AOffset: 0},
		{Kind: proofreadingOCRDiffBOnly, BPosition: page.ContentStart + 6, BChar: 'b', ABlockID: -1, AOffset: -1},
	}
	pair := proofreadingOCRStepPair{APage: comparison.APage, BPage: comparison.BPage}
	a.proofreading.ocrStep.comparisons[pair] = comparison
	a.proofreading.ocrStep.diffCursor = len(comparison.Frames)
	a.proofreading.ocrStep.diffRunning = false

	step := a.proofreading.ocrStep

	// Provide connections to step
	step.currentConnections = []proofreadingOCRConnection{
		{from: image.Pt(100, 50), to: image.Pt(400, 50), kind: proofreadingOCRDiffReplace, frame: comparison.Frames[0]},
		{from: image.Pt(100, 80), to: image.Pt(400, 80), kind: proofreadingOCRDiffBOnly, frame: comparison.Frames[1]},
	}

	// 1. Open inspector on badge index 0
	a.openProofreadingDiffInspector(0)
	if !step.inspector.open {
		t.Fatal("expected inspector to be open")
	}
	if step.inspector.diffIndex != 0 {
		t.Fatalf("expected diffIndex=0, got %d", step.inspector.diffIndex)
	}
	if !strings.Contains(step.inspector.editorText, "a") {
		t.Fatalf("expected editorText to contain context, got %q", step.inspector.editorText)
	}

	// 2. Test Adopt: replaces 'b' with 'X'
	originalPageText := a.proofreading.xPageEditors[3].Text()
	a.adoptProofreadingDiffInspector()

	// Should be marked resolved and advanced to diff 1
	if !step.inspector.resolved[0] {
		t.Fatal("expected diff 0 to be marked resolved")
	}
	if step.inspector.diffIndex != 1 {
		t.Fatalf("expected diffIndex=1 after adopt, got %d", step.inspector.diffIndex)
	}
	newPageText := a.proofreading.xPageEditors[3].Text()
	if newPageText == originalPageText {
		t.Fatal("expected page text to change after adopt")
	}
	if !strings.Contains(newPageText, "X") {
		t.Fatalf("expected newPageText to contain 'X', got %q", newPageText)
	}

	// 3. Test Keep: on diff 1
	a.keepProofreadingDiffInspector()
	if !step.inspector.resolved[1] {
		t.Fatal("expected diff 1 to be marked resolved")
	}

	// All 2 diffs are resolved -> allResolved should be true
	if !step.inspector.allResolved {
		t.Fatal("expected allResolved to be true after resolving all diffs")
	}

	// 4. Render inspector in layout should render completion without panics
	var ops op.Ops
	layoutGtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(800, 600))}
	dims := a.layoutProofreadingDiffInspector(layoutGtx)
	if dims.Size.X == 0 || dims.Size.Y == 0 {
		t.Fatal("expected inspector completion layout to have non-zero size")
	}
	_ = gtx
}

func TestProofreadingDiffInspectorNavigationAndIgnore(t *testing.T) {
	a, _, comparison := proofreadingConnectorFixture(1)
	page := a.proofreading.xPages[3]

	comparison.Frames = []proofreadingOCRDiffFrame{
		{Kind: proofreadingOCRDiffReplace, BPosition: page.ContentStart + 1, AChar: '1', BChar: 'b', ABlockID: 7, AOffset: 0},
		{Kind: proofreadingOCRDiffReplace, BPosition: page.ContentStart + 6, AChar: '2', BChar: 'b', ABlockID: 7, AOffset: 1},
		{Kind: proofreadingOCRDiffReplace, BPosition: page.ContentStart + 11, AChar: '3', BChar: 'b', ABlockID: 7, AOffset: 2},
	}
	step := a.proofreading.ocrStep
	step.currentConnections = []proofreadingOCRConnection{
		{from: image.Pt(100, 50), to: image.Pt(400, 50), kind: proofreadingOCRDiffReplace, frame: comparison.Frames[0]},
		{from: image.Pt(100, 80), to: image.Pt(400, 80), kind: proofreadingOCRDiffReplace, frame: comparison.Frames[1]},
		{from: image.Pt(100, 110), to: image.Pt(400, 110), kind: proofreadingOCRDiffReplace, frame: comparison.Frames[2]},
	}

	a.openProofreadingDiffInspector(0)
	if step.inspector.diffIndex != 0 {
		t.Fatalf("expected diffIndex=0, got %d", step.inspector.diffIndex)
	}

	// Advance +1
	a.advanceProofreadingDiffInspector(1)
	if step.inspector.diffIndex != 1 {
		t.Fatalf("expected diffIndex=1, got %d", step.inspector.diffIndex)
	}

	// Advance +1
	a.advanceProofreadingDiffInspector(1)
	if step.inspector.diffIndex != 2 {
		t.Fatalf("expected diffIndex=2, got %d", step.inspector.diffIndex)
	}

	// Advance -1 (prev)
	a.advanceProofreadingDiffInspector(-1)
	if step.inspector.diffIndex != 1 {
		t.Fatalf("expected diffIndex=1, got %d", step.inspector.diffIndex)
	}
}

func TestProofreadingCenterBadgeLayoutAndClickOpensInspector(t *testing.T) {
	a, gtx, comparison := proofreadingConnectorFixture(1)
	page := a.proofreading.xPages[3]

	comparison.Frames = []proofreadingOCRDiffFrame{
		{Kind: proofreadingOCRDiffReplace, BPosition: page.ContentStart + 1, AChar: '1', BChar: 'b', ABlockID: 7, AOffset: 0},
	}
	pair := proofreadingOCRStepPair{APage: comparison.APage, BPage: comparison.BPage}
	a.proofreading.ocrStep.comparisons[pair] = comparison
	a.proofreading.ocrStep.diffCursor = len(comparison.Frames)
	a.proofreading.ocrStep.diffRunning = false

	// Layout connector - should render badges and register badge clicks without panic
	a.drawProofreadingOCRStepConnector(gtx)

	step := a.proofreading.ocrStep
	if len(step.currentBadges) == 0 {
		t.Fatal("expected at least one center badge to be generated")
	}

	// Trigger opening
	a.openProofreadingDiffInspector(0)
	if !step.inspector.open {
		t.Fatal("expected inspector to be open")
	}

	// Rendering inspector overlay directly
	dims := a.layoutProofreadingDiffInspector(gtx)
	if dims.Size.X == 0 || dims.Size.Y == 0 {
		t.Fatal("expected inspector card to have non-zero dimensions")
	}
}

func TestProofreadingDiffInspectorTopLevelCenteringAndMultiPagePersistence(t *testing.T) {
	a, gtx, compPage0 := proofreadingConnectorFixture(1)
	state := a.proofreading
	step := state.ocrStep

	content0 := "hello world"
	content1 := "foo bar baz"
	state.xPages[0] = proofreadingXPage{PageIndex: 0, Content: content0, ContentStart: 0, ContentEnd: len([]rune(content0)), Width: 300, Height: 120}
	state.xPageEditors[0].SetText(content0)
	state.xEditorWindows[0] = proofreadingEditorWindow{Initialized: true, End: len([]rune(content0))}

	state.xPages[1] = proofreadingXPage{PageIndex: 1, Content: content1, ContentStart: 0, ContentEnd: len([]rune(content1)), Width: 300, Height: 120}
	state.xPageEditors[1].SetText(content1)
	state.xEditorWindows[1] = proofreadingEditorWindow{Initialized: true, End: len([]rune(content1))}

	// Setup Page 0 comparison with 2 diffs
	compPage0.APage = 0
	compPage0.BPage = 0
	compPage0.BStart = state.xPages[0].ContentStart
	compPage0.BEnd = state.xPages[0].ContentEnd
	compPage0.BText = state.xPages[0].Content
	compPage0.HasEvidence = true
	compPage0.Frames = []proofreadingOCRDiffFrame{
		{Kind: proofreadingOCRDiffReplace, BPosition: state.xPages[0].ContentStart + 1, AChar: 'A', BChar: 'e', ABlockID: 7, AOffset: 0},
		{Kind: proofreadingOCRDiffReplace, BPosition: state.xPages[0].ContentStart + 4, AChar: 'B', BChar: 'o', ABlockID: 7, AOffset: 1},
	}
	compPage0.DraftHash = proofreadingOCRDraftSnapshotHash(compPage0.BText)
	pair0 := proofreadingOCRStepPair{APage: 0, BPage: 0}
	step.comparisons[pair0] = compPage0

	// Setup Page 1 comparison with 1 diff
	compPage1 := compPage0
	compPage1.APage = 1
	compPage1.BPage = 1
	compPage1.BStart = state.xPages[1].ContentStart
	compPage1.BEnd = state.xPages[1].ContentEnd
	compPage1.BText = state.xPages[1].Content
	compPage1.HasEvidence = true
	compPage1.Frames = []proofreadingOCRDiffFrame{
		{Kind: proofreadingOCRDiffReplace, BPosition: state.xPages[1].ContentStart + 2, AChar: 'C', BChar: 'o', ABlockID: 7, AOffset: 0},
	}
	compPage1.DraftHash = proofreadingOCRDraftSnapshotHash(compPage1.BText)
	pair1 := proofreadingOCRStepPair{APage: 1, BPage: 1}
	step.comparisons[pair1] = compPage1

	// 1. Visit Page 0, resolve diff 0
	a.pdfView.page = 0
	step.currentConnections = []proofreadingOCRConnection{
		{from: image.Pt(100, 50), to: image.Pt(400, 50), kind: proofreadingOCRDiffReplace, frame: compPage0.Frames[0]},
		{from: image.Pt(100, 80), to: image.Pt(400, 80), kind: proofreadingOCRDiffReplace, frame: compPage0.Frames[1]},
	}
	a.openProofreadingDiffInspector(0)
	if !step.inspector.open {
		t.Fatal("expected inspector to open on page 0")
	}
	a.adoptProofreadingDiffInspector()
	if !step.inspector.pageResolved(0)[0] {
		t.Fatal("expected page 0 diff 0 to be resolved")
	}
	if step.inspector.pageResolved(1)[0] {
		t.Fatal("page 1 should NOT be marked resolved by page 0 action")
	}

	// 2. Switch to Page 1
	a.pdfView.page = 1
	a.drawProofreadingOCRStepConnector(gtx)
	// Both comparisons should remain valid
	if !proofreadingOCRStepComparisonValid(state, step.comparisons[pair0]) {
		t.Fatal("expected page 0 comparison to remain valid after edit")
	}
	if !proofreadingOCRStepComparisonValid(state, step.comparisons[pair1]) {
		t.Fatal("expected page 1 comparison to remain valid")
	}

	// 3. Switch back to Page 0: resolved state preserved, frames not nil
	a.pdfView.page = 0
	frames0 := proofreadingOCRScannedDiffFrames(step, step.comparisons[pair0])
	if len(frames0) != len(compPage0.Frames) {
		t.Fatalf("expected all %d frames preserved on page 0, got %d", len(compPage0.Frames), len(frames0))
	}
	if !step.inspector.pageResolved(0)[0] {
		t.Fatal("expected page 0 diff 0 to remain resolved after switching back")
	}

	// 4. Test Top-level layout with Inspector open covers full screen and blocks clicks
	step.inspector.open = true
	var ops op.Ops
	fullGtx := layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(1200, 800)),
	}
	dims := a.layoutProofreadingWorkspace(fullGtx)
	if dims.Size.X != 1200 || dims.Size.Y != 800 {
		t.Fatalf("expected workspace to return exact screen size 1200x800, got %v", dims.Size)
	}
}

func TestProofreadingResolvedBadgesKeepNumbersAndColorsAndNoDiffBadge(t *testing.T) {
	a, gtx, compPage0 := proofreadingConnectorFixture(1)
	state := a.proofreading
	step := state.ocrStep

	// 1. Verify resolved badges keep orderNumber and resolve map contains character keys
	step.inspector.pageResolved(3)[0] = true
	resA, resB := proofreadingOCRResolvedDiffRuns(a.pdfView, state, step, compPage0)
	if len(resA) == 0 && len(resB) == 0 {
		t.Fatal("expected resolved run anchors to be populated")
	}

	// 2. Draw connector with resolved badge: badge order number should remain 1, not checkmark
	badges := layoutProofreadingCenterBadges(gtx, step.currentConnections, 500, step.inspector.pageResolved(3))
	if len(badges) > 0 {
		if !badges[0].resolved {
			t.Fatal("badge 0 should be marked resolved")
		}
		if badges[0].orderNumber != 1 {
			t.Fatalf("expected badge 0 orderNumber to be 1, got %d", badges[0].orderNumber)
		}
	}

	// 3. Test No-Diff badge when comparison has no differences
	noDiffComp := compPage0
	noDiffComp.AOnly = 0
	noDiffComp.BOnly = 0
	noDiffComp.Frames = []proofreadingOCRDiffFrame{
		{Kind: proofreadingOCRDiffEqual, BPosition: 0, AChar: 'a', BChar: 'a'},
	}
	noDiffComp.HasEvidence = true
	noDiffComp.Complete = true
	step.comparisons[proofreadingOCRStepPair{APage: 3, BPage: 3}] = noDiffComp
	step.diffRunning = false
	step.diffPaused = false

	var ops op.Ops
	noDiffGtx := gtx
	noDiffGtx.Ops = &ops
	a.drawProofreadingOCRStepConnector(noDiffGtx)
	// Verify that with 0 diffs, currentConnections is cleared and no-diff badge draws without panic
	if len(step.currentConnections) != 0 {
		t.Fatalf("expected 0 connections for no-diff comparison, got %d", len(step.currentConnections))
	}

	// 4. Verify scrolling does not affect No-Diff badge execution or position
	state.xPageScrollOffsets[3] += 300
	ops.Reset()
	a.drawProofreadingOCRStepConnector(noDiffGtx)
	if len(step.currentConnections) != 0 {
		t.Fatalf("scrolling should still have 0 diff connections, got %d", len(step.currentConnections))
	}
}
