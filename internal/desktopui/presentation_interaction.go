package desktopui

import (
	"image"
	"image/color"
	"math"
	"strconv"

	"gioui.org/f32"
	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"glance/internal/document"
)

const (
	presentationDragThresholdDP unit.Dp = 4
	presentationHandleHitDP     unit.Dp = 20
	presentationHandlePaintDP   unit.Dp = 8
	presentationRotationGapDP   unit.Dp = 24
	presentationMoveInsetDP     unit.Dp = 12
)

type presentationTransformGestureMode uint8

const (
	presentationGestureNone presentationTransformGestureMode = iota
	presentationGestureMove
	presentationGestureResize
	presentationGestureRotate
)

type presentationShapeInteraction struct {
	moveDrag     gesture.Drag
	resizeDrags  [8]gesture.Drag
	rotationDrag gesture.Drag
	cropDrags    [8]gesture.Drag
	cropPanDrag  gesture.Drag
}

// presentationTransformGesture keeps committed geometry separate from the
// pointer preview. AnchorSlide is normalized once from the input event.
type presentationTransformGesture struct {
	active      bool
	started     bool
	shapeID     int64
	mode        presentationTransformGestureMode
	handle      presentationTransformHandle
	original    presentationTransform
	preview     presentationTransform
	pressDevice f32.Point
	anchorSlide document.PresentationPoint
}

func (v *presentationViewState) shapeInteraction(id int64) *presentationShapeInteraction {
	if v.transformInputs == nil {
		v.transformInputs = make(map[int64]*presentationShapeInteraction)
	}
	interaction := v.transformInputs[id]
	if interaction == nil {
		interaction = &presentationShapeInteraction{}
		v.transformInputs[id] = interaction
	}
	return interaction
}

func (a *App) updatePresentationElementGestures(gtx layout.Context, v *presentationViewState, elem document.PresentationElement, interaction *presentationShapeInteraction) {
	if interaction == nil || v == nil {
		return
	}
	if len(presentationSelectedIDs(v)) > 1 {
		return
	}
	if v.pictureCrop.active && v.pictureCrop.shapeID == elem.ID {
		a.updatePresentationCropGestures(gtx, v, elem, interaction)
		return
	}
	if !a.saving {
		a.updatePresentationGesture(gtx, v, elem, &interaction.moveDrag, presentationGestureMove, 0)
	}
	if v.selectedShape != elem.ID || a.saving {
		return
	}
	for index := range interaction.resizeDrags {
		a.updatePresentationGesture(gtx, v, elem, &interaction.resizeDrags[index], presentationGestureResize, presentationTransformHandle(index))
	}
	a.updatePresentationGesture(gtx, v, elem, &interaction.rotationDrag, presentationGestureRotate, 0)
}

func (a *App) updatePresentationGesture(gtx layout.Context, v *presentationViewState, elem document.PresentationElement, drag *gesture.Drag, mode presentationTransformGestureMode, handle presentationTransformHandle) {
	for event, ok := drag.Update(gtx.Metric, gtx.Source, gesture.Both); ok; event, ok = drag.Update(gtx.Metric, gtx.Source, gesture.Both) {
		if a.saving || a.pptxDoc == nil || v.activeSlide < 0 {
			continue
		}
		switch event.Kind {
		case pointer.Press:
			if v.transformGesture.active {
				continue
			}
			v.transformGesture = presentationTransformGesture{
				active:      true,
				shapeID:     elem.ID,
				mode:        mode,
				handle:      handle,
				original:    presentationTransform{Bounds: elem.Bounds, RotationDeg: elem.RotationDeg},
				preview:     presentationTransform{Bounds: elem.Bounds, RotationDeg: elem.RotationDeg},
				pressDevice: event.Position,
				anchorSlide: presentationDevicePointToSlide(event.Position, f32.Point{}, v.canvasScale),
			}
		case pointer.Drag:
			gestureState := &v.transformGesture
			if !gestureState.active || gestureState.shapeID != elem.ID || gestureState.mode != mode {
				continue
			}
			delta := event.Position.Sub(gestureState.pressDevice)
			slop := float32(gtx.Dp(presentationDragThresholdDP))
			if !gestureState.started && delta.X*delta.X+delta.Y*delta.Y <= slop*slop {
				continue
			}
			gestureState.started = true
			v.selectedShape = elem.ID
			if a.documentSessionState != nil && a.documentSessionState.pptxDoc == a.pptxDoc {
				a.documentSessionState.setPresentationDraftDirty(a.windowID, presentationTransformDraft, true)
			}
			current := presentationDevicePointToSlide(event.Position, f32.Point{}, v.canvasScale)
			gestureState.preview = previewPresentationGesture(*gestureState, current, elem.AspectRatioLocked)
		case pointer.Release:
			gestureState := v.transformGesture
			if !gestureState.active || gestureState.shapeID != elem.ID || gestureState.mode != mode {
				continue
			}
			if gestureState.started {
				current := presentationDevicePointToSlide(event.Position, f32.Point{}, v.canvasScale)
				gestureState.preview = previewPresentationGesture(gestureState, current, elem.AspectRatioLocked)
				a.commitPresentationTransformGesture(v, gestureState)
			}
			if a.documentSessionState != nil && a.documentSessionState.pptxDoc == a.pptxDoc {
				a.documentSessionState.setPresentationDraftDirty(a.windowID, presentationTransformDraft, false)
			}
			v.transformGesture = presentationTransformGesture{}
		case pointer.Cancel:
			cancelPresentationTransformGesture(v, elem.ID, mode)
			if a.documentSessionState != nil && a.documentSessionState.pptxDoc == a.pptxDoc {
				a.documentSessionState.setPresentationDraftDirty(a.windowID, presentationTransformDraft, false)
			}
		}
	}
}

