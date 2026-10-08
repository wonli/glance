package desktopui

import (
	"fmt"
	"image"
	"math"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"

	"glance/internal/document"
)

func TestPresentationCanvasSelectsTextShapeAndPicture(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	h.frame()
	point := h.textClickPoint(110, 10)
	h.click(point)
	if h.view.selectedShape != h.elem.ID || h.view.editingShape != 0 {
		t.Fatalf("first nonempty-text click = selected %d, editing %d; want select-only (history=%+v hit=%+v point=%v)", h.view.selectedShape, h.view.editingShape, h.view.shapeClick(h.elem.ID).History(), h.view.shapeHitAreas[h.elem.ID], point)
	}
	h.click(point)
	if h.view.editingShape != h.elem.ID {
		t.Fatalf("second nonempty-text click did not enter editing: editing=%d", h.view.editingShape)
	}

	picture := document.PresentationElement{
		ID: 82001, Name: "selectable picture", Kind: document.ElementKindPicture,
		Bounds: document.PresentationRect{X: 450, Y: 350, W: 120, H: 80}, Image: rotationTestImage(),
	}
	h.view.currentSlide().Elements = append(h.view.currentSlide().Elements, picture)
	h.frame()
	center := h.view.canvasCardOrigin.Add(image.Pt(int((picture.Bounds.X+picture.Bounds.W/2)*h.view.canvasScale), int((picture.Bounds.Y+picture.Bounds.H/2)*h.view.canvasScale)))
	h.click(f32.Pt(float32(center.X), float32(center.Y)))
	if h.view.selectedShape != picture.ID || h.view.editingShape != 0 {
		t.Fatalf("picture click = selected %d, editing %d; want picture selection", h.view.selectedShape, h.view.editingShape)
	}
}

func TestPresentationSelectedTextUsesEnterAndEscape(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	h.frame()
	h.click(h.textClickPoint(110, 10))
	if h.view.selectedShape != h.elem.ID || h.view.editingShape != 0 {
		t.Fatalf("first text click = selected %d, editing %d; want selection only (history=%+v hit=%+v)", h.view.selectedShape, h.view.editingShape, h.view.shapeClick(h.elem.ID).History(), h.view.shapeHitAreas[h.elem.ID])
	}
	h.router.Queue(key.Event{Name: key.NameReturn, State: key.Press})
	h.frame()
	if h.view.editingShape != h.elem.ID {
		t.Fatalf("Enter did not edit selected text: editing=%d", h.view.editingShape)
	}
	h.view.shapeEditor.SetText("通过 Escape 提交")
	h.frame()
	h.router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	h.frame()
	if h.view.editingShape != 0 || h.view.selectedShape != h.elem.ID {
		t.Fatalf("Escape did not exit edit mode while retaining selection: editing=%d selected=%d", h.view.editingShape, h.view.selectedShape)
	}
	snapshot := h.app.pptxDoc.Snapshot()
	if got := presentationElementByID(t, &snapshot.Slides[0], h.elem.ID).PlainText(); got != "通过 Escape 提交" {
		t.Fatalf("Escape did not commit latest draft: got %q", got)
	}
}

func TestPresentationCanvasMovesObjectsOnDrag(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	h.view.selectedShape = h.elem.ID
	h.frame()
	before := presentationElementByID(t, h.view.currentSlide(), h.elem.ID)
	revision := h.app.pptxDoc.Revision()
	start := presentationCanvasPoint(h.view, before.Bounds.X+40, before.Bounds.Y+20)
	end := start.Add(f32.Pt(30, 18))

	beginPresentationCanvasDrag(h, start)
	movePresentationCanvasDrag(h, end)
	if h.app.pptxDoc.Revision() != revision {
		t.Fatalf("preview changed document revision before release: %d -> %d", revision, h.app.pptxDoc.Revision())
	}
	preview := h.view.transformGesture.preview
	if !h.view.transformGesture.active || math.Abs(preview.Bounds.X-(before.Bounds.X+30)) > 0.01 || math.Abs(preview.Bounds.Y-(before.Bounds.Y+18)) > 0.01 {
		t.Fatalf("move preview = %+v, active=%t; want +30,+18 from %+v", preview, h.view.transformGesture.active, before.Bounds)
	}
	finishPresentationCanvasDrag(h, end)
	after := presentationElementByID(t, h.view.currentSlide(), h.elem.ID)
	if after.Bounds.X != before.Bounds.X+30 || after.Bounds.Y != before.Bounds.Y+18 {
		t.Fatalf("committed move = %+v, want %+v", after.Bounds, document.PresentationRect{X: before.Bounds.X + 30, Y: before.Bounds.Y + 18, W: before.Bounds.W, H: before.Bounds.H})
	}
	if h.app.pptxDoc.Revision() != revision+1 {
		t.Fatalf("move commit revision = %d, want %d", h.app.pptxDoc.Revision(), revision+1)
	}
}

