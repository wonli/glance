package desktopui

import (
	"bytes"
	"image"
	"image/png"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/format/docx"
)

func TestSelectingWordImageKeepsDocumentGeometryAndShowsAllResizeHandles(t *testing.T) {
	word := syntheticWordTableDocument(t)
	defer word.Close()
	state := newDocumentSessionState("image-selection-layout.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	var imageID, beforeRunID, afterRunID string
	for _, paragraph := range allWordParagraphs(state.wordBlocks) {
		for _, inline := range paragraph.Inlines {
			if inline.Image != nil {
				imageID = inline.Image.ID
			}
			if inline.Text != nil && strings.Contains(inline.Text.Text, "Document before table") {
				beforeRunID = inline.Text.ID
			}
			if inline.Text != nil && strings.Contains(inline.Text.Text, "Document after table") {
				afterRunID = inline.Text.ID
			}
		}
	}
	if imageID == "" || beforeRunID == "" || afterRunID == "" {
		t.Fatalf("visual fixture lacks its image or surrounding text runs: image=%q before=%q after=%q", imageID, beforeRunID, afterRunID)
	}
	imageData, err := word.WordImageBytes(imageID)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeWordPreview(imageData)
	if err != nil {
		t.Fatal(err)
	}
	state.wordImageCache = newWordImageCache(wordImageCacheLimit)
	state.wordImageCache.Put(imageID, decoded)
	app := &App{
		documentSessionState: state,
		theme:                material.NewTheme(),
		wordCommandResults:   make(chan wordCommandResult, 4),
	}
	router := new(input.Router)
	var ops op.Ops
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: metric, Constraints: layout.Constraints{Max: image.Pt(1100, 900)}}
		app.layoutWord(gtx)
		router.Frame(&ops)
	}
	findBounds := func(description string) image.Rectangle {
		t.Helper()
		var bounds image.Rectangle
		var visit func([]input.SemanticNode)
		visit = func(nodes []input.SemanticNode) {
			for _, node := range nodes {
				if node.Desc.Description == description {
					bounds = node.Desc.Bounds
				}
				visit(node.Children)
			}
		}
		visit(router.AppendSemantics(nil))
		return bounds
	}
	click := func(position image.Point, pointerID pointer.ID) {
		t.Helper()
		point := f32.Pt(float32(position.X), float32(position.Y))
		router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: pointerID, Buttons: pointer.ButtonPrimary, Position: point})
		frame()
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: pointerID, Position: point})
		frame()
		frame()
	}
	frame()
	focusWordRunForInteraction(t, state, router, frame, beforeRunID)
	if state.selectedWordRunID != beforeRunID {
		t.Fatalf("text focus selected run %q; want %q before selecting the image", state.selectedWordRunID, beforeRunID)
	}
	imageBoundsBefore := findBounds("内嵌图片 " + imageID)
	afterBoundsBefore := findBounds("正文文字 " + afterRunID)
	canvasBoundsBefore := findBounds("Word 文档画布")
	if imageBoundsBefore.Empty() || afterBoundsBefore.Empty() || canvasBoundsBefore.Empty() {
		t.Fatalf("unselected image/following text/canvas bounds are missing: image=%v following=%v canvas=%v", imageBoundsBefore, afterBoundsBefore, canvasBoundsBefore)
	}
	click(image.Pt(imageBoundsBefore.Min.X+imageBoundsBefore.Dx()/2, imageBoundsBefore.Min.Y+imageBoundsBefore.Dy()/2), 81)
	if state.selectedWordImageID != imageID {
		t.Fatalf("image click selected %q; want %q", state.selectedWordImageID, imageID)
	}
	if !router.Source().Focused(wordObjectButton(state, imageID)) {
		t.Fatal("image click left keyboard focus on the old text editor")
	}
	imageBoundsAfter := findBounds("内嵌图片 " + imageID)
	afterBoundsAfter := findBounds("正文文字 " + afterRunID)
	canvasBoundsAfter := findBounds("Word 文档画布")
	if imageBoundsAfter != imageBoundsBefore {
		t.Errorf("image bounds moved on selection: before=%v after=%v", imageBoundsBefore, imageBoundsAfter)
	}
	if afterBoundsAfter != afterBoundsBefore {
		t.Errorf("document content after the image shifted on selection: before=%v after=%v", afterBoundsBefore, afterBoundsAfter)
	}
	if canvasBoundsAfter != canvasBoundsBefore {
		t.Errorf("Word canvas moved on image selection: before=%v after=%v", canvasBoundsBefore, canvasBoundsAfter)
	}
	if canvasBoundsAfter.Empty() {
		t.Fatal("Word canvas is missing from the image selection frame")
	}
	for _, handle := range wordImageResizeHandles {
		bounds := findBounds("拖动调整图片大小：" + handle.name)
		if bounds.Empty() {
			t.Errorf("selected image has no visible resize handle %q", handle.name)
			continue
		}
		center := image.Pt(bounds.Min.X+bounds.Dx()/2, bounds.Min.Y+bounds.Dy()/2)
		if !center.In(canvasBoundsAfter) {
			t.Errorf("resize handle %q center %v is outside the visible canvas %v", handle.name, center, canvasBoundsAfter)
		}
	}
	focusWordRunForInteraction(t, state, router, frame, beforeRunID)
	if state.selectedWordRunID != beforeRunID || state.selectedWordImageID != "" {
		t.Fatalf("clicking text after the image selected run=%q image=%q; want the text run only", state.selectedWordRunID, state.selectedWordImageID)
	}
	click(image.Pt(imageBoundsBefore.Min.X+imageBoundsBefore.Dx()/2, imageBoundsBefore.Min.Y+imageBoundsBefore.Dy()/2), 82)
	if state.selectedWordImageID != imageID {
		t.Fatalf("switching back to the image selected %q; want %q", state.selectedWordImageID, imageID)
	}
	blockCount := len(state.wordBlocks)
	blankPoint := image.Pt(canvasBoundsAfter.Min.X+20, canvasBoundsAfter.Min.Y+40)
	click(blankPoint, 83)
	if state.selectedWordImageID != "" {
		t.Fatalf("clicking blank page margin left image %q selected", state.selectedWordImageID)
	}
	if len(state.wordBlocks) != blockCount {
		t.Fatalf("blank page click changed document block count from %d to %d", blockCount, len(state.wordBlocks))
	}
	imageBounds := findBounds("内嵌图片 " + imageID)
	click(image.Pt(imageBounds.Min.X+imageBounds.Dx()/2, imageBounds.Min.Y+imageBounds.Dy()/2), 84)
	if state.selectedWordImageID != imageID {
		t.Fatalf("could not reselect the image before testing the document-end blank area: %q", state.selectedWordImageID)
	}
	appendBounds := findBounds("单击此处继续输入")
	if appendBounds.Empty() {
		t.Fatal("document-end blank target has no accessible bounds")
	}
	click(image.Pt(appendBounds.Min.X+appendBounds.Dx()/2, appendBounds.Min.Y+appendBounds.Dy()/2), 85)
	if state.selectedWordImageID != "" {
		t.Fatalf("clicking document-end blank target left image %q selected", state.selectedWordImageID)
	}
	if len(state.wordBlocks) != blockCount || state.wordInsertionRunning || state.wordInsertAfterDraftFlush {
		t.Fatalf("deselecting from document-end blank area inserted content: blocks=%d/%d running=%v queued=%v", len(state.wordBlocks), blockCount, state.wordInsertionRunning, state.wordInsertAfterDraftFlush)
	}
}