func cancelPresentationTransformGesture(v *presentationViewState, shapeID int64, mode presentationTransformGestureMode) {
	if v != nil && v.transformGesture.active && v.transformGesture.shapeID == shapeID && v.transformGesture.mode == mode {
		v.transformGesture = presentationTransformGesture{}
	}
}

func previewPresentationGesture(gestureState presentationTransformGesture, current document.PresentationPoint, aspectRatioLocked bool) presentationTransform {
	switch gestureState.mode {
	case presentationGestureMove:
		return presentationMovePreviewPoints(gestureState.original, gestureState.anchorSlide, current)
	case presentationGestureResize:
		return presentationResizePreviewPoints(gestureState.original, gestureState.handle, gestureState.anchorSlide, current, aspectRatioLocked)
	case presentationGestureRotate:
		return presentationRotatePreviewPoints(gestureState.original, gestureState.anchorSlide, current)
	default:
		return gestureState.original
	}
}

func (a *App) commitPresentationTransformGesture(v *presentationViewState, gestureState presentationTransformGesture) {
	if v == nil || a.pptxDoc == nil || !gestureState.started {
		return
	}
	if gestureState.preview.Bounds == gestureState.original.Bounds && presentationNormalizeAngle(gestureState.preview.RotationDeg) == presentationNormalizeAngle(gestureState.original.RotationDeg) {
		return
	}
	slide := v.currentSlide()
	if slide == nil || slide.ID == 0 {
		return
	}
	state := a.documentSessionState
	var err error
	if state != nil && state.pptxDoc == a.pptxDoc && a.applicationSessionRegistered(state.documentID) {
		token, ok := state.beginOperation(sessionOperationMutation)
		if !ok {
			a.setStatus("演示文稿正在处理其他操作")
			return
		}
		_, err = a.executeRibbonApplicationCommand(token, "presentation.element.transform.set", "presentation_element", strconv.FormatInt(gestureState.shapeID, 10), "", map[string]any{
			"slide_id": slide.ID, "element_id": gestureState.shapeID,
			"x": gestureState.preview.Bounds.X, "y": gestureState.preview.Bounds.Y,
			"width": gestureState.preview.Bounds.W, "height": gestureState.preview.Bounds.H,
			"rotation_deg": gestureState.preview.RotationDeg,
		})
		state.finishOperation(token)
	} else if state == nil || state.pptxDoc != a.pptxDoc {
		err = a.pptxDoc.UpdateSlideElementTransformByID(slide.ID, gestureState.shapeID, gestureState.preview.Bounds, gestureState.preview.RotationDeg)
	} else {
		before := a.pptxDoc.Snapshot().Revision
		unlock := lockSessionAdapterWrite(state)
		err = a.pptxDoc.UpdateSlideElementTransformByID(slide.ID, gestureState.shapeID, gestureState.preview.Bounds, gestureState.preview.RotationDeg)
		unlock()
		if err == nil && a.pptxDoc.Snapshot().Revision != before {
			a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load()+1)
		}
	}
	if err != nil {
		a.setStatus("对象变换失败：" + err.Error())
		return
	}
	v.snapshot = a.pptxDoc.Snapshot()
	v.selectedShape = gestureState.shapeID
	a.setStatus("对象位置或大小已修改；点击保存写入文件")
}

