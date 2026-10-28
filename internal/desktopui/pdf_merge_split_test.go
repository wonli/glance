package desktopui

import (
	"context"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/application"
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
	defer closePDFMergeSplitTestApp(app)
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
	app.processPDFMergeSplitToolEvents(gtx)
	app.layoutHomeContentPage(gtx)
	if app.pdfMergeSplitToolPage {
		t.Fatal("workbench back action did not return to the tools catalog")
	}
}

func TestPDFMergeSplitToolHeaderBackButtonDoesNotCoverTitle(t *testing.T) {
	app := New(nil, "")
	defer closePDFMergeSplitTestApp(app)
	app.pdfMergeSplitToolPage = true
	var ops op.Ops
	var router input.Router
	gtx := pdfMergeSplitLayoutContext(&ops, &router, 1, image.Pt(1000, 100))
	gtx.Constraints = layout.Constraints{Max: image.Pt(1000, 100)}
	frame := func() {
		ops.Reset()
		gtx.Ops = &ops
		gtx.Source = router.Source()
		app.processPDFMergeSplitToolEvents(gtx)
		app.layoutPDFMergeSplitHeader(gtx)
		router.Frame(&ops)
	}
	click := func(position f32.Point) {
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 31, Buttons: pointer.ButtonPrimary, Position: position})
		frame()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 31, Position: position})
		frame()
		frame()
	}
	frame()
	click(f32.Pt(600, 21))
	if !app.pdfMergeSplitToolPage {
		t.Fatal("clicking the header title area triggered the back button")
	}
	click(f32.Pt(120, 21))
	if app.pdfMergeSplitToolPage {
		t.Fatal("clicking the right side of the Chinese back label did not activate the back button; the button is too narrow")
	}
	click(f32.Pt(56, 21))
	if app.pdfMergeSplitToolPage {
		t.Fatal("clicking the back button did not leave the workbench")
	}
}

func TestPDFMergeSplitToolCanReplaceSplitSource(t *testing.T) {
	app := New(nil, "")
	defer closePDFMergeSplitTestApp(app)
	app.pdfMergeSplitToolPage = true
	app.pdfMergeSplitTool.mode = pdfMergeSplitModeSplit
	first, second := writePDFMergeSplitFixture(t, "first.pdf"), writePDFMergeSplitFixture(t, "second.pdf")
	draft := &app.pdfMergeSplitTool.splitDraft
	draft.source = &pdfMergeSplitFile{path: first, name: filepath.Base(first), pageCount: 4, sizeBytes: 100, identity: 1}
	draft.pickPDFFile = func(context.Context, filePickerLabels) (string, error) { return second, nil }
	var ops op.Ops
	var router input.Router
	gtx := pdfMergeSplitLayoutContext(&ops, &router, 1, image.Pt(1000, 700))
	frame := func() {
		ops.Reset()
		gtx.Ops = &ops
		gtx.Source = router.Source()
		app.processPDFMergeSplitToolEvents(gtx)
		app.layoutPDFMergeSplitSplitSource(gtx)
		router.Frame(&ops)
	}
	frame()
	position := f32.Pt(500, 83)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 32, Buttons: pointer.ButtonPrimary, Position: position})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 32, Position: position})
	frame()
	frame()
	if !app.pdfMergeSplitTool.busy {
		t.Fatal("selected split source has no reachable replace control")
	}
	waitPDFMergeSplitJob(t, app)
	if draft.source == nil || draft.source.path != second {
		t.Fatalf("replacing the split source left %+v, want %q", draft.source, second)
	}
}

func TestPDFMergeSplitToolMergeRowCanRemoveFile(t *testing.T) {
	app := New(nil, "")
	defer closePDFMergeSplitTestApp(app)
	app.pdfMergeSplitToolPage = true
	state := &app.pdfMergeSplitTool.mergeDraft
	file := pdfMergeSplitFile{path: "/one.pdf", name: "one.pdf", pageCount: 1, sizeBytes: 100, identity: 1}
	state.files = []pdfMergeSplitFile{file}
	state.fileButtons[file.identity] = &pdfMergeSplitFileButtons{}
	app.recalculatePDFMergeSplitMergeSummary()
	var ops op.Ops
	var router input.Router
	gtx := pdfMergeSplitLayoutContext(&ops, &router, 1, image.Pt(1000, 200))
	frame := func() {
		ops.Reset()
		gtx.Ops = &ops
		gtx.Source = router.Source()
		app.processPDFMergeSplitToolEvents(gtx)
		app.layoutPDFMergeSplitMergeFiles(gtx)
		router.Frame(&ops)
	}
	frame()
	position := f32.Pt(980, 27)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 33, Buttons: pointer.ButtonPrimary, Position: position})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 33, Position: position})
	frame()
	frame()
	if len(state.files) != 0 {
		t.Fatalf("clicking the merge row remove control left %+v", state.files)
	}
}