func TestPresentationCanvasResizesAllHandles(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	h.view.selectedShape = h.elem.ID
	for _, handle := range presentationResizeHandles {
		h.frame()
		before := presentationElementByID(t, h.view.currentSlide(), h.elem.ID)
		scale := h.view.canvasScale
		rect := presentationBoundsInPixels(before.Bounds, scale)
		center := presentationTransformHandleCenter(rect, handle)
		delta := presentationHandleResizeTestDelta(handle)
		start := h.view.canvasCardOrigin.Add(center)
		end := f32.Pt(float32(start.X)+delta.X, float32(start.Y)+delta.Y)
		beginPresentationCanvasDrag(h, f32.Pt(float32(start.X), float32(start.Y)))
		movePresentationCanvasDrag(h, f32.Pt(float32(end.X), float32(end.Y)))
		finishPresentationCanvasDrag(h, f32.Pt(float32(end.X), float32(end.Y)))
		after := presentationElementByID(t, h.view.currentSlide(), h.elem.ID)
		if after.Bounds == before.Bounds {
			t.Fatalf("handle %v did not resize object: before=%+v after=%+v", handle, before.Bounds, after.Bounds)
		}
	}
}

func TestPresentationCanvasRotatesObjects(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	h.view.selectedShape = h.elem.ID
	h.frame()
	before := presentationElementByID(t, h.view.currentSlide(), h.elem.ID)
	rect := presentationBoundsInPixels(before.Bounds, h.view.canvasScale)
	center := image.Pt(rect.Min.X+rect.Dx()/2, rect.Min.Y+rect.Dy()/2)
	rotationHandle := presentationRotationHandleCenter(rect, 24)
	start := h.view.canvasCardOrigin.Add(rotationHandle)
	end := h.view.canvasCardOrigin.Add(image.Pt(center.X+30, center.Y))
	beginPresentationCanvasDrag(h, f32.Pt(float32(start.X), float32(start.Y)))
	movePresentationCanvasDrag(h, f32.Pt(float32(end.X), float32(end.Y)))
	if h.view.transformGesture.preview.RotationDeg == before.RotationDeg {
		t.Fatalf("rotation preview did not change: %+v", h.view.transformGesture)
	}
	finishPresentationCanvasDrag(h, f32.Pt(float32(end.X), float32(end.Y)))
	after := presentationElementByID(t, h.view.currentSlide(), h.elem.ID)
	if after.RotationDeg == before.RotationDeg {
		t.Fatalf("rotation release did not commit: before=%g after=%g", before.RotationDeg, after.RotationDeg)
	}
}

func TestPresentationCropPreviewDoesNotMutateDocument(t *testing.T) {
	h, picture := newPresentationPicturePickerHarness(t)
	h.view.selectedShape = picture.ID
	if !h.app.beginPresentationPictureCrop(h.view) {
		t.Fatal("supported selected picture did not enter crop mode")
	}
	h.frame()
	revision := h.app.pptxDoc.Revision()
	rect := presentationBoundsInPixels(picture.Bounds, h.view.canvasScale)
	start := h.view.canvasCardOrigin.Add(presentationTransformHandleCenter(rect, presentationHandleRight))
	end := start.Add(image.Pt(-18, 0))
	beginPresentationCanvasDrag(h, f32.Pt(float32(start.X), float32(start.Y)))
	movePresentationCanvasDrag(h, f32.Pt(float32(end.X), float32(end.Y)))
	if h.app.pptxDoc.Revision() != revision {
		t.Fatalf("crop preview changed document revision before completion: %d -> %d", revision, h.app.pptxDoc.Revision())
	}
	if h.view.pictureCrop.preview == h.view.pictureCrop.original {
		t.Fatalf("crop handle drag did not update preview: %+v", h.view.pictureCrop)
	}
	if got := presentationElementByID(t, h.view.currentSlide(), picture.ID).Crop; got != h.view.pictureCrop.original {
		t.Fatalf("preview mutated committed crop: got %+v want %+v", got, h.view.pictureCrop.original)
	}
}

