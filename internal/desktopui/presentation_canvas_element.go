package desktopui

import (
	"glance/internal/document"
	"image"
	"image/color"
	"strings"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// layoutPresentationElement renders an individual shape, text box, or image on the slide.
func (a *App) layoutPresentationElement(gtx layout.Context, v *presentationViewState, elem document.PresentationElement, scale float64) {
	interaction := v.shapeInteraction(elem.ID)
	a.updatePresentationElementGestures(gtx, v, elem, interaction)
	if v.transformGesture.active && v.transformGesture.shapeID == elem.ID {
		elem.Bounds = v.transformGesture.preview.Bounds
		elem.RotationDeg = v.transformGesture.preview.RotationDeg
	} else if latest, found := presentationCanvasElementByID(v, elem.ID); found {
		elem = latest
	}
	elemX := int(elem.Bounds.X * scale)
	elemY := int(elem.Bounds.Y * scale)
	elemW := int(elem.Bounds.W * scale)
	elemH := int(elem.Bounds.H * scale)

	if elemW <= 0 {
		elemW = int(200 * scale)
	}
	if elemH <= 0 {
		elemH = int(40 * scale)
	}
	selectionRect := image.Rect(elemX, elemY, elemX+elemW, elemY+elemH)
	rotation := pushPresentationElementRotation(gtx, image.Rect(elemX, elemY, elemX+elemW, elemY+elemH), elem.RotationDeg)

	btn := v.shapeClick(elem.ID)
	isEditing := v.editingShape == elem.ID
	click, clicked := btn.Update(gtx)
	if isEditing {
		if !a.saving && clicked {
			if history := btn.History(); len(history) > 0 && !history[len(history)-1].Cancelled {
				editorOrigin := presentationEditorOrigin(elemX, elemY)
				point := v.canvasCardOrigin.Add(editorOrigin).Add(history[len(history)-1].Position)
				presentationSetEditorCaretAtCanvasPoint(gtx, a.theme, v, point)
			}
		}
	}
	if !a.saving && !isEditing && clicked {
		var clickInElement image.Point
		hitArea := v.shapeHitAreas[elem.ID]
		hasClickPosition := false
		if history := btn.History(); len(history) > 0 && !history[len(history)-1].Cancelled {
			clickInElement = history[len(history)-1].Position.Add(hitArea.hitOrigin).Sub(hitArea.elementOrigin)
			hasClickPosition = true
		}
		if v.editingShape != 0 {
			if err := a.flushPresentationTextDraft(); err != nil {
				return
			}
		}
		wasSelected := v.selectedShape == elem.ID
		if v.pictureCrop.active && v.pictureCrop.shapeID != elem.ID {
			a.cancelPresentationPictureCrop(v)
		}
		multiSelect := click.Modifiers.Contain(key.ModCtrl | key.ModCommand | key.ModShift)
		if multiSelect {
			togglePresentationSelection(v, elem.ID)
			v.editingShape = 0
			a.markPresentationTextEditorOpen(v, false)
			v.shapeDraftText = ""
			gtx.Execute(key.FocusCmd{Tag: &v.canvasBlankClick})
		} else {
			setPresentationSingleSelection(v, elem.ID)
			if elem.Kind == document.ElementKindPicture && elem.CanEditPicture {
				v.activeRibbonTab = "picture"
			} else if v.activeRibbonTab == "picture" {
				v.activeRibbonTab = "home"
			}
			if elem.Kind == document.ElementKindText {
				if presentationElementHasPlaceholderPrompt(elem) || wasSelected || click.NumClicks > 1 {
					v.beginPresentationTextEdit(elem)
					a.markPresentationTextEditorOpen(v, true)
				} else {
					v.editingShape = 0
					a.markPresentationTextEditorOpen(v, false)
					v.shapeDraftText = ""
					gtx.Execute(key.FocusCmd{Tag: &v.canvasBlankClick})
				}
				if v.editingShape == elem.ID && hasClickPosition && !presentationElementHasPlaceholderPrompt(elem) {
					inset := presentationEditorLayoutInset(gtx, elem, scale)
					editorInset := image.Pt(gtx.Dp(inset.Left), gtx.Dp(inset.Top))
					textMaxWidth := max(1, elemW-2*editorInset.X)
					fontSize := presentationElementFontSize(elem)
					textSize := presentationCanvasFontSize(gtx, fontSize, scale)
					caret := presentationCaretOffsetAt(gtx, a.theme, presentationElementEditorFont(elem), v.shapeDraftText, clickInElement.Sub(editorInset), textSize, textMaxWidth, textMaxWidth, presentationElementTextAlignment(elem), presentationElementParagraphMetrics(elem), scale)
					v.shapeEditor.SetCaret(caret, caret)
				}
				if v.editingShape == elem.ID {
					gtx.Execute(key.FocusCmd{Tag: &v.shapeEditor})
				}
			} else {
				v.editingShape = 0
				a.markPresentationTextEditorOpen(v, false)
				v.shapeDraftText = ""
				gtx.Execute(key.FocusCmd{Tag: &v.canvasBlankClick})
			}
		}
	}
	// Editing and selection share one frame over the object's original bounds.
	isSelected := v.selectedShape == elem.ID || v.selectedShapes[elem.ID]
	isEditing = v.editingShape == elem.ID

	// 1. Draw the shape geometry and fill using the same renderer as thumbnails.
	shapeOffset := op.Offset(image.Pt(elemX, elemY)).Push(gtx.Ops)
	shapeToDraw := elem
	if isEditing {
		shapeToDraw.BorderColor = ""
	}
	drawPresentationElementShape(gtx, shapeToDraw, image.Pt(elemW, elemH), scale)
	shapeOffset.Pop()
	if elem.Kind == document.ElementKindAudio || elem.Kind == document.ElementKindVideo {
		labelText := "音频"
		if elem.Kind == document.ElementKindVideo {
			labelText = "视频"
		}
		labelOffset := op.Offset(image.Pt(elemX+8, elemY+8)).Push(gtx.Ops)
		label := material.Label(a.theme, unit.Sp(float32(max(10, int(14*scale)))), labelText)
		label.Color = color.NRGBA{R: 31, G: 41, B: 55, A: 255}
		label.Font.Weight = font.Medium
		label.Layout(gtx)
		labelOffset.Pop()
	}

	// 3. Render picture
	if elem.Kind == document.ElementKindPicture && elem.Image != nil {
		if v.pictureCrop.active && v.pictureCrop.shapeID == elem.ID && v.pictureCrop.slideIndex == v.activeSlide {
			elem.Crop = v.pictureCrop.preview
		}
		off := op.Offset(image.Pt(elemX, elemY)).Push(gtx.Ops)
		picGtx := gtx
		drawPresentationPicture(picGtx, elem, image.Pt(elemW, elemH))
		off.Pop()
		shapeOffset := op.Offset(image.Pt(elemX, elemY)).Push(gtx.Ops)
		drawPresentationElementOutline(gtx, elem, image.Pt(elemW, elemH), scale)
		shapeOffset.Pop()
	}

	// 4. In-place text editor
	if isEditing {
		if a.documentSessionState != nil && a.documentSessionState.pptxDoc == a.pptxDoc {
			a.documentSessionState.setPresentationDraftDirty(a.windowID, presentationTextEditorOpen, true)
		}
		editorOrigin := presentationEditorOrigin(elemX, elemY)
		off := op.Offset(editorOrigin).Push(gtx.Ops)
		edGtx := gtx
		editorInset := presentationEditorLayoutInset(gtx, elem, scale)
		edGtx.Constraints = layout.Exact(image.Pt(elemW, elemH))
		fontSize := presentationElementFontSize(elem)
		textSize := presentationCanvasFontSize(gtx, fontSize, scale)
		ed := material.Editor(a.theme, &v.shapeEditor, "输入文本…")
		v.shapeEditor.ReadOnly = a.saving
		ed.Font = presentationElementEditorFont(elem)
		ed.Editor.Alignment = presentationElementTextAlignment(elem)
		ed.TextSize = textSize
		applyPresentationEditorParagraphMetrics(&ed, gtx, elem, scale)
		ed.Color = presentationElementEditorColor(elem)

		semantic.DescriptionOp("幻灯片文本编辑框").Add(edGtx.Ops)
		edDims := editorInset.Layout(edGtx, ed.Layout)
		editorClip := clip.Rect{Max: edDims.Size}.Push(gtx.Ops)
		btnGtx := gtx
		btnGtx.Constraints = layout.Exact(edDims.Size)
		btn.Layout(btnGtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: gtx.Constraints.Min}
		})
		editorClip.Pop()
		v.editingArea = selectionRect
		v.shapeDraftText = v.shapeEditor.Text()
		if a.documentSessionState != nil && a.documentSessionState.pptxDoc == a.pptxDoc {
			a.documentSessionState.setPresentationDraftDirty(a.windowID, presentationTextDraft, v.shapeDraftText != elem.PlainText())
		}
		off.Pop()
		if isSelected {
			drawPresentationTransformSelection(gtx, selectionRect)
		}
		rotation.Pop()
		registerPresentationTransformInputs(gtx, v, elem, interaction, selectionRect, v.selectedShape == elem.ID && len(presentationSelectedIDs(v)) == 1, true)
		return
	} else if len(elem.Paragraphs) > 0 || presentationElementHasPlaceholderPrompt(elem) {
		// 5. Read-only formatted text rendering
		displayParagraphs := presentationElementDisplayParagraphs(elem)
		// Determine paragraph alignment
		align := "left"
		for _, p := range displayParagraphs {
			if p.Align != "" {
				align = p.Align
				break
			}
		}

		textGtx := gtx
		textGtx.Constraints.Min = image.Pt(0, 0)
		textMaxSize := presentationElementTextMaxSize(elemW, elem, scale)
		textGtx.Constraints.Max = textMaxSize.Add(image.Pt(
			gtx.Dp(2*presentationTextInsetX),
			gtx.Dp(2*presentationTextInsetY),
		))

		recorder := op.Record(gtx.Ops)
		textDims := layout.Inset{
			Left: presentationTextInsetX, Right: presentationTextInsetX,
			Top: presentationTextInsetY, Bottom: presentationTextInsetY,
		}.Layout(textGtx, func(gtx layout.Context) layout.Dimensions {
			var paragraphs []layout.FlexChild
			for _, p := range displayParagraphs {
				para := p
				paragraphs = append(paragraphs, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					textStr := para.PlainText()
					if textStr == "" {
						return layout.Dimensions{}
					}
					if presentationParagraphNeedsRichLayout(para) {
						return layoutPresentationRichParagraph(gtx, a.theme, para, elem.WrapText, scale)
					}

					fontSize := 14.0
					fontFamilies := make([]string, 0, len(para.Runs))
					seenFamilies := make(map[string]struct{}, len(para.Runs))
					for _, r := range para.Runs {
						family := strings.TrimSpace(r.Font)
						if family != "" {
							if _, exists := seenFamilies[family]; !exists {
								seenFamilies[family] = struct{}{}
								fontFamilies = append(fontFamilies, family)
							}
						}
						if r.SizePT > 0 && len(strings.TrimSpace(r.Text)) > 0 {
							fontSize = r.SizePT
							break
						}
					}

					bold := false
					italic := false
					col := pptxTextDark
					if len(para.Runs) > 0 {
						bold = para.Runs[0].Bold
						italic = para.Runs[0].Italic
						if para.Runs[0].ColorHex != "" {
							col = parsePresentationHexColor(para.Runs[0].ColorHex, pptxTextDark)
						}
					}

					lbl := material.Label(a.theme, presentationCanvasFontSize(gtx, fontSize, scale), textStr)
					lbl.LineHeightScale = presentationTextLineHeightScale
					lbl.Font.Typeface = presentationTextTypeface(strings.Join(fontFamilies, ", "))
					lbl.Color = col
					if bold {
						lbl.Font.Weight = font.Bold
					}
					if italic {
						lbl.Font.Style = font.Italic
					}
					lbl.Alignment = presentationParagraphTextAlignment(para.Align)
					return lbl.Layout(gtx)
				}))
			}

			if len(paragraphs) == 0 {
				return layout.Dimensions{}
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(unit.Dp(2))}.Layout(gtx, paragraphs...)
		})
		call := recorder.Stop()

		textW := textDims.Size.X
		textH := textDims.Size.Y

		var drawX int
		switch presentationParagraphTextAlignment(align) {
		case text.Middle:
			centerX := elemX + elemW/2
			drawX = centerX - textW/2
		case text.End:
			rightX := elemX + elemW
			drawX = rightX - textW
		default: // "left"
			drawX = elemX
		}

		drawY := elemY
		if elemH > textH && (elem.FillColorHex != "" || elem.BorderColor != "") {
			drawY = elemY + (elemH-textH)/2
		}

		off := op.Offset(image.Pt(drawX, drawY)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		off.Pop()

		// Hit-test and click area for text
		hitX := min(elemX, drawX)
		hitY := min(elemY, drawY)
		hitW := max(elemW, max(textW, drawX+textW-hitX))
		hitH := max(elemH, max(textH, drawY+textH-hitY))
		if hitW < int(20*scale) {
			hitW = int(20 * scale)
		}
		if hitH < int(14*scale) {
			hitH = int(14 * scale)
		}
		v.shapeHitAreas[elem.ID] = presentationShapeHitArea{
			elementOrigin: image.Pt(elemX, elemY),
			hitOrigin:     image.Pt(hitX, hitY),
			textOrigin:    image.Pt(drawX-elemX+gtx.Dp(unit.Dp(2)), drawY-elemY+gtx.Dp(unit.Dp(1))),
			size:          image.Pt(hitW, hitH),
		}

		btnGtx := gtx
		btnGtx.Constraints = layout.Exact(image.Pt(hitW, hitH))
		hitOffset := op.Offset(image.Pt(hitX, hitY)).Push(gtx.Ops)
		btn.Layout(btnGtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: gtx.Constraints.Min}
		})

		hitOffset.Pop()
		if isSelected {
			drawPresentationTransformSelection(gtx, selectionRect)
		}
		rotation.Pop()
		registerPresentationTransformInputs(gtx, v, elem, interaction, selectionRect, v.selectedShape == elem.ID && len(presentationSelectedIDs(v)) == 1, isEditing)
		return
	}

	// For picture or shape without text
	hitX := elemX
	hitY := elemY
	hitW := elemW
	hitH := elemH
	v.shapeHitAreas[elem.ID] = presentationShapeHitArea{
		elementOrigin: image.Pt(elemX, elemY),
		hitOrigin:     image.Pt(hitX, hitY),
		size:          image.Pt(hitW, hitH),
	}
	btnGtx := gtx
	btnGtx.Constraints = layout.Exact(image.Pt(hitW, hitH))
	hitOffset := op.Offset(image.Pt(hitX, hitY)).Push(gtx.Ops)
	btn.Layout(btnGtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	})
	hitOffset.Pop()
	if isSelected {
		if v.pictureCrop.active && v.pictureCrop.shapeID == elem.ID {
			drawPresentationCropSelection(gtx, selectionRect)
		} else {
			drawPresentationTransformSelection(gtx, selectionRect)
		}
	}
	rotation.Pop()
	if v.pictureCrop.active && v.pictureCrop.shapeID == elem.ID {
		registerPresentationCropInputs(gtx, v, elem, interaction, selectionRect)
	} else {
		registerPresentationTransformInputs(gtx, v, elem, interaction, selectionRect, v.selectedShape == elem.ID && len(presentationSelectedIDs(v)) == 1, isEditing)
	}
}
