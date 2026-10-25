package desktopui

import (
	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"glance/internal/ocr"
	"image"
	"image/color"
	"runtime"
	"strings"
	"testing"
)

func TestProofreadingCircuitMonotoneNetsAvoidCrossing(t *testing.T) {
	for _, up := range []bool{false, true} {
		var connections []proofreadingOCRConnection
		for i := 0; i < 4; i++ {
			from, to := 20+i*20, 150+i*20
			if up {
				from, to = to, from
			}
			connections = append(connections, proofreadingOCRConnection{from: image.Pt(100, from), to: image.Pt(800, to), kind: proofreadingOCRDiffReplace})
		}
		routes := proofreadingCircuitRoutes(connections, image.Rect(300, 0, 600, 400), 8)
		if len(routes) != 4 {
			t.Fatal(routes)
		}
		for i, route := range routes {
			if proofreadingCircuitCrossings(routes[:i], route) != 0 {
				t.Fatalf("up=%v: monotone route %d crosses another wire: %+v", up, i, route)
			}
			if route.points[0] != connections[i].from || route.points[len(route.points)-1] != connections[i].to {
				t.Fatal("router moved a character endpoint")
			}
			for j := 1; j < len(route.points); j++ {
				if route.points[j].X != route.points[j-1].X && route.points[j].Y != route.points[j-1].Y {
					t.Fatal("diagonal circuit segment")
				}
			}
		}
	}
}
func TestProofreadingCircuitReversedOrderCrossesNaturally(t *testing.T) {
	connections := []proofreadingOCRConnection{
		{from: image.Pt(100, 20), to: image.Pt(800, 160)},
		{from: image.Pt(100, 40), to: image.Pt(800, 140)},
	}
	routes := proofreadingCircuitRoutes(connections, image.Rect(300, 0, 600, 200), 8)
	if proofreadingCircuitCrossings(routes[:1], routes[1]) == 0 {
		t.Fatal("expected natural crossing for reversed endpoint order")
	}
	var step proofreadingOCRStepState
	first := step.circuitRoutes(connections, image.Rect(300, 0, 600, 200), 8)
	connections[0].active = true
	second := step.circuitRoutes(connections, image.Rect(300, 0, 600, 200), 8)
	if &first[0] != &second[0] {
		t.Fatal("scan highlight unnecessarily rerouted unchanged wires")
	}
}
func TestProofreadingCircuitSourceUsesActualCharacterBox(t *testing.T) {
	a, _, comparison := proofreadingConnectorFixture(1)
	frame := comparison.Frames[1]
	bounds, ok := proofreadingOCRStepSourceCharacterBounds(a.proofreading.ocrStep, comparison, frame, image.Pt(300, 120))
	if !ok || bounds != image.Rect(30, 10, 38, 16) {
		t.Fatalf("character geometry=%v valid=%v, expected the second measured glyph", bounds, ok)
	}
	frame.AOffset = 999
	if _, ok := proofreadingOCRStepSourceCharacterBounds(a.proofreading.ocrStep, comparison, frame, image.Pt(300, 120)); ok {
		t.Fatal("unlocalized character fabricated a box")
	}
}
func TestProofreadingOCRFramesKeepSourceRuneOffsets(t *testing.T) {
	a, _, _ := proofreadingConnectorFixture(1)
	layout := a.proofreading.ocrStep.pages[3].layout
	layout.Blocks = layout.Blocks[:1]
	layout.Blocks[0].Text = "甲 乙\n丙丁"
	diff := proofreadingOCRCharacterDiff(layout, "甲戊丙丁", 0)
	if len(diff.Frames) != 4 {
		t.Fatalf("frames=%+v", diff.Frames)
	}
	for i, want := range []int{0, 2, 4, 5} {
		if diff.Frames[i].AOffset != want {
			t.Fatalf("frame %d source rune offset=%d want %d", i, diff.Frames[i].AOffset, want)
		}
	}
}

func TestProofreadingCircuitActiveGlyphHasItsOwnConnection(t *testing.T) {
	a, gtx, comparison := proofreadingConnectorFixture(1)
	step := a.proofreading.ocrStep
	step.diffRunning = true
	step.diffCursor = 3
	connections := proofreadingOCRStepConnections(gtx, a.pdfView, a.proofreading, step, comparison)
	if len(connections) != 2 {
		t.Fatalf("expected retained run wire plus current glyph wire: %v", connections)
	}
	if connections[0].active || !connections[1].active || connections[0].from == connections[1].from || connections[0].to == connections[1].to {
		t.Fatalf("active glyph did not have its own measured endpoints: %v", connections)
	}
	step.diffRunning = false
	completed := proofreadingOCRStepConnections(gtx, a.pdfView, a.proofreading, step, comparison)
	if len(completed) != 4 {
		t.Fatalf("completed history must show all four difference runs despite the old playback cursor: %v", completed)
	}
	for _, connection := range completed {
		if connection.active {
			t.Fatal("completed history retained an active glyph wire")
		}
	}
}

