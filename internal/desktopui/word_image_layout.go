package desktopui

import (
	"fmt"
	"glance/internal/document"
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/gesture"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

func (a *App) requestWordImageDecode(state *documentSessionState, imageID string) {
	if state == nil || state.word == nil || imageID == "" {
		return
	}
	if state.wordImagePending == nil {
		state.wordImagePending = make(map[string]uint64)
	}
	if state.wordImageVersions == nil {
		state.wordImageVersions = make(map[string]uint64)
	}
	if state.wordImageErrors == nil {
		state.wordImageErrors = make(map[string]string)
	}
	version := state.wordImageVersions[imageID]
	if pendingVersion, pending := state.wordImagePending[imageID]; pending && pendingVersion == version {
		return
	}
	if state.wordImageDecodeActive.Add(1) > wordImageDecodeConcurrency {
		state.wordImageDecodeActive.Add(-1)
		return
	}
	if a.wordImageResults == nil {
		a.wordImageResults = make(chan wordImageDecodeResult, 64)
	}
	state.wordImagePending[imageID] = version
	shutdown := a.shutdown
	results := a.wordImageResults
	window := a.window
	go func() {
		resultSent := false
		defer func() {
			if !resultSent {
				state.wordImageDecodeActive.Add(-1)
			}
		}()
		unlock := lockSessionAdapterRead(state)
		word := state.word
		var data []byte
		var err error
		if word == nil {
			err = fmt.Errorf("Word 文档已关闭")
		} else {
			data, err = word.WordImageBytes(imageID)
		}
		unlock()
		var decoded image.Image
		if err == nil {
			decoded, err = decodeWordPreview(data)
		}
		if err == nil {
			decoded = compactWordPreview(decoded)
		}
		result := wordImageDecodeResult{state: state, id: imageID, version: version, image: decoded, err: err, decodeSlot: true}
		select {
		case results <- result:
			resultSent = true
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	}()
}

func invalidateWordImagePreview(state *documentSessionState, imageID string) {
	if state == nil || imageID == "" {
		return
	}
	if state.wordImageVersions == nil {
		state.wordImageVersions = make(map[string]uint64)
	}
	state.wordImageVersions[imageID]++
	state.wordImageCache.Delete(imageID)
	delete(state.wordImageErrors, imageID)
}

func invalidateAllWordImagePreviews(state *documentSessionState) {
	if state == nil {
		return
	}
	if state.wordImageCache != nil {
		state.wordImageCache.Clear()
	}
	if state.wordImageVersions == nil {
		state.wordImageVersions = make(map[string]uint64)
	}
	for id := range state.wordImagePending {
		state.wordImageVersions[id]++
	}
	if state.word != nil {
		for _, block := range wordBlocksForState(state) {
			for _, paragraph := range wordParagraphsInBlock(block) {
				for _, inline := range paragraph.Inlines {
					if inline.Image != nil {
						state.wordImageVersions[inline.Image.ID]++
						delete(state.wordImageErrors, inline.Image.ID)
					}
				}
			}
		}
	}
}

func wordParagraphsInBlock(block document.WordBlock) []document.WordParagraph {
	switch block.Kind {
	case document.WordParagraphBlock:
		if block.Paragraph != nil {
			return []document.WordParagraph{*block.Paragraph}
		}
	case document.WordTableBlock:
		if block.Table != nil {
			var paragraphs []document.WordParagraph
			for _, row := range block.Table.Rows {
				for _, cell := range row.Cells {
					paragraphs = append(paragraphs, cell.Paragraphs...)
				}
			}
			return paragraphs
		}
	}
	return nil
}

func reconcileWordSelection(state *documentSessionState, syncDraft bool) {
	if state == nil || state.word == nil {
		return
	}
	blocks := wordBlocksForState(state)
	if state.selectedWordRunID != "" {
		if run, ok := selectedWordRun(blocks, state.selectedWordRunID); ok {
			if editor := state.wordRunEditors[run.ID]; editor != nil {
				if _, draft := state.wordDraft(run.ID); !draft {
					if editor.Text() != run.Text {
						editor.SetText(run.Text)
					}
					state.setCommittedWordText(run.ID, run.Text)
				}
			}
			if syncDraft {
				state.wordEditor.SetText(run.Text)
			}
		} else {
			state.selectedWordRunID = ""
			state.selectedWordParagraphID = ""
			state.wordEditor.SetText("")
		}
	}
	if state.selectedWordImageID != "" && findWordImage(blocks, state.selectedWordImageID) == nil {
		state.selectedWordImageID = ""
		state.wordImageFocusRequestID = ""
	}
}

func (a *App) layoutWordImage(gtx layout.Context, paragraph document.WordParagraph, item document.WordImage, overlay *wordInlineLayoutOverlay) layout.Dimensions {
	state := a.documentSessionState
	interaction := wordImageInteractionFor(state, item.ID)
	if !state.wordReadingMode {
		a.updateWordImageMoveGesture(gtx, interaction, item, paragraph.ID)
	}
	button := wordObjectButton(state, item.ID)
	decoded, cached := state.wordImageCache.Get(item.ID)
	if !cached && state.wordImageErrors[item.ID] == "" {
		a.requestWordImageDecode(state, item.ID)
	}
	preview, previewMode := wordImageLayoutPreview(interaction)
	widthEMU, heightEMU := item.WidthEMU, item.HeightEMU
	rotationDegrees := item.RotationDegrees
	if preview.Active && previewMode == wordImageGestureResize {
		widthEMU, heightEMU = preview.WidthEMU, preview.HeightEMU
	}
	if preview.Active && previewMode == wordImageGestureRotate {
		rotationDegrees = preview.RotationDegrees
	}
	maxWidth := max(1, gtx.Constraints.Max.X)
	maxHeight := gtx.Dp(unit.Dp(360))
	var sourceSize, visualSize image.Point
	if decoded != nil {
		if widthEMU <= 0 || heightEMU <= 0 {
			sourceSize = fitRotatedWordImageSize(decoded.Bounds().Dx(), decoded.Bounds().Dy(), maxWidth, maxHeight, rotationDegrees)
		} else {
			visualWidth := wordImageEMUToPixels(widthEMU, gtx.Metric.PxPerDp)
			visualHeight := wordImageEMUToPixels(heightEMU, gtx.Metric.PxPerDp)
			sourceSize = fitRotatedWordImageSize(visualWidth, visualHeight, maxWidth, maxHeight, rotationDegrees)
		}
		visualSize = rotatedWordImageBounds(sourceSize, rotationDegrees)
	}
	if !state.wordReadingMode && button.Clicked(gtx) {
		state.wordCanvasContentClickHandled = true
		selectWordImage(state, item.ID)
		state.wordImageFocusRequestID = item.ID
		if a.window != nil {
			a.window.Invalidate()
		}
	}
	if state.selectedWordImageID == item.ID && state.wordImageFocusRequestID == item.ID {
		if gtx.Focused(button) {
			state.wordImageFocusRequestID = ""
		} else {
			gtx.Execute(key.FocusCmd{Tag: button})
		}
	}
	selected := !state.wordReadingMode && state.selectedWordImageID == item.ID
	return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return wordMeasuredBackground(gtx, contentSurface, func(gtx layout.Context) layout.Dimensions {
			if decoded == nil {
				return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					if !state.wordReadingMode {
						interaction.moveDrag.Add(gtx.Ops)
					}
					message := state.wordImageErrors[item.ID]
					if message == "" {
						message = "正在载入图片…"
					} else {
						message = "图片无法预览：" + message
					}
					return wordReadOnlyPlaceholder(gtx, a.theme, message)
				})
			}

			// Reserve the same interaction frame before and after selection so
			// showing handles never changes paragraph wrapping or document scroll.
			const imageHandlePaddingXDP = 10
			const imageRotationPaddingTopDP = 14
			const imageHandlePaddingBottomDP = 10
			paddingX := gtx.Dp(unit.Dp(imageHandlePaddingXDP))
			topPadding := gtx.Dp(unit.Dp(imageRotationPaddingTopDP))
			bottomPadding := gtx.Dp(unit.Dp(imageHandlePaddingBottomDP))
			imageRect := image.Rectangle{Min: image.Pt(paddingX, topPadding), Max: image.Pt(paddingX+visualSize.X, topPadding+visualSize.Y)}
			containerSize := image.Pt(visualSize.X+paddingX*2, visualSize.Y+topPadding+bottomPadding)
			gtx.Constraints = layout.Constraints{Min: containerSize, Max: containerSize}
			dimensions := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if !state.wordReadingMode {
					layoutWordImageMoveDragArea(gtx, interaction, imageRect, selected)
				}
				imageBounds := clip.Rect{Max: containerSize}.Push(gtx.Ops)
				semantic.DescriptionOp("内嵌图片 " + item.ID).Add(gtx.Ops)
				imageBounds.Pop()
				a.drawWordImage(gtx, decoded, sourceSize, visualSize, imageRect, rotationDegrees)
				if selected {
					a.drawWordImageSelection(gtx, imageRect)
				}
				return layout.Dimensions{Size: containerSize}
			})
			if selected {
				// Keep the handles outside the image button's clip and replay them above
				// every inline sibling. This also keeps right-edge controls visible.
				overlayRecorder := op.Record(gtx.Ops)
				a.layoutWordImageResizeHandles(gtx, interaction, item, visualSize, imageRect)
				a.layoutWordImageRotationHandle(gtx, interaction, item, imageRect)
				overlay.call = overlayRecorder.Stop()
				overlay.ready = true
			}
			return dimensions
		})
	})
}