func TestPDFMergeSplitToolModeSwitch(t *testing.T) {
	app := New(nil, "")
	defer closePDFMergeSplitTestApp(app)
	app.pdfMergeSplitToolPage = true
	state := &app.pdfMergeSplitTool
	state.mergeDraft.outputName.SetText("joined.pdf")
	state.splitDraft.rangeInput.SetText("1-3, 7")
	state.splitDraft.prefix.SetText("extract")
	var ops op.Ops
	var router input.Router
	gtx := pdfMergeSplitLayoutContext(&ops, &router, 1, image.Pt(1000, 700))

	state.splitModeButton.Click()
	app.processPDFMergeSplitToolEvents(gtx)
	if state.mode != pdfMergeSplitModeSplit {
		t.Fatalf("mode after split click = %v, want split", state.mode)
	}
	state.mergeModeButton.Click()
	app.processPDFMergeSplitToolEvents(gtx)
	if state.mode != pdfMergeSplitModeMerge {
		t.Fatalf("mode after merge click = %v, want merge", state.mode)
	}
	if state.mergeDraft.outputName.Text() != "joined.pdf" || state.splitDraft.rangeInput.Text() != "1-3, 7" || state.splitDraft.prefix.Text() != "extract" {
		t.Fatalf("mode switch lost an independent draft: merge=%q range=%q prefix=%q", state.mergeDraft.outputName.Text(), state.splitDraft.rangeInput.Text(), state.splitDraft.prefix.Text())
	}
}

func TestPDFMergeSplitToolStableState(t *testing.T) {
	app := New(nil, "")
	defer closePDFMergeSplitTestApp(app)
	state := &app.pdfMergeSplitTool
	mergeEditor := &state.mergeDraft.outputName
	rangeEditor := &state.splitDraft.rangeInput
	prefixEditor := &state.splitDraft.prefix
	backButton := &state.backButton
	mergeButton := &state.mergeModeButton
	list := &state.mergeDraft.fileList
	outputList := &state.outputList
	var ops op.Ops
	var router input.Router
	gtx := pdfMergeSplitLayoutContext(&ops, &router, 1, image.Pt(1000, 700))
	app.pdfMergeSplitToolPage = true
	app.layoutPDFMergeSplitTool(gtx)
	ops.Reset()
	app.layoutPDFMergeSplitTool(gtx)
	if mergeEditor != &app.pdfMergeSplitTool.mergeDraft.outputName || rangeEditor != &app.pdfMergeSplitTool.splitDraft.rangeInput || prefixEditor != &app.pdfMergeSplitTool.splitDraft.prefix || backButton != &app.pdfMergeSplitTool.backButton || mergeButton != &app.pdfMergeSplitTool.mergeModeButton || list != &app.pdfMergeSplitTool.mergeDraft.fileList || outputList != &app.pdfMergeSplitTool.outputList {
		t.Fatal("PDF workbench replaced a persistent editor, list or clickable across layout calls")
	}
}

