package desktopui

import (
	"glance/internal/document"
	"image"

	"gioui.org/layout"
	"gioui.org/widget/material"
)

func (v *presentationViewState) beginPresentationTextEdit(elem document.PresentationElement) {
	if elem.Kind != document.ElementKindText {
		return
	}
	v.selectedShape = elem.ID
	v.editingShape = elem.ID
	v.shapeDraftText = elem.PlainText()
	v.shapeEditor.SetText(v.shapeDraftText)
	v.shapeEditor.Alignment = presentationElementTextAlignment(elem)
}

func presentationCanvasElementByID(v *presentationViewState, id int64) (document.PresentationElement, bool) {
	if v == nil || id == 0 {
		return document.PresentationElement{}, false
	}
	if slide := v.currentSlide(); slide != nil {
		for _, elem := range slide.Elements {
			if elem.ID == id {
				return elem, true
			}
		}
	}
	return document.PresentationElement{}, false
}

func presentationCanvasPointHitsInteractive(gtx layout.Context, v *presentationViewState, point image.Point) bool {
	if v == nil {
		return false
	}
	point = point.Sub(v.canvasCardOrigin)
	if v.editingShape != 0 && point.In(v.editingArea) {
		return true
	}
	slide := v.currentSlide()
	if slide == nil {
		return false
	}
	for _, elem := range slide.Elements {
		rect := presentationBoundsInPixels(elem.Bounds, v.canvasScale)
		if point.In(presentationRotatedBounds(rect, elem.RotationDeg)) {
			return true
		}
		area, ok := v.shapeHitAreas[elem.ID]
		if ok {
			bounds := image.Rectangle{Min: area.hitOrigin, Max: area.hitOrigin.Add(area.size)}
			if point.In(bounds) {
				return true
			}
		}
		if v.selectedShape == elem.ID {
			center := image.Pt(rect.Min.X+rect.Dx()/2, rect.Min.Y+rect.Dy()/2)
			if v.pictureCrop.active && v.pictureCrop.shapeID == elem.ID {
				for _, handle := range presentationResizeHandles {
					handleCenter := rotatePresentationPixel(presentationTransformHandleCenter(rect, handle), center, elem.RotationDeg)
					if point.In(presentationInteractionHitRect(handleCenter, presentationHandleHitSize(gtx))) {
						return true
					}
				}
				continue
			}
			for _, handle := range presentationResizeHandles {
				handleCenter := rotatePresentationPixel(presentationTransformHandleCenter(rect, handle), center, elem.RotationDeg)
				if point.In(presentationInteractionHitRect(handleCenter, presentationHandleHitSize(gtx))) {
					return true
				}
			}
			rotationCenter := presentationRotationHandleCenter(rect, presentationRotationHandleGap(gtx))
			rotationCenter = rotatePresentationPixel(rotationCenter, center, elem.RotationDeg)
			if point.In(presentationInteractionHitRect(rotationCenter, presentationHandleHitSize(gtx))) {
				return true
			}
		}
	}
	return false
}

func presentationInteractionHitRect(center image.Point, size int) image.Rectangle {
	return image.Rect(center.X-size/2, center.Y-size/2, center.X+(size+1)/2, center.Y+(size+1)/2)
}

func presentationSetEditorCaretAtCanvasPoint(gtx layout.Context, theme *material.Theme, v *presentationViewState, point image.Point) {
	if v == nil || v.editingShape == 0 || v.canvasScale <= 0 || theme == nil {
		return
	}
	var elem document.PresentationElement
	found := false
	if slide := v.currentSlide(); slide != nil {
		for _, candidate := range slide.Elements {
			if candidate.ID == v.editingShape && candidate.Kind == document.ElementKindText {
				elem = candidate
				found = true
				break
			}
		}
	}
	if !found {
		return
	}
	inset := presentationEditorLayoutInset(gtx, elem, v.canvasScale)
	maxTextW := max(1, v.editingArea.Dx()-gtx.Dp(inset.Left+inset.Right))
	textSize := presentationCanvasFontSize(gtx, presentationElementFontSize(elem), v.canvasScale)
	textPoint := point.Sub(v.canvasCardOrigin).Sub(v.editingArea.Min).Sub(image.Pt(gtx.Dp(inset.Left), gtx.Dp(inset.Top)))
	caret := presentationCaretOffsetAt(gtx, theme, presentationElementEditorFont(elem), v.shapeEditor.Text(), textPoint, textSize, maxTextW, maxTextW, presentationElementTextAlignment(elem), presentationElementParagraphMetrics(elem), v.canvasScale)
	v.shapeEditor.SetCaret(caret, caret)
}
