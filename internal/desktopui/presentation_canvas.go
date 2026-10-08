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
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/image/math/fixed"

	"glance/internal/document"
)

const (
	presentationEditorInsetX unit.Dp = 8
	presentationEditorInsetY unit.Dp = 4
	presentationTextInsetX   unit.Dp = 2
	presentationTextInsetY   unit.Dp = 1
)

// layoutPresentationCanvas renders the center slide canvas on #E9ECF1 background matching Pen frame C2TtCZ.
func (a *App) layoutPresentationCanvas(gtx layout.Context, v *presentationViewState) layout.Dimensions {
	// Fill canvas background
	paint.FillShape(gtx.Ops, pptxCanvasBg, clip.Rect{Max: gtx.Constraints.Max}.Op())
	blankArea := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	v.canvasBlankClick.Add(gtx.Ops)
	pass.Pop()
	blankArea.Pop()
	for event, ok := v.canvasBlankClick.Update(gtx.Source); ok; event, ok = v.canvasBlankClick.Update(gtx.Source) {
		if event.Kind != gesture.KindClick || a.saving {
			continue
		}
		if presentationCanvasPointHitsInteractive(v, event.Position) {
			continue
		}
		if v.editingShape != 0 {
			if err := a.flushPresentationTextDraft(); err != nil {
				continue
			}
		}
		v.editingShape = 0
		v.selectedShape = 0
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
			a.drawPresentationStaticElements(gtx, slide.InheritedElements, scale, image.Rectangle{Max: gtx.Constraints.Min})

			// Render slide-local elements.
			for _, elem := range slide.Elements {
				a.layoutPresentationElement(gtx, v, elem, scale)
			}

			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
	)
}

