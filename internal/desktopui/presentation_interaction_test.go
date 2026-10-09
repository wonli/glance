package desktopui

import (
	"fmt"
	"image"

	"testing"
	"time"

	"gioui.org/f32"

	"gioui.org/io/pointer"

	"glance/internal/document"
)

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
