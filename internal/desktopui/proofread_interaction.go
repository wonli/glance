package desktopui

import (
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type proofreadingPagePointerTag struct {
	pageIndex int
}

const proofreadingXTextSizeMaxStep = 19

func proofreadingXTextScale(step int) float64 {
	step = max(-3, min(proofreadingXTextSizeMaxStep, step))
	if step <= 5 {
		return 1 + float64(step)*0.1
	}
	return 1.5 + float64(step-5)*0.25
}

func (a *App) layoutProofreadingXTextSizeButton(gtx layout.Context, state *proofreadingWorkspace, delta int) layout.Dimensions {
	button := &state.xTextSmaller
	label, description := "A-", "proofread.x_text_size.smaller"
	if delta > 0 {
		button = &state.xTextLarger
		label, description = "A+", "proofread.x_text_size.larger"
	}
	size := image.Pt(gtx.Dp(36), gtx.Dp(30))
	gtx.Constraints = layout.Exact(size)
	enabled := state.bDocument != nil && state.xTextSizeStep+delta >= -3 && state.xTextSizeStep+delta <= proofreadingXTextSizeMaxStep
	if button.Clicked(gtx) && enabled {
		state.xTextSizeStep += delta
		a.reflowProofreadingXPageBreaksForTextSize()
		a.rebuildProofreadingXPages()
		a.queueProofreadingDraftPersistence()
		if a.window != nil {
			a.window.Invalidate()
		}
	}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		fill, ink := color.NRGBA{R: 255, G: 255, B: 255, A: 255}, color.NRGBA{R: 75, G: 85, B: 99, A: 255}
		if !enabled {
			fill, ink = proofreadPaginationSurface, color.NRGBA{R: 166, G: 174, B: 183, A: 255}
		}
		paint.FillShape(gtx.Ops, fill, clip.UniformRRect(image.Rect(0, 0, size.X, size.Y), gtx.Dp(6)).Op(gtx.Ops))
		proofreadRoundedBorder(gtx, size, 6, color.NRGBA{R: 213, G: 221, B: 229, A: 255}, max(1, gtx.Dp(1)))
		semantic.DescriptionOp(a.textID(description)).Add(gtx.Ops)
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			text := material.Label(a.theme, 12, label)
			text.Color = ink
			text.Font.Weight = font.SemiBold
			return text.Layout(gtx)
		})
	})
}

func (a *App) handleProofreadingXPagePointer(gtx layout.Context, view *pdfViewState, pageIndex int, page proofreadingXPage) {
	state := a.proofreading
	if state == nil || view == nil || pageIndex < 0 || pageIndex >= len(state.xPagePointerTags) {
		return
	}
	target := &state.xPagePointerTags[pageIndex]
	if target.pageIndex != pageIndex {
		return
	}
	for value, ok := gtx.Event(pointer.Filter{Target: target, Kinds: pointer.Press}); ok; value, ok = gtx.Event(pointer.Filter{Target: target, Kinds: pointer.Press}) {
		event, ok := value.(pointer.Event)
		if !ok || event.Kind != pointer.Press || event.Source != pointer.Mouse {
			continue
		}
		secondaryClick := event.Buttons&pointer.ButtonSecondary != 0
		primaryClick := event.Buttons&pointer.ButtonPrimary != 0
		if !secondaryClick && !primaryClick {
			continue
		}
		if state.xPageMenuOpen {
			continue
		}
		if state.bDocument == nil {
			state.xPageMenuOpen = false
			state.xPageMenuBounds = image.Rectangle{}
			continue
		}
		point := image.Pt(int(event.Position.X+0.5), int(event.Position.Y+0.5))
		state.xPageMenuOpen = true
		state.xPageMenuPage = pageIndex
		state.xPageMenuBounds = image.Rectangle{}
		state.xPageMenuPoint = proofreadingPagePoint{
			X: clampFloat64(float64(point.X)/float64(max(1, gtx.Constraints.Max.X)), 0, 1),
			Y: clampFloat64(float64(point.Y)/float64(max(1, gtx.Constraints.Max.Y)), 0, 1),
		}
	}
}

func (a *App) registerProofreadingXCanvasScrollPointer(gtx layout.Context) {
	state := a.proofreading
	if state == nil {
		return
	}
	pass := pointer.PassOp{}.Push(gtx.Ops)
	defer pass.Pop()
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, &state.xCanvasScrollPointerTag)
	area.Pop()
}

