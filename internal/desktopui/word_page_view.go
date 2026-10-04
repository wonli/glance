package desktopui

import (
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/wordlayout"
)

var (
	wordPageSurface        = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	wordPageShadow         = color.NRGBA{R: 25, G: 32, B: 42, A: 38}
	wordPageTableLine      = color.NRGBA{R: 136, G: 143, B: 151, A: 255}
	wordPageWarningSurface = color.NRGBA{R: 255, G: 247, B: 224, A: 255}
	wordPageErrorSurface   = color.NRGBA{R: 255, G: 237, B: 235, A: 255}
)

// layoutWordPageView presents the same accepted page scenes in continuous and
// print layouts. The presentation changes page surfaces and gaps only.
func (a *App) layoutWordPageView(gtx layout.Context, state *documentSessionState) layout.Dimensions {
	presentation := wordPageContinuous
	if state.wordViewMode == wordViewPrintLayout {
		presentation = wordPagePrint
	}
	if state.wordRulerHidden || state.wordReadingMode {
		return a.layoutWordPages(gtx, state, presentation)
	}
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return a.layoutWordPages(gtx, state, presentation)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			rulerWidth := gtx.Dp(unit.Dp(wordRulerWidthDP))
			totalWidth := gtx.Dp(unit.Dp(state.wordCanvasWidthDP))
			if totalWidth <= 0 {
				totalWidth = gtx.Dp(unit.Dp(wordPaperMaxWidthDP))
			}
			if state.wordLayoutResult != nil && len(state.wordLayoutResult.Pages) > 0 {
				first := state.wordLayoutResult.Pages[0]
				scale := wordScenePixelsPerPoint(gtx, first, state.wordLayoutZoom)
				firstWidth := max(1, int(math.Ceil(first.Clip.WidthPT*scale)))
				totalWidth = firstWidth
			}
			canvasLeft := max(0, (gtx.Constraints.Max.X-totalWidth)/2) - int(math.Round(state.wordPageScrollX))
			left := max(0, canvasLeft-rulerWidth-gtx.Dp(unit.Dp(wordRulerGapDP)))
			leftDP := float32(left) / max32(0.1, gtx.Metric.PxPerDp)
			topDP := float32(min(gtx.Dp(unit.Dp(wordVerticalRulerTopDP)), gtx.Constraints.Max.Y)) / max32(0.1, gtx.Metric.PxPerDp)
			bottomDP := float32(min(gtx.Dp(unit.Dp(wordVerticalRulerBottomDP)), max(0, gtx.Constraints.Max.Y-gtx.Dp(unit.Dp(wordVerticalRulerTopDP))))) / max32(0.1, gtx.Metric.PxPerDp)
			return layout.Inset{Top: unit.Dp(topDP), Bottom: unit.Dp(bottomDP), Left: unit.Dp(leftDP)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layoutWordVerticalRuler(gtx, a.theme)
			})
		}),
	)
}

func wordPageFitZoom(page wordlayout.PageScene, availableWidthPX, availableHeightPX, pixelsPerDP float64) float64 {
	return wordPageFitZoomForColumns(page, availableWidthPX, availableHeightPX, pixelsPerDP, 1)
}

func wordPageFitZoomForColumns(page wordlayout.PageScene, availableWidthPX, availableHeightPX, pixelsPerDP float64, columns int) float64 {
	widthPT, heightPT := page.Clip.WidthPT, page.Clip.HeightPT
	if widthPT <= 0 || heightPT <= 0 || pixelsPerDP <= 0 {
		return 1
	}
	columns = max(1, columns)
	pixelsPerPoint := pixelsPerDP * (4.0 / 3.0)
	fitWidth := availableWidthPX / (float64(columns) * widthPT * pixelsPerPoint)
	fitHeight := availableHeightPX / (heightPT * pixelsPerPoint)
	return wordStatusZoomClamp(math.Min(fitWidth, fitHeight))
}

func wordScenePixelsPerPoint(gtx layout.Context, _ wordlayout.PageScene, zoom float64) float64 {
	if zoom <= 0 || math.IsNaN(zoom) || math.IsInf(zoom, 0) {
		zoom = 1
	}
	return float64(gtx.Metric.PxPerDp) * (4.0 / 3.0) * zoom
}

