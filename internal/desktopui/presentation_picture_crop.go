package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/i18n"
	"image"
	"math"

	"gioui.org/f32"
	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"glance/internal/application"
	"glance/internal/document"
)

type presentationPictureCropState struct {
	active          bool
	committing      bool
	slideIndex      int
	baseRevision    uint64
	baseRevisionSet bool
	shapeID         int64
	original        document.PresentationCrop
	preview         document.PresentationCrop
	gesture         presentationPictureCropGesture
}

type presentationPictureCropGestureMode uint8

const (
	presentationCropGestureResize presentationPictureCropGestureMode = iota + 1
	presentationCropGesturePan
)

type presentationPictureCropGesture struct {
	active      bool
	started     bool
	shapeID     int64
	mode        presentationPictureCropGestureMode
	handle      presentationTransformHandle
	original    document.PresentationCrop
	pressDevice f32.Point
	rotationDeg float64
	frame       document.PresentationRect
}

func (a *App) beginPresentationPictureCrop(v *presentationViewState) bool {
	if v == nil || a.pptxDoc == nil || a.saving || (a.documentSessionState != nil && a.documentSessionState.formatRunning.Load()) {
		return false
	}
	slideIndex, picture, ok := presentationSelectedEditablePicture(v)
	if !ok || picture.Bounds.W <= 0 || picture.Bounds.H <= 0 {
		return false
	}
	if v.pictureCrop.active {
		return v.pictureCrop.shapeID == picture.ID && v.pictureCrop.slideIndex == slideIndex
	}
	baseRevision, baseRevisionSet := uint64(0), false
	if a.documentSessionState != nil {
		baseRevision = a.documentSessionState.wordRevisionAtomic.Load()
		baseRevisionSet = true
	}
	v.pictureCrop = presentationPictureCropState{
		active: true, slideIndex: slideIndex, baseRevision: baseRevision, baseRevisionSet: baseRevisionSet, shapeID: picture.ID,
		original: picture.Crop, preview: picture.Crop,
	}
	a.markPresentationCropDraft(v, true)
	v.activeRibbonTab = "picture"
	a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.drag_a_picture_edge_to_crop_it_or_drag_the_0ea7df"}, feedbackInfo, notificationToast)
	if a.window != nil {
		a.window.Invalidate()
	}
	return true
}

func (a *App) cancelPresentationPictureCrop(v *presentationViewState) {
	if v == nil || !v.pictureCrop.active || v.pictureCrop.committing {
		return
	}
	v.pictureCrop = presentationPictureCropState{}
	a.markPresentationCropDraft(v, false)
	a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.picture_cropping_canceled_4ccc77"}, feedbackSuccess, notificationSuppressed)
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) resetPresentationPictureCrop(v *presentationViewState) {
	if v == nil || !v.pictureCrop.active || v.pictureCrop.committing {
		return
	}
	v.pictureCrop.preview = document.PresentationCrop{}
	v.pictureCrop.gesture = presentationPictureCropGesture{}
	a.markPresentationCropDraft(v, true)
	a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.original_picture_bounds_previewed_finish_c_8bb660"}, feedbackSuccess, notificationToast)
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) markPresentationCropDraft(v *presentationViewState, dirty bool) {
	if a == nil || v == nil || a.documentSessionState == nil || a.documentSessionState.pptxDoc != a.pptxDoc {
		return
	}
	a.documentSessionState.setPresentationDraftDirty(a.windowID, presentationCropDraft, dirty)
}

type presentationpictureSlideIdElementIdArguments struct {
	SlideId   uint32 `json:"slide_id"`
	ElementId int64  `json:"element_id"`
	Left      int    `json:"left"`
	Top       int    `json:"top"`
	Right     int    `json:"right"`
	Bottom    int    `json:"bottom"`
}

