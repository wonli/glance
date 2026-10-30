package desktopui

import (
	"image"
	"image/color"
	"math"
	"runtime"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/gpu/headless"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/ocr"
)

func TestProofreadingOCRConnectorSparsePageAnchor(t *testing.T) {
	for _, pageIndex := range []int{3, 891} {
		step := &proofreadingOCRStepState{pages: map[int]proofreadingOCRPage{
			pageIndex: {layout: ocr.Layout{Width: 100, Height: 200, Blocks: []ocr.LayoutBlock{{ID: 7, Bounds: [4]float64{10, 20, 90, 100}}}}},
		}}
		got, ok := proofreadingOCRStepSourceAnchor(step, proofreadingOCRStepComparison{APage: pageIndex}, 7, image.Pt(200, 400))
		if !ok || got != image.Pt(180, 120) {
			t.Fatalf("page %d, one cached page: anchor=%v valid=%v; want (180,120)", pageIndex+1, got, ok)
		}
		if _, ok := proofreadingOCRStepSourceAnchor(step, proofreadingOCRStepComparison{APage: pageIndex + 1}, 7, image.Pt(200, 400)); ok {
			t.Fatal("missing cached page produced an anchor")
		}
	}
}

func proofreadingConnectorFixture(scale float32) (*App, layout.Context, proofreadingOCRStepComparison) {
	const pageIndex = 3
	content := strings.Repeat("abcd\n", 12)
	count := len([]rune(content))
	theme := material.NewTheme()
	state := &proofreadingWorkspace{
		aiProofreading: true, aSourceHash: "source", bDraftText: content,
		xPages: make([]proofreadingXPage, 4), xPageEditors: make([]widget.Editor, 4),
		xEditorWindows: make([]proofreadingEditorWindow, 4), xPageScrollOffsets: make([]float64, 4),
	}
	state.xPages[pageIndex] = proofreadingXPage{PageIndex: pageIndex, Content: content, ContentStart: 0, ContentEnd: count, Width: 300, Height: 120}
	state.xEditorWindows[pageIndex] = proofreadingEditorWindow{Initialized: true, End: count}
	editor := &state.xPageEditors[pageIndex]
	editor.SetText(content)
	var ops op.Ops
	shapeContext := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(240, 1<<20)}}
	style := material.Editor(theme, editor, "")
	style.TextSize = 12
	style.LineHeightScale = proofreadingXEditorLineHeightScale
	style.Layout(shapeContext)
	ops.Reset()
	comparison := proofreadingOCRStepComparison{APage: pageIndex, BPage: pageIndex, SourceHash: "source", DraftHash: proofreadingOCRDraftSnapshotHash(content), BText: content, BEnd: count}
	for index, current := range []rune(content) {
		if current != '\n' {
			kind := proofreadingOCRDiffEqual
			if index%5 == 1 || index%5 == 2 {
				kind = []proofreadingOCRDiffOpKind{proofreadingOCRDiffReplace, proofreadingOCRDiffAOnly, proofreadingOCRDiffBOnly}[(index/5)%3]
			}
			comparison.Frames = append(comparison.Frames, proofreadingOCRDiffFrame{Kind: kind, ABlockID: 7, AOffset: index, BPosition: index, AChar: current, BChar: current})
		}
	}
	state.ocrStep = &proofreadingOCRStepState{
		badgeConnections: make(map[proofreadingOCRStepPair][]proofreadingOCRConnection),
		pages:            map[int]proofreadingOCRPage{pageIndex: {layout: ocr.Layout{Width: 300, Height: 120, Blocks: []ocr.LayoutBlock{{ID: 7, Bounds: [4]float64{20, 20, 250, 100}}}}}},
		comparisons:      map[proofreadingOCRStepPair]proofreadingOCRStepComparison{{APage: pageIndex, BPage: pageIndex}: comparison},
		diffPair:         proofreadingOCRStepPair{APage: pageIndex, BPage: pageIndex}, diffDraftHash: comparison.DraftHash, diffCursor: len(comparison.Frames),
	}
	localized := state.ocrStep.pages[pageIndex]
	localized.layout.CharacterVersion = ocr.CharacterGeometryVersion
	localized.layout.Blocks[0].Text = content
	for index, r := range []rune(content) {
		if r == '\n' {
			continue
		}
		x, y := float64(20+(index%5)*10), float64(10+(index/5)*8)
		localized.layout.Characters = append(localized.layout.Characters, ocr.CharacterBox{BlockID: 7, Offset: index, Text: string(r), Bounds: [4]float64{x, y, x + 8, y + 6}})
	}
	state.ocrStep.pages[pageIndex] = localized
	view := &pdfViewState{info: document.PDFInfo{Pages: 4}, page: pageIndex, pageMap: []int{pageIndex}, scale: float64(scale), pageSizeHintW: 300, pageSizeHintH: 120}
	a := &App{theme: theme, proofreading: state, documentSessionState: &documentSessionState{pdfView: view, sharedDocumentSession: &sharedDocumentSession{}}}
	gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(image.Pt(int(1000*scale), int(500*scale)))}
	return a, gtx, comparison
}