func layoutWordPageMessage(gtx layout.Context, theme *material.Theme, message string, background color.NRGBA) layout.Dimensions {
	paint.FillShape(gtx.Ops, background, clip.Rect{Max: gtx.Constraints.Max}.Op())
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(24), Right: unit.Dp(24), Top: unit.Dp(12), Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			label := material.Body1(theme, message)
			label.Color = wordStatusText
			label.MaxLines = 4
			return label.Layout(gtx)
		})
	})
}

func (a *App) drawWordPageScene(gtx layout.Context, state *documentSessionState, page wordlayout.PageScene, origin image.Point, pixelsPerPoint float64) {
	for _, cell := range page.TableCells {
		drawWordPageCellBackground(gtx, origin, cell, pixelsPerPoint)
	}
	for _, cell := range page.TableCells {
		drawWordPageCellBorder(gtx, origin, cell.Bounds, pixelsPerPoint)
	}
	drawWordPageSelection(gtx, state, page, origin, pixelsPerPoint)
	for _, fragment := range page.Text {
		drawWordPageTextFragment(gtx, origin, fragment, pixelsPerPoint)
	}
	for _, border := range page.ParagraphBorders {
		drawWordPageParagraphBorder(gtx, origin, border, pixelsPerPoint)
	}
	for _, placement := range page.Images {
		placement = wordPageImagePreviewPlacement(state, placement)
		decoded, ok := state.wordImageCache.Get(placement.ImageID)
		if !ok {
			if state.wordImageErrors[placement.ImageID] == "" {
				a.requestWordImageDecode(state, placement.ImageID)
			}
			drawWordPageImagePlaceholder(gtx, origin, placement.Bounds, pixelsPerPoint)
			a.drawWordPageImageSelection(gtx, state, placement, origin, pixelsPerPoint)
			continue
		}
		drawWordPageImage(gtx, origin, decoded, placement, pixelsPerPoint)
		a.drawWordPageImageSelection(gtx, state, placement, origin, pixelsPerPoint)
	}
	result := state.wordLayoutResult
	if result != nil {
		drawWordPageCaret(gtx, state, result, page.Index, origin, pixelsPerPoint)
	}
}

func wordPageImagePreviewPlacement(state *documentSessionState, placement wordlayout.ImagePlacement) wordlayout.ImagePlacement {
	if state == nil || state.selectedWordImageID != placement.ImageID {
		return placement
	}
	interaction := wordImageInteractionFor(state, placement.ImageID)
	preview, mode := wordImageLayoutPreview(interaction)
	if !preview.Active {
		return placement
	}
	switch mode {
	case wordImageGestureResize:
		if preview.OriginalWidthEMU > 0 && preview.OriginalHeightEMU > 0 {
			widthRatio := float64(preview.WidthEMU) / float64(preview.OriginalWidthEMU)
			heightRatio := float64(preview.HeightEMU) / float64(preview.OriginalHeightEMU)
			oldHeight := placement.Bounds.HeightPT
			placement.Bounds.WidthPT *= widthRatio
			placement.Bounds.HeightPT *= heightRatio
			placement.Bounds.YPT += oldHeight - placement.Bounds.HeightPT
		}
	case wordImageGestureRotate:
		placement.RotationDegrees = preview.RotationDegrees
	}
	return placement
}

func (a *App) drawWordPageImageSelection(gtx layout.Context, state *documentSessionState, placement wordlayout.ImagePlacement, origin image.Point, pixelsPerPoint float64) {
	if state.wordReadingMode || state.selectedWordImageID != placement.ImageID {
		return
	}
	x := origin.X + int(math.Round(placement.Bounds.XPT*pixelsPerPoint))
	y := origin.Y + int(math.Round(placement.Bounds.YPT*pixelsPerPoint))
	width := max(1, int(math.Ceil(placement.Bounds.WidthPT*pixelsPerPoint)))
	height := max(1, int(math.Ceil(placement.Bounds.HeightPT*pixelsPerPoint)))
	imageRect := image.Rect(x, y, x+width, y+height)
	a.drawWordImageSelection(gtx, imageRect)
	item := findWordImage(wordBlocksForState(state), placement.ImageID)
	if item == nil {
		return
	}
	interaction := wordImageInteractionFor(state, placement.ImageID)
	a.layoutWordImageResizeHandles(gtx, interaction, *item, imageRect.Size(), imageRect)
	a.layoutWordImageRotationHandle(gtx, interaction, *item, imageRect)
}