func (a *App) commitPresentationPictureCrop(v *presentationViewState) {
	if v == nil || !v.pictureCrop.active || v.pictureCrop.committing || a.pptxDoc == nil || a.documentSessionState == nil {
		return
	}
	state, doc := a.documentSessionState, a.pptxDoc
	cropState := v.pictureCrop
	if cropState.slideIndex != v.activeSlide || cropState.shapeID != v.selectedShape || state.pptxDoc != doc || state.closed.Load() || state.formatRunning.Load() || a.isSaving() {
		return
	}
	if cropState.baseRevisionSet && state.wordRevisionAtomic.Load() != cropState.baseRevision {
		v.pictureCrop = presentationPictureCropState{}
		a.markPresentationCropDraft(v, false)
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_document_changed_while_the_picture_was_0ea260"}, feedbackInfo, notificationToast)
		return
	}
	if cropState.preview == cropState.original {
		a.cancelPresentationPictureCrop(v)
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_picture_crop_has_not_changed_6db3f6"}, feedbackInfo, notificationToast)
		return
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	if cropState.baseRevisionSet && token.revision != cropState.baseRevision {
		state.finishOperation(token)
		v.pictureCrop = presentationPictureCropState{}
		a.markPresentationCropDraft(v, false)
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_document_changed_while_the_picture_was_0ea260"}, feedbackInfo, notificationToast)
		return
	}
	if cropState.slideIndex < 0 || cropState.slideIndex >= len(v.snapshot.Slides) {
		state.finishOperation(token)
		v.pictureCrop = presentationPictureCropState{}
		a.markPresentationCropDraft(v, false)
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_picture_crop_target_no_longer_exists_b524e9"}, feedbackInfo, notificationToast)
		return
	}
	slideID := v.snapshot.Slides[cropState.slideIndex].ID
	v.pictureCrop.committing = true
	a.markPresentationCropDraft(v, false)
	if a.pptxPicCommandResults == nil {
		a.pptxPicCommandResults = make(chan presentationPictureCommandResult, 1)
	}
	result := presentationPictureCommandResult{
		state: state, doc: doc, token: token, operation: "裁剪图片",
		generation: a.documentGeneration, slideIndex: cropState.slideIndex,
		shapeID: cropState.shapeID, crop: cropState.preview,
	}
	results, shutdown, window := a.pptxPicCommandResults, a.shutdown, a.window
	a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.finishing_the_picture_crop_9bd9d9"}, feedbackInfo, notificationSuppressed)
	a.launchManagedBackground(func() {
		beforeRevision := doc.Revision()
		if a.applicationSessionRegistered(token.documentID) {
			commandResult, err := a.executeRibbonApplicationCommand(token, "presentation.element.picture.crop.set", "presentation_picture", fmt.Sprint(result.shapeID), "", presentationpictureSlideIdElementIdArguments{SlideId: slideID, ElementId: result.shapeID, Left: result.crop.Left, Top: result.crop.Top, Right: result.crop.Right, Bottom: result.crop.Bottom})
			result.err = err
			if err == nil {
				var payload struct {
					SlideID   uint32 `json:"slide_id"`
					ElementID int64  `json:"element_id"`
					Changed   bool   `json:"changed"`
				}
				if json.Unmarshal(commandResult.Data, &payload) != nil || payload.SlideID != slideID || payload.ElementID != result.shapeID {
					result.err = application.ErrInvalidCommandResult
				} else {
					result.mutated = payload.Changed
				}
			}
		} else {
			unlock := lockSessionAdapterWrite(state)
			result.err = doc.UpdateSlidePictureCrop(result.slideIndex, result.shapeID, result.crop)
			unlock()
		}
		if result.err == nil {
			result.snapshot = doc.Snapshot()
			result.mutated = result.mutated || doc.Revision() != beforeRevision
		}
		state.finishOperation(token)
		result.revision = state.wordRevisionAtomic.Load()
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	})
}

func presentationCropLocalPointerDelta(start, current f32.Point, rotationDeg, scale float64) document.PresentationPoint {
	if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		scale = 1
	}
	dx := float64(current.X-start.X) / scale
	dy := float64(current.Y-start.Y) / scale
	angle := presentationNormalizeAngle(rotationDeg) * math.Pi / 180
	cosine, sine := math.Cos(angle), math.Sin(angle)
	return document.PresentationPoint{
		X: dx*cosine + dy*sine,
		Y: -dx*sine + dy*cosine,
	}
}