func TestProofreadingOCRConnectorRetainsScannedRowsAfterScroll(t *testing.T) {
	a, gtx, comparison := proofreadingConnectorFixture(1)
	step := a.proofreading.ocrStep
	before := proofreadingOCRStepConnections(gtx, a.pdfView, a.proofreading, step, comparison)
	if len(before) < 2 || len(before) >= len(comparison.Frames) {
		t.Fatalf("completed scan must retain visible rows and merge characters on a row: %d connections", len(before))
	}
	for _, connection := range before {
		if connection.active {
			t.Fatal("last scanned character should be outside the small page's viewport in this fixture")
		}
	}
	a.proofreading.xPageScrollOffsets[3] = 7.25
	after := proofreadingOCRStepConnections(gtx, a.pdfView, a.proofreading, step, comparison)
	if len(after) < 2 || before[0].to == after[0].to {
		t.Fatalf("retained connections did not follow X text scrolling: before=%v after=%v", before, after)
	}
	step.diffCursor = 2
	step.diffRunning = true
	if got := proofreadingOCRStepConnections(gtx, a.pdfView, a.proofreading, step, comparison); len(got) != 1 || !got[0].active {
		t.Fatalf("scan must reveal only processed characters: %v", got)
	}
	step.diffCursor = 0
	if got := proofreadingOCRStepConnections(gtx, a.pdfView, a.proofreading, step, comparison); len(got) != 0 {
		t.Fatalf("replay did not clear the revealed connections: %v", got)
	}
}

func TestProofreadingOCRConnectorCrossPanePixels(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		a, gtx, _ := proofreadingConnectorFixture(scale)
		var router input.Router
		gtx.Source = router.Source()
		window, err := headless.NewWindow(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)
		if err != nil {
			t.Skipf("headless GPU unavailable: %v", err)
		}
		a.drawProofreadingOCRStepConnector(gtx)
		router.Frame(gtx.Ops)
		badges := a.proofreading.ocrStep.currentBadges
		if len(badges) == 0 {
			window.Release()
			t.Fatal("no badge to hover")
		}
		b := badges[0].rect
		router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(float32((b.Min.X+b.Max.X)/2), float32((b.Min.Y+b.Max.Y)/2))})
		gtx.Ops.Reset()
		paint.Fill(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		a.drawProofreadingOCRStepConnector(gtx)
		if err := window.Frame(gtx.Ops); err != nil {
			window.Release()
			t.Fatal(err)
		}
		shot := image.NewRGBA(image.Rectangle{Max: gtx.Constraints.Max})
		err = window.Screenshot(shot)
		window.Release()
		if err != nil {
			t.Fatal(err)
		}
		colored := 0
		for y := int(44 * scale); y < gtx.Constraints.Max.Y; y++ {
			for x := int(350 * scale); x < int(470*scale); x++ {
				pixel := shot.RGBAAt(x, y)
				if int(pixel.R) > int(pixel.G)+10 || int(pixel.B) > int(pixel.R)+10 {
					colored++
				}
			}
		}
		if colored == 0 {
			t.Fatalf("%gx: hovered fourth-page badge drew no difference line", scale)
		}
	}
}