func (a *App) registerProofreadingXPagePointer(gtx layout.Context, pageIndex int, pageSize image.Point) {
	state := a.proofreading
	if state == nil || pageIndex < 0 || pageIndex >= len(state.xPagePointerTags) {
		return
	}
	area := clip.Rect{Max: pageSize}.Push(gtx.Ops)
	semantic.DescriptionOp(a.textID("proofread.x_page.click_for_break")).Add(gtx.Ops)
	event.Op(gtx.Ops, &state.xPagePointerTags[pageIndex])
	area.Pop()
}

func (a *App) layoutProofreadingXPageMenuOverlay(gtx layout.Context, view *pdfViewState) {
	state := a.proofreading
	if state == nil || !state.xPageMenuOpen {
		return
	}
	point, visible := proofreadingXPageMenuRootPoint(gtx, view, state)
	if !visible {
		state.xPageMenuOpen = false
		state.xPageMenuBounds = image.Rectangle{}
		return
	}
	if state.xPageMenuDismiss.Clicked(gtx) {
		history := state.xPageMenuDismiss.History()
		if len(history) != 0 && !history[len(history)-1].Position.In(state.xPageMenuBounds) {
			state.xPageMenuOpen = false
			state.xPageMenuBounds = image.Rectangle{}
			if a.window != nil {
				a.window.Invalidate()
			}
			return
		}
	}
	layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			pass := pointer.PassOp{}.Push(gtx.Ops)
			dimensions := state.xPageMenuDismiss.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
			pass.Pop()
			return dimensions
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return a.layoutProofreadingXPageMenu(gtx, point)
		}),
	)
}

func proofreadingXPageMenuRootPoint(gtx layout.Context, view *pdfViewState, state *proofreadingWorkspace) (image.Point, bool) {
	pageIndex := state.xPageMenuPage
	first := state.xCanvas.Position.First
	if view == nil || pageIndex < first || pageIndex >= len(state.pageFrames) {
		return image.Point{}, false
	}
	rowTop := -state.xCanvas.Position.Offset
	for index := first; index < pageIndex; index++ {
		pageSize := proofreadingXPagePixelSize(gtx, view, state, index)
		rowTop += pageSize.Y + 2*gtx.Dp(12)
	}
	pageSize := proofreadingXPagePixelSize(gtx, view, state, pageIndex)
	pagePoint := image.Pt(int(state.xPageMenuPoint.X*float64(pageSize.X)), int(state.xPageMenuPoint.Y*float64(pageSize.Y)))
	point := image.Pt((gtx.Constraints.Max.X-pageSize.X)/2+pagePoint.X, rowTop+gtx.Dp(12)+pagePoint.Y)
	if point.X < 0 || point.X >= gtx.Constraints.Max.X-gtx.Dp(30) || point.Y < 0 || point.Y >= gtx.Constraints.Max.Y {
		return image.Point{}, false
	}
	return point, true
}

func proofreadingXPagePixelSize(gtx layout.Context, view *pdfViewState, state *proofreadingWorkspace, pageIndex int) image.Point {
	pageWidth, pageHeight := pdfCachedPageSize(view, pageIndex)
	if pageIndex >= 0 && pageIndex < len(state.xPages) {
		page := state.xPages[pageIndex]
		if page.Width > 0 && page.Height > 0 {
			pageWidth, pageHeight = page.Width, page.Height
		}
	}
	pageWidth, pageHeight = max(1, pageWidth), max(1, pageHeight)
	scale := view.scale
	if scale <= 0 || scale != scale {
		availableW := max(1, gtx.Constraints.Max.X-gtx.Dp(48))
		availableH := max(1, gtx.Constraints.Max.Y-gtx.Dp(48))
		scale = min(float64(availableW)/pageWidth, float64(availableH)/pageHeight)
	}
	return image.Pt(max(1, int(pageWidth*scale)), max(1, int(pageHeight*scale)))
}

func clampFloat64(value, minimum, maximum float64) float64 {
	return math.Max(minimum, math.Min(maximum, value))
}

