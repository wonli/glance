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