func presentationBoundsInPixels(bounds document.PresentationRect, scale float64) image.Rectangle {
	left := int(math.Round(bounds.X * scale))
	top := int(math.Round(bounds.Y * scale))
	right := int(math.Round((bounds.X + bounds.W) * scale))
	bottom := int(math.Round((bounds.Y + bounds.H) * scale))
	if right <= left {
		right = left + 1
	}
	if bottom <= top {
		bottom = top + 1
	}
	return image.Rect(left, top, right, bottom)
}

func presentationTransformHandleCenter(rect image.Rectangle, handle presentationTransformHandle) image.Point {
	center := image.Pt(rect.Min.X+rect.Dx()/2, rect.Min.Y+rect.Dy()/2)
	x, y := center.X, center.Y
	switch handle {
	case presentationHandleTopLeft, presentationHandleLeft, presentationHandleBottomLeft:
		x = rect.Min.X
	case presentationHandleTopRight, presentationHandleRight, presentationHandleBottomRight:
		x = rect.Max.X
	}
	switch handle {
	case presentationHandleTopLeft, presentationHandleTop, presentationHandleTopRight:
		y = rect.Min.Y
	case presentationHandleBottomLeft, presentationHandleBottom, presentationHandleBottomRight:
		y = rect.Max.Y
	}
	return image.Pt(x, y)
}

func presentationRotationHandleCenter(rect image.Rectangle, gapPixels int) image.Point {
	return image.Pt(rect.Min.X+rect.Dx()/2, rect.Min.Y-gapPixels)
}

func rotatePresentationPixel(point, center image.Point, rotationDeg float64) image.Point {
	angle := presentationNormalizeAngle(rotationDeg) * math.Pi / 180
	dx, dy := float64(point.X-center.X), float64(point.Y-center.Y)
	return image.Pt(
		center.X+int(math.Round(dx*math.Cos(angle)-dy*math.Sin(angle))),
		center.Y+int(math.Round(dx*math.Sin(angle)+dy*math.Cos(angle))),
	)
}

func presentationRotatedBounds(rect image.Rectangle, rotationDeg float64) image.Rectangle {
	if math.Mod(presentationNormalizeAngle(rotationDeg), 360) == 0 {
		return rect
	}
	center := image.Pt(rect.Min.X+rect.Dx()/2, rect.Min.Y+rect.Dy()/2)
	corners := []image.Point{rect.Min, {X: rect.Max.X, Y: rect.Min.Y}, rect.Max, {X: rect.Min.X, Y: rect.Max.Y}}
	rotated := make([]image.Point, 0, len(corners))
	for _, corner := range corners {
		rotated = append(rotated, rotatePresentationPixel(corner, center, rotationDeg))
	}
	result := image.Rectangle{Min: rotated[0], Max: rotated[0]}
	for _, point := range rotated[1:] {
		result.Min.X = min(result.Min.X, point.X)
		result.Min.Y = min(result.Min.Y, point.Y)
		result.Max.X = max(result.Max.X, point.X)
		result.Max.Y = max(result.Max.Y, point.Y)
	}
	return result
}

func presentationRotatedRectArea(gtx layout.Context, outer, inner image.Rectangle, rotationDeg float64) clip.Stack {
	center := image.Pt(outer.Min.X+outer.Dx()/2, outer.Min.Y+outer.Dy()/2)
	corners := []image.Point{inner.Min, {X: inner.Max.X, Y: inner.Min.Y}, inner.Max, {X: inner.Min.X, Y: inner.Max.Y}}
	var path clip.Path
	path.Begin(gtx.Ops)
	for index, corner := range corners {
		point := rotatePresentationPixel(corner, center, rotationDeg)
		if index == 0 {
			path.MoveTo(f32.Pt(float32(point.X), float32(point.Y)))
		} else {
			path.LineTo(f32.Pt(float32(point.X), float32(point.Y)))
		}
	}
	path.Close()
	return clip.Outline{Path: path.End()}.Op().Push(gtx.Ops)
}

func presentationRotationHandleGap(gtx layout.Context) int {
	return gtx.Dp(presentationRotationGapDP)
}

func presentationHandleHitSize(gtx layout.Context) int {
	return gtx.Dp(presentationHandleHitDP)
}

func presentationHandlePaintSize(gtx layout.Context) int {
	return gtx.Dp(presentationHandlePaintDP)
}