func TestPresentationCropHandlesAndPanKeepObjectFrameFixed(t *testing.T) {
	h, picture := newPresentationPicturePickerHarness(t)
	if err := h.app.pptxDoc.UpdateSlideElementTransform(0, picture.ID, picture.Bounds, 28); err != nil {
		t.Fatalf("rotate picture fixture: %v", err)
	}
	h.view.snapshot = h.app.pptxDoc.Snapshot()
	picture = presentationElementByID(t, h.view.currentSlide(), picture.ID)
	h.view.selectedShape = picture.ID
	if !h.app.beginPresentationPictureCrop(h.view) {
		t.Fatal("supported selected picture did not enter crop mode")
	}
	h.frame()
	beforeFrame := presentationBoundsInPixels(picture.Bounds, h.view.canvasScale)
	beforeCrop := h.view.pictureCrop.preview
	center := image.Pt(beforeFrame.Min.X+beforeFrame.Dx()/2, beforeFrame.Min.Y+beforeFrame.Dy()/2)
	localRight := presentationTransformHandleCenter(beforeFrame, presentationHandleRight)
	rotatedRight := rotatePresentationPixel(localRight, center, picture.RotationDeg)
	start := h.view.canvasCardOrigin.Add(rotatedRight)
	localDelta := image.Pt(-18, 0)
	rotatedDelta := rotatePresentationPixel(center.Add(localDelta), center, picture.RotationDeg).Sub(center)
	end := start.Add(rotatedDelta)
	beginPresentationCanvasDrag(h, f32.Pt(float32(start.X), float32(start.Y)))
	movePresentationCanvasDrag(h, f32.Pt(float32(end.X), float32(end.Y)))
	finishPresentationCanvasDrag(h, f32.Pt(float32(end.X), float32(end.Y)))
	h.frame()
	afterHandle := h.view.pictureCrop.preview
	if afterHandle == beforeCrop {
		t.Fatalf("rotated crop handle did not change source crop: before=%+v after=%+v", beforeCrop, afterHandle)
	}
	frameAfterHandle := presentationBoundsInPixels(presentationElementByID(t, h.view.currentSlide(), picture.ID).Bounds, h.view.canvasScale)
	if frameAfterHandle != beforeFrame {
		t.Fatalf("crop handle changed object frame: before=%v after=%v", beforeFrame, frameAfterHandle)
	}
	visibleWidth := 100000 - afterHandle.Left - afterHandle.Right
	visibleHeight := 100000 - afterHandle.Top - afterHandle.Bottom
	panStart := h.view.canvasCardOrigin.Add(center)
	panEnd := panStart.Add(image.Pt(14, 9))
	beginPresentationCanvasDrag(h, f32.Pt(float32(panStart.X), float32(panStart.Y)))
	movePresentationCanvasDrag(h, f32.Pt(float32(panEnd.X), float32(panEnd.Y)))
	finishPresentationCanvasDrag(h, f32.Pt(float32(panEnd.X), float32(panEnd.Y)))
	if got := h.view.pictureCrop.preview; 100000-got.Left-got.Right != visibleWidth || 100000-got.Top-got.Bottom != visibleHeight {
		t.Fatalf("source pan changed visible crop size: before %dx%d after %+v", visibleWidth, visibleHeight, got)
	}
	if got := presentationBoundsInPixels(presentationElementByID(t, h.view.currentSlide(), picture.ID).Bounds, h.view.canvasScale); got != beforeFrame {
		t.Fatalf("source pan changed object frame: got %v want %v", got, beforeFrame)
	}
}

