package desktopui

import (
	"image"
	"math"
	"testing"

	"gioui.org/f32"

	"glance/internal/document"
)

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
