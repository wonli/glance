package desktopui

import (
	"image"
	"image/color"
	"runtime"
	"testing"
	"time"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/textalign"
)

func TestProofreadingOCRHighlightClipsToEditorWindow(t *testing.T) {
	segment := proofreadingOCRRangeSegment{PageIndex: 2, Start: 108, End: 130}
	window := proofreadingEditorWindow{Start: 120, End: 128}
	got, ok := proofreadingOCRHighlightLocalRange(segment, window)
	if !ok || got != (textalign.Range{Start: 0, End: 8}) {
		t.Fatalf("visible highlight = %+v, ok=%v; want editor-local [0,8)", got, ok)
	}
	if _, ok := proofreadingOCRHighlightLocalRange(segment, proofreadingEditorWindow{Start: 130, End: 140}); ok {
		t.Fatal("fully off-window OCR evidence produced a highlight")
	}
}

func TestProofreadingOCRHighlightRejectsStaleOwnershipAndGeneration(t *testing.T) {
	const draft = "甲乙丙丁"
	scan := &proofreadingOCRScan{
		generation:          12,
		markdownSnapshot:    draft,
		markdownSnapshotSet: true,
		sourceAHash:         "source-A",
		sourceBHash:         "source-B",
		pages: []proofreadingOCRPage{{result: textalign.Result{
			Page: 1, Status: "matched",
			Selected: &textalign.Candidate{Evidence: []textalign.Evidence{{BlockID: 7, Range: textalign.Range{Start: 1, End: 3}}}},
		}}},
	}
	state := &proofreadingWorkspace{aSourceHash: "source-A", sourceBHash: "source-B", bDraftText: draft}
	highlight := proofreadingOCRHighlightState{
		Active: true, APage: 0, BlockID: 7, BRange: textalign.Range{Start: 1, End: 3},
		SourceAHash: "source-A", SourceBHash: "source-B", OCRGeneration: 12, ModelRevision: 8,
	}
	if !proofreadingOCRHighlightCurrent(highlight, scan, state, 8) {
		t.Fatal("current highlight was rejected")
	}
	for _, test := range []struct {
		name   string
		mutate func(*proofreadingOCRHighlightState, *proofreadingOCRScan, *proofreadingWorkspace) uint64
	}{
		{name: "B draft changed", mutate: func(_ *proofreadingOCRHighlightState, _ *proofreadingOCRScan, s *proofreadingWorkspace) uint64 {
			s.bDraftText = "甲乙修改"
			return 8
		}},
		{name: "model ownership revision changed", mutate: func(_ *proofreadingOCRHighlightState, _ *proofreadingOCRScan, _ *proofreadingWorkspace) uint64 {
			return 9
		}},
		{name: "OCR generation changed", mutate: func(_ *proofreadingOCRHighlightState, s *proofreadingOCRScan, _ *proofreadingWorkspace) uint64 {
			s.generation++
			return 8
		}},
		{name: "PDF source changed", mutate: func(_ *proofreadingOCRHighlightState, _ *proofreadingOCRScan, s *proofreadingWorkspace) uint64 {
			s.aSourceHash = "new-source"
			return 8
		}},
		{name: "OCR source changed", mutate: func(_ *proofreadingOCRHighlightState, s *proofreadingOCRScan, _ *proofreadingWorkspace) uint64 {
			s.sourceAHash = "new-source"
			return 8
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			currentHighlight, currentScan := highlight, *scan
			currentState := *state
			revision := test.mutate(&currentHighlight, &currentScan, &currentState)
			if proofreadingOCRHighlightCurrent(currentHighlight, &currentScan, &currentState, revision) {
				t.Fatal("stale OCR highlight remained active")
			}
		})
	}
}