func TestProofreadingCircuitCrossingPixels(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	window, err := headless.NewWindow(900, 220)
	if err != nil {
		t.Skipf("headless GPU unavailable: %v", err)
	}
	defer window.Release()
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(900, 220))}
	connections := []proofreadingOCRConnection{{from: image.Pt(100, 20), to: image.Pt(800, 160)}, {from: image.Pt(100, 40), to: image.Pt(800, 140)}}
	routes := proofreadingCircuitRoutes(connections, image.Rect(300, 0, 600, 220), 8)
	paint.Fill(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	for _, route := range routes {
		drawProofreadingCircuitRoute(gtx, route, proofreadDiffAOnlyInk, proofreadDiffBOnlyInk, false)
	}
	if err = window.Frame(&ops); err != nil {
		t.Fatal(err)
	}
	shot := image.NewRGBA(image.Rect(0, 0, 900, 220))
	if err = window.Screenshot(shot); err != nil {
		t.Fatal(err)
	}
	colored := 0
	for y := 0; y < 220; y++ {
		for x := 290; x < 610; x++ {
			p := shot.RGBAAt(x, y)
			if int(p.B) > int(p.R)+10 || int(p.R) > int(p.G)+10 {
				colored++
			}
		}
	}
	if colored < 100 {
		t.Fatalf("circuit crossing rendering produced too few colored pixels: %d", colored)
	}
}

func TestProofreadingCharacterCoverageExcludesFurniture(t *testing.T) {
	layout := ocr.Layout{Blocks: []ocr.LayoutBlock{{ID: 1, Label: "header", Text: "页眉"}, {ID: 2, Label: "text", Text: "甲 乙"}}, Characters: []ocr.CharacterBox{{BlockID: 1, Offset: 0, Text: "页"}, {BlockID: 1, Offset: 1, Text: "眉"}, {BlockID: 2, Offset: 0, Text: "甲"}, {BlockID: 2, Offset: 0, Text: "甲"}}}
	located, total := proofreadingOCRCharacterCoverage(layout)
	if located != 1 || total != 2 {
		t.Fatalf("coverage includes furniture/duplicates: %d/%d", located, total)
	}
}

func proofreadingCircuitCrossings(prior []proofreadingCircuitRoute, route proofreadingCircuitRoute) int {
	n := 0
	for _, other := range prior {
		for i := 1; i < len(route.points); i++ {
			for j := 1; j < len(other.points); j++ {
				_, cross, _ := proofreadingCircuitIntersection(route.points[i-1], route.points[i], other.points[j-1], other.points[j])
				if cross {
					n++
				}
			}
		}
	}
	return n
}

func TestProofreadingOCRBOnlyDoesNotBorrowNeighbourBoxOrConnect(t *testing.T) {
	layout := ocr.Layout{
		Width: 300, Height: 120,
		Blocks: []ocr.LayoutBlock{
			{ID: 1, Text: "送吉中孚校书归楚州 2218"},
		},
		Characters: []ocr.CharacterBox{
			{BlockID: 1, Offset: 8, Text: "州", Bounds: [4]float64{100, 20, 110, 30}},
		},
	}
	diff := proofreadingOCRCharacterDiff(layout, "送吉中孚校书归楚州旧山2218", 0)
	if len(diff.Frames) == 0 {
		t.Fatal("empty frames")
	}
	var bOnlyFrames []proofreadingOCRDiffFrame
	for _, f := range diff.Frames {
		if f.Kind == proofreadingOCRDiffBOnly {
			bOnlyFrames = append(bOnlyFrames, f)
			if f.ABlockID != -1 || f.AOffset != -1 {
				t.Fatalf("BOnly frame borrowed neighbour character box: %+v", f)
			}
		}
	}
	if len(bOnlyFrames) != 2 {
		t.Fatalf("expected 2 BOnly frames for 旧山, got %d", len(bOnlyFrames))
	}

	step := &proofreadingOCRStepState{
		pages: map[int]proofreadingOCRPage{
			0: {layout: layout},
		},
	}
	comparison := proofreadingOCRStepComparison{
		APage: 0, BPage: 0,
		Frames: diff.Frames,
	}
	for _, f := range bOnlyFrames {
		if _, ok := proofreadingOCRStepSourceCharacterBounds(step, comparison, f, image.Pt(300, 120)); ok {
			t.Fatalf("BOnly frame fabricated character bounds: %+v", f)
		}
	}
}

func TestProofreadingXRegionUsesCharacterBoxesWithoutBackgroundFill(t *testing.T) {
	a, gtx, comparison := proofreadingConnectorFixture(1)
	editor := &a.proofreading.xPageEditors[3]
	page := a.proofreading.xPages[3]
	window := a.proofreading.xEditorWindows[3]
	// Rendering comparison highlights into ops should not panic and should register ops.
	a.drawProofreadingOCRStepComparisonHighlights(gtx, editor, page, window)
	drawProofreadingOCRCharacterBoxes(gtx, image.Pt(300, 120), a.proofreading.ocrStep, comparison)
}

