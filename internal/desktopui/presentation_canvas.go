package desktopui

import (
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"

	"gioui.org/font"
	"gioui.org/gesture"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"golang.org/x/image/math/fixed"

	"glance/internal/document"
)

const (
	presentationTextInsetX          unit.Dp = 2
	presentationTextInsetY          unit.Dp = 1
	presentationEditorInsetX                = presentationTextInsetX
	presentationEditorInsetY                = presentationTextInsetY
	presentationTextLineHeightScale float32 = 1.18
)

// layoutPresentationCanvas renders the center slide canvas on #E9ECF1 background matching Pen frame C2TtCZ.
func (a *App) layoutPresentationCanvas(gtx layout.Context, v *presentationViewState) layout.Dimensions {
	if v.pictureCrop.active && (v.pictureCrop.slideIndex != v.activeSlide || v.pictureCrop.shapeID != v.selectedShape) {
		a.cancelPresentationPictureCrop(v)
	}
	// Fill canvas background
	paint.FillShape(gtx.Ops, pptxCanvasBg, clip.Rect{Max: gtx.Constraints.Max}.Op())
	blankArea := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	v.canvasBlankClick.Add(gtx.Ops)
	pass.Pop()
	blankArea.Pop()
	for {
		if _, ok := gtx.Event(key.FocusFilter{Target: &v.canvasBlankClick}); !ok {
			break
		}
	}
	if v.editingShape != 0 {
		if ev, ok := gtx.Event(key.Filter{Focus: &v.shapeEditor, Name: key.NameEscape}); ok {
			if ke, yes := ev.(key.Event); yes && ke.State == key.Press {
				if err := a.flushPresentationTextDraft(); err == nil {
					v.editingShape = 0
					v.shapeDraftText = ""
					v.editingArea = image.Rectangle{}
					gtx.Execute(key.FocusCmd{Tag: &v.canvasBlankClick})
				}
			}
		}
	} else if v.pictureCrop.active {
		if ev, ok := gtx.Event(key.Filter{Focus: &v.canvasBlankClick, Name: key.NameEscape}); ok {
			if ke, yes := ev.(key.Event); yes && ke.State == key.Press && !v.pictureCrop.committing {
				a.cancelPresentationPictureCrop(v)
				gtx.Execute(key.FocusCmd{Tag: &v.canvasBlankClick})
			}
		}
	}
	if ev, ok := gtx.Event(key.Filter{Focus: &v.canvasBlankClick, Name: key.NameReturn}); ok {
		if ke, yes := ev.(key.Event); yes && ke.State == key.Press && v.editingShape == 0 {
			if elem, found := presentationCanvasElementByID(v, v.selectedShape); found && elem.Kind == document.ElementKindText {
				v.beginPresentationTextEdit(elem)
				gtx.Execute(key.FocusCmd{Tag: &v.shapeEditor})
			}
		}
	}
	for event, ok := v.canvasBlankClick.Update(gtx.Source); ok; event, ok = v.canvasBlankClick.Update(gtx.Source) {
		if event.Kind != gesture.KindClick || a.saving {
			continue
		}
		if presentationCanvasPointHitsInteractive(gtx, v, event.Position) {
			continue
		}
		if v.editingShape != 0 {
			if err := a.flushPresentationTextDraft(); err != nil {
				continue
			}
		}
		v.editingShape = 0
		v.selectedShape = 0
		a.cancelPresentationPictureCrop(v)
		if v.activeRibbonTab == "picture" {
			v.activeRibbonTab = "home"
		}
		v.shapeDraftText = ""
		v.editingArea = image.Rectangle{}
		gtx.Execute(key.FocusCmd{Tag: &v.canvasBlankClick})
	}

	slide := v.currentSlide()
	if slide == nil {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(a.theme, unit.Sp(14), "暂无幻灯片内容")
			lbl.Color = pptxTextMuted
			return lbl.Layout(gtx)
		})
	}

	slideW := slide.WidthPT
	slideH := slide.HeightPT
	if slideW <= 0 {
		slideW = 960.0
	}
	if slideH <= 0 {
		slideH = 540.0
	}

	// Compute scaling factor
	scale := v.zoom
	if v.fitMode {
		margin := float64(gtx.Dp(unit.Dp(48)))
		availW := float64(gtx.Constraints.Max.X) - margin
		availH := float64(gtx.Constraints.Max.Y) - margin
		if availW > 100 && availH > 100 {
			scaleX := availW / slideW
			scaleY := availH / slideH
			fitScale := min(scaleX, scaleY)
			if fitScale > 0 {
				scale = fitScale * v.zoom
			}
		}
	}
	if scale < 0.2 {
		scale = 0.2
	}

	cardW := int(slideW * scale)
	cardH := int(slideH * scale)
	v.canvasCardOrigin = image.Pt((gtx.Constraints.Max.X-cardW)/2, (gtx.Constraints.Max.Y-cardH)/2)
	v.canvasCardSize = image.Pt(cardW, cardH)
	v.canvasScale = scale

	// Center the slide card in the available canvas space
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = image.Pt(cardW, cardH)
			gtx.Constraints.Max = gtx.Constraints.Min

			// White card surface with rounded corners and subtle drop shadow
			r := gtx.Dp(unit.Dp(6))
			shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, r)

			// Drop shadow / subtle outline
			paint.FillShape(gtx.Ops, color.NRGBA{R: 205, G: 210, B: 220, A: 160}, clip.Stroke{
				Path:  shape.Path(gtx.Ops),
				Width: float32(gtx.Dp(unit.Dp(1))),
			}.Op())

			clipStack := shape.Push(gtx.Ops)
			defer clipStack.Pop()
			drawPresentationBackground(gtx, *slide, gtx.Constraints.Min)

			// Read-only inherited artwork sits between the background and editable
			// slide-local elements, matching PowerPoint layer order.
			a.drawPresentationStaticElements(gtx, slide.InheritedElements, scale)

			// Render slide-local elements.
			v.shapeHitAreas = make(map[int64]presentationShapeHitArea, len(slide.Elements))
			for _, elem := range slide.Elements {
				a.layoutPresentationElement(gtx, v, elem, scale)
			}

			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
	)
}

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
		v.selectedShape = elem.ID
		if elem.Kind == document.ElementKindPicture && elem.CanEditPicture {
			v.activeRibbonTab = "picture"
		} else if v.activeRibbonTab == "picture" {
			v.activeRibbonTab = "home"
		}
		if elem.Kind == document.ElementKindText {
			if presentationElementHasPlaceholderPrompt(elem) || wasSelected || click.NumClicks > 1 {
				v.beginPresentationTextEdit(elem)
			} else {
				v.editingShape = 0
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
			v.shapeDraftText = ""
			gtx.Execute(key.FocusCmd{Tag: &v.canvasBlankClick})
		}
	}
	// Editing and selection share one frame over the object's original bounds.
	isSelected := v.selectedShape == elem.ID
	isEditing = v.editingShape == elem.ID

	// 1. Draw the shape geometry and fill using the same renderer as thumbnails.
	shapeOffset := op.Offset(image.Pt(elemX, elemY)).Push(gtx.Ops)
	shapeToDraw := elem
	if isEditing {
		shapeToDraw.BorderColor = ""
	}
	drawPresentationElementShape(gtx, shapeToDraw, image.Pt(elemW, elemH), scale)
	shapeOffset.Pop()

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
		off.Pop()
		if isSelected {
			drawPresentationTransformSelection(gtx, selectionRect)
		}
		rotation.Pop()
		registerPresentationTransformInputs(gtx, v, elem, interaction, selectionRect, isSelected, true)
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
		registerPresentationTransformInputs(gtx, v, elem, interaction, selectionRect, isSelected, isEditing)
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
		registerPresentationTransformInputs(gtx, v, elem, interaction, selectionRect, isSelected, isEditing)
	}
}

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