func TestProofreadingOCRLinkedRangeVisual(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	const draft = "甲乙丙丁戊己庚辛壬癸"
	runes := []rune(draft)
	matchedRange := textalign.Range{Start: 2, End: 8}
	state := &proofreadingWorkspace{
		aSourceHash:    "source-A",
		sourceBHash:    "source-B",
		bDraftText:     draft,
		xPages:         []proofreadingXPage{{PageIndex: 0, ContentStart: 0, ContentEnd: len(runes)}},
		xEditorWindows: []proofreadingEditorWindow{{Initialized: true, Start: 0, End: len(runes)}},
		xPageEditors:   []widget.Editor{{}},
	}
	state.xPageEditors[0].SetText(draft)
	state.ocrScan = &proofreadingOCRScan{
		generation:          17,
		markdownSnapshot:    draft,
		markdownSnapshotSet: true,
		sourceAHash:         "source-A",
		sourceBHash:         "source-B",
		pages: []proofreadingOCRPage{{result: textalign.Result{
			Page: 1, Status: "matched",
			Selected: &textalign.Candidate{Evidence: []textalign.Evidence{{BlockID: 4, Range: matchedRange}}},
		}}},
		highlight: proofreadingOCRHighlightState{
			Active: true, APage: 0, BlockID: 4, BRange: matchedRange,
			SourceAHash: "source-A", SourceBHash: "source-B", OCRGeneration: 17,
		},
	}
	app := &App{theme: material.NewTheme(), proofreading: state}
	page := state.xPages[0]
	pageSize := image.Pt(62, 92)
	window, err := headless.NewWindow(pageSize.X, pageSize.Y)
	if err != nil {
		t.Skipf("headless GPU is unavailable: %v", err)
	}
	defer window.Release()
	var ops op.Ops
	render := func(highlight bool) (*image.RGBA, []image.Rectangle) {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Constraints: layout.Exact(pageSize), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
		paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: pageSize}.Op())
		editor := &state.xPageEditors[0]
		editor.SingleLine = false
		editor.WrapPolicy = text.WrapWords
		style := material.Editor(app.theme, editor, "")
		style.TextSize = unit.Sp(16)
		style.Color = proofreadInk
		style.LineHeightScale = 1.28
		style.Layout(gtx)
		regions := editor.Regions(matchedRange.Start, matchedRange.End, nil)
		bounds := make([]image.Rectangle, len(regions))
		for i := range regions {
			bounds[i] = regions[i].Bounds
		}
		if highlight {
			app.drawProofreadingOCRMatchHighlight(gtx, editor, page, state.xEditorWindows[0])
		}
		if err := window.Frame(&ops); err != nil {
			t.Fatalf("render proofreading OCR linked range: %v", err)
		}
		shot := image.NewRGBA(image.Rect(0, 0, pageSize.X, pageSize.Y))
		if err := window.Screenshot(shot); err != nil {
			t.Fatalf("capture proofreading OCR linked range: %v", err)
		}
		return shot, bounds
	}

	base, baseRegions := render(false)
	marked, markedRegions := render(true)
	if len(baseRegions) < 2 || len(markedRegions) < 2 {
		t.Fatalf("matched range regions = %d / %d; want the range to wrap across at least two lines", len(baseRegions), len(markedRegions))
	}
	if got := state.xPageEditors[0].Text(); got != draft {
		t.Fatalf("OCR highlight changed B draft: got %q, want %q", got, draft)
	}
	changedRegions, readableLines := 0, 0
	for _, region := range markedRegions {
		bounds := region.Intersect(image.Rect(0, 0, pageSize.X, pageSize.Y))
		changed, readableInk := false, false
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				before := color.NRGBAModel.Convert(base.At(x, y)).(color.NRGBA)
				after := color.NRGBAModel.Convert(marked.At(x, y)).(color.NRGBA)
				if before != after && int(after.G) > int(after.R)+8 {
					changed = true
				}
				if after.R < 175 && after.G < 190 && after.B < 180 {
					readableInk = true
				}
			}
		}
		if changed {
			changedRegions++
		}
		if readableInk {
			readableLines++
		}
	}
	if changedRegions < 2 || readableLines < 2 {
		t.Fatalf("wrapped match overlay changed/readable lines = %d/%d; want translucent overlay and visible glyph ink on both wrapped lines", changedRegions, readableLines)
	}
}

func TestProofreadingOCRLaserVisual(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		t.Run(map[float32]string{1: "1x", 2: "2x"}[scale], func(t *testing.T) {
			size := image.Pt(int(320*scale), int(480*scale))
			window, err := headless.NewWindow(size.X, size.Y)
			if err != nil {
				t.Skipf("headless GPU is unavailable: %v", err)
			}
			defer window.Release()
			now := time.Now()
			started := now.Add(-725 * time.Millisecond)
			var ops op.Ops
			paint.FillShape(&ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: size}.Op())
			gtx := layout.Context{Ops: &ops, Now: now, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
			drawProofreadingLaser(gtx, size, started)
			if err := window.Frame(&ops); err != nil {
				t.Fatalf("render OCR laser at %gx: %v", scale, err)
			}
			shot := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
			if err := window.Screenshot(shot); err != nil {
				t.Fatalf("capture OCR laser at %gx: %v", scale, err)
			}
			expectedY := int((float64(now.Sub(started)) / float64(1450*time.Millisecond)) * float64(size.Y))
			blue := 0
			for y := max(0, expectedY-3*int(scale)); y < min(size.Y, expectedY+3*int(scale)); y++ {
				for x := 0; x < size.X; x++ {
					pixel := color.NRGBAModel.Convert(shot.At(x, y)).(color.NRGBA)
					if pixel.B > 200 && pixel.R < 80 && pixel.G > 80 && pixel.G < 160 {
						blue++
					}
				}
			}
			if blue < size.X/2 {
				t.Fatalf("laser was not rendered around its progress position at %gx: blue pixels=%d", scale, blue)
			}
		})
	}
}