func previewPresentationPictureCrop(original document.PresentationCrop, mode presentationPictureCropGestureMode, handle presentationTransformHandle, localDelta document.PresentationPoint, bounds document.PresentationRect) document.PresentationCrop {
	if bounds.W <= 0 || bounds.H <= 0 {
		return original
	}
	deltaX := int(math.Round(localDelta.X / bounds.W * 100000))
	deltaY := int(math.Round(localDelta.Y / bounds.H * 100000))
	if mode == presentationCropGesturePan {
		shiftX := clampPresentationCropShift(-deltaX, -original.Left, 100000-original.Right)
		shiftY := clampPresentationCropShift(-deltaY, -original.Top, 100000-original.Bottom)
		original.Left += shiftX
		original.Right -= shiftX
		original.Top += shiftY
		original.Bottom -= shiftY
		return original
	}
	left, right, top, bottom, ok := presentationResizeHandleSides(handle)
	if !ok {
		return original
	}
	if left {
		original.Left = min(99999-original.Right, max(0, original.Left+deltaX))
	} else if right {
		original.Right = min(99999-original.Left, max(0, original.Right-deltaX))
	}
	if top {
		original.Top = min(99999-original.Bottom, max(0, original.Top+deltaY))
	} else if bottom {
		original.Bottom = min(99999-original.Top, max(0, original.Bottom-deltaY))
	}
	return original
}

func clampPresentationCropShift(shift, minimum, maximum int) int {
	if minimum > maximum {
		return 0
	}
	return min(maximum, max(minimum, shift))
}

func (a *App) updatePresentationCropGestures(gtx layout.Context, v *presentationViewState, elem document.PresentationElement, interaction *presentationShapeInteraction) {
	if v == nil || interaction == nil || !v.pictureCrop.active || v.pictureCrop.committing || v.pictureCrop.shapeID != elem.ID || a.saving || a.pptxDoc == nil {
		return
	}
	for index := range interaction.cropDrags {
		a.updatePresentationCropGesture(gtx, v, elem, &interaction.cropDrags[index], presentationCropGestureResize, presentationTransformHandle(index))
	}
	a.updatePresentationCropGesture(gtx, v, elem, &interaction.cropPanDrag, presentationCropGesturePan, 0)
}