func presentationCanvasFontSize(gtx layout.Context, sizePT, scale float64) unit.Sp {
	// scale already converts slide points to physical pixels. Convert back to
	// Gio scalable points so the display pixel density is not applied twice.
	pxPerSp := float64(gtx.Metric.PxPerSp)
	if pxPerSp <= 0 {
		pxPerSp = 1
	}
	return unit.Sp(float32(sizePT * scale / pxPerSp))
}

func presentationElementFontSize(elem document.PresentationElement) float64 {
	for _, paragraph := range elem.Paragraphs {
		for _, run := range paragraph.Runs {
			if strings.TrimSpace(run.Text) != "" && run.SizePT > 0 {
				return run.SizePT
			}
		}
	}
	switch elem.PlaceholderType {
	case "title", "ctrTitle":
		return 44
	case "subTitle":
		return 32
	case "body", "obj", "content":
		return 24
	}
	return 14
}

func presentationElementEditorColor(elem document.PresentationElement) color.NRGBA {
	for _, paragraph := range elem.Paragraphs {
		for _, run := range paragraph.Runs {
			if strings.TrimSpace(run.Text) != "" && run.ColorHex != "" {
				return parsePresentationHexColor(run.ColorHex, pptxTextDark)
			}
		}
	}
	return pptxTextDark
}

