package desktopui

import (
	"image"
	"image/color"
	"runtime"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func pdfMergeSplitLayoutContext(ops *op.Ops, router *input.Router, scale float32, size image.Point) layout.Context {
	return layout.Context{
		Ops:         ops,
		Source:      router.Source(),
		Now:         time.Now(),
		Metric:      unit.Metric{PxPerDp: scale, PxPerSp: scale},
		Constraints: layout.Exact(size),
	}
}

func TestPDFMergeSplitToolNavigation(t *testing.T) {
	app := New(nil, "")
	app.homeSection = homeSectionTools
	app.pdfMergeSplitToolPage = false
	var ops op.Ops
	var router input.Router
	gtx := pdfMergeSplitLayoutContext(&ops, &router, 1, image.Pt(1000, 700))

	entryFound := false
	for _, entry := range homeToolCatalog {
		if entry.id == "pdf-merge-split" {
			entryFound = true
			break
		}
	}
	if !entryFound {
		t.Fatal("PDF merge/split card is missing from the home tools catalog")
	}
	app.homeNavButton("tools-pdf-merge-split").Click()
	app.layoutHomeToolsPDFMergeSplitCard(gtx)
	if !app.pdfMergeSplitToolPage {
		t.Fatal("PDF merge/split card click did not open its workbench route")
	}
	app.pdfMergeSplitTool.backButton.Click()
	app.layoutHomeContentPage(gtx)
	if app.pdfMergeSplitToolPage {
		t.Fatal("workbench back action did not return to the tools catalog")
	}
}

func TestPDFMergeSplitToolModeSwitch(t *testing.T) {
	app := New(nil, "")
	app.pdfMergeSplitToolPage = true
	state := &app.pdfMergeSplitTool
	state.mergeDraft.outputName.SetText("joined.pdf")
	state.splitDraft.rangeInput.SetText("1-3, 7")
	state.splitDraft.prefix.SetText("extract")
	var ops op.Ops
	var router input.Router
	gtx := pdfMergeSplitLayoutContext(&ops, &router, 1, image.Pt(1000, 700))

	state.splitModeButton.Click()
	app.layoutPDFMergeSplitTool(gtx)
	if state.mode != pdfMergeSplitModeSplit {
		t.Fatalf("mode after split click = %v, want split", state.mode)
	}
	state.mergeModeButton.Click()
	app.layoutPDFMergeSplitTool(gtx)
	if state.mode != pdfMergeSplitModeMerge {
		t.Fatalf("mode after merge click = %v, want merge", state.mode)
	}
	if state.mergeDraft.outputName.Text() != "joined.pdf" || state.splitDraft.rangeInput.Text() != "1-3, 7" || state.splitDraft.prefix.Text() != "extract" {
		t.Fatalf("mode switch lost an independent draft: merge=%q range=%q prefix=%q", state.mergeDraft.outputName.Text(), state.splitDraft.rangeInput.Text(), state.splitDraft.prefix.Text())
	}
}

func TestPDFMergeSplitToolStableState(t *testing.T) {
	app := New(nil, "")
	state := &app.pdfMergeSplitTool
	mergeEditor := &state.mergeDraft.outputName
	rangeEditor := &state.splitDraft.rangeInput
	prefixEditor := &state.splitDraft.prefix
	backButton := &state.backButton
	mergeButton := &state.mergeModeButton
	list := &state.mergeDraft.fileList
	var ops op.Ops
	var router input.Router
	gtx := pdfMergeSplitLayoutContext(&ops, &router, 1, image.Pt(1000, 700))
	app.pdfMergeSplitToolPage = true
	app.layoutPDFMergeSplitTool(gtx)
	ops.Reset()
	app.layoutPDFMergeSplitTool(gtx)
	if mergeEditor != &app.pdfMergeSplitTool.mergeDraft.outputName || rangeEditor != &app.pdfMergeSplitTool.splitDraft.rangeInput || prefixEditor != &app.pdfMergeSplitTool.splitDraft.prefix || backButton != &app.pdfMergeSplitTool.backButton || mergeButton != &app.pdfMergeSplitTool.mergeModeButton || list != &app.pdfMergeSplitTool.mergeDraft.fileList {
		t.Fatal("PDF workbench replaced a persistent editor, list or clickable across layout calls")
	}
}

func TestPDFMergeSplitToolVisual(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, scale := range []float32{1, 2} {
		app := New(nil, "")
		var ops op.Ops
		var router input.Router
		cardSize := image.Pt(int(720*scale), int(200*scale))
		gtx := pdfMergeSplitLayoutContext(&ops, &router, scale, cardSize)
		dims := app.layoutHomeToolsPDFMergeSplitCard(gtx)
		if dims.Size != cardSize {
			t.Fatalf("entry card at %gx = %v, want %v", scale, dims.Size, cardSize)
		}
		card := renderPresentationTestOps(t, &ops, cardSize)
		accent := countPDFToolAccent(card, image.Rect(0, 0, min(card.Bounds().Dx(), int(72*scale)), min(card.Bounds().Dy(), int(72*scale))))
		if accent < int(8*scale) {
			t.Fatalf("entry card PDF icon has only %d visible accent pixels at %gx", accent, scale)
		}

		app.pdfMergeSplitToolPage = true
		ops.Reset()
		workbenchSize := image.Pt(int(1000*scale), int(700*scale))
		gtx = pdfMergeSplitLayoutContext(&ops, &router, scale, workbenchSize)
		workbenchDims := app.layoutPDFMergeSplitTool(gtx)
		if workbenchDims.Size.X != workbenchSize.X || workbenchDims.Size.Y <= 0 {
			t.Fatalf("workbench at %gx has invalid geometry %v", scale, workbenchDims.Size)
		}
		workbench := renderPresentationTestOps(t, &ops, workbenchSize)
		accent = countPDFToolAccent(workbench, workbench.Bounds())
		if accent < int(80*scale) {
			t.Fatalf("workbench mode/action color has only %d visible accent pixels at %gx", accent, scale)
		}
	}
}

func countPDFToolAccent(img *image.RGBA, bounds image.Rectangle) int {
	count := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			p := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if p.A > 0 && p.R > p.G+45 && p.R > p.B+25 {
				count++
			}
		}
	}
	return count
}
