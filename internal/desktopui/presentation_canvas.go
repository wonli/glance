package desktopui

import (
	"image"
	"image/color"
	"strconv"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
)

// layoutPresentationCanvas renders the center slide canvas on #E9ECF1 background matching Pen frame C2TtCZ.
func (a *App) layoutPresentationCanvas(gtx layout.Context, v *presentationViewState) layout.Dimensions {
	// Fill canvas background
	paint.FillShape(gtx.Ops, pptxCanvasBg, clip.Rect{Max: gtx.Constraints.Max}.Op())

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

			// Background fill
			bgColor := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			if slide.BgColorHex != "" {
				bgColor = parsePresentationHexColor(slide.BgColorHex, bgColor)
			}
			paint.FillShape(gtx.Ops, bgColor, shape.Op(gtx.Ops))

			clipStack := shape.Push(gtx.Ops)
			defer clipStack.Pop()

			// Render slide elements
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
	if btn.Clicked(gtx) {
		v.selectedShape = elem.ID
		v.editingShape = elem.ID
		v.shapeEditor.SetText(elem.PlainText())
	}
	isSelected := v.selectedShape == elem.ID
	isEditing := v.editingShape == elem.ID

	// 1. Draw the shape geometry and fill using the same renderer as thumbnails.
	shapeOffset := op.Offset(image.Pt(elemX, elemY)).Push(gtx.Ops)
	drawPresentationElementShape(gtx, elem, image.Pt(elemW, elemH), scale)
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
		edW := max(elemW, int(150*scale))
		edH := max(elemH, int(30*scale))
		off := op.Offset(image.Pt(elemX, elemY)).Push(gtx.Ops)
		edGtx := gtx
		edGtx.Constraints.Min = image.Pt(edW, edH)
		edGtx.Constraints.Max = image.Pt(max(edW, gtx.Constraints.Max.X-elemX), 100000)

		ed := material.Editor(a.theme, &v.shapeEditor, "输入文本…")
		fontSize := 14.0
		if len(elem.Paragraphs) > 0 && len(elem.Paragraphs[0].Runs) > 0 && elem.Paragraphs[0].Runs[0].SizePT > 0 {
			fontSize = elem.Paragraphs[0].Runs[0].SizePT
		}
		ed.TextSize = presentationCanvasFontSize(gtx, fontSize, scale)
		ed.Color = pptxTextDark

		if edText := v.shapeEditor.Text(); edText != elem.PlainText() && a.pptxDoc != nil {
			_ = a.pptxDoc.UpdateSlideText(v.activeSlide, elem.ID, edText)
			v.snapshot = a.pptxDoc.Snapshot()
		}

		layout.Inset{Left: unit.Dp(4), Right: unit.Dp(4), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(edGtx, ed.Layout)
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
		if elem.WrapText {
			textGtx.Constraints.Max = image.Pt(max(elemW, int(40*scale)), 100000)
		} else {
			textGtx.Constraints.Max = image.Pt(100000, 100000)
		}

		recorder := op.Record(gtx.Ops)
		textDims := layout.Inset{
			Left: unit.Dp(2), Right: unit.Dp(2), Top: unit.Dp(1), Bottom: unit.Dp(1),
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
					lbl.Font.Typeface = presentationTextTypeface(strings.Join(fontFamilies, ", "))
					lbl.Color = col
					if bold {
						lbl.Font.Weight = font.Bold
					}
					if italic {
						lbl.Font.Style = font.Italic
					}
					switch para.Align {
					case "center":
						lbl.Alignment = text.Middle
					case "right":
						lbl.Alignment = text.End
					default:
						lbl.Alignment = text.Start
					}
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
		switch align {
		case "center":
			centerX := elemX + elemW/2
			drawX = centerX - textW/2
		case "right":
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

func presentationCanvasFontSize(gtx layout.Context, sizePT, scale float64) unit.Sp {
	// scale already converts slide points to physical pixels. Convert back to
	// Gio scalable points so the display pixel density is not applied twice.
	pxPerSp := float64(gtx.Metric.PxPerSp)
	if pxPerSp <= 0 {
		pxPerSp = 1
	}
	return unit.Sp(float32(sizePT * scale / pxPerSp))
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