func TestProofreadingScrollAwayAndBackShowsResultsDirectly(t *testing.T) {
	a, gtx, comparison := proofreadingConnectorFixture(1)
	comparison.HasEvidence = true
	pair := proofreadingOCRStepPair{APage: comparison.APage, BPage: comparison.BPage}
	a.proofreading.ocrStep.comparisons[pair] = comparison

	step := a.proofreading.ocrStep
	step.diffPair = proofreadingOCRStepPair{APage: -1, BPage: -1}
	step.diffPlayedPairs = map[proofreadingOCRStepPair]string{}

	// With results available, display differences directly without playing animation
	a.advanceProofreadingOCRDiffPlayback(gtx, step)
	if step.diffRunning {
		t.Fatal("expected no animation playback when results are available")
	}
	if step.diffCursor != len(comparison.Frames) {
		t.Fatalf("expected diffCursor at end (%d), got %d", len(comparison.Frames), step.diffCursor)
	}

	// User scrolls away to another pair
	step.diffPair = proofreadingOCRStepPair{APage: 0, BPage: 0}

	// User scrolls back to the original pair
	a.advanceProofreadingOCRDiffPlayback(gtx, step)
	if step.diffRunning {
		t.Fatal("expected returning to pair not to re-run animation")
	}
	if step.diffCursor != len(comparison.Frames) {
		t.Fatalf("expected diffCursor at end (%d), got %d", len(comparison.Frames), step.diffCursor)
	}
}

func TestProofreadingXHeaderControlsAreRemoved(t *testing.T) {
	a, gtx, _ := proofreadingConnectorFixture(1)
	dims := a.layoutProofreadingOCRDiffHeaderControls(gtx)
	if dims.Size != (image.Point{}) {
		t.Fatalf("expected empty dims for X header controls, got %+v", dims)
	}
}

func TestProofreadingStatusTextInAIMode(t *testing.T) {
	a, _, comparison := proofreadingConnectorFixture(1)
	comparison.HasEvidence = true
	pair := proofreadingOCRStepPair{APage: comparison.APage, BPage: comparison.BPage}
	a.proofreading.ocrStep.comparisons[pair] = comparison
	step := a.proofreading.ocrStep

	// During diff playback
	step.diffRunning = true
	step.diffCursor = 5
	msg, _, ok := a.proofreadingOCRStatusText()
	if !ok || !strings.Contains(msg, "5") {
		t.Fatalf("expected progress status message with 5, got %q (ok=%v)", msg, ok)
	}

	// After diff playback finishes
	step.diffRunning = false
	step.diffCursor = len(comparison.Frames)
	msg, _, ok = a.proofreadingOCRStatusText()
	if !ok || (!strings.Contains(msg, "比对完成") && !strings.Contains(msg, "Compared")) {
		t.Fatalf("expected completion status message, got %q (ok=%v)", msg, ok)
	}
}

func TestProofreadingDiffVisualDistinctForBOnlyAOnlyAndReplace(t *testing.T) {
	a, gtx, comparison := proofreadingConnectorFixture(1)
	editor := &a.proofreading.xPageEditors[3]
	page := a.proofreading.xPages[3]
	window := a.proofreading.xEditorWindows[3]

	// Add BOnly, AOnly, and Replace frames to comparison
	comparison.Frames = []proofreadingOCRDiffFrame{
		{Kind: proofreadingOCRDiffBOnly, BPosition: page.ContentStart + 1, BChar: 'b'},
		{Kind: proofreadingOCRDiffAOnly, BPosition: page.ContentStart + 2, AChar: 'x', ABlockID: 7, AOffset: 0},
		{Kind: proofreadingOCRDiffReplace, BPosition: page.ContentStart + 3, AChar: 'y', BChar: 'z', ABlockID: 7, AOffset: 1},
	}
	pair := proofreadingOCRStepPair{APage: comparison.APage, BPage: comparison.BPage}
	a.proofreading.ocrStep.comparisons[pair] = comparison
	a.proofreading.ocrStep.diffCursor = len(comparison.Frames)
	a.proofreading.ocrStep.diffRunning = false

	// Render comparison highlights should successfully process all 3 kinds without panic
	a.drawProofreadingOCRStepComparisonHighlights(gtx, editor, page, window)
}

func TestProofreadingComparisonCachePersistence(t *testing.T) {
	_, _, comparison := proofreadingConnectorFixture(1)
	comparison.SourceHash = strings.Repeat("a", 64)
	comparison.DraftHash = "testhash123"

	// Save comparison cache
	if err := saveProofreadingOCRComparisonCache(comparison); err != nil {
		t.Fatalf("save comparison cache failed: %v", err)
	}

	// Load comparison cache
	loaded, hit, err := loadProofreadingOCRComparisonCache(comparison.SourceHash, comparison.APage, comparison.BPage, comparison.DraftHash)
	if err != nil {
		t.Fatalf("load comparison cache error: %v", err)
	}
	if !hit {
		t.Fatal("expected comparison cache hit")
	}
	if loaded.Matches != comparison.Matches || loaded.AOnly != comparison.AOnly || loaded.BOnly != comparison.BOnly {
		t.Fatalf("loaded comparison mismatch: got matches=%d, want=%d", loaded.Matches, comparison.Matches)
	}
}