func TestWordImageResizeAndRotationHandlesCommitPointerDrags(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 240, 120))); err != nil {
		t.Fatal(err)
	}
	paragraph := word.WordBlocks()[0].Paragraph
	imageID, err := word.InsertInlineImage(paragraph.ID, "", "image/png", encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState("image-handles.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	if !selectWordImage(state, imageID) {
		t.Fatal("could not select the fixture image")
	}
	state.wordImageCache = newWordImageCache(wordImageCacheLimit)
	decodedFixture := image.NewNRGBA(image.Rect(0, 0, 240, 120))
	state.wordImageCache.Put(imageID, decodedFixture)
	app := &App{documentSessionState: state, theme: material.NewTheme(), wordCommandResults: make(chan wordCommandResult, 1)}
	router := new(input.Router)
	var ops op.Ops
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	frame := func() {
		t.Helper()
		ops.Reset()
		gtx := layout.Context{Source: router.Source(), Ops: &ops, Now: time.Now(), Metric: metric, Constraints: layout.Constraints{Max: image.Pt(1100, 900)}}
		app.layoutWordFlow(gtx)
		router.Frame(&ops)
	}
	findBounds := func(description string) image.Rectangle {
		t.Helper()
		var bounds image.Rectangle
		var visit func([]input.SemanticNode)
		visit = func(nodes []input.SemanticNode) {
			for _, node := range nodes {
				if node.Desc.Description == description {
					bounds = node.Desc.Bounds
				}
				visit(node.Children)
			}
		}
		visit(router.AppendSemantics(nil))
		return bounds
	}
	waitForCommand := func() {
		t.Helper()
		select {
		case result := <-app.wordCommandResults:
			if result.err != nil {
				t.Fatalf("Word image handle command failed: %v", result.err)
			}
			app.wordCommandResults <- result
			app.processAsyncResults()
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for asynchronous Word image handle command")
		}
	}
	frame()
	container := findBounds("内嵌图片 " + imageID)
	if container.Empty() {
		t.Fatal("selected Word image has no accessible bounds")
	}
	imageMin := container.Min.Add(image.Pt(10, 14))
	initial := findWordImage(word.WordBlocks(), imageID)
	if initial == nil || initial.WidthEMU <= 0 {
		t.Fatal("inserted Word image has no initial extent")
	}
	initialWidth := initial.WidthEMU
	rightHandleBounds := findBounds("拖动调整图片大小：右")
	if rightHandleBounds.Empty() {
		t.Fatal("selected Word image has no accessible right resize handle")
	}
	resizeStart := f32.Pt(float32(rightHandleBounds.Min.X+rightHandleBounds.Dx()/2), float32(rightHandleBounds.Min.Y+rightHandleBounds.Dy()/2))
	resizeEnd := resizeStart.Add(f32.Pt(48, 0))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 71, Buttons: pointer.ButtonPrimary, Position: resizeStart})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 71, Buttons: pointer.ButtonPrimary, Position: resizeEnd})
	frame()
	preview := wordImageInteractionFor(state, imageID).preview.Preview()
	if !preview.Active || preview.WidthEMU <= initialWidth {
		t.Fatalf("dragging the right resize handle produced preview %#v from initial width %d; want an active wider preview", preview, initialWidth)
	}
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 71, Position: resizeEnd})
	frame()
	interaction := wordImageInteractionFor(state, imageID)
	if !interaction.commitPending {
		t.Fatal("resizing released its preview before the asynchronous image edit completed")
	}
	frame()
	pendingBounds := findBounds("内嵌图片 " + imageID)
	if pendingBounds.Empty() || pendingBounds.Dx() <= container.Dx() {
		t.Fatalf("image bounds during asynchronous resize = %v; want the resized preview wider than %v", pendingBounds, container)
	}
	if _, ok := state.wordImageCache.Get(imageID); !ok {
		t.Fatal("resizing discarded the decoded image preview")
	}
	waitForCommand()
	if interaction.commitPending {
		t.Fatal("completed resize kept a pending visual preview")
	}
	resized := findWordImage(word.WordBlocks(), imageID)
	if resized == nil || resized.WidthEMU <= initialWidth {
		t.Fatalf("right resize handle committed width %v; want more than %d EMU (running=%v preview=%#v status=%q)", resized, initialWidth, state.formatRunning.Load(), wordImageInteractionFor(state, imageID).preview.Preview(), app.status)
	}
	frame()
	container = findBounds("内嵌图片 " + imageID)
	absDiff := func(left, right int) int {
		if left < right {
			return right - left
		}
		return left - right
	}
	if container.Min != pendingBounds.Min || absDiff(container.Dx(), pendingBounds.Dx()) > 2 || absDiff(container.Dy(), pendingBounds.Dy()) > 2 {
		t.Fatalf("image bounds jumped after resize completed: pending=%v committed=%v", pendingBounds, container)
	}
	imageMin = container.Min.Add(image.Pt(10, 14))
	visualWidth := wordImageEMUToPixels(resized.WidthEMU, metric.PxPerDp)
	visualHeight := wordImageEMUToPixels(resized.HeightEMU, metric.PxPerDp)
	center := f32.Pt(float32(imageMin.X+visualWidth/2), float32(imageMin.Y+visualHeight/2))
	rotationHandleBounds := findBounds("拖动旋转图片")
	if rotationHandleBounds.Empty() {
		var descriptions []string
		var collect func([]input.SemanticNode)
		collect = func(nodes []input.SemanticNode) {
			for _, node := range nodes {
				if node.Desc.Description != "" {
					descriptions = append(descriptions, node.Desc.Description)
				}
				collect(node.Children)
			}
		}
		collect(router.AppendSemantics(nil))
		t.Fatalf("selected Word image has no accessible rotation handle (selected=%q, container=%v, semantics=%v)", state.selectedWordImageID, container, descriptions)
	}
	rotationStart := f32.Pt(float32(rotationHandleBounds.Min.X+rotationHandleBounds.Dx()/2), float32(rotationHandleBounds.Min.Y+rotationHandleBounds.Dy()/2))
	rotationEnd := f32.Pt(center.X+float32(visualWidth/2), center.Y)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 72, Buttons: pointer.ButtonPrimary, Position: rotationStart})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 72, Buttons: pointer.ButtonPrimary, Position: rotationEnd})
	frame()
	preview = wordImageInteractionFor(state, imageID).preview.Preview()
	if !preview.Active || preview.RotationDegrees == resized.RotationDegrees {
		t.Fatalf("dragging the rotation handle produced preview %#v from rotation %g; want a changed active preview", preview, resized.RotationDegrees)
	}
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 72, Position: rotationEnd})
	frame()
	waitForCommand()
	rotated := findWordImage(word.WordBlocks(), imageID)
	if rotated == nil || rotated.RotationDegrees == resized.RotationDegrees {
		t.Fatalf("rotation handle committed image %#v; want rotation different from %g degrees", rotated, resized.RotationDegrees)
	}
}