func layoutWordImageMoveDragArea(gtx layout.Context, interaction *wordImageInteraction, imageRect image.Rectangle, selected bool) {
	moveBounds := imageRect
	if selected {
		// Keep the move gesture out of the resize handle hit zones at the image edges.
		inset := gtx.Dp(unit.Dp(12))
		moveBounds = moveBounds.Inset(inset)
		if moveBounds.Empty() {
			return
		}
	}
	area := clip.Rect{Min: moveBounds.Min, Max: moveBounds.Max}.Push(gtx.Ops)
	interaction.moveDrag.Add(gtx.Ops)
	area.Pop()
}

func (a *App) drawWordImage(gtx layout.Context, decoded image.Image, sourceSize, visualSize image.Point, imageRect image.Rectangle, rotationDegrees float64) {
	center := f32.Pt(float32(visualSize.X)/2, float32(visualSize.Y)/2)
	offset := op.Offset(imageRect.Min).Push(gtx.Ops)
	imageOffset := op.Offset(image.Pt((visualSize.X-sourceSize.X)/2, (visualSize.Y-sourceSize.Y)/2)).Push(gtx.Ops)
	rotation := op.Affine(f32.AffineId().Rotate(center, float32(rotationDegrees*math.Pi/180))).Push(gtx.Ops)
	imageGtx := gtx
	imageGtx.Constraints = layout.Constraints{Min: sourceSize, Max: sourceSize}
	widget.Image{Src: paint.NewImageOp(decoded), Fit: widget.Contain, Position: layout.Center, Scale: float32(sourceSize.X) / float32(max(1, decoded.Bounds().Dx())) / max32(1, gtx.Metric.PxPerDp)}.Layout(imageGtx)
	rotation.Pop()
	imageOffset.Pop()
	offset.Pop()
}