func TestPresentationCropAllEightHandlesAreInteractive(t *testing.T) {
	h, picture := newPresentationPicturePickerHarness(t)
	if err := h.app.pptxDoc.UpdateSlideElementTransform(0, picture.ID, picture.Bounds, 0); err != nil {
		t.Fatalf("reset picture rotation: %v", err)
	}
	h.view.snapshot = h.app.pptxDoc.Snapshot()
	picture = presentationElementByID(t, h.view.currentSlide(), picture.ID)
	h.view.selectedShape = picture.ID
	if !h.app.beginPresentationPictureCrop(h.view) {
		t.Fatal("supported selected picture did not enter crop mode")
	}
	h.frame()
	base := h.view.pictureCrop.preview
	frame := presentationBoundsInPixels(picture.Bounds, h.view.canvasScale)
	for _, handle := range presentationResizeHandles {
		h.view.pictureCrop.preview = base
		delta := image.Point{}
		left, right, top, bottom, _ := presentationResizeHandleSides(handle)
		if left {
			delta.X = 12
		} else if right {
			delta.X = -12
		}
		if top {
			delta.Y = 10
		} else if bottom {
			delta.Y = -10
		}
		h.frame()
		dragPresentationCropHandle(t, h, picture, handle, delta)
		if h.view.pictureCrop.preview == base {
			t.Fatalf("crop handle %d did not update preview", handle)
		}
		if got := presentationBoundsInPixels(presentationElementByID(t, h.view.currentSlide(), picture.ID).Bounds, h.view.canvasScale); got != frame {
			t.Fatalf("crop handle %d changed object frame: got %v want %v", handle, got, frame)
		}
	}
	h.app.cancelPresentationPictureCrop(h.view)
}

func TestPresentationCropCommitCancelAndEscape(t *testing.T) {
	h, picture := newPresentationPicturePickerHarness(t)
	h.view.selectedShape = picture.ID
	h.frame()
	center := presentationCanvasPoint(h.view, picture.Bounds.X+picture.Bounds.W/2, picture.Bounds.Y+picture.Bounds.H/2)
	h.click(center)
	initialRevision := h.app.pptxDoc.Revision()
	initialCrop := presentationElementByID(t, h.view.currentSlide(), picture.ID).Crop
	if !h.app.beginPresentationPictureCrop(h.view) {
		t.Fatal("supported selected picture did not enter crop mode")
	}
	h.frame()
	dragPresentationCropHandle(t, h, picture, presentationHandleRight, image.Pt(-16, 0))
	h.app.handlePresentationRibbonAction(h.view, "picture_crop_cancel")
	if h.view.pictureCrop.active || h.app.pptxDoc.Revision() != initialRevision || presentationElementByID(t, h.view.currentSlide(), picture.ID).Crop != initialCrop {
		t.Fatalf("cancel changed crop state or document: mode=%+v revision=%d crop=%+v", h.view.pictureCrop, h.app.pptxDoc.Revision(), presentationElementByID(t, h.view.currentSlide(), picture.ID).Crop)
	}

	if !h.app.beginPresentationPictureCrop(h.view) {
		t.Fatal("picture could not re-enter crop mode after cancel")
	}
	h.frame()
	dragPresentationCropHandle(t, h, picture, presentationHandleRight, image.Pt(-16, 0))
	committed := h.view.pictureCrop.preview
	h.app.handlePresentationRibbonAction(h.view, "picture_crop_done")
	result := receivePresentationPictureCommand(t, h.app)
	h.app.applyPresentationPictureCommandResult(result)
	if result.err != nil {
		t.Fatalf("asynchronous crop commit failed: %v", result.err)
	}
	if h.view.pictureCrop.active || h.app.pptxDoc.Revision() != initialRevision+1 {
		t.Fatalf("crop completion did not commit once: mode=%+v revision=%d want=%d", h.view.pictureCrop, h.app.pptxDoc.Revision(), initialRevision+1)
	}
	if got := presentationElementByID(t, h.view.currentSlide(), picture.ID).Crop; got != committed {
		t.Fatalf("committed crop = %+v, want preview %+v", got, committed)
	}
	if !h.app.beginPresentationPictureCrop(h.view) {
		t.Fatal("picture could not re-enter crop mode to test reset")
	}
	h.frame()
	dragPresentationCropHandle(t, h, picture, presentationHandleBottom, image.Pt(0, -12))
	h.app.handlePresentationRibbonAction(h.view, "picture_crop_reset")
	if h.view.pictureCrop.preview != (document.PresentationCrop{}) || presentationElementByID(t, h.view.currentSlide(), picture.ID).Crop != committed {
		t.Fatalf("reset did not preview the full image without committing: preview=%+v committed=%+v", h.view.pictureCrop.preview, presentationElementByID(t, h.view.currentSlide(), picture.ID).Crop)
	}
	h.app.handlePresentationRibbonAction(h.view, "picture_crop_cancel")

	if !h.app.beginPresentationPictureCrop(h.view) {
		t.Fatal("picture could not re-enter crop mode after commit")
	}
	h.frame()
	dragPresentationCropHandle(t, h, picture, presentationHandleBottom, image.Pt(0, 12))
	revision := h.app.pptxDoc.Revision()
	h.router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	h.frame()
	if h.view.pictureCrop.active || h.app.pptxDoc.Revision() != revision || presentationElementByID(t, h.view.currentSlide(), picture.ID).Crop != committed {
		t.Fatalf("Escape did not discard crop preview: mode=%+v revision=%d crop=%+v", h.view.pictureCrop, h.app.pptxDoc.Revision(), presentationElementByID(t, h.view.currentSlide(), picture.ID).Crop)
	}
	if !containsPresentationAction(presentationActionsForView(h.view), "picture_crop") {
		t.Fatal("leaving crop mode did not restore normal picture transform actions")
	}
	beforeMove := presentationElementByID(t, h.view.currentSlide(), picture.ID).Bounds
	moveStart := presentationCanvasPoint(h.view, beforeMove.X+beforeMove.W/2, beforeMove.Y+beforeMove.H/2)
	moveEnd := moveStart.Add(f32.Pt(18, 11))
	beginPresentationCanvasDrag(h, moveStart)
	movePresentationCanvasDrag(h, moveEnd)
	finishPresentationCanvasDrag(h, moveEnd)
	afterMove := presentationElementByID(t, h.view.currentSlide(), picture.ID).Bounds
	if math.Abs(afterMove.X-beforeMove.X-18) > 0.02 || math.Abs(afterMove.Y-beforeMove.Y-11) > 0.02 {
		t.Fatalf("normal picture transform did not resume after crop mode: before=%+v after=%+v", beforeMove, afterMove)
	}
}