func TestProofreadingOCRConnectorDifferencesOnlyAndStableRunAnchor(t *testing.T) {
	a, gtx, comparison := proofreadingConnectorFixture(1)
	step := a.proofreading.ocrStep
	for _, connection := range proofreadingOCRStepConnections(gtx, a.pdfView, a.proofreading, step, comparison) {
		if connection.kind == proofreadingOCRDiffEqual {
			t.Fatal("identical text produced a connector")
		}
	}
	prefix := comparison
	prefix.Frames = comparison.Frames[:2]
	first := proofreadingOCRStepConnections(gtx, a.pdfView, a.proofreading, step, prefix)
	prefix.Frames = comparison.Frames[:3]
	next := proofreadingOCRStepConnections(gtx, a.pdfView, a.proofreading, step, prefix)
	if len(first) != 1 || len(next) != 1 || first[0].to != next[0].to {
		t.Fatalf("advancing within the same difference moved its anchor: %v -> %v", first, next)
	}
	for index := range comparison.Frames {
		comparison.Frames[index].Kind = proofreadingOCRDiffEqual
	}
	step.diffCursor = len(comparison.Frames)
	if got := proofreadingOCRStepConnections(gtx, a.pdfView, a.proofreading, step, comparison); len(got) != 0 {
		t.Fatalf("identical page produced %d connectors", len(got))
	}
}

func TestProofreadingOCRConnectorPageCoordinatesSurviveZoomAndPan(t *testing.T) {
	for _, deviceScale := range []float32{1, 2} {
		a, gtx, comparison := proofreadingConnectorFixture(deviceScale)
		state, step := a.proofreading, a.proofreading.ocrStep
		frame := comparison.Frames[1]
		logical, ok := proofreadingOCRStepDraftPageAnchor(state, 3, frame)
		if !ok {
			t.Fatal("missing draft page anchor")
		}
		source, ok := proofreadingOCRStepSourcePageAnchor(step, comparison, 7)
		if !ok || source.X != 250.0/300 || source.Y != 0.5 {
			t.Fatalf("OCR page anchor=%+v valid=%v", source, ok)
		}
		for _, zoom := range []float64{0.75, 1, 1.25} {
			a.pdfView.scale = float64(deviceScale) * zoom
			size := proofreadingXPagePixelSize(gtx, a.pdfView, state, 3)
			for _, scroll := range []float64{0, 7.25} {
				state.xPageScrollOffsets[3] = scroll
				point, ok := proofreadingOCRStepDraftPageAnchor(state, 3, frame)
				if !ok || point != logical {
					t.Fatalf("zoom/scroll changed document page point: %+v -> %+v", logical, point)
				}
				geometry := proofreadingXPageTextGeometry(state.xPages[3], size, state.xEditorWindows[3].Top, scroll)
				pixel := geometry.pixelPoint(point)
				inverseX := float64(pixel.X-geometry.Content.Min.X)/geometry.Scale + geometry.MarginX
				inverseY := float64(pixel.Y-geometry.Content.Min.Y)/geometry.Scale + geometry.MarginY + geometry.WindowTop - float64(geometry.WindowShiftY)
				if math.Abs(inverseX-point.X) > 0.51/geometry.Scale || math.Abs(inverseY-point.Y) > 0.51/geometry.Scale {
					t.Fatalf("%gx zoom %.2f scroll %.2f: projected point drifted", deviceScale, zoom, scroll)
				}
			}
		}
		a.pdfView.scale = float64(deviceScale)
		state.xPageScrollOffsets[3] = 0
		step.diffCursor = 2
		step.diffRunning = true
		before := proofreadingOCRStepConnections(gtx, a.pdfView, state, step, comparison)
		pan := int(9 * deviceScale)
		a.pdfView.horizontal.Position.Offset = pan
		a.pdfView.canvas.Position.Offset = pan
		state.xCanvas.Position.Offset = pan
		after := proofreadingOCRStepConnections(gtx, a.pdfView, state, step, comparison)
		if len(before) != 1 || len(after) != 1 || after[0].from != before[0].from.Sub(image.Pt(pan, pan)) || after[0].to != before[0].to.Sub(image.Pt(0, pan)) {
			t.Fatalf("%gx: canvas pan did not preserve page anchors: %v -> %v", deviceScale, before, after)
		}
	}
}