func (a *App) layoutProofreadingXPageMenu(gtx layout.Context, anchor image.Point) layout.Dimensions {
	state := a.proofreading
	if state == nil || !state.xPageMenuOpen {
		return layout.Dimensions{}
	}
	if !state.xPageMenuOpen {
		return layout.Dimensions{}
	}
	menuSize := image.Pt(gtx.Dp(289), gtx.Dp(81))
	margin := gtx.Dp(5)
	rightLimit := max(menuSize.X+margin, gtx.Constraints.Max.X-gtx.Dp(30))
	pointerHeight, pointerHalfWidth := gtx.Dp(7), gtx.Dp(6)
	point := image.Pt(anchor.X-gtx.Dp(23), anchor.Y-menuSize.Y-pointerHeight)
	pointerBelow := true
	if point.Y < margin {
		point.Y = anchor.Y + pointerHeight
		pointerBelow = false
	}
	if point.X+menuSize.X > rightLimit-margin {
		point.X = anchor.X - menuSize.X + gtx.Dp(23)
	}
	point.X = max(margin, min(rightLimit-menuSize.X-margin, point.X))
	point.Y = max(margin, min(gtx.Constraints.Max.Y-menuSize.Y-margin, point.Y))
	state.xPageMenuBounds = image.Rectangle{Min: point, Max: point.Add(menuSize)}
	pointerX := max(pointerHalfWidth+gtx.Dp(2), min(menuSize.X-pointerHalfWidth-gtx.Dp(2), anchor.X-point.X))
	offset := op.Offset(point).Push(gtx.Ops)
	defer offset.Pop()
	gtx.Constraints = layout.Exact(menuSize)
	shadowPoint := op.Offset(image.Pt(0, gtx.Dp(5))).Push(gtx.Ops)
	paint.FillShape(gtx.Ops, color.NRGBA{R: 27, G: 42, B: 58, A: 28}, clip.UniformRRect(image.Rect(0, 0, menuSize.X, menuSize.Y), gtx.Dp(7)).Op(gtx.Ops))
	shadowPoint.Pop()
	paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.UniformRRect(image.Rect(0, 0, menuSize.X, menuSize.Y), gtx.Dp(7)).Op(gtx.Ops))
	proofreadRoundedBorder(gtx, menuSize, 7, color.NRGBA{R: 216, G: 222, B: 229, A: 255}, max(1, gtx.Dp(1)))
	drawProofreadingMenuPointer(gtx, menuSize, pointerX, pointerHalfWidth, pointerHeight, pointerBelow)
	return a.layoutProofreadingPageMenuRows(gtx)
}

func drawProofreadingMenuPointer(gtx layout.Context, menuSize image.Point, centerX, halfWidth, height int, below bool) {
	baseY, tipY := menuSize.Y-gtx.Dp(1), menuSize.Y+height
	if !below {
		baseY, tipY = gtx.Dp(1), -height
	}
	leftX, rightX := centerX-halfWidth, centerX+halfWidth
	var fillPath clip.Path
	fillPath.Begin(gtx.Ops)
	fillPath.MoveTo(f32.Pt(float32(leftX), float32(baseY)))
	fillPath.LineTo(f32.Pt(float32(centerX), float32(tipY)))
	fillPath.LineTo(f32.Pt(float32(rightX), float32(baseY)))
	fillPath.Close()
	paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Outline{Path: fillPath.End()}.Op())
	var outline clip.Path
	outline.Begin(gtx.Ops)
	outline.MoveTo(f32.Pt(float32(leftX), float32(baseY)))
	outline.LineTo(f32.Pt(float32(centerX), float32(tipY)))
	outline.LineTo(f32.Pt(float32(rightX), float32(baseY)))
	paint.FillShape(gtx.Ops, color.NRGBA{R: 216, G: 222, B: 229, A: 255}, clip.Stroke{Path: outline.End(), Width: float32(max(1, gtx.Dp(1)))}.Op())
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	if below {
		paint.FillShape(gtx.Ops, white, clip.Rect{Min: image.Pt(leftX+gtx.Dp(1), baseY), Max: image.Pt(rightX-gtx.Dp(1), baseY+gtx.Dp(1))}.Op())
	} else {
		paint.FillShape(gtx.Ops, white, clip.Rect{Min: image.Pt(leftX+gtx.Dp(1), 0), Max: image.Pt(rightX-gtx.Dp(1), gtx.Dp(1))}.Op())
	}
}

func layoutProofreadingMenuAction(gtx layout.Context, a *App, button *widget.Clickable, iconName, label string, enabled bool, action func()) layout.Dimensions {
	ink := color.NRGBA{R: 53, G: 69, B: 83, A: 255}
	iconTint := color.NRGBA{R: 98, G: 119, B: 135, A: 255}
	if !enabled {
		ink = color.NRGBA{R: 166, G: 174, B: 183, A: 255}
		iconTint = color.NRGBA{R: 177, G: 184, B: 190, A: 255}
	}
	if button != nil && button.Clicked(gtx) && enabled && action != nil {
		action()
	}
	content := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if iconName == "" {
					return layout.Dimensions{}
				}
				return layoutProofreadingIcon(gtx, "home", iconName, 14, iconTint)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(gtx.Dp(4), 0)}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				style := material.Label(a.theme, 11, label)
				style.Color = ink
				style.MaxLines = 1
				return layoutProofreadingOpticallyCenteredLabel(gtx, style)
			}),
		)
	}
	if button == nil {
		return layout.Center.Layout(gtx, content)
	}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, content)
	})
}