func TestPDFMergeSplitToolVisual(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	grip, ok := svgIconTintAtSize("home", "grip-vertical", 36, pdfMergeSplitMute)
	if !ok || countPDFToolVisible(grip) == 0 {
		t.Fatal("merge reorder grip icon is missing visible pixels")
	}
	for _, scale := range []float32{1, 2} {
		app := New(nil, "")
		defer closePDFMergeSplitTestApp(app)
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
		state := &app.pdfMergeSplitTool
		state.lastError = "synthetic merge failure"
		state.lastOutputs = []string{filepath.Join(t.TempDir(), "partial-01.pdf")}
		ops.Reset()
		app.layoutPDFMergeSplitTool(gtx)
		partial := renderPresentationTestOps(t, &ops, workbenchSize)
		if countPDFToolErrorPixels(partial) < int(10*scale) {
			t.Fatalf("partial-result error is not visibly rendered at %gx", scale)
		}
		state.busy = true
		state.lastError, state.lastOutputs = "", nil
		ops.Reset()
		app.layoutPDFMergeSplitTool(gtx)
		if dims := renderPresentationTestOps(t, &ops, workbenchSize); dims == nil {
			t.Fatalf("busy progress state did not render at %gx", scale)
		}
	}
}

func TestPDFMergeSplitToolPanelsFitPrimaryActions(t *testing.T) {
	app := New(nil, "")
	defer closePDFMergeSplitTestApp(app)
	app.pdfMergeSplitToolPage = true
	app.pdfMergeSplitTool.mergeDraft.files = []pdfMergeSplitFile{
		{path: "/source/quarterly-report.pdf", name: "quarterly-report.pdf", pageCount: 12, sizeBytes: 2_400_000, identity: 1},
		{path: "/source/appendix.pdf", name: "appendix.pdf", pageCount: 4, sizeBytes: 870_000, identity: 2},
	}
	app.pdfMergeSplitTool.mergeDraft.fileButtons[1] = &pdfMergeSplitFileButtons{}
	app.pdfMergeSplitTool.mergeDraft.fileButtons[2] = &pdfMergeSplitFileButtons{}
	app.recalculatePDFMergeSplitMergeSummary()
	app.setPDFMergeSplitSource(application.PDFUtilityFileInfo{Path: "/source/quarterly-report.pdf", PageCount: 12, SizeBytes: 2_400_000})
	twoFiles := append([]pdfMergeSplitFile(nil), app.pdfMergeSplitTool.mergeDraft.files...)
	splitSource := app.pdfMergeSplitTool.splitDraft.source

	for _, scale := range []float32{1, 2} {
		var ops op.Ops
		var router input.Router
		gtx := pdfMergeSplitLayoutContext(&ops, &router, scale, image.Pt(int(1200*scale), int(800*scale)))
		merge := app.layoutPDFMergeSplitMergeDraft(gtx)
		if minimum := gtx.Dp(unit.Dp(560)); merge.Size.Y < minimum {
			t.Errorf("merge panel at %gx is %dpx high, want at least %dpx so the output folder and merge action both fit", scale, merge.Size.Y, minimum)
		}
		app.pdfMergeSplitTool.mergeDraft.files = twoFiles[:1]
		app.recalculatePDFMergeSplitMergeSummary()
		ops.Reset()
		if oneFile := app.layoutPDFMergeSplitMergeDraft(gtx); oneFile.Size.Y != gtx.Dp(unit.Dp(506)) {
			t.Errorf("one-file merge panel at %gx is %dpx high, want %dpx", scale, oneFile.Size.Y, gtx.Dp(unit.Dp(506)))
		}
		app.pdfMergeSplitTool.mergeDraft.files = nil
		app.recalculatePDFMergeSplitMergeSummary()
		ops.Reset()
		if empty := app.layoutPDFMergeSplitMergeDraft(gtx); empty.Size.Y != gtx.Dp(unit.Dp(468)) {
			t.Errorf("empty merge panel at %gx is %dpx high, want %dpx", scale, empty.Size.Y, gtx.Dp(unit.Dp(468)))
		}
		app.pdfMergeSplitTool.mergeDraft.files = twoFiles
		app.recalculatePDFMergeSplitMergeSummary()
		app.pdfMergeSplitTool.mode = pdfMergeSplitModeSplit
		ops.Reset()
		split := app.layoutPDFMergeSplitSplitDraft(gtx)
		if minimum := gtx.Dp(unit.Dp(552)); split.Size.Y < minimum {
			t.Errorf("split panel at %gx is %dpx high, want at least %dpx so the output folder and split action both fit", scale, split.Size.Y, minimum)
		}
		if addZone := app.layoutPDFMergeSplitAddDropZone(gtx); addZone.Size.Y != gtx.Dp(unit.Dp(88)) {
			t.Errorf("merge add-file zone at %gx is %dpx high, want %dpx for a compact in-list action", scale, addZone.Size.Y, gtx.Dp(unit.Dp(88)))
		}
		app.pdfMergeSplitTool.splitDraft.source = nil
		ops.Reset()
		if emptySplit := app.layoutPDFMergeSplitSplitDraft(gtx); emptySplit.Size.Y != gtx.Dp(unit.Dp(560)) {
			t.Errorf("empty split panel at %gx is %dpx high, want %dpx", scale, emptySplit.Size.Y, gtx.Dp(unit.Dp(560)))
		}
		app.pdfMergeSplitTool.splitDraft.source = splitSource
		app.pdfMergeSplitTool.mode = pdfMergeSplitModeMerge
	}
}