func TestPresentationCropInverseRotatesPointerCoordinates(t *testing.T) {
	for _, rotation := range []float64{0, 28, 90, 225} {
		delta := f32.Pt(30, 18)
		angle := rotation * math.Pi / 180
		deviceDelta := f32.Pt(float32(float64(delta.X)*math.Cos(angle)-float64(delta.Y)*math.Sin(angle)), float32(float64(delta.X)*math.Sin(angle)+float64(delta.Y)*math.Cos(angle)))
		got := presentationCropLocalPointerDelta(f32.Pt(100, 100), f32.Pt(100, 100).Add(deviceDelta), rotation, 1)
		if math.Abs(got.X-float64(delta.X)) > 1 || math.Abs(got.Y-float64(delta.Y)) > 1 {
			t.Fatalf("inverse rotation %g mapped pointer delta %+v to %+v", rotation, deviceDelta, got)
		}
	}
}

func containsPresentationAction(actions []ribbonAction, id string) bool {
	for _, action := range actions {
		if action.id == id {
			return true
		}
	}
	return false
}

func dragPresentationCropHandle(t *testing.T, h presentationCanvasHarness, picture document.PresentationElement, handle presentationTransformHandle, delta image.Point) {
	t.Helper()
	rect := presentationBoundsInPixels(picture.Bounds, h.view.canvasScale)
	center := image.Pt(rect.Min.X+rect.Dx()/2, rect.Min.Y+rect.Dy()/2)
	local := presentationTransformHandleCenter(rect, handle)
	start := rotatePresentationPixel(local, center, picture.RotationDeg)
	rotatedDelta := rotatePresentationPixel(center.Add(delta), center, picture.RotationDeg).Sub(center)
	start = h.view.canvasCardOrigin.Add(start)
	end := start.Add(rotatedDelta)
	beginPresentationCanvasDrag(h, f32.Pt(float32(start.X), float32(start.Y)))
	movePresentationCanvasDrag(h, f32.Pt(float32(end.X), float32(end.Y)))
	finishPresentationCanvasDrag(h, f32.Pt(float32(end.X), float32(end.Y)))
}

