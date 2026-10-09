package desktopui

import (
	"image"
	"math"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/key"

	"glance/internal/document"
)

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