// layoutPresentationElement renders an individual shape, text box, or image on the slide.
func (a *App) layoutPresentationElement(gtx layout.Context, v *presentationViewState, elem document.PresentationElement, scale float64) {
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

	btn := v.shapeClick(elem.ID)
	isEditing := v.editingShape == elem.ID
	if isEditing {
		if !a.saving && btn.Clicked(gtx) {
			if history := btn.History(); len(history) > 0 && !history[len(history)-1].Cancelled {
				editorOrigin := presentationEditorOrigin(gtx, elemX, elemY, v.shapeHitAreas[elem.ID])
				point := v.canvasCardOrigin.Add(editorOrigin).Add(history[len(history)-1].Position)
				presentationSetEditorCaretAtCanvasPoint(gtx, a.theme, v, point)
			}
		}
	}
	if !a.saving && !isEditing && btn.Clicked(gtx) {
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
		v.selectedShape = elem.ID
		if elem.Kind == document.ElementKindText {
			v.editingShape = elem.ID
			v.shapeDraftText = elem.PlainText()
			v.shapeEditor.SetText(v.shapeDraftText)
			v.shapeEditor.Alignment = presentationElementTextAlignment(elem)
			if hasClickPosition {
				fontSize := presentationElementFontSize(elem)
				textMaxSize := presentationElementTextMaxSize(elemW, elem, scale)
				textSize := presentationCanvasFontSize(gtx, fontSize, scale)
				caret := presentationCaretOffsetAt(gtx, a.theme, presentationElementEditorFont(elem), v.shapeDraftText, clickInElement.Sub(hitArea.textOrigin), textSize, 0, max(1, textMaxSize.X))
				v.shapeEditor.SetCaret(caret, caret)
			}
			gtx.Execute(key.FocusCmd{Tag: &v.shapeEditor})
		} else {
			v.editingShape = 0
			v.shapeDraftText = ""
		}
	}
	// The editor draws its own draft-sized boundary while editing. Keep the
	// normal selection outline hidden so the two frames don't overlap.
	isSelected := v.selectedShape == elem.ID && v.editingShape == 0
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
		off := op.Offset(image.Pt(elemX, elemY)).Push(gtx.Ops)
		picGtx := gtx
		picGtx.Constraints = layout.Exact(image.Pt(elemW, elemH))
		widget.Image{
			Src:      paint.NewImageOp(elem.Image),
			Fit:      widget.Contain,
			Position: layout.Center,
		}.Layout(picGtx)
		off.Pop()
	}

	// 4. In-place text editor
	if isEditing {
		hitArea := v.shapeHitAreas[elem.ID]
		editorOrigin := presentationEditorOrigin(gtx, elemX, elemY, hitArea)
		off := op.Offset(editorOrigin).Push(gtx.Ops)
		edGtx := gtx
		editorInset := layout.Inset{Left: presentationEditorInsetX, Right: presentationEditorInsetX, Top: presentationEditorInsetY, Bottom: presentationEditorInsetY}
		textMaxSize := presentationElementTextMaxSize(elemW, elem, scale)
		maxEditorSize := textMaxSize.Add(image.Pt(gtx.Dp(2*presentationEditorInsetX), gtx.Dp(2*presentationEditorInsetY)))
		fontSize := presentationElementFontSize(elem)
		textSize := presentationCanvasFontSize(gtx, fontSize, scale)
		value := v.shapeEditor.Text()
		editorSize := presentationTextEditorSize(gtx, a.theme, elem, value, scale, maxEditorSize)
		edGtx.Constraints.Min = editorSize
		edGtx.Constraints.Max = maxEditorSize
		edGtx.Constraints.Min.X = min(edGtx.Constraints.Min.X, edGtx.Constraints.Max.X)
		edGtx.Constraints.Min.Y = min(edGtx.Constraints.Min.Y, edGtx.Constraints.Max.Y)
		ed := material.Editor(a.theme, &v.shapeEditor, "输入文本…")
		v.shapeEditor.ReadOnly = a.saving
		ed.Font = presentationElementEditorFont(elem)
		ed.Editor.Alignment = presentationElementTextAlignment(elem)
		ed.TextSize = textSize
		ed.LineHeightScale = 1.18
		ed.Color = pptxTextDark

		semantic.DescriptionOp("幻灯片文本编辑框").Add(edGtx.Ops)
		edDims := editorInset.Layout(edGtx, ed.Layout)
		editorClip := clip.Rect{Max: edDims.Size}.Push(gtx.Ops)
		btnGtx := gtx
		btnGtx.Constraints = layout.Exact(edDims.Size)
		btn.Layout(btnGtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: gtx.Constraints.Min}
		})
		editorClip.Pop()
		v.editingArea = image.Rectangle{Min: editorOrigin, Max: editorOrigin.Add(edDims.Size)}
		paint.FillShape(gtx.Ops, pptxAccentColor, clip.Stroke{
			Path:  clip.Rect{Max: edDims.Size}.Path(),
			Width: float32(gtx.Dp(unit.Dp(1.5))),
		}.Op())
		drawPresentationCornerHandles(gtx, edDims.Size)
		v.shapeDraftText = v.shapeEditor.Text()
		off.Pop()
	} else if len(elem.Paragraphs) > 0 {
		// 5. Read-only formatted text rendering
		// Determine paragraph alignment
		align := "left"
		for _, p := range elem.Paragraphs {
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
			for _, p := range elem.Paragraphs {
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
					lbl.LineHeightScale = 1.18
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

		if isSelected {
			outlineColor := pptxAccentColor
			paint.FillShape(gtx.Ops, outlineColor, clip.Stroke{
				Path:  clip.Rect{Max: image.Pt(hitW, hitH)}.Path(),
				Width: float32(gtx.Dp(unit.Dp(1.5))),
			}.Op())

			handleSize := gtx.Dp(unit.Dp(6))
			corners := []image.Point{
				{X: 0, Y: 0},
				{X: hitW - handleSize, Y: 0},
				{X: 0, Y: hitH - handleSize},
				{X: hitW - handleSize, Y: hitH - handleSize},
			}
			for _, pt := range corners {
				handleRect := image.Rectangle{Min: pt, Max: image.Pt(pt.X+handleSize, pt.Y+handleSize)}
				paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect(handleRect).Op())
				paint.FillShape(gtx.Ops, outlineColor, clip.Stroke{
					Path:  clip.Rect(handleRect).Path(),
					Width: 1.0,
				}.Op())
			}
		}

		hitOffset.Pop()
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
	if isSelected {
		outlineColor := pptxAccentColor
		paint.FillShape(gtx.Ops, outlineColor, clip.Stroke{
			Path:  clip.Rect{Max: image.Pt(hitW, hitH)}.Path(),
			Width: float32(gtx.Dp(unit.Dp(1.5))),
		}.Op())

		handleSize := gtx.Dp(unit.Dp(6))
		corners := []image.Point{
			{X: 0, Y: 0},
			{X: hitW - handleSize, Y: 0},
			{X: 0, Y: hitH - handleSize},
			{X: hitW - handleSize, Y: hitH - handleSize},
		}
		for _, pt := range corners {
			handleRect := image.Rectangle{Min: pt, Max: image.Pt(pt.X+handleSize, pt.Y+handleSize)}
			paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect(handleRect).Op())
			paint.FillShape(gtx.Ops, outlineColor, clip.Stroke{
				Path:  clip.Rect(handleRect).Path(),
				Width: 1.0,
			}.Op())
		}
	}
	hitOffset.Pop()
}

