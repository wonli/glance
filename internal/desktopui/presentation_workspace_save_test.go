package desktopui

import (
	"bytes"
	"image"

	"math"
	"os"
	"path/filepath"

	"testing"

	"gioui.org/f32"

	"glance/internal/document"
	"glance/internal/format/pptx"
)

func TestPresentationSaveAsAndExtensionValidation(t *testing.T) {
	if err := validateSaveAsExtension("pptx", "presentation.pptx"); err != nil {
		t.Fatalf("validateSaveAsExtension pptx failed: %v", err)
	}
	if err := validateSaveAsExtension("pptx", "presentation.docx"); err == nil {
		t.Fatalf("expected error for mismatching extension, got nil")
	}

	doc, err := pptx.NewBlank()
	if err != nil {
		t.Fatalf("NewBlank failed: %v", err)
	}
	defer doc.Close()

	tmpPath := filepath.Join(t.TempDir(), "savedas.pptx")
	if err := doc.SaveAs(tmpPath); err != nil {
		t.Fatalf("SaveAs failed: %v", err)
	}
	if _, err := os.Stat(tmpPath); err != nil {
		t.Fatalf("file not found after SaveAs: %v", err)
	}
}

func TestPPTXCanvasTransformSaveReopenPreservesUntouchedContent(t *testing.T) {
	fixturePath := filepath.Join("testdata", "pptx-transform-fixture.pptx")
	fixtureBytes, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read transform fixture: %v", err)
	}
	sourcePath := filepath.Join(t.TempDir(), "source.pptx")
	if err := os.WriteFile(sourcePath, fixtureBytes, 0o600); err != nil {
		t.Fatalf("copy transform fixture: %v", err)
	}
	h := newPresentationCanvasHarnessFromPath(t, sourcePath)
	slide := h.view.currentSlide()
	findByName := func(name string) document.PresentationElement {
		t.Helper()
		for _, elem := range slide.Elements {
			if elem.Name == name {
				return elem
			}
		}
		t.Fatalf("fixture element %q not found", name)
		return document.PresentationElement{}
	}
	textElem := findByName("Transform text")
	shapeElem := findByName("Transform shape")
	var pictureElem document.PresentationElement
	for _, elem := range slide.Elements {
		if elem.Kind == document.ElementKindPicture {
			pictureElem = elem
			break
		}
	}
	if textElem.Kind != document.ElementKindText || shapeElem.Kind != document.ElementKindShape || pictureElem.ID == 0 || pictureElem.Kind != document.ElementKindPicture {
		t.Fatalf("fixture object kinds = text:%q shape:%q picture:%q", textElem.Kind, shapeElem.Kind, pictureElem.Kind)
	}
	untouched := findByName("Untouched reference")
	emptyTextBox := findByName("Empty text box")
	if emptyTextBox.PlainText() != "" {
		t.Fatalf("fixture empty text box contains unexpected text %q", emptyTextBox.PlainText())
	}
	originalPictureBytes := append([]byte(nil), pictureElem.ImageBytes...)

	want := make(map[int64]presentationTransform, 3)
	for _, id := range []int64{textElem.ID, shapeElem.ID, pictureElem.ID} {
		before, found := presentationCanvasElementByID(h.view, id)
		if !found {
			t.Fatalf("canvas element %d disappeared before transform", id)
		}
		h.view.selectedShape = id
		h.frame()

		moveStart := presentationCanvasPoint(h.view, before.Bounds.X+before.Bounds.W/2, before.Bounds.Y+before.Bounds.H/2)
		moveEnd := moveStart.Add(f32.Pt(22, 14))
		beginPresentationCanvasDrag(h, moveStart)
		movePresentationCanvasDrag(h, moveEnd)
		finishPresentationCanvasDrag(h, moveEnd)
		afterMove := presentationElementByID(t, h.view.currentSlide(), id)
		if math.Abs(afterMove.Bounds.X-(before.Bounds.X+22)) > 0.02 || math.Abs(afterMove.Bounds.Y-(before.Bounds.Y+14)) > 0.02 {
			t.Fatalf("canvas move for element %d = %+v, want +22,+14 from %+v", id, afterMove.Bounds, before.Bounds)
		}

		h.frame()
		resizeRect := presentationBoundsInPixels(afterMove.Bounds, h.view.canvasScale)
		resizePoint := h.view.canvasCardOrigin.Add(presentationTransformHandleCenter(resizeRect, presentationHandleBottomRight))
		resizeStart := f32.Pt(float32(resizePoint.X), float32(resizePoint.Y))
		resizeEnd := resizeStart.Add(f32.Pt(18, 14))
		beginPresentationCanvasDrag(h, resizeStart)
		movePresentationCanvasDrag(h, resizeEnd)
		finishPresentationCanvasDrag(h, resizeEnd)
		afterResize := presentationElementByID(t, h.view.currentSlide(), id)
		if afterResize.Bounds.W <= afterMove.Bounds.W || afterResize.Bounds.H <= afterMove.Bounds.H {
			t.Fatalf("canvas resize for element %d = %+v, did not expand %+v", id, afterResize.Bounds, afterMove.Bounds)
		}

		h.frame()
		rotateRect := presentationBoundsInPixels(afterResize.Bounds, h.view.canvasScale)
		center := image.Pt(rotateRect.Min.X+rotateRect.Dx()/2, rotateRect.Min.Y+rotateRect.Dy()/2)
		rotationPoint := presentationRotationHandleCenter(rotateRect, 24)
		rotateStart := h.view.canvasCardOrigin.Add(rotationPoint)
		rotateEnd := h.view.canvasCardOrigin.Add(image.Pt(center.X+40, center.Y))
		beginPresentationCanvasDrag(h, f32.Pt(float32(rotateStart.X), float32(rotateStart.Y)))
		movePresentationCanvasDrag(h, f32.Pt(float32(rotateEnd.X), float32(rotateEnd.Y)))
		finishPresentationCanvasDrag(h, f32.Pt(float32(rotateEnd.X), float32(rotateEnd.Y)))
		afterRotate := presentationElementByID(t, h.view.currentSlide(), id)
		if afterRotate.RotationDeg == afterResize.RotationDeg {
			t.Fatalf("canvas rotation for element %d did not change angle: %+v", id, afterRotate)
		}
		want[id] = presentationTransform{Bounds: afterRotate.Bounds, RotationDeg: afterRotate.RotationDeg}
	}
	if !h.app.isDirty() {
		t.Fatal("canvas object transforms did not mark the source presentation dirty")
	}

	outputPath := filepath.Join(t.TempDir(), "transformed.pptx")
	if err := h.app.pptxDoc.SaveAs(outputPath); err != nil {
		t.Fatalf("save transformed PPTX copy: %v", err)
	}
	reopened, err := pptx.Open(outputPath)
	if err != nil {
		t.Fatalf("reopen transformed PPTX: %v", err)
	}
	defer reopened.Close()
	reopenedSlide := reopened.Snapshot().Slides[0]
	for id, expected := range want {
		got := presentationElementByID(t, &reopenedSlide, id)
		if math.Abs(got.Bounds.X-expected.Bounds.X) > 0.001 || math.Abs(got.Bounds.Y-expected.Bounds.Y) > 0.001 || math.Abs(got.Bounds.W-expected.Bounds.W) > 0.001 || math.Abs(got.Bounds.H-expected.Bounds.H) > 0.001 || math.Abs(got.RotationDeg-expected.RotationDeg) > 1.0/60000 {
			t.Errorf("reopened element %d transform = bounds %+v rotation %.8f, want %+v %.8f", id, got.Bounds, got.RotationDeg, expected.Bounds, expected.RotationDeg)
		}
	}
	gotText := presentationElementByID(t, &reopenedSlide, textElem.ID)
	if gotText.PlainText() != "Editable title" {
		t.Errorf("transformed text changed during save: %q", gotText.PlainText())
	}
	gotUntouched := presentationElementByID(t, &reopenedSlide, untouched.ID)
	if gotUntouched.PlainText() != untouched.PlainText() || gotUntouched.Bounds != untouched.Bounds || gotUntouched.RotationDeg != untouched.RotationDeg {
		t.Errorf("untouched text changed: before=%+v after=%+v", untouched, gotUntouched)
	}
	gotEmpty := presentationElementByID(t, &reopenedSlide, emptyTextBox.ID)
	if gotEmpty.PlainText() != "" {
		t.Errorf("empty text box changed during transform save: %q", gotEmpty.PlainText())
	}
	gotPicture := presentationElementByID(t, &reopenedSlide, pictureElem.ID)
	if !bytes.Equal(gotPicture.ImageBytes, originalPictureBytes) {
		t.Error("picture bytes changed while applying its transform")
	}
	if after, err := os.ReadFile(sourcePath); err != nil || !bytes.Equal(after, fixtureBytes) {
		t.Fatalf("source package changed during SaveAs: err=%v", err)
	}
	if after, err := os.ReadFile(fixturePath); err != nil || !bytes.Equal(after, fixtureBytes) {
		t.Fatalf("checked-in transform fixture changed: err=%v", err)
	}
}