func countPDFToolVisible(img image.Image) int {
	count := 0
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			if _, _, _, alpha := img.At(x, y).RGBA(); alpha != 0 {
				count++
			}
		}
	}
	return count
}

func countPDFToolErrorPixels(img image.Image) int {
	count := 0
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			p := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if p.A > 0 && p.R > p.G+25 && p.R > p.B+18 {
				count++
			}
		}
	}
	return count
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

func TestPDFMergeSplitToolFileFlow(t *testing.T) {
	app := New(nil, "")
	defer closePDFMergeSplitTestApp(app)
	app.pdfMergeSplitToolPage = true
	first, second := writePDFMergeSplitFixture(t, "first.pdf"), writePDFMergeSplitFixture(t, "second.pdf")
	state := &app.pdfMergeSplitTool
	state.mergeDraft.pickPDFFile = func(context.Context, filePickerLabels) (string, error) {
		if len(state.mergeDraft.files) == 0 {
			return first, nil
		}
		return second, nil
	}
	var ops op.Ops
	var router input.Router
	gtx := pdfMergeSplitLayoutContext(&ops, &router, 1, image.Pt(1000, 700))

	state.mergeDraft.addButton.Click()
	app.processPDFMergeSplitToolEvents(gtx)
	waitPDFMergeSplitJob(t, app)
	state.mergeDraft.addButton.Click()
	app.processPDFMergeSplitToolEvents(gtx)
	waitPDFMergeSplitJob(t, app)
	if len(state.mergeDraft.files) != 2 || state.mergeDraft.totalPageCount != 8 {
		t.Fatalf("inspected merge files = %d with %d pages, want 2 files and 8 pages", len(state.mergeDraft.files), state.mergeDraft.totalPageCount)
	}
	if state.mergeDraft.totalSizeBytes <= 0 || state.mergeDraft.files[0].path != first || state.mergeDraft.files[1].path != second {
		t.Fatalf("metadata or selection order missing: %+v", state.mergeDraft.files)
	}
	mergeFolder := t.TempDir()
	state.mergeDraft.chooseOutputFolder = func(context.Context, filePickerLabels) (string, error) { return mergeFolder, nil }
	state.mergeDraft.outputName.SetText("combined.pdf")
	state.mergeDraft.mergeButton.Click()
	app.processPDFMergeSplitToolEvents(gtx)
	waitPDFMergeSplitJob(t, app)
	if !state.busy {
		t.Fatal("selecting an output folder did not continue the pending merge")
	}
	waitPDFMergeSplitJob(t, app)
	output := filepath.Join(mergeFolder, "combined.pdf")
	if state.busy || state.lastError != "" || len(state.lastOutputs) != 1 || state.lastOutputs[0] != output {
		t.Fatalf("merge completion state: busy=%v error=%q outputs=%v", state.busy, state.lastError, state.lastOutputs)
	}
	mergedInfo, err := app.desktopRuntime.applicationRuntime.InspectPDFUtilityFile(context.Background(), output)
	if err != nil || mergedInfo.PageCount != 8 {
		t.Fatalf("merged output pages = %v, err=%v; want 8", mergedInfo.PageCount, err)
	}
	state.mergeDraft.fileButtons[state.mergeDraft.files[0].identity].remove.Click()
	app.processPDFMergeSplitToolEvents(gtx)
	if len(state.mergeDraft.files) != 1 || state.mergeDraft.files[0].path != second {
		t.Fatalf("removing a merge input left files %+v", state.mergeDraft.files)
	}
	state.mergeDraft.clearButton.Click()
	app.processPDFMergeSplitToolEvents(gtx)
	if len(state.mergeDraft.files) != 0 || state.mergeDraft.totalPageCount != 0 || state.mergeDraft.totalSizeBytes != 0 {
		t.Fatalf("clearing merge inputs did not reset list totals: %+v", state.mergeDraft)
	}
	state.splitModeButton.Click()
	app.processPDFMergeSplitToolEvents(gtx)
	state.splitDraft.pickPDFFile = func(context.Context, filePickerLabels) (string, error) { return first, nil }
	state.splitDraft.outputFolder = t.TempDir()
	state.splitDraft.selectButton.Click()
	app.processPDFMergeSplitToolEvents(gtx)
	waitPDFMergeSplitJob(t, app)
	state.splitDraft.rangeInput.SetText("1,2")
	app.updatePDFMergeSplitRangePreview()
	state.splitDraft.splitButton.Click()
	app.processPDFMergeSplitToolEvents(gtx)
	waitPDFMergeSplitJob(t, app)
	if len(state.lastOutputs) != 2 || state.lastOutputs[0] != filepath.Join(state.splitDraft.outputFolder, "first-01.pdf") || state.lastOutputs[1] != filepath.Join(state.splitDraft.outputFolder, "first-02.pdf") {
		t.Fatalf("range split outputs = %v", state.lastOutputs)
	}
	state.splitDraft.prefix.SetText("each")
	state.splitDraft.byPageButton.Click()
	app.processPDFMergeSplitToolEvents(gtx)
	state.splitDraft.splitButton.Click()
	app.processPDFMergeSplitToolEvents(gtx)
	waitPDFMergeSplitJob(t, app)
	if len(state.lastOutputs) != 4 || state.lastOutputs[0] != filepath.Join(state.splitDraft.outputFolder, "each-01.pdf") {
		t.Fatalf("single-page split outputs = %v", state.lastOutputs)
	}
}