func (a *App) drawWordImageSelection(gtx layout.Context, imageRect image.Rectangle) {
	border := max(1, gtx.Dp(unit.Dp(1)))
	blue := color.NRGBA{R: 38, G: 117, B: 214, A: 255}
	paint.FillShape(gtx.Ops, blue, clip.Rect{Min: imageRect.Min, Max: image.Pt(imageRect.Max.X, imageRect.Min.Y+border)}.Op())
	paint.FillShape(gtx.Ops, blue, clip.Rect{Min: image.Pt(imageRect.Min.X, imageRect.Max.Y-border), Max: imageRect.Max}.Op())
	paint.FillShape(gtx.Ops, blue, clip.Rect{Min: imageRect.Min, Max: image.Pt(imageRect.Min.X+border, imageRect.Max.Y)}.Op())
	paint.FillShape(gtx.Ops, blue, clip.Rect{Min: image.Pt(imageRect.Max.X-border, imageRect.Min.Y), Max: imageRect.Max}.Op())
}

func (a *App) updateWordImageMoveGesture(gtx layout.Context, interaction *wordImageInteraction, item document.WordImage, paragraphID string) {
	for event, ok := interaction.moveDrag.Update(gtx.Metric, gtx.Source, gesture.Both); ok; event, ok = interaction.moveDrag.Update(gtx.Metric, gtx.Source, gesture.Both) {
		if a.documentSessionState != nil {
			a.documentSessionState.wordCanvasContentClickHandled = true
		}
		switch event.Kind {
		case pointer.Press:
			interaction.moveStart = event.Position
			interaction.preview.BeginMove(item.ID, paragraphID)
		case pointer.Release:
			if interaction.preview.mode != wordImageGestureMove {
				continue
			}
			interaction.preview.ReleaseMove()
			preview := interaction.preview.Preview()
			if interaction.preview.CommitMove() {
				a.runWordImageCommand("移动图片", item.ID, func(editor document.WordImageEditor) error {
					return editor.MoveInlineImage(item.ID, preview.TargetParagraph, preview.AfterInlineID)
				}, false)
			}
			interaction.preview.Cancel()
		case pointer.Cancel:
			if interaction.preview.mode == wordImageGestureMove {
				interaction.preview.Cancel()
			}
		}
	}
}