func TestProofreadingOCRConnectorPagePointSurvivesTextWindowAndReflow(t *testing.T) {
	a, gtx, comparison := proofreadingConnectorFixture(1)
	state := a.proofreading
	frame := comparison.Frames[9] // Second character of the third explicit line.
	before, ok := proofreadingOCRStepDraftPageAnchor(state, 3, frame)
	if !ok {
		t.Fatal("missing anchor before changing the virtual text window")
	}
	editor := &state.xPageEditors[3]
	first := editor.Regions(0, 1, nil)[0].Bounds
	third := editor.Regions(10, 11, nil)[0].Bounds
	state.xEditorWindows[3].Start = 10
	state.xEditorWindows[3].Top = float64(third.Min.Y - first.Min.Y)
	editor.SetText(string([]rune(state.xPages[3].Content)[10:]))
	shape := func(size unit.Sp) {
		child := gtx
		child.Constraints = layout.Constraints{Max: image.Pt(240, 1<<20)}
		style := material.Editor(a.theme, editor, "")
		style.TextSize = size
		style.LineHeightScale = proofreadingXEditorLineHeightScale
		style.Layout(child)
	}
	shape(12)
	after, ok := proofreadingOCRStepDraftPageAnchor(state, 3, frame)
	if !ok || before != after {
		t.Fatalf("virtual text window moved the page-relative character: %+v -> %+v valid=%v", before, after, ok)
	}
	shape(24)
	reflowed, ok := proofreadingOCRStepDraftPageAnchor(state, 3, frame)
	if !ok || reflowed.X <= after.X {
		t.Fatalf("font enlargement did not obtain fresh character geometry: %+v -> %+v valid=%v", after, reflowed, ok)
	}
}

func TestProofreadingOCRConnectorWithoutCharacterGeometry(t *testing.T) {
	a, gtx, comparison := proofreadingConnectorFixture(1)
	step := a.proofreading.ocrStep
	page := step.pages[comparison.APage]
	page.layout.Characters = nil
	page.layout.CharacterVersion = 0
	step.pages[comparison.APage] = page
	a.drawProofreadingOCRStepConnector(gtx)
	if len(step.currentBadges) == 0 || len(step.currentConnections) == 0 {
		t.Fatal("block geometry must preserve badges when character localization is unavailable")
	}
	live := proofreadingOCRStepConnections(gtx, a.pdfView, a.proofreading, step, comparison)
	if len(live) == 0 {
		t.Fatal("right-side connectors must remain when PDF character positions are unavailable")
	}
	for _, connection := range live {
		if !connection.fromUnlocated {
			t.Fatal("unlocated PDF character has a source endpoint")
		}
		anchor, ok := proofreadingOCRStepDraftPageAnchor(a.proofreading, comparison.BPage, connection.frame)
		if !ok {
			t.Fatal("missing draft glyph location")
		}
		_, origin, _ := proofreadingOCRStepConnectorPageOrigins(gtx, a.pdfView, a.proofreading, comparison.APage, comparison.BPage)
		size := proofreadingXPagePixelSize(gtx, a.pdfView, a.proofreading, comparison.BPage)
		window := a.proofreading.xEditorWindows[comparison.BPage]
		geometry := proofreadingXPageTextGeometry(a.proofreading.xPages[comparison.BPage], size, window.Top, a.proofreading.xPageScrollOffsets[comparison.BPage])
		if want := origin.Add(geometry.pixelPoint(anchor)); connection.toOutside == 0 && connection.to != want {
			t.Fatalf("right endpoint = %v, want %v", connection.to, want)
		}
	}

}

func TestProofreadingPaginationCompactHeight(t *testing.T) {
	a, gtx, _ := proofreadingConnectorFixture(1)
	gtx.Constraints.Min = image.Point{}
	if got := a.layoutProofreadingPagination(gtx).Size.Y; got != 70 {
		t.Fatalf("pagination height = %d, want 70", got)
	}
	if got := a.layoutProofreadingEmptyFilmstrip(gtx).Size.Y; got != 48 {
		t.Fatalf("empty filmstrip height = %d, want 48", got)
	}
}