func TestPDFMergeSplitToolReorderAndRangePreview(t *testing.T) {
	app := New(nil, "")
	defer closePDFMergeSplitTestApp(app)
	app.pdfMergeSplitToolPage = true
	state := &app.pdfMergeSplitTool
	state.mergeDraft.files = []pdfMergeSplitFile{{path: "/a.pdf", name: "a.pdf", pageCount: 2, sizeBytes: 10, identity: 1}, {path: "/b.pdf", name: "b.pdf", pageCount: 4, sizeBytes: 20, identity: 2}, {path: "/c.pdf", name: "c.pdf", pageCount: 1, sizeBytes: 30, identity: 3}}
	app.recalculatePDFMergeSplitMergeSummary()
	if !app.movePDFMergeSplitFile(0, 2) || state.mergeDraft.files[2].path != "/a.pdf" {
		t.Fatalf("merge reorder did not move first input to last: %+v", state.mergeDraft.files)
	}
	app.setPDFMergeSplitSource(application.PDFUtilityFileInfo{Path: "/source.pdf", PageCount: 10, SizeBytes: 100})
	state.splitDraft.rangeInput.SetText("1-3, 7, 9-10")
	app.updatePDFMergeSplitRangePreview()
	if state.splitDraft.rangeCount != 3 || state.splitDraft.rangeError != "" || state.splitDraft.prefix.Text() != "source" {
		t.Fatalf("valid split range preview count=%d error=%q prefix=%q", state.splitDraft.rangeCount, state.splitDraft.rangeError, state.splitDraft.prefix.Text())
	}
	state.splitDraft.rangeInput.SetText("1-3, 3-4")
	app.updatePDFMergeSplitRangePreview()
	if state.splitDraft.rangeCount != 0 || state.splitDraft.rangeError == "" {
		t.Fatalf("overlapping range preview accepted: count=%d error=%q", state.splitDraft.rangeCount, state.splitDraft.rangeError)
	}
	state.splitDraft.rangeMode = pdfMergeSplitSplitModeByPage
	app.updatePDFMergeSplitRangePreview()
	if state.splitDraft.rangeCount != 10 || state.splitDraft.rangeError != "" {
		t.Fatalf("by-page preview count=%d error=%q, want 10", state.splitDraft.rangeCount, state.splitDraft.rangeError)
	}
}