func presentationElementTextAlignment(elem document.PresentationElement) text.Alignment {
	for _, paragraph := range presentationElementDisplayParagraphs(elem) {
		if paragraph.Align != "" {
			return presentationParagraphTextAlignment(paragraph.Align)
		}
	}
	// Read-only rendering defaults paragraphs with no explicit alignment to
	// left. Keep the editor on that same effective alignment; placeholder type
	// alone must not override the text that is already visible on the slide.
	return text.Start
}

func presentationElementDisplayParagraphs(elem document.PresentationElement) []document.PresentationParagraph {
	if elem.PlaceholderType == "" || elem.PlainText() != "" {
		return elem.Paragraphs
	}
	label := presentationPlaceholderPrompt(elem.PlaceholderType)
	if label == "" {
		return elem.Paragraphs
	}
	var sourceSpacing document.PresentationParagraph
	if len(elem.Paragraphs) > 0 {
		sourceSpacing = elem.Paragraphs[0]
	}
	colorHex := "#888888"
	align := "left"
	switch elem.PlaceholderType {
	case "title", "ctrTitle":
		colorHex = "#111111"
		align = "center"
	case "subTitle":
		label = "单击此处添加副标题"
		align = "center"
	}
	return []document.PresentationParagraph{{
		Align:       align,
		LineSpacing: sourceSpacing.LineSpacing,
		SpaceBefore: sourceSpacing.SpaceBefore,
		SpaceAfter:  sourceSpacing.SpaceAfter,
		Runs: []document.PresentationTextRun{{
			Text: label, SizePT: presentationElementFontSize(elem), ColorHex: colorHex,
		}},
	}}
}

func presentationElementHasPlaceholderPrompt(elem document.PresentationElement) bool {
	return elem.PlainText() == "" && presentationPlaceholderPrompt(elem.PlaceholderType) != ""
}

func presentationPlaceholderPrompt(placeholderType string) string {
	switch placeholderType {
	case "title", "ctrTitle":
		return "单击此处添加标题"
	case "subTitle":
		return "单击此处添加副标题"
	case "body", "obj", "content", "vert", "vert270", "vertTitle", "vertBody":
		return "单击此处添加文本"
	default:
		return ""
	}
}

func presentationParagraphTextAlignment(align string) text.Alignment {
	switch align {
	case "center":
		return text.Middle
	case "right":
		return text.End
	default:
		return text.Start
	}
}

func presentationEditorOrigin(elemX, elemY int) image.Point {
	return image.Pt(elemX, elemY)
}

func presentationElementTextMaxSize(elemW int, elem document.PresentationElement, scale float64) image.Point {
	maxW := 100000
	if elem.WrapText {
		maxW = max(elemW, int(40*scale))
	}
	return image.Pt(max(1, maxW), 100000)
}