func (a *App) updatePresentationCropGesture(gtx layout.Context, v *presentationViewState, elem document.PresentationElement, drag *gesture.Drag, mode presentationPictureCropGestureMode, handle presentationTransformHandle) {
	for event, ok := drag.Update(gtx.Metric, gtx.Source, gesture.Both); ok; event, ok = drag.Update(gtx.Metric, gtx.Source, gesture.Both) {
		if !v.pictureCrop.active || v.pictureCrop.shapeID != elem.ID || v.pictureCrop.committing || a.saving {
			continue
		}
		switch event.Kind {
		case pointer.Press:
			if v.pictureCrop.gesture.active {
				continue
			}
			v.pictureCrop.gesture = presentationPictureCropGesture{
				active: true, shapeID: elem.ID, mode: mode, handle: handle,
				original: v.pictureCrop.preview, pressDevice: event.Position,
				rotationDeg: elem.RotationDeg, frame: elem.Bounds,
			}
		case pointer.Drag:
			gestureState := &v.pictureCrop.gesture
			if !gestureState.active || gestureState.shapeID != elem.ID || gestureState.mode != mode || gestureState.handle != handle {
				continue
			}
			dx := float64(event.Position.X - gestureState.pressDevice.X)
			dy := float64(event.Position.Y - gestureState.pressDevice.Y)
			slop := float64(gtx.Dp(presentationDragThresholdDP))
			if !gestureState.started && dx*dx+dy*dy <= slop*slop {
				continue
			}
			gestureState.started = true
			localDelta := presentationCropLocalPointerDelta(gestureState.pressDevice, event.Position, gestureState.rotationDeg, v.canvasScale)
			v.pictureCrop.preview = previewPresentationPictureCrop(gestureState.original, gestureState.mode, gestureState.handle, localDelta, gestureState.frame)
		case pointer.Release:
			gestureState := v.pictureCrop.gesture
			if !gestureState.active || gestureState.shapeID != elem.ID || gestureState.mode != mode || gestureState.handle != handle {
				continue
			}
			if gestureState.started {
				localDelta := presentationCropLocalPointerDelta(gestureState.pressDevice, event.Position, gestureState.rotationDeg, v.canvasScale)
				v.pictureCrop.preview = previewPresentationPictureCrop(gestureState.original, gestureState.mode, gestureState.handle, localDelta, gestureState.frame)
			}
			v.pictureCrop.gesture = presentationPictureCropGesture{}
		case pointer.Cancel:
			gestureState := v.pictureCrop.gesture
			if gestureState.active && gestureState.shapeID == elem.ID && gestureState.mode == mode && gestureState.handle == handle {
				v.pictureCrop.preview = gestureState.original
				v.pictureCrop.gesture = presentationPictureCropGesture{}
			}
		}
	}
}

func drawPresentationCropSelection(gtx layout.Context, rect image.Rectangle) {
	outline := pptxAccentColor
	paint.FillShape(gtx.Ops, outline, clip.Stroke{Path: clip.Rect{Min: rect.Min, Max: rect.Max}.Path(), Width: float32(gtx.Dp(unit.Dp(1.5)))}.Op())
	for _, handle := range presentationResizeHandles {
		center := presentationTransformHandleCenter(rect, handle)
		size := presentationHandlePaintSize(gtx)
		bounds := image.Rect(center.X-size/2, center.Y-size/2, center.X+(size+1)/2, center.Y+(size+1)/2)
		paint.FillShape(gtx.Ops, colorWhite, clip.Rect(bounds).Op())
		paint.FillShape(gtx.Ops, outline, clip.Stroke{Path: clip.Rect(bounds).Path(), Width: 1}.Op())
	}
}

func registerPresentationCropInputs(gtx layout.Context, app *App, v *presentationViewState, elem document.PresentationElement, interaction *presentationShapeInteraction, rect image.Rectangle) {
	if interaction == nil || v == nil || !v.pictureCrop.active || v.pictureCrop.committing {
		return
	}
	pass := pointer.PassOp{}.Push(gtx.Ops)
	center := image.Pt(rect.Min.X+rect.Dx()/2, rect.Min.Y+rect.Dy()/2)
	panArea := presentationRotatedRectArea(gtx, rect, rect.Inset(presentationHandleHitSize(gtx)/2), elem.RotationDeg)
	interaction.cropPanDrag.Add(gtx.Ops)
	semantic.DescriptionOp(app.textID("presentation.accessibility.pan_crop")).Add(gtx.Ops)
	panArea.Pop()
	hitSize := presentationHandleHitSize(gtx)
	for index, handle := range presentationResizeHandles {
		local := presentationTransformHandleCenter(rect, handle)
		handleCenter := rotatePresentationPixel(local, center, elem.RotationDeg)
		hitRect := presentationInteractionHitRect(handleCenter, hitSize)
		area := clip.Rect{Min: hitRect.Min, Max: hitRect.Max}.Push(gtx.Ops)
		interaction.cropDrags[index].Add(gtx.Ops)
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(app.textID("presentation.accessibility.crop_handle")).Add(gtx.Ops)
		area.Pop()
	}
	pass.Pop()
}
