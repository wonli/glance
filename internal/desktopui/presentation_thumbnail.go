package desktopui

import (
	"image"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/document"
)

// layoutPresentationThumbnail draws a static, non-interactive preview of a slide.
// Its scale converts the slide's point coordinates directly into thumbnail pixels.
func (a *App) layoutPresentationThumbnail(gtx layout.Context, slide document.PresentationSlide) layout.Dimensions {
	slideW, slideH := slide.WidthPT, slide.HeightPT
	if slideW <= 0 {
		slideW = 960
	}
	if slideH <= 0 {
		slideH = 540
	}
	if gtx.Constraints.Max.X <= 0 || gtx.Constraints.Max.Y <= 0 {
		return layout.Dimensions{}
	}

	scale := min(float64(gtx.Constraints.Max.X)/slideW, float64(gtx.Constraints.Max.Y)/slideH)
	slideSize := image.Pt(max(1, int(slideW*scale)), max(1, int(slideH*scale)))
	gtx.Constraints = layout.Exact(slideSize)

	drawPresentationBackground(gtx, slide, slideSize)

	slideClip := clip.Rect{Max: slideSize}.Push(gtx.Ops)
	defer slideClip.Pop()
	a.drawPresentationStaticElements(gtx, slide.InheritedElements, scale)
	for _, elem := range slide.Elements {
		a.drawPresentationThumbnailElement(gtx, elem, scale)
	}

	return layout.Dimensions{Size: slideSize}
}

func (a *App) drawPresentationThumbnailElement(gtx layout.Context, elem document.PresentationElement, scale float64) {
	box := elem.Bounds
	isLine := presentationThumbnailIsLine(elem.ShapeType)
	if box.W <= 0 && !isLine {
		box.W = 200
	}
	if box.H <= 0 && !isLine {
		box.H = 40
	}
	if box.W <= 0 {
		box.W = 200
	}
	if box.H <= 0 {
		box.H = 1
	}
	left, top := int(box.X*scale), int(box.Y*scale)
	right, bottom := int((box.X+box.W)*scale), int((box.Y+box.H)*scale)
	if (right <= left || bottom <= top) && !isLine {
		return
	}
	shapeRect := image.Rectangle{Min: image.Pt(left, top), Max: image.Pt(right, bottom)}
	rotation := pushPresentationElementRotation(gtx, shapeRect, elem.RotationDeg)
	defer rotation.Pop()
	shapeOffset := op.Offset(shapeRect.Min).Push(gtx.Ops)
	drawPresentationElementShape(gtx, elem, shapeRect.Size(), scale)
	shapeOffset.Pop()

	if elem.Kind == document.ElementKindPicture && elem.Image != nil {
		off := op.Offset(shapeRect.Min).Push(gtx.Ops)
		drawPresentationPicture(gtx, elem, shapeRect.Size())
		off.Pop()
		outlineOffset := op.Offset(shapeRect.Min).Push(gtx.Ops)
		drawPresentationElementOutline(gtx, elem, shapeRect.Size(), scale)
		outlineOffset.Pop()
	}

	if len(elem.Paragraphs) > 0 || presentationElementHasPlaceholderPrompt(elem) {
		displayElem := elem
		displayElem.Paragraphs = a.presentationElementDisplayParagraphs(elem)
		a.drawPresentationThumbnailText(gtx, displayElem, shapeRect, scale)
	}
}

func presentationThumbnailIsLine(shapeType string) bool {
	shapeType = strings.ToLower(shapeType)
	return shapeType == "line" || shapeType == "lineinv" || strings.Contains(shapeType, "connector")
}

func (a *App) drawPresentationThumbnailText(gtx layout.Context, elem document.PresentationElement, box image.Rectangle, scale float64) {
	if box.Empty() {
		return
	}
	textBounds := image.Rectangle{Max: box.Size()}
	off := op.Offset(box.Min).Push(gtx.Ops)
	clipStack := clip.Rect(textBounds).Push(gtx.Ops)

	recorder := op.Record(gtx.Ops)
	textGtx := gtx
	textGtx.Constraints = layout.Constraints{Max: box.Size()}
	paragraphs := make([]layout.FlexChild, 0, len(elem.Paragraphs))
	for _, paragraph := range elem.Paragraphs {
		p := paragraph
		paragraphs = append(paragraphs, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			textValue := p.PlainText()
			if textValue == "" {
				return layout.Dimensions{}
			}
			if presentationParagraphNeedsRichLayout(p) {
				return layoutPresentationRichParagraph(gtx, a.theme, p, elem.WrapText, scale)
			}
			gtx.Constraints.Min.X = gtx.Constraints.Max.X

			fontSize := 14.0
			fontFamilies := make([]string, 0, len(p.Runs))
			seenFamilies := make(map[string]struct{}, len(p.Runs))
			bold, italic := false, false
			textColor := pptxTextDark
			if len(p.Runs) > 0 {
				first := p.Runs[0]
				if first.SizePT > 0 {
					fontSize = first.SizePT
				}
				bold, italic = first.Bold, first.Italic
				if first.ColorHex != "" {
					textColor = parsePresentationHexColor(first.ColorHex, textColor)
				}
			}
			for _, run := range p.Runs {
				family := strings.TrimSpace(run.Font)
				if family == "" {
					continue
				}
				if _, exists := seenFamilies[family]; exists {
					continue
				}
				seenFamilies[family] = struct{}{}
				fontFamilies = append(fontFamilies, family)
			}

			label := material.Label(a.theme, presentationCanvasFontSize(gtx, fontSize, scale), textValue)
			label.LineHeightScale = 1.18
			label.Font.Typeface = presentationTextTypeface(strings.Join(fontFamilies, ", "))
			label.Color = textColor
			if bold {
				label.Font.Weight = font.Bold
			}
			if italic {
				label.Font.Style = font.Italic
			}
			switch p.Align {
			case "center":
				label.Alignment = text.Middle
			case "right":
				label.Alignment = text.End
			default:
				label.Alignment = text.Start
			}
			if !elem.WrapText {
				label.MaxLines = 1
			}
			return label.Layout(gtx)
		}))
	}

	textDims := layout.Dimensions{}
	if len(paragraphs) > 0 {
		textDims = layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(unit.Dp(1))}.Layout(textGtx, paragraphs...)
	}
	call := recorder.Stop()

	drawY := 0
	if box.Dy() > 0 && (elem.FillColorHex != "" || elem.BorderColor != "") {
		// Keep the text block visually centered in filled text boxes when there is room.
		// The recorded layout is clipped by the shape bounds, so overflowing copy stays local.
		drawY = max(0, (box.Dy()-textDims.Size.Y)/2)
	}
	textOffset := op.Offset(image.Pt(0, drawY)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	textOffset.Pop()
	off.Pop()
	clipStack.Pop()
}
