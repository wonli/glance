package desktopui

import (
	"math"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/layout"
)

const (
	pdfZoomMinimum = 0.25
	pdfZoomMaximum = 4.0
)

func clampPDFZoom(zoom float64) float64 {
	if math.IsNaN(zoom) || math.IsInf(zoom, 0) || zoom <= 0 {
		return 1
	}
	return math.Max(pdfZoomMinimum, math.Min(pdfZoomMaximum, zoom))
}

func (a *App) handlePDFCanvasScroll(gtx layout.Context, v *pdfViewState) {
	const scrollLimit = 1 << 30
	filter := pointer.Filter{
		Target:  &v.zoomScroll,
		Kinds:   pointer.Scroll,
		ScrollX: pointer.ScrollRange{Min: -scrollLimit, Max: scrollLimit},
		ScrollY: pointer.ScrollRange{Min: -scrollLimit, Max: scrollLimit},
	}
	for event, ok := gtx.Event(filter); ok; event, ok = gtx.Event(filter) {
		pe, ok := event.(pointer.Event)
		if !ok || pe.Kind != pointer.Scroll {
			continue
		}
		if zoomScrollModifier(pe.Modifiers) {
			zoom, remainder, changed := zoomFromScroll(clampPDFZoom(v.currentZoom(gtx.Metric)), zoomScrollAxisDelta(pe.Scroll), zoomScrollScale(), v.zoomScrollRemainder, pdfZoomMinimum, pdfZoomMaximum)
			v.zoomScrollRemainder = remainder
			if changed {
				a.setPDFZoomAt(gtx, v, zoom, pe.Position)
				if a.window != nil {
					a.window.Invalidate()
				}
			}
			continue
		}
		if applyPDFCanvasScroll(v, pe.Scroll) {
			if a.window != nil {
				a.window.Invalidate()
			}
		}
	}
}

// The zoomScroll listener accepts the wheel event before the PDF Lists. Apply
// unmodified wheel motion to the Lists here so ordinary page scrolling remains
// available alongside modifier-wheel zoom.
func applyPDFCanvasScroll(v *pdfViewState, scroll f32.Point) bool {
	if v == nil {
		return false
	}
	x := float64(scroll.X) + v.canvasScrollRemainderX
	y := float64(scroll.Y) + v.canvasScrollRemainderY
	wholeX, wholeY := int(x), int(y)
	v.canvasScrollRemainderX = x - float64(wholeX)
	v.canvasScrollRemainderY = y - float64(wholeY)
	if wholeX != 0 {
		v.horizontal.Position.Offset += wholeX
	}
	if wholeY != 0 {
		v.canvas.Position.Offset += wholeY
	}
	return wholeX != 0 || wholeY != 0
}

func (a *App) setPDFZoomAt(gtx layout.Context, v *pdfViewState, zoom float64, point f32.Point) {
	if v == nil {
		return
	}
	zoom = clampPDFZoom(zoom)
	oldScale := v.scale
	newScale := float64(gtx.Metric.PxPerDp) * (4.0 / 3.0) * zoom
	v.zoom = zoom
	v.fit = ""
	if oldScale <= 0 || newScale <= 0 || math.IsNaN(oldScale) || math.IsNaN(newScale) || math.IsInf(oldScale, 0) || math.IsInf(newScale, 0) {
		return
	}

	viewport := gtx.Constraints.Max
	topInset := float64(gtx.Dp(12))
	first := max(0, min(v.canvas.Position.First, max(0, v.info.Pages-1)))
	firstTop := -float64(v.canvas.Position.Offset)
	anchorPage, anchorY := first, 0.0
	newRowsBeforeAnchor := 0.0
	bestDistance := math.Inf(1)
	rowTop := firstTop
	rowsBefore := 0.0
	visibleCount := max(1, v.canvas.Position.Count) + pdfPageViewportOverscan
	end := min(max(1, v.info.Pages), first+max(pdfPageCacheMaxEntries, visibleCount))
	for index := first; index < end; index++ {
		_, pageHeight := pdfCachedPageSize(v, index)
		pageTop := rowTop + topInset
		pageBottom := pageTop + pageHeight*oldScale
		pointY := float64(point.Y)
		localY := math.Max(0, math.Min(pageHeight, (pointY-pageTop)/oldScale))
		distance := 0.0
		if pointY < pageTop {
			distance = pageTop - pointY
		} else if pointY > pageBottom {
			distance = pointY - pageBottom
		}
		if distance < bestDistance {
			bestDistance = distance
			anchorPage, anchorY = index, localY
			newRowsBeforeAnchor = rowsBefore
		}
		if distance == 0 {
			break
		}
		rowHeight := pageHeight*oldScale + 2*topInset
		rowTop += rowHeight
		rowsBefore += pageHeight*newScale + 2*topInset
	}

	anchorPageWidth, _ := pdfCachedPageSize(v, anchorPage)
	refWidth, _ := pdfReferencePageSize(v)
	padding := gtx.Dp(48)
	oldContentWidth := max(viewport.X, int(refWidth*oldScale)+padding)
	newContentWidth := max(viewport.X, int(refWidth*newScale)+padding)
	oldPageLeft := float64(oldContentWidth-int(anchorPageWidth*oldScale)) / 2
	newPageLeft := float64(newContentWidth-int(anchorPageWidth*newScale)) / 2
	localX := (float64(point.X) + float64(v.horizontal.Position.Offset) - oldPageLeft) / oldScale
	localX = math.Max(0, math.Min(anchorPageWidth, localX))
	newHorizontalOffset := newPageLeft + localX*newScale - float64(point.X)
	newHorizontalOffset = math.Max(0, math.Min(float64(max(0, newContentWidth-viewport.X)), newHorizontalOffset))
	v.horizontal.Position.First = 0
	v.horizontal.Position.Offset = int(math.Round(newHorizontalOffset))

	newVerticalOffset := newRowsBeforeAnchor + topInset + anchorY*newScale - float64(point.Y)
	v.canvas.Position.Offset = max(0, int(math.Round(newVerticalOffset)))
}

func pdfCachedPageSize(v *pdfViewState, index int) (width, height float64) {
	if result, ok := v.cache[index]; ok && result.page.Width > 0 && result.page.Height > 0 {
		return result.page.Width, result.page.Height
	}
	return pdfReferencePageSize(v)
}

func pdfReferencePageSize(v *pdfViewState) (width, height float64) {
	if v != nil {
		if result, ok := v.cache[v.page]; ok && result.page.Width > 0 && result.page.Height > 0 {
			return result.page.Width, result.page.Height
		}
		if v.pageSizeHintW > 0 && v.pageSizeHintH > 0 {
			return v.pageSizeHintW, v.pageSizeHintH
		}
		for _, result := range v.cache {
			if result.page.Width > 0 && result.page.Height > 0 {
				return result.page.Width, result.page.Height
			}
		}
	}
	return 595, 842
}