func presentationCanvasPointHitsInteractive(v *presentationViewState, point image.Point) bool {
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
		area, ok := v.shapeHitAreas[elem.ID]
		if !ok {
			continue
		}
		bounds := image.Rectangle{Min: area.hitOrigin, Max: area.hitOrigin.Add(area.size)}
		if point.In(bounds) {
			return true
		}
	}
	return false
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
	maxTextW := max(1, v.editingArea.Dx()-gtx.Dp(2*presentationEditorInsetX))
	textSize := presentationCanvasFontSize(gtx, presentationElementFontSize(elem), v.canvasScale)
	textPoint := point.Sub(v.canvasCardOrigin).Sub(v.editingArea.Min).Sub(image.Pt(gtx.Dp(presentationEditorInsetX), gtx.Dp(presentationEditorInsetY)))
	caret := presentationCaretOffsetAt(gtx, theme, presentationElementEditorFont(elem), v.shapeEditor.Text(), textPoint, textSize, 0, maxTextW)
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
	if len(elem.Paragraphs) > 0 && len(elem.Paragraphs[0].Runs) > 0 && elem.Paragraphs[0].Runs[0].SizePT > 0 {
		return elem.Paragraphs[0].Runs[0].SizePT
	}
	return 14
}

func presentationElementTextAlignment(elem document.PresentationElement) text.Alignment {
	for _, paragraph := range elem.Paragraphs {
		if paragraph.Align != "" {
			return presentationParagraphTextAlignment(paragraph.Align)
		}
	}
	return text.Start
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

func presentationEditorOrigin(gtx layout.Context, elemX, elemY int, hitArea presentationShapeHitArea) image.Point {
	return image.Pt(
		elemX+hitArea.textOrigin.X-gtx.Dp(presentationEditorInsetX),
		elemY+hitArea.textOrigin.Y-gtx.Dp(presentationEditorInsetY),
	)
}

func presentationElementTextMaxSize(elemW int, elem document.PresentationElement, scale float64) image.Point {
	maxW := 100000
	if elem.WrapText {
		maxW = max(elemW, int(40*scale))
	}
	return image.Pt(max(1, maxW), 100000)
}

func presentationCaretOffsetAt(gtx layout.Context, theme *material.Theme, face font.Font, value string, point image.Point, size unit.Sp, minWidth, maxWidth int) int {
	if theme == nil || theme.Shaper == nil || value == "" {
		return 0
	}
	if maxWidth < minWidth {
		maxWidth = minWidth
	}
	theme.Shaper.LayoutString(text.Parameters{
		Font:            face,
		PxPerEm:         fixed.I(gtx.Sp(size)),
		MinWidth:        minWidth,
		MaxWidth:        maxWidth,
		Locale:          gtx.Locale,
		WrapPolicy:      text.WrapHeuristically,
		LineHeightScale: 1.18,
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

func presentationTextEditorSize(gtx layout.Context, theme *material.Theme, elem document.PresentationElement, value string, scale float64, maxSize image.Point) image.Point {
	if value == "" {
		value = "输入文本…"
	}
	hInset := gtx.Dp(2 * presentationEditorInsetX)
	vInset := gtx.Dp(2 * presentationEditorInsetY)
	maxTextSize := image.Pt(max(1, maxSize.X-hInset), max(1, maxSize.Y-vInset))
	measureGtx := gtx
	measureGtx.Constraints = layout.Constraints{Max: maxTextSize}
	lbl := material.Label(theme, presentationCanvasFontSize(gtx, presentationElementFontSize(elem), scale), value)
	lbl.Font = presentationElementEditorFont(elem)
	lbl.Alignment = presentationElementTextAlignment(elem)
	lbl.LineHeightScale = 1.18
	measureOps := op.Record(measureGtx.Ops)
	contentSize := lbl.Layout(measureGtx).Size
	_ = measureOps.Stop()
	return image.Pt(
		min(maxSize.X, max(1, contentSize.X+hInset)),
		min(maxSize.Y, max(1, contentSize.Y+vInset)),
	)
}

func drawPresentationCornerHandles(gtx layout.Context, size image.Point) {
	handleSize := min(gtx.Dp(unit.Dp(6)), min(size.X, size.Y))
	if handleSize <= 0 {
		return
	}
	corners := []image.Point{
		{X: 0, Y: 0},
		{X: size.X - handleSize, Y: 0},
		{X: 0, Y: size.Y - handleSize},
		{X: size.X - handleSize, Y: size.Y - handleSize},
	}
	for _, point := range corners {
		rect := image.Rectangle{Min: point, Max: point.Add(image.Pt(handleSize, handleSize))}
		paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect(rect).Op())
		paint.FillShape(gtx.Ops, pptxAccentColor, clip.Stroke{
			Path:  clip.Rect(rect).Path(),
			Width: 1,
		}.Op())
	}
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
