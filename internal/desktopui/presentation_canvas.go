package desktopui

import (
	"image"
	"image/color"

	"gioui.org/gesture"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"

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
	} else if v.insertTextBox || v.insertShape || v.insertPicturePending {
		if ev, ok := gtx.Event(key.Filter{Focus: &v.canvasBlankClick, Name: key.NameEscape}); ok {
			if ke, yes := ev.(key.Event); yes && ke.State == key.Press {
				v.insertTextBox, v.insertShape, v.insertPicturePending = false, false, false
				v.insertPictureData, v.insertPictureMediaType = nil, ""
				v.insertPictureWidth, v.insertPictureHeight, v.insertPictureGeneration = 0, 0, 0
				a.setStatus("已取消放置操作")
				gtx.Execute(key.FocusCmd{Tag: &v.canvasBlankClick})
			}
		}
	}
	if ev, ok := gtx.Event(key.Filter{Focus: &v.canvasBlankClick, Name: key.NameReturn}); ok {
		if ke, yes := ev.(key.Event); yes && ke.State == key.Press && v.editingShape == 0 {
			if elem, found := presentationCanvasElementByID(v, v.selectedShape); found && elem.Kind == document.ElementKindText {
				v.beginPresentationTextEdit(elem)
				a.markPresentationTextEditorOpen(v, true)
				gtx.Execute(key.FocusCmd{Tag: &v.shapeEditor})
			}
		}
	}
	for event, ok := v.canvasBlankClick.Update(gtx.Source); ok; event, ok = v.canvasBlankClick.Update(gtx.Source) {
		if event.Kind != gesture.KindClick || a.saving {
			continue
		}
		if v.insertTextBox || v.insertShape || v.insertPicturePending {
			if v.insertPicturePending && v.insertPictureGeneration != a.documentGeneration {
				v.insertPicturePending = false
				v.insertPictureData = nil
				v.insertPictureMediaType = ""
				v.insertPictureWidth, v.insertPictureHeight = 0, 0
				a.setStatus("文档已切换，请重新选择要插入的图片")
				continue
			}
			placingShape := v.insertShape
			placingPicture := v.insertPicturePending
			page := image.Rectangle{Min: v.canvasCardOrigin, Max: v.canvasCardOrigin.Add(v.canvasCardSize)}
			if !event.Position.In(page) {
				a.setStatus("请点击白色幻灯片页面内的位置")
				continue
			}
			slide := v.currentSlide()
			if slide == nil || v.canvasScale <= 0 {
				v.insertTextBox = false
				v.insertShape = false
				v.insertPicturePending = false
				v.insertPictureData = nil
				v.insertPictureMediaType = ""
				v.insertPictureWidth, v.insertPictureHeight = 0, 0
				continue
			}
			slideWidth, slideHeight := slide.WidthPT, slide.HeightPT
			if slideWidth <= 0 {
				slideWidth = 960
			}
			if slideHeight <= 0 {
				slideHeight = 540
			}
			point := event.Position.Sub(v.canvasCardOrigin)
			x := max(0.0, min(slideWidth-8, float64(point.X)/v.canvasScale))
			y := max(0.0, min(slideHeight-8, float64(point.Y)/v.canvasScale))
			width, height := 240.0, 90.0
			if placingShape {
				width, height = 180, 120
			} else if placingPicture {
				pixelWidth, pixelHeight := float64(v.insertPictureWidth), float64(v.insertPictureHeight)
				if pixelWidth > 0 && pixelHeight > 0 {
					scale := min(240/pixelWidth, 180/pixelHeight)
					width, height = max(8.0, pixelWidth*scale), max(8.0, pixelHeight*scale)
				}
			}
			bounds := document.PresentationRect{X: x, Y: y, W: min(width, slideWidth-x), H: min(height, slideHeight-y)}
			v.insertTextBox = false
			v.insertShape = false
			pictureData, pictureMediaType := v.insertPictureData, v.insertPictureMediaType
			v.insertPicturePending = false
			v.insertPictureData = nil
			v.insertPictureMediaType = ""
			v.insertPictureWidth, v.insertPictureHeight = 0, 0
			v.insertPictureGeneration = 0
			if placingShape {
				a.addPresentationShapeAt(v, slide.ID, bounds, "rect", "")
			} else if placingPicture {
				a.addPresentationPictureAt(v, slide.ID, bounds, "contain", pictureMediaType, pictureData)
			} else {
				a.addPresentationTextBoxAt(v, slide.ID, bounds, "")
			}
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
		a.markPresentationTextEditorOpen(v, false)
		clearPresentationSelection(v)
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
	dimensions := layout.Stack{Alignment: layout.Center}.Layout(gtx,
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
	a.drawPresentationRulers(gtx, v, slideW, slideH, scale)
	return dimensions
}