func (a *App) layoutWordImageResizeHandles(gtx layout.Context, interaction *wordImageInteraction, item document.WordImage, frame image.Point, imageRect image.Rectangle) {
	const hitSizeDP = 18
	hitSize := gtx.Dp(unit.Dp(hitSizeDP))
	dotSize := max(6, gtx.Dp(unit.Dp(8)))
	for index, handle := range wordImageResizeHandles {
		center := image.Pt(imageRect.Min.X+frame.X/2+(handle.x*frame.X)/2, imageRect.Min.Y+frame.Y/2+(handle.y*frame.Y)/2)
		hitRect := image.Rectangle{Min: image.Pt(center.X-hitSize/2, center.Y-hitSize/2), Max: image.Pt(center.X+(hitSize+1)/2, center.Y+(hitSize+1)/2)}
		drag := &interaction.resizeDrags[index]
		area := clip.Rect{Min: hitRect.Min, Max: hitRect.Max}.Push(gtx.Ops)
		drag.Add(gtx.Ops)
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp("拖动调整图片大小：" + handle.name).Add(gtx.Ops)
		area.Pop()
		for event, ok := drag.Update(gtx.Metric, gtx.Source, gesture.Both); ok; event, ok = drag.Update(gtx.Metric, gtx.Source, gesture.Both) {
			a.documentSessionState.wordCanvasContentClickHandled = true
			switch event.Kind {
			case pointer.Press:
				interaction.resizeStart = event.Position
				interaction.resizeStartFrame = frame
				interaction.resizeStartEMU = item.WidthEMU
				interaction.preview.BeginResize(item.ID, item.WidthEMU, item.HeightEMU)
			case pointer.Drag:
				width := wordImageResizeWidthForHandle(interaction.resizeStartEMU, interaction.resizeStartFrame, index, event.Position.Sub(interaction.resizeStart))
				interaction.preview.ResizePreview(width)
			case pointer.Release:
				interaction.preview.ReleaseResize()
				preview := interaction.preview.Preview()
				if interaction.preview.CommitResize() {
					interaction.committingPreview = preview
					interaction.committingMode = wordImageGestureResize
					interaction.commitPending = true
					if !a.runWordImageCommand("调整图片", item.ID, func(editor document.WordImageEditor) error {
						return editor.ResizeInlineImage(item.ID, preview.WidthEMU)
					}, false) {
						finishWordImagePreviewCommand(a.documentSessionState, item.ID)
					}
				}
			case pointer.Cancel:
				interaction.preview.Cancel()
			}
		}
		paintWordImageHandle(gtx, center, dotSize)
	}
}