func TestProofreadingOCRConnectorKeepsDifferencesWithoutIndividualBoxes(t *testing.T) {
	a, gtx, comparison := proofreadingConnectorFixture(1)
	step := a.proofreading.ocrStep
	comparison.Frames = nil
	for _, i := range []int{1, 6, 11, 16, 21} {
		comparison.Frames = append(comparison.Frames, proofreadingOCRDiffFrame{Kind: proofreadingOCRDiffReplace, ABlockID: 7, AOffset: i, BPosition: i, AChar: 'b', BChar: 'b'})
	}
	page := step.pages[comparison.APage]
	var boxes []ocr.CharacterBox
	for _, box := range page.layout.Characters {
		if box.Offset != 6 && box.Offset != 16 {
			boxes = append(boxes, box)
		}
	}
	page.layout.Characters = boxes
	step.pages[comparison.APage] = page
	pair := proofreadingOCRStepPair{APage: comparison.APage, BPage: comparison.BPage}
	step.comparisons[pair] = comparison
	step.diffCursor = len(comparison.Frames)
	a.drawProofreadingOCRStepConnector(gtx)
	if len(step.currentBadges) != 5 {
		t.Fatalf("got %d badges for five separate differences", len(step.currentBadges))
	}
	live := proofreadingOCRStepConnections(gtx, a.pdfView, a.proofreading, step, comparison)
	if len(live) != 5 {
		t.Fatalf("got %d visible connections for five differences", len(live))
	}
	for _, frame := range []proofreadingOCRDiffFrame{comparison.Frames[1], comparison.Frames[3]} {
		if _, ok := proofreadingOCRStepSourceCharacterBounds(step, comparison, frame, image.Pt(300, 120)); ok {
			t.Fatal("missing glyph was presented as a character box")
		}
	}
}

func TestProofreadingHoveredConnectionSurvivesDraftScroll(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		a, gtx, comparison := proofreadingConnectorFixture(scale)
		comparison.Frames = []proofreadingOCRDiffFrame{comparison.Frames[9]}
		comparison.Frames[0].Kind = proofreadingOCRDiffReplace
		state := a.proofreading
		state.xPageScrollOffsets[3] = 200
		connections := proofreadingOCRStepConnections(gtx, a.pdfView, state, state.ocrStep, comparison)
		if len(connections) != 1 {
			t.Fatalf("%gx: missing connection after X text scrolled out of view", scale)
		}
		if connections[0].toOutside != -1 {
			t.Fatalf("%gx: scrolled-up text should point above viewport: %+v", scale, connections[0])
		}
		state.xPageScrollOffsets[3] = 0
		comparison.Frames[0] = proofreadingOCRDiffFrame{Kind: proofreadingOCRDiffReplace, ABlockID: 7, AOffset: 46, BPosition: 46, AChar: 'b', BChar: 'b'}
		connections = proofreadingOCRStepConnections(gtx, a.pdfView, state, state.ocrStep, comparison)
		if len(connections) != 1 || connections[0].toOutside != 1 {
			t.Fatalf("%gx: hidden text below viewport must have downward direction: %+v", scale, connections)
		}
		state.xEditorWindows[3].Start = 50
		connections = proofreadingOCRStepConnections(gtx, a.pdfView, state, state.ocrStep, comparison)
		if len(connections) != 1 || connections[0].toOutside != -1 {
			t.Fatalf("%gx: virtualized text above window lost direction: %+v", scale, connections)
		}
		state.xCanvas.Position.Offset = int(400 * scale)
		connections = proofreadingOCRStepConnections(gtx, a.pdfView, state, state.ocrStep, comparison)
		if len(connections) != 1 || connections[0].toOutside != -1 {
			t.Fatalf("%gx: X page above viewport lost direction: %+v", scale, connections)
		}
		a.pdfView.info.Pages = 5
		a.pdfView.pageMap = nil
		a.pdfView.canvas.Position.First = 3
		state.xCanvas.Position.First = 4
		state.xCanvas.Position.Offset = 0
		connections = proofreadingOCRStepConnections(gtx, a.pdfView, state, state.ocrStep, comparison)
		if len(connections) != 1 || connections[0].toOutside != -1 {
			t.Fatalf("%gx: X page before the first visible row lost direction: %+v", scale, connections)
		}
	}
}