func TestPresentationTextEditorKeepsSelectionFrameAndSupportsTransforms(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	h.frame()
	point := h.textClickPoint(110, 10)
	h.click(point)
	h.click(point)
	if h.view.editingShape != h.elem.ID || h.view.selectedShape != h.elem.ID {
		t.Fatalf("text did not enter selected edit mode: editing=%d selected=%d", h.view.editingShape, h.view.selectedShape)
	}

	elem := presentationElementByID(t, h.view.currentSlide(), h.elem.ID)
	wantFrame := presentationBoundsInPixels(elem.Bounds, h.view.canvasScale)
	if h.view.editingArea != wantFrame {
		t.Fatalf("edit frame = %v, want the object's selection bounds %v", h.view.editingArea, wantFrame)
	}
	draft := "编辑时保持几何不变"
	h.view.shapeEditor.SetText(draft)
	h.frame()
	if got := h.view.editingArea; got != wantFrame {
		t.Fatalf("editing frame changed after text input: got %v want %v", got, wantFrame)
	}

	// The frame band remains a move target while the editor interior stays
	// available for text selection and caret placement.
	moveStart := presentationCanvasPoint(h.view, elem.Bounds.X+elem.Bounds.W/4, elem.Bounds.Y+2)
	moveEnd := moveStart.Add(f32.Pt(22, 12))
	beginPresentationCanvasDrag(h, moveStart)
	movePresentationCanvasDrag(h, moveEnd)
	finishPresentationCanvasDrag(h, moveEnd)
	moved := presentationElementByID(t, h.view.currentSlide(), h.elem.ID)
	if math.Abs(moved.Bounds.X-(elem.Bounds.X+22)) > 0.02 || math.Abs(moved.Bounds.Y-(elem.Bounds.Y+12)) > 0.02 {
		t.Fatalf("edit-mode frame drag did not move the object: before=%+v after=%+v", elem.Bounds, moved.Bounds)
	}
	if h.view.editingShape != h.elem.ID || h.view.shapeEditor.Text() != draft {
		t.Fatalf("move exited editing or lost the draft: editing=%d draft=%q", h.view.editingShape, h.view.shapeEditor.Text())
	}

	h.frame()
	resizedBefore := presentationElementByID(t, h.view.currentSlide(), h.elem.ID)
	resizeRect := presentationBoundsInPixels(resizedBefore.Bounds, h.view.canvasScale)
	resizeStart := h.view.canvasCardOrigin.Add(presentationTransformHandleCenter(resizeRect, presentationHandleBottomRight))
	resizeEnd := resizeStart.Add(image.Pt(18, 14))
	beginPresentationCanvasDrag(h, f32.Pt(float32(resizeStart.X), float32(resizeStart.Y)))
	movePresentationCanvasDrag(h, f32.Pt(float32(resizeEnd.X), float32(resizeEnd.Y)))
	finishPresentationCanvasDrag(h, f32.Pt(float32(resizeEnd.X), float32(resizeEnd.Y)))
	resized := presentationElementByID(t, h.view.currentSlide(), h.elem.ID)
	if resized.Bounds == resizedBefore.Bounds {
		t.Fatalf("edit-mode resize handle did not change bounds: before=%+v after=%+v", resizedBefore.Bounds, resized.Bounds)
	}
	if h.view.editingShape != h.elem.ID {
		t.Fatalf("edit-mode resize exited editing: editing=%d", h.view.editingShape)
	}

	h.frame()
	rotatedBefore := presentationElementByID(t, h.view.currentSlide(), h.elem.ID)
	rotateRect := presentationBoundsInPixels(rotatedBefore.Bounds, h.view.canvasScale)
	center := image.Pt(rotateRect.Min.X+rotateRect.Dx()/2, rotateRect.Min.Y+rotateRect.Dy()/2)
	rotateStart := h.view.canvasCardOrigin.Add(presentationRotationHandleCenter(rotateRect, 24))
	rotateEnd := h.view.canvasCardOrigin.Add(center).Add(image.Pt(30, 0))
	beginPresentationCanvasDrag(h, f32.Pt(float32(rotateStart.X), float32(rotateStart.Y)))
	movePresentationCanvasDrag(h, f32.Pt(float32(rotateEnd.X), float32(rotateEnd.Y)))
	finishPresentationCanvasDrag(h, f32.Pt(float32(rotateEnd.X), float32(rotateEnd.Y)))
	rotated := presentationElementByID(t, h.view.currentSlide(), h.elem.ID)
	if math.Abs(rotated.RotationDeg-rotatedBefore.RotationDeg) < 0.01 {
		t.Fatalf("edit-mode rotation handle did not rotate object: before=%g after=%g", rotatedBefore.RotationDeg, rotated.RotationDeg)
	}
	if h.view.editingShape != h.elem.ID || h.view.shapeEditor.Text() != draft {
		t.Fatalf("rotation exited editing or lost the draft: editing=%d draft=%q", h.view.editingShape, h.view.shapeEditor.Text())
	}
	wantFrame = presentationBoundsInPixels(rotated.Bounds, h.view.canvasScale)
	if h.view.editingArea != wantFrame {
		t.Fatalf("transformed edit frame = %v, want same object bounds as selection %v", h.view.editingArea, wantFrame)
	}
}

