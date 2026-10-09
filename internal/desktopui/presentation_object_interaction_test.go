package desktopui

import (
	"image"
	"math"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/key"

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