func drawPresentationTransformSelection(gtx layout.Context, rect image.Rectangle) {
	outline := pptxAccentColor
	paint.FillShape(gtx.Ops, outline, clip.Stroke{Path: clip.Rect{Min: rect.Min, Max: rect.Max}.Path(), Width: float32(gtx.Dp(unit.Dp(1.5)))}.Op())
	paintHandle := func(center image.Point) {
		size := presentationHandlePaintSize(gtx)
		handle := image.Rectangle{Min: image.Pt(center.X-size/2, center.Y-size/2), Max: image.Pt(center.X+(size+1)/2, center.Y+(size+1)/2)}
		paint.FillShape(gtx.Ops, colorWhite, clip.Rect(handle).Op())
		paint.FillShape(gtx.Ops, outline, clip.Stroke{Path: clip.Rect(handle).Path(), Width: 1}.Op())
	}
	for _, handle := range presentationResizeHandles {
		paintHandle(presentationTransformHandleCenter(rect, handle))
	}
	rotationPoint := presentationRotationHandleCenter(rect, presentationRotationHandleGap(gtx))
	stem := image.Rect(rect.Min.X+rect.Dx()/2-1, rotationPoint.Y, rect.Min.X+rect.Dx()/2+1, rect.Min.Y)
	paint.FillShape(gtx.Ops, outline, clip.Rect(stem).Op())
	paintHandle(rotationPoint)
}

var colorWhite = color.NRGBA{R: 255, G: 255, B: 255, A: 255}

var presentationResizeHandles = [...]presentationTransformHandle{
	presentationHandleTopLeft, presentationHandleTop, presentationHandleTopRight, presentationHandleRight,
	presentationHandleBottomRight, presentationHandleBottom, presentationHandleBottomLeft, presentationHandleLeft,
}

func registerPresentationTransformInputs(gtx layout.Context, v *presentationViewState, elem document.PresentationElement, interaction *presentationShapeInteraction, rect image.Rectangle, selected, editing bool) {
	if interaction == nil || v == nil {
		return
	}
	var moveRects []image.Rectangle
	switch {
	case editing && selected:
		// Keep the text editor's interior available for caret placement and
		// selection. The inset frame band remains a move target while editing.
		band := min(gtx.Dp(presentationMoveInsetDP), max(1, min(rect.Dx(), rect.Dy())/3))
		moveRects = []image.Rectangle{
			image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, min(rect.Max.Y, rect.Min.Y+band)),
			image.Rect(rect.Min.X, max(rect.Min.Y, rect.Max.Y-band), rect.Max.X, rect.Max.Y),
			image.Rect(rect.Min.X, rect.Min.Y+band, min(rect.Max.X, rect.Min.X+band), rect.Max.Y-band),
			image.Rect(max(rect.Min.X, rect.Max.X-band), rect.Min.Y+band, rect.Max.X, rect.Max.Y-band),
		}
	case selected:
		moveRects = []image.Rectangle{rect.Inset(gtx.Dp(presentationMoveInsetDP))}
	default:
		moveRects = []image.Rectangle{rect}
	}
	pass := pointer.PassOp{}.Push(gtx.Ops)
	for _, moveRect := range moveRects {
		if moveRect.Empty() {
			continue
		}
		area := presentationRotatedRectArea(gtx, rect, moveRect, elem.RotationDeg)
		interaction.moveDrag.Add(gtx.Ops)
		area.Pop()
	}
	pass.Pop()
	if !selected {
		return
	}
	hitSize := presentationHandleHitSize(gtx)
	center := image.Pt(rect.Min.X+rect.Dx()/2, rect.Min.Y+rect.Dy()/2)
	for index, handle := range presentationResizeHandles {
		local := presentationTransformHandleCenter(rect, handle)
		handleCenter := rotatePresentationPixel(local, center, elem.RotationDeg)
		hitRect := image.Rect(handleCenter.X-hitSize/2, handleCenter.Y-hitSize/2, handleCenter.X+(hitSize+1)/2, handleCenter.Y+(hitSize+1)/2)
		area := clip.Rect{Min: hitRect.Min, Max: hitRect.Max}.Push(gtx.Ops)
		interaction.resizeDrags[index].Add(gtx.Ops)
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp("拖动调整对象大小").Add(gtx.Ops)
		area.Pop()
	}
	rotationPoint := presentationRotationHandleCenter(rect, presentationRotationHandleGap(gtx))
	rotationPoint = rotatePresentationPixel(rotationPoint, center, elem.RotationDeg)
	hitRect := image.Rect(rotationPoint.X-hitSize/2, rotationPoint.Y-hitSize/2, rotationPoint.X+(hitSize+1)/2, rotationPoint.Y+(hitSize+1)/2)
	area := clip.Rect{Min: hitRect.Min, Max: hitRect.Max}.Push(gtx.Ops)
	interaction.rotationDrag.Add(gtx.Ops)
	semantic.Button.Add(gtx.Ops)
	semantic.DescriptionOp("拖动旋转对象").Add(gtx.Ops)
	area.Pop()
}