func TestPresentationTransformCancelAndNoopRelease(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	h.view.selectedShape = h.elem.ID
	h.frame()
	before := presentationElementByID(t, h.view.currentSlide(), h.elem.ID)
	revision := h.app.pptxDoc.Revision()
	start := presentationCanvasPoint(h.view, before.Bounds.X+50, before.Bounds.Y+20)
	moved := start.Add(f32.Pt(24, 10))
	beginPresentationCanvasDrag(h, start)
	movePresentationCanvasDrag(h, moved)
	cancelPresentationTransformGesture(h.view, h.elem.ID, presentationGestureMove)
	h.frame()
	if h.app.pptxDoc.Revision() != revision || h.view.transformGesture.active {
		t.Fatalf("cancel committed or kept preview: revision=%d want=%d gesture=%+v", h.app.pptxDoc.Revision(), revision, h.view.transformGesture)
	}
	if got := presentationElementByID(t, h.view.currentSlide(), h.elem.ID).Bounds; got != before.Bounds {
		t.Fatalf("cancel changed committed bounds: got %+v want %+v", got, before.Bounds)
	}

	start = presentationCanvasPoint(h.view, before.Bounds.X+50, before.Bounds.Y+20)
	moved = start.Add(f32.Pt(12, 0))
	beginPresentationCanvasDrag(h, start)
	movePresentationCanvasDrag(h, moved)
	movePresentationCanvasDrag(h, start)
	finishPresentationCanvasDrag(h, start)
	if h.app.pptxDoc.Revision() != revision {
		t.Fatalf("no-op release changed revision: got %d want %d", h.app.pptxDoc.Revision(), revision)
	}
}