func (a *App) layoutWordImageRotationHandle(gtx layout.Context, interaction *wordImageInteraction, item document.WordImage, imageRect image.Rectangle) {
	const handleSizeDP = 18
	size := gtx.Dp(unit.Dp(handleSizeDP))
	gap := gtx.Dp(unit.Dp(5))
	center := f32.Pt(float32(imageRect.Min.X+imageRect.Dx()/2), float32(imageRect.Min.Y+imageRect.Dy()/2))
	rotationPoint := image.Pt(int(center.X), imageRect.Min.Y-gap)
	hitRect := image.Rectangle{Min: image.Pt(rotationPoint.X-size/2, rotationPoint.Y-size/2), Max: image.Pt(rotationPoint.X+(size+1)/2, rotationPoint.Y+(size+1)/2)}
	area := clip.Rect{Min: hitRect.Min, Max: hitRect.Max}.Push(gtx.Ops)
	interaction.rotationDrag.Add(gtx.Ops)
	semantic.Button.Add(gtx.Ops)
	semantic.DescriptionOp("拖动旋转图片").Add(gtx.Ops)
	area.Pop()
	for event, ok := interaction.rotationDrag.Update(gtx.Metric, gtx.Source, gesture.Both); ok; event, ok = interaction.rotationDrag.Update(gtx.Metric, gtx.Source, gesture.Both) {
		a.documentSessionState.wordCanvasContentClickHandled = true
		switch event.Kind {
		case pointer.Press:
			interaction.rotationStart = event.Position
			interaction.rotationCenter = center
			interaction.rotationStartDeg = item.RotationDegrees
			interaction.preview.BeginRotate(item.ID, item.RotationDegrees)
		case pointer.Drag:
			interaction.preview.RotatePreview(wordImageRotationForPointer(interaction.rotationStartDeg, interaction.rotationCenter, interaction.rotationStart, event.Position))
		case pointer.Release:
			interaction.preview.ReleaseRotate()
			preview := interaction.preview.Preview()
			if interaction.preview.CommitRotate() {
				interaction.committingPreview = preview
				interaction.committingMode = wordImageGestureRotate
				interaction.commitPending = true
				if !a.runWordImageRotationCommand(item.ID, preview.RotationDegrees) {
					finishWordImagePreviewCommand(a.documentSessionState, item.ID)
				}
			}
		case pointer.Cancel:
			interaction.preview.Cancel()
		}
	}
	dotSize := max(6, gtx.Dp(unit.Dp(8)))
	stem := image.Rectangle{Min: image.Pt(rotationPoint.X-1, rotationPoint.Y+dotSize/2), Max: image.Pt(rotationPoint.X+1, imageRect.Min.Y+1)}
	paint.FillShape(gtx.Ops, color.NRGBA{R: 38, G: 117, B: 214, A: 255}, clip.Rect{Min: stem.Min, Max: stem.Max}.Op())
	paintWordImageHandle(gtx, rotationPoint, dotSize)
}

func paintWordImageHandle(gtx layout.Context, center image.Point, size int) {
	outer := image.Rectangle{Min: image.Pt(center.X-size/2, center.Y-size/2), Max: image.Pt(center.X+(size+1)/2, center.Y+(size+1)/2)}
	paint.FillShape(gtx.Ops, color.NRGBA{R: 38, G: 117, B: 214, A: 255}, clip.Ellipse(outer).Op(gtx.Ops))
	innerSize := max(2, size-4)
	inner := image.Rectangle{Min: image.Pt(center.X-innerSize/2, center.Y-innerSize/2), Max: image.Pt(center.X+(innerSize+1)/2, center.Y+(innerSize+1)/2)}
	paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Ellipse(inner).Op(gtx.Ops))
}

func (a *App) runWordImageRotationCommand(imageID string, rotation float64) bool {
	return a.runWordImageCommand("旋转图片", imageID, func(editor document.WordImageEditor) error {
		rotator, ok := any(editor).(document.WordImageRotator)
		if !ok {
			return fmt.Errorf("当前 Word 适配器不支持旋转图片")
		}
		return rotator.SetInlineImageRotation(imageID, rotation)
	}, false)
}

func wordImageEMUToPixels(widthEMU int64, pxPerDp float32) int {
	if widthEMU <= 0 || pxPerDp <= 0 {
		return 0
	}
	pixels := float64(widthEMU) * 96 * float64(pxPerDp) / 914400
	if pixels >= float64(math.MaxInt) {
		return math.MaxInt
	}
	return max(1, int(math.Round(pixels)))
}
