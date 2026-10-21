package desktopui

import (
	"image"
	"image/color"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget/material"
	"glance/internal/i18n"
)

func pdfPageScrollbarRange(position layout.Position, pageCount, viewportHeight int) (start, end float32, scrollable bool) {
	if pageCount <= 1 || position.Length <= viewportHeight || position.Length <= 0 || viewportHeight <= 0 {
		return 0, 1, false
	}
	length := float32(position.Length)
	itemLength := length / float32(pageCount)
	start = clampPDFFraction((float32(position.First)*itemLength + float32(position.Offset)) / length)
	end = clampPDFFraction((float32(position.First+position.Count)*itemLength + float32(position.OffsetLast)) / length)
	if end < start {
		end = start
	}
	viewportFraction := min(1, float32(viewportHeight)/length)
	visibleFraction := end - start
	if visibleFraction < 1 {
		missingFraction := viewportFraction - visibleFraction
		start -= start / (1 - visibleFraction) * missingFraction
		end += (1 - end) / (1 - visibleFraction) * missingFraction
	}
	start = clampPDFFraction(start)
	end = clampPDFFraction(end)
	return start, end, end-start < 1
}

func clampPDFFraction(value float32) float32 {
	return max(0, min(1, value))
}

func (a *App) layoutPDFPageScrollbar(gtx layout.Context, v *pdfViewState, pageCount int) layout.Dimensions {
	start, end, scrollable := pdfPageScrollbarRange(v.canvas.Position, pageCount, gtx.Constraints.Max.Y)
	if !scrollable {
		return layout.Dimensions{}
	}
	style := material.Scrollbar(a.theme, &v.canvas.Scrollbar)
	trackWidth := gtx.Dp(style.Width())
	trackClip := clip.Rect{
		Min: image.Pt(max(0, gtx.Constraints.Max.X-trackWidth), 0),
		Max: gtx.Constraints.Max,
	}.Push(gtx.Ops)
	semantic.DescriptionOp(a.textID("pdf.accessibility.page_scrollbar")).Add(gtx.Ops)
	trackClip.Pop()
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	dimensions := layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return style.Layout(gtx, layout.Vertical, start, end)
	})
	if delta := v.canvas.ScrollDistance(); delta != 0 {
		v.canvas.ScrollBy(delta * float32(pageCount))
		v.syncPageFromCanvas()
		gtx.Execute(op.InvalidateCmd{})
	}
	if v.canvas.Scrollbar.Dragging() || v.canvas.ScrollDistance() != 0 {
		a.layoutPDFPageScrollbarHint(gtx, v, pageCount, trackWidth)
	}
	return dimensions
}

func (a *App) layoutPDFPageScrollbarHint(gtx layout.Context, v *pdfViewState, pageCount, trackWidth int) {
	width, height := gtx.Dp(96), gtx.Dp(28)
	viewport := gtx.Constraints.Max
	x := max(0, viewport.X-trackWidth-gtx.Dp(8)-width)
	y := int((float64(v.page)+0.5)/float64(pageCount)*float64(viewport.Y) - float64(height)/2)
	y = max(0, min(max(0, viewport.Y-height), y))
	offset := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	defer offset.Pop()
	hintSize := image.Pt(width, height)
	paint.FillShape(gtx.Ops, color.NRGBA{R: 55, G: 64, B: 76, A: 238}, clip.UniformRRect(image.Rectangle{Max: hintSize}, gtx.Dp(5)).Op(gtx.Ops))
	hintGtx := gtx
	hintGtx.Constraints = layout.Exact(hintSize)
	locale := a.localeSnapshot().Resolved
	label := a.textArgs("pdf.scrollbar.page_indicator", i18n.Args{
		"Page":  i18n.FormatNumber(locale, float64(v.page+1)),
		"Pages": i18n.FormatNumber(locale, float64(pageCount)),
	})
	l := material.Label(a.theme, 11, label)
	l.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	layout.Center.Layout(hintGtx, l.Layout)
}