func presentationCaretOffsetAt(gtx layout.Context, theme *material.Theme, face font.Font, value string, point image.Point, size unit.Sp, minWidth, maxWidth int, alignment text.Alignment, metrics presentationParagraphLayoutMetrics, scale float64) int {
	if theme == nil || theme.Shaper == nil || value == "" {
		return 0
	}
	if maxWidth < minWidth {
		maxWidth = minWidth
	}
	theme.Shaper.LayoutString(text.Parameters{
		Font:            face,
		Alignment:       alignment,
		PxPerEm:         fixed.I(gtx.Sp(size)),
		MinWidth:        minWidth,
		MaxWidth:        maxWidth,
		Locale:          gtx.Locale,
		WrapPolicy:      text.WrapHeuristically,
		LineHeight:      fixed.I(gtx.Sp(presentationParagraphLineHeight(gtx, metrics, scale))),
		LineHeightScale: 1,
	}, value)

	bestOffset := 0
	bestScore := float64(1 << 62)
	runeOffset := 0
	clusterMinX, clusterMaxX := 0.0, 0.0
	clusterHasGlyph := false
	clusterY := 0.0
	clusterAscent, clusterDescent := 0.0, 0.0
	for glyph, ok := theme.Shaper.NextGlyph(); ok; glyph, ok = theme.Shaper.NextGlyph() {
		x := float64(glyph.X) / 64
		advance := float64(glyph.Advance) / 64
		left, right := min(x, x+advance), max(x, x+advance)
		if !clusterHasGlyph {
			clusterMinX, clusterMaxX = left, right
			clusterHasGlyph = true
		} else {
			clusterMinX = min(clusterMinX, left)
			clusterMaxX = max(clusterMaxX, right)
		}
		clusterY = float64(glyph.Y)
		clusterAscent = float64(glyph.Ascent) / 64
		clusterDescent = float64(glyph.Descent) / 64
		if glyph.Flags&text.FlagClusterBreak == 0 {
			continue
		}

		clusterRunes := int(glyph.Runes)
		if clusterRunes > 0 {
			distanceX := math.Abs(float64(point.X) - max(clusterMinX, min(clusterMaxX, float64(point.X))))
			lineTop, lineBottom := clusterY-clusterAscent, clusterY+clusterDescent
			distanceY := math.Abs(float64(point.Y) - max(lineTop, min(lineBottom, float64(point.Y))))
			score := distanceY*4 + distanceX
			if score < bestScore {
				bestScore = score
				bestOffset = runeOffset
				if float64(point.X) > (clusterMinX+clusterMaxX)/2 {
					bestOffset += clusterRunes
				}
			}
			runeOffset += clusterRunes
		}
		clusterHasGlyph = false
	}
	return min(max(0, bestOffset), len([]rune(value)))
}

func presentationElementEditorFont(elem document.PresentationElement) font.Font {
	families := make([]string, 0)
	seenFamilies := make(map[string]struct{})
	var style font.Font
	styleSet := false
	for _, paragraph := range elem.Paragraphs {
		for _, run := range paragraph.Runs {
			family := strings.TrimSpace(run.Font)
			if family != "" {
				if _, exists := seenFamilies[family]; !exists {
					seenFamilies[family] = struct{}{}
					families = append(families, family)
				}
			}
			if !styleSet && strings.TrimSpace(run.Text) != "" {
				style.Weight = font.Normal
				style.Style = font.Regular
				if run.Bold {
					style.Weight = font.Bold
				}
				if run.Italic {
					style.Style = font.Italic
				}
				styleSet = true
			}
		}
	}
	style.Typeface = presentationTextTypeface(strings.Join(families, ", "))
	return style
}

func presentationTextTypeface(documentFamily string) font.Typeface {
	families := []string{
		strings.TrimSpace(documentFamily),
		"Cambria Math", "STIX Two Math", "STIXGeneral",
		"Apple Color Emoji", "Segoe UI Emoji", "Noto Color Emoji",
		string(preferredSystemTypeface),
	}
	ordered := make([]string, 0, len(families))
	seen := make(map[string]struct{}, len(families))
	for _, family := range families {
		for _, candidate := range strings.Split(family, ",") {
			candidate = strings.TrimSpace(candidate)
			if candidate != "" {
				if _, exists := seen[candidate]; exists {
					continue
				}
				seen[candidate] = struct{}{}
				ordered = append(ordered, candidate)
			}
		}
	}
	return font.Typeface(strings.Join(ordered, ", "))
}

// parsePresentationHexColor parses "#RRGGBB", "RRGGBB", or "#RGB" hex color string.
func parsePresentationHexColor(hex string, def color.NRGBA) color.NRGBA {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) == 3 {
		r, _ := strconv.ParseUint(string(hex[0])+string(hex[0]), 16, 8)
		g, _ := strconv.ParseUint(string(hex[1])+string(hex[1]), 16, 8)
		b, _ := strconv.ParseUint(string(hex[2])+string(hex[2]), 16, 8)
		return color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}
	}
	if len(hex) == 6 {
		r, _ := strconv.ParseUint(hex[0:2], 16, 8)
		g, _ := strconv.ParseUint(hex[2:4], 16, 8)
		b, _ := strconv.ParseUint(hex[4:6], 16, 8)
		return color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}
	}
	if len(hex) == 8 {
		r, _ := strconv.ParseUint(hex[0:2], 16, 8)
		g, _ := strconv.ParseUint(hex[2:4], 16, 8)
		b, _ := strconv.ParseUint(hex[4:6], 16, 8)
		a, _ := strconv.ParseUint(hex[6:8], 16, 8)
		return color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: uint8(a)}
	}
	return def
}