func TestPDFMergeSplitToolPickerAndFolderCancel(t *testing.T) {
	app := New(nil, "")
	defer closePDFMergeSplitTestApp(app)
	app.pdfMergeSplitToolPage = true
	state := &app.pdfMergeSplitTool
	state.mergeDraft.outputFolder = "/previous"
	state.mergeDraft.chooseOutputFolder = func(context.Context, filePickerLabels) (string, error) { return "", nil }
	var ops op.Ops
	var router input.Router
	gtx := pdfMergeSplitLayoutContext(&ops, &router, 1, image.Pt(1000, 700))
	state.mergeDraft.outputButton.Click()
	app.processPDFMergeSplitToolEvents(gtx)
	waitPDFMergeSplitJob(t, app)
	if state.busy || state.mergeDraft.outputFolder != "/previous" {
		t.Fatalf("canceling folder picker changed state: busy=%v folder=%q", state.busy, state.mergeDraft.outputFolder)
	}
	state.mergeDraft.pickPDFFile = func(_ context.Context, labels filePickerLabels) (string, error) {
		if len(labels.AllowedExtensions) != 1 || labels.AllowedExtensions[0] != "pdf" {
			t.Errorf("PDF picker filter = %v, want only pdf", labels.AllowedExtensions)
		}
		return "", nil
	}
	state.mergeDraft.addButton.Click()
	app.processPDFMergeSplitToolEvents(gtx)
	waitPDFMergeSplitJob(t, app)
	if state.busy || len(state.mergeDraft.files) != 0 {
		t.Fatalf("canceling file picker changed input state: busy=%v files=%v", state.busy, state.mergeDraft.files)
	}
	started := make(chan struct{})
	state.mergeDraft.pickPDFFile = func(ctx context.Context, _ filePickerLabels) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	}
	state.mergeDraft.addButton.Click()
	app.processPDFMergeSplitToolEvents(gtx)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("cancellable PDF picker did not start")
	}
	state.cancelButton.Click()
	app.processPDFMergeSplitToolEvents(gtx)
	waitPDFMergeSplitJob(t, app)
	if state.busy || !state.cancelled || len(state.mergeDraft.files) != 0 {
		t.Fatalf("canceling active PDF picker left state busy or failed: busy=%v canceled=%v files=%d", state.busy, state.cancelled, len(state.mergeDraft.files))
	}
}

func TestPDFMergeSplitToolIgnoresStaleJobResults(t *testing.T) {
	app := New(nil, "")
	defer closePDFMergeSplitTestApp(app)
	state := &app.pdfMergeSplitTool
	state.generation = 7
	state.mode = pdfMergeSplitModeSplit
	state.jobResults <- pdfMergeSplitJobResult{generation: 6, mode: pdfMergeSplitModeMerge, kind: pdfMergeSplitJobOperation}
	app.cancelPDFMergeSplitJobForExit()
	app.processPDFMergeSplitToolJobResults()
	if state.busy || state.generation != 8 || len(state.lastOutputs) != 0 {
		t.Fatalf("stale job mutated current workbench state: busy=%v generation=%d outputs=%v", state.busy, state.generation, state.lastOutputs)
	}
	ctx, cancel := context.WithCancel(context.Background())
	state.busy, state.cancel = true, cancel
	app.cancelPDFMergeSplitJobForShutdown()
	if ctx.Err() == nil {
		t.Fatal("window shutdown did not cancel the active PDF utility job")
	}
}

func writePDFMergeSplitFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, pdfTestBytes(t), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func waitPDFMergeSplitJob(t *testing.T, app *App) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case result := <-app.pdfMergeSplitTool.jobResults:
			app.applyPDFMergeSplitToolJobResult(result)
			return
		case <-deadline:
			t.Fatal("PDF workbench job did not complete")
		}
	}
}

func closePDFMergeSplitTestApp(app *App) {
	app.cancelPDFMergeSplitJobForShutdown()
	app.shutdownOnce.Do(func() { close(app.shutdown) })
	app.stopAndWaitManagedBackground()
	if app.desktopRuntime != nil && app.desktopRuntime.applicationRuntime != nil {
		_ = app.desktopRuntime.applicationRuntime.Close(context.Background())
	}
}