func drawWordPageCellBackground(gtx layout.Context, origin image.Point, cell wordlayout.TableCellPlacement, pixelsPerPoint float64) {
	fill, ok := parseWordRunColor(cell.FillColor)
	if !ok || cell.Bounds.WidthPT <= 0 || cell.Bounds.HeightPT <= 0 {
		return
	}
	x := origin.X + int(math.Round(cell.Bounds.XPT*pixelsPerPoint))
	y := origin.Y + int(math.Round(cell.Bounds.YPT*pixelsPerPoint))
	width := max(1, int(math.Ceil(cell.Bounds.WidthPT*pixelsPerPoint)))
	height := max(1, int(math.Ceil(cell.Bounds.HeightPT*pixelsPerPoint)))
	paint.FillShape(gtx.Ops, fill, clip.Rect{Min: image.Pt(x, y), Max: image.Pt(x+width, y+height)}.Op())
}

func drawWordPageParagraphBorder(gtx layout.Context, origin image.Point, border wordlayout.ParagraphBorderPlacement, pixelsPerPoint float64) {
	if border.Style == "none" || border.Style == "nil" || border.Bounds.WidthPT <= 0 || border.Bounds.HeightPT <= 0 {
		return
	}
	ink, ok := parseWordRunColor(border.Color)
	if !ok {
		ink = color.NRGBA{A: 255}
	}
	x := origin.X + int(math.Round(border.Bounds.XPT*pixelsPerPoint))
	y := origin.Y + int(math.Round(border.Bounds.YPT*pixelsPerPoint))
	width := max(1, int(math.Ceil(border.Bounds.WidthPT*pixelsPerPoint)))
	height := max(1, int(math.Ceil(border.Bounds.HeightPT*pixelsPerPoint)))
	paint.FillShape(gtx.Ops, ink, clip.Rect{Min: image.Pt(x, y), Max: image.Pt(x+width, y+height)}.Op())
}

func drawWordPageTextFragment(gtx layout.Context, origin image.Point, fragment wordlayout.TextFragment, pixelsPerPoint float64) {
	pen := 0.0
	for _, glyph := range fragment.Glyphs {
		pen += glyph.AdvanceXPT
	}
	if highlight, ok := wordHighlightDisplayColor(fragment.Style.Highlight); ok && pen > 0 {
		x := origin.X + int(math.Round(fragment.XPT*pixelsPerPoint))
		top := origin.Y + int(math.Round((fragment.BaselineYPT-fragment.Style.FontSize*0.88)*pixelsPerPoint))
		width := max(1, int(math.Ceil(pen*pixelsPerPoint)))
		height := max(1, int(math.Ceil(fragment.Style.FontSize*1.18*pixelsPerPoint)))
		paint.FillShape(gtx.Ops, highlight, clip.Rect{Min: image.Pt(x, top), Max: image.Pt(x+width, top+height)}.Op())
	}
	for _, glyph := range fragment.Glyphs {
		if len(glyph.Outline) > 0 {
			drawWordPageGlyph(gtx, origin, glyph, wordPageTextColor(fragment.Style), pixelsPerPoint)
		}
	}
	if fragment.Style.Underline && pen > 0 {
		x := origin.X + int(math.Round(fragment.XPT*pixelsPerPoint))
		baseline := origin.Y + int(math.Round((fragment.BaselineYPT+max(0.5, fragment.Style.FontSize*0.08))*pixelsPerPoint))
		width := max(1, int(math.Ceil(pen*pixelsPerPoint)))
		height := max(1, int(math.Ceil(0.6*pixelsPerPoint)))
		paint.FillShape(gtx.Ops, wordPageTextColor(fragment.Style), clip.Rect{Min: image.Pt(x, baseline), Max: image.Pt(x+width, baseline+height)}.Op())
	}
}

