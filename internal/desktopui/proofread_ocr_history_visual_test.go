package desktopui

import (
	"image"
	"image/color"
	"runtime"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/widget/material"
)

func renderProofreadingHistory(t *testing.T, gtx layout.Context, draw func(layout.Context)) *image.RGBA {
	t.Helper()
	window, err := headless.NewWindow(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)
	if err != nil {
		t.Skipf("headless GPU unavailable: %v", err)
	}
	defer window.Release()
	gtx.Ops.Reset()
	paint.Fill(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	draw(gtx)
	if err := window.Frame(gtx.Ops); err != nil {
		t.Fatal(err)
	}
	shot := image.NewRGBA(image.Rectangle{Max: gtx.Constraints.Max})
	if err := window.Screenshot(shot); err != nil {
		t.Fatal(err)
	}
	return shot
}

func proofreadingHistoryInkBounds(shot *image.RGBA, region image.Rectangle, green bool) image.Rectangle {
	var bounds image.Rectangle
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			p := shot.RGBAAt(x, y)
			marked := int(p.R) > int(p.G)+40 && int(p.R) > int(p.B)+40
			if green {
				marked = int(p.G) > int(p.R)+40 && int(p.G) > int(p.B)+40
			}
			if marked {
				bounds = bounds.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return bounds
}

func TestProofreadingNoDiffBadgeStaysWithinPaper(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a, gtx, comparison := proofreadingConnectorFixture(1)
	gtx.Constraints.Min = image.Point{}
	comparison.Frames = nil
	comparison.HasEvidence, comparison.Complete = true, true
	comparison.AOnly, comparison.BOnly = 0, 0
	a.proofreading.ocrStep.comparisons[proofreadingOCRStepPair{APage: 3, BPage: 3}] = comparison
	region := image.Rect(400, 44, 560, 300)
	before := proofreadingHistoryInkBounds(renderProofreadingHistory(t, gtx, a.drawProofreadingOCRStepConnector), region, true)
	a.proofreading.xPageScrollOffsets[3] = 25
	afterText := proofreadingHistoryInkBounds(renderProofreadingHistory(t, gtx, a.drawProofreadingOCRStepConnector), region, true)
	if before.Empty() || afterText != before {
		t.Fatalf("no-difference badge moved with text: %v -> %v", before, afterText)
	}
	a.pdfView.canvas.Position.Offset = 25
	a.proofreading.xCanvas.Position.Offset = 25
	afterPaper := proofreadingHistoryInkBounds(renderProofreadingHistory(t, gtx, a.drawProofreadingOCRStepConnector), region, true)
	if afterPaper != before.Sub(image.Pt(0, 25)) {
		t.Fatalf("no-difference badge detached from its paper: %v -> %v", before, afterPaper)
	}
}

func TestProofreadingManualHistoryDrawsBothTextBoxes(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, selected := range []bool{true, false} {
		a, gtx, comparison := proofreadingConnectorFixture(1)
		a.proofreading.aiProofreading = false
		if !selected {
			a.pdfView.page = 0
		}
		comparison.Frames = comparison.Frames[1:2]
		comparison.Frames[0].Kind = proofreadingOCRDiffReplace
		a.proofreading.ocrStep.comparisons[proofreadingOCRStepPair{APage: 3, BPage: 3}] = comparison
		a.proofreading.ocrStep.pageSourceHash = map[int]string{3: "source"}
		a.proofreading.xPageRenderScales = make([]float64, 4)
		a.proofreading.xPageViewportHeights = make([]float64, 4)
		shot := renderProofreadingHistory(t, gtx, func(c layout.Context) {
			for _, source := range []bool{true, false} {
				x := 20
				if !source {
					x = 420
				}
				position := op.Offset(image.Pt(x, 20)).Push(c.Ops)
				child := c
				child.Constraints = layout.Exact(image.Pt(300, 120))
				if source {
					a.drawProofreadingOCRPageOverlay(child, image.Pt(300, 120), 3)
				} else {
					a.drawProofreadingXContent(child, a.proofreading.xPages[3], image.Pt(300, 120))
				}
				position.Pop()
			}
		})
		for _, region := range []image.Rectangle{image.Rect(20, 20, 320, 140), image.Rect(420, 20, 720, 140)} {
			if proofreadingHistoryInkBounds(shot, region, false).Empty() {
				t.Fatalf("selected=%v: manual history omitted a difference text box in %v", selected, region)
			}
		}
	}
}

func TestProofreadingVisibleHistoryKeepsConnectorLines(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a, gtx, comparison := proofreadingConnectorFixture(1)
	s, step := a.proofreading, a.proofreading.ocrStep
	a.pdfView.pageMap = []int{2, 3}
	s.xPages[2] = s.xPages[3]
	s.xPages[2].PageIndex = 2
	s.xEditorWindows[2] = s.xEditorWindows[3]
	s.xPageEditors[2].SetText(s.xPages[2].Content)
	shape := gtx
	shape.Constraints = layout.Constraints{Max: image.Pt(240, 1<<20)}
	style := material.Editor(a.theme, &s.xPageEditors[2], "")
	style.TextSize = 12
	style.LineHeightScale = proofreadingXEditorLineHeightScale
	style.Layout(shape)
	step.pages[2] = step.pages[3]
	comparison.APage, comparison.BPage = 2, 2
	step.comparisons[proofreadingOCRStepPair{APage: 2, BPage: 2}] = comparison
	shot := renderProofreadingHistory(t, gtx, a.drawProofreadingOCRStepConnector)
	if proofreadingHistoryInkBounds(shot, image.Rect(405, 60, 465, 170), false).Empty() {
		t.Fatal("visible history page lost its connector because another page was selected")
	}
}