func TestEmptyPlaceholderPromptStartsBlankAndReappears(t *testing.T) {
	h := newPresentationCanvasHarness(t)
	if err := h.app.pptxDoc.UpdateSlideText(0, h.elem.ID, ""); err != nil {
		t.Fatalf("clear title text: %v", err)
	}
	h.view.snapshot = h.app.pptxDoc.Snapshot()
	placeholder := &h.view.snapshot.Slides[0].Elements[0]
	placeholder.Kind = document.ElementKindText
	placeholder.PlaceholderType = "ctrTitle"
	h.elem = *placeholder
	h.frame()
	h.click(h.textClickPoint(20, 20))
	if h.view.editingShape != h.elem.ID || h.view.shapeEditor.Text() != "" {
		t.Fatalf("empty prompt did not open a blank editor: editing=%d text=%q selected=%d elem=%+v hit=%+v history=%+v", h.view.editingShape, h.view.shapeEditor.Text(), h.view.selectedShape, h.elem, h.view.shapeHitAreas[h.elem.ID], h.view.shapeClick(h.elem.ID).History())
	}
	h.click(f32.Pt(float32(h.view.canvasCardOrigin.X+1050), float32(h.view.canvasCardOrigin.Y+600)))
	if h.view.editingShape != 0 || h.view.shapeEditor.Text() != "" {
		t.Fatalf("empty placeholder did not return to prompt state: editing=%d text=%q", h.view.editingShape, h.view.shapeEditor.Text())
	}
	if !presentationElementHasPlaceholderPrompt(presentationElementByID(t, h.view.currentSlide(), h.elem.ID)) {
		t.Fatal("display-only placeholder prompt did not reappear after empty edit")
	}

	if err := h.app.pptxDoc.UpdateSlideText(0, h.elem.ID, ""); err != nil {
		t.Fatalf("reset placeholder text: %v", err)
	}
	h.view.snapshot = h.app.pptxDoc.Snapshot()
	h.view.snapshot.Slides[0].Elements[0].Kind = document.ElementKindText
	h.view.snapshot.Slides[0].Elements[0].PlaceholderType = "ctrTitle"
	h.frame()
	h.click(h.textClickPoint(20, 20))
	h.view.shapeEditor.SetText("中文标题")
	h.frame()
	h.click(f32.Pt(float32(h.view.canvasCardOrigin.X+1050), float32(h.view.canvasCardOrigin.Y+600)))
	snapshot := h.app.pptxDoc.Snapshot()
	got := presentationElementByID(t, &snapshot.Slides[0], h.elem.ID).PlainText()
	if got != "中文标题" {
		t.Fatalf("Chinese text entry did not commit: got %q", got)
	}
}

func TestPresentationTransformGesturesAtDeviceScales(t *testing.T) {
	for _, scale := range []float64{1, 1.5, 2} {
		t.Run(unitNameForScale(scale), func(t *testing.T) {
			h := newPresentationCanvasHarness(t)
			h.view.fitMode = false
			h.view.zoom = scale
			h.view.selectedShape = h.elem.ID
			h.frame()
			before := presentationElementByID(t, h.view.currentSlide(), h.elem.ID)
			start := presentationCanvasPoint(h.view, before.Bounds.X+50, before.Bounds.Y+20)
			end := start.Add(f32.Pt(float32(30*scale), float32(12*scale)))
			beginPresentationCanvasDrag(h, start)
			movePresentationCanvasDrag(h, end)
			finishPresentationCanvasDrag(h, end)
			after := presentationElementByID(t, h.view.currentSlide(), h.elem.ID)
			if math.Abs(after.Bounds.X-(before.Bounds.X+30)) > 0.02 || math.Abs(after.Bounds.Y-(before.Bounds.Y+12)) > 0.02 {
				t.Fatalf("scale %g move = %+v, want +30,+12 from %+v", scale, after.Bounds, before.Bounds)
			}
		})
	}
}

func presentationHandleResizeTestDelta(handle presentationTransformHandle) f32.Point {
	left, right, top, bottom, _ := presentationResizeHandleSides(handle)
	dx, dy := float32(0), float32(0)
	if left {
		dx = -12
	} else if right {
		dx = 12
	}
	if top {
		dy = -10
	} else if bottom {
		dy = 10
	}
	return f32.Pt(dx, dy)
}

func presentationCanvasPoint(view *presentationViewState, x, y float64) f32.Point {
	return f32.Pt(float32(view.canvasCardOrigin.X)+float32(x*view.canvasScale), float32(view.canvasCardOrigin.Y)+float32(y*view.canvasScale))
}

func beginPresentationCanvasDrag(h presentationCanvasHarness, point f32.Point) {
	h.router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: point, Time: time.Duration(time.Now().UnixNano())})
	h.frame()
}

func movePresentationCanvasDrag(h presentationCanvasHarness, point f32.Point) {
	h.router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: point, Time: time.Duration(time.Now().UnixNano())})
	h.frame()
}

func finishPresentationCanvasDrag(h presentationCanvasHarness, point f32.Point) {
	h.router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point, Time: time.Duration(time.Now().UnixNano())})
	h.frame()
}

func unitNameForScale(scale float64) string {
	return fmt.Sprintf("scale-%g", scale)
}

func presentationElementByID(t *testing.T, slide *document.PresentationSlide, id int64) document.PresentationElement {
	t.Helper()
	for _, elem := range slide.Elements {
		if elem.ID == id {
			return elem
		}
	}
	t.Fatalf("presentation element %d not found", id)
	return document.PresentationElement{}
}