func drawWordPageGlyph(gtx layout.Context, origin image.Point, glyph wordlayout.PositionedGlyph, ink color.NRGBA, pixelsPerPoint float64) {
	var path clip.Path
	path.Begin(gtx.Ops)
	for _, command := range glyph.Outline {
		point := func(value wordlayout.GlyphPathPoint) f32.Point {
			return f32.Pt(float32((glyph.XPT+value.XPT)*pixelsPerPoint), float32((glyph.YPT+value.YPT)*pixelsPerPoint))
		}
		switch command.Op {
		case wordlayout.GlyphPathMoveTo:
			path.MoveTo(point(command.Points[0]))
		case wordlayout.GlyphPathLineTo:
			path.LineTo(point(command.Points[0]))
		case wordlayout.GlyphPathCurveTo:
			path.CubeTo(point(command.Points[0]), point(command.Points[1]), point(command.Points[2]))
		}
	}
	shape := clip.Outline{Path: path.End()}.Op()
	transform := op.Offset(origin).Push(gtx.Ops)
	paint.FillShape(gtx.Ops, ink, shape)
	transform.Pop()
}

func wordPageTextColor(style document.WordTextStyle) color.NRGBA {
	if value, ok := parseWordRunColor(style.FontColor); ok {
		return value
	}
	return color.NRGBA{A: 255}
}

func drawWordPageCellBorder(gtx layout.Context, origin image.Point, bounds wordlayout.Rect, pixelsPerPoint float64) {
	x := origin.X + int(math.Round(bounds.XPT*pixelsPerPoint))
	y := origin.Y + int(math.Round(bounds.YPT*pixelsPerPoint))
	width := max(1, int(math.Ceil(bounds.WidthPT*pixelsPerPoint)))
	height := max(1, int(math.Ceil(bounds.HeightPT*pixelsPerPoint)))
	thickness := max(1, int(math.Ceil(0.5*pixelsPerPoint)))
	for _, line := range []image.Rectangle{
		image.Rect(x, y, x+width, y+thickness),
		image.Rect(x, y+height-thickness, x+width, y+height),
		image.Rect(x, y, x+thickness, y+height),
		image.Rect(x+width-thickness, y, x+width, y+height),
	} {
		paint.FillShape(gtx.Ops, wordPageTableLine, clip.Rect{Min: line.Min, Max: line.Max}.Op())
	}
}

func drawWordPageImage(gtx layout.Context, origin image.Point, decoded image.Image, placement wordlayout.ImagePlacement, pixelsPerPoint float64) {
	width := max(1, int(math.Ceil(placement.Bounds.WidthPT*pixelsPerPoint)))
	height := max(1, int(math.Ceil(placement.Bounds.HeightPT*pixelsPerPoint)))
	x := int(math.Round(placement.Bounds.XPT * pixelsPerPoint))
	y := int(math.Round(placement.Bounds.YPT * pixelsPerPoint))
	transform := op.Offset(image.Pt(origin.X+x, origin.Y+y)).Push(gtx.Ops)
	if placement.RotationDegrees != 0 {
		center := f32.Pt(float32(width)/2, float32(height)/2)
		rotation := op.Affine(f32.AffineId().Rotate(center, float32(placement.RotationDegrees*math.Pi/180))).Push(gtx.Ops)
		imageGtx := gtx
		imageGtx.Constraints.Min, imageGtx.Constraints.Max = image.Pt(width, height), image.Pt(width, height)
		widget.Image{Src: paint.NewImageOp(decoded), Fit: widget.Fill, Position: layout.Center}.Layout(imageGtx)
		rotation.Pop()
	} else {
		imageGtx := gtx
		imageGtx.Constraints.Min, imageGtx.Constraints.Max = image.Pt(width, height), image.Pt(width, height)
		widget.Image{Src: paint.NewImageOp(decoded), Fit: widget.Fill, Position: layout.Center}.Layout(imageGtx)
	}
	transform.Pop()
}

func drawWordPageImagePlaceholder(gtx layout.Context, origin image.Point, bounds wordlayout.Rect, pixelsPerPoint float64) {
	x := origin.X + int(math.Round(bounds.XPT*pixelsPerPoint))
	y := origin.Y + int(math.Round(bounds.YPT*pixelsPerPoint))
	width := max(1, int(math.Ceil(bounds.WidthPT*pixelsPerPoint)))
	height := max(1, int(math.Ceil(bounds.HeightPT*pixelsPerPoint)))
	paint.FillShape(gtx.Ops, color.NRGBA{R: 239, G: 241, B: 244, A: 255}, clip.Rect{Min: image.Pt(x, y), Max: image.Pt(x+width, y+height)}.Op())
}
