package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/io/semantic"
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

const (
	wordPageBottomGapDP   = 24
	wordPageTopGapDP      = 16
	wordMultiplePageGapDP = 24
)

// layoutWordPageView replays the accepted point-based display list. It does not
// shape, measure, wrap, or paginate content; those operations happen in the
// background wordlayout engine and are shared with PDF/print adapters.
func (a *App) layoutWordPageView(gtx layout.Context, state *documentSessionState) layout.Dimensions {
	state.wordPageListState.List.Axis = layout.Vertical
	paint.FillShape(gtx.Ops, wordWorkspaceBackground, clip.Rect{Max: gtx.Constraints.Max}.Op())
	result := state.wordLayoutResult
	if state.wordLayoutError != "" {
		return layoutWordPageMessage(gtx, a.theme, "分页失败："+state.wordLayoutError, wordPageErrorSurface)
	}
	if result == nil {
		message := "正在计算页面布局…"
		if state.wordLayoutPendingKey == nil {
			message = "页面数尚未计算"
		}
		return layoutWordPageMessage(gtx, a.theme, message, wordPageWarningSurface)
	}
	if !result.CanPreview {
		message := "此文档包含暂不支持的版式，无法保证页面与打印结果一致。"
		for _, diagnostic := range result.Diagnostics {
			if diagnostic.Severity == wordlayout.DiagnosticError {
				message = diagnostic.Detail
				break
			}
		}
		return layoutWordPageMessage(gtx, a.theme, message, wordPageErrorSurface)
	}
	if len(result.Pages) == 0 {
		return layoutWordPageMessage(gtx, a.theme, "文档没有可显示的页面。", wordPageWarningSurface)
	}
	columns, columnGap := 1, 0
	if state.wordMultiplePagesMode {
		columns = 2
		columnGap = gtx.Dp(unit.Dp(wordMultiplePageGapDP))
	}
	if state.wordPageFitMode {
		pageIndex := max(0, min(state.wordPageListState.Position.First*columns, len(result.Pages)-1))
		availableWidth := float64(max(1, gtx.Constraints.Max.X-gtx.Dp(unit.Dp(48))-columnGap))
		availableHeight := float64(max(1, gtx.Constraints.Max.Y-gtx.Dp(unit.Dp(wordPageTopGapDP+wordPageBottomGapDP))))
		state.wordLayoutZoom = wordPageFitZoomForColumns(result.Pages[pageIndex], availableWidth, availableHeight, float64(gtx.Metric.PxPerDp), columns)
	}
	if columns == 2 {
		return a.layoutWordMultiplePageRows(gtx, state, result, columnGap)
	}
	dimensions := state.wordPageListState.Layout(gtx, len(result.Pages), func(gtx layout.Context, index int) layout.Dimensions {
		page := result.Pages[index]
		pageScale := wordScenePixelsPerPoint(gtx, page, state.wordLayoutZoom)
		pageWidth := max(1, int(math.Ceil(page.Clip.WidthPT*pageScale)))
		pageHeight := max(1, int(math.Ceil(page.Clip.HeightPT*pageScale)))
		gapTop := gtx.Dp(unit.Dp(wordPageTopGapDP))
		gapBottom := gtx.Dp(unit.Dp(wordPageBottomGapDP))
		itemHeight := gapTop + pageHeight + gapBottom
		itemGtx := gtx
		itemGtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, itemHeight)
		itemGtx.Constraints.Max = itemGtx.Constraints.Min
		paint.FillShape(gtx.Ops, wordWorkspaceBackground, clip.Rect{Max: itemGtx.Constraints.Max}.Op())
		left := max(0, (itemGtx.Constraints.Max.X-pageWidth)/2)
		top := gapTop
		pageRect := image.Rectangle{Min: image.Pt(left, top), Max: image.Pt(left+pageWidth, top+pageHeight)}
		a.drawWordPageSurface(gtx, state, page, index, len(result.Pages), pageRect, pageScale)
		return layout.Dimensions{Size: itemGtx.Constraints.Max}
	})
	state.wordLayoutActivePage = state.wordPageListState.Position.First
	return dimensions
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

func (a *App) layoutWordMultiplePageRows(gtx layout.Context, state *documentSessionState, result *wordlayout.Result, columnGap int) layout.Dimensions {
	rowCount := (len(result.Pages) + 1) / 2
	dimensions := state.wordPageListState.Layout(gtx, rowCount, func(gtx layout.Context, rowIndex int) layout.Dimensions {
		firstPageIndex := rowIndex * 2
		firstPage := result.Pages[firstPageIndex]
		firstScale := wordScenePixelsPerPoint(gtx, firstPage, state.wordLayoutZoom)
		firstWidth := max(1, int(math.Ceil(firstPage.Clip.WidthPT*firstScale)))
		firstHeight := max(1, int(math.Ceil(firstPage.Clip.HeightPT*firstScale)))
		secondPageIndex := firstPageIndex + 1
		hasSecond := secondPageIndex < len(result.Pages)
		secondWidth, secondHeight := 0, 0
		secondScale := firstScale
		if hasSecond {
			secondPage := result.Pages[secondPageIndex]
			secondScale = wordScenePixelsPerPoint(gtx, secondPage, state.wordLayoutZoom)
			secondWidth = max(1, int(math.Ceil(secondPage.Clip.WidthPT*secondScale)))
			secondHeight = max(1, int(math.Ceil(secondPage.Clip.HeightPT*secondScale)))
		}
		gapTop := gtx.Dp(unit.Dp(wordPageTopGapDP))
		gapBottom := gtx.Dp(unit.Dp(wordPageBottomGapDP))
		rowHeight := gapTop + max(firstHeight, secondHeight) + gapBottom
		rowGtx := gtx
		rowGtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, rowHeight)
		rowGtx.Constraints.Max = rowGtx.Constraints.Min
		paint.FillShape(gtx.Ops, wordWorkspaceBackground, clip.Rect{Max: rowGtx.Constraints.Max}.Op())
		totalWidth := firstWidth
		if hasSecond {
			totalWidth += columnGap + secondWidth
		}
		left := max(0, (rowGtx.Constraints.Max.X-totalWidth)/2)
		firstRect := image.Rect(left, gapTop, left+firstWidth, gapTop+firstHeight)
		a.drawWordPageSurface(gtx, state, firstPage, firstPageIndex, len(result.Pages), firstRect, firstScale)
		if hasSecond {
			secondRect := image.Rect(firstRect.Max.X+columnGap, gapTop, firstRect.Max.X+columnGap+secondWidth, gapTop+secondHeight)
			a.drawWordPageSurface(gtx, state, result.Pages[secondPageIndex], secondPageIndex, len(result.Pages), secondRect, secondScale)
		}
		return layout.Dimensions{Size: rowGtx.Constraints.Max}
	})
	state.wordLayoutActivePage = state.wordPageListState.Position.First * 2
	return dimensions
}

func (a *App) drawWordPageSurface(gtx layout.Context, state *documentSessionState, page wordlayout.PageScene, index, total int, pageRect image.Rectangle, pageScale float64) {
	shadowRect := pageRect.Add(image.Pt(gtx.Dp(unit.Dp(2)), gtx.Dp(unit.Dp(3))))
	paint.FillShape(gtx.Ops, wordPageShadow, clip.Rect{Min: shadowRect.Min, Max: shadowRect.Max}.Op())
	paint.FillShape(gtx.Ops, wordPageSurface, clip.Rect{Min: pageRect.Min, Max: pageRect.Max}.Op())
	if state.wordGridlinesVisible && !state.wordReadingMode {
		drawWordGridlines(gtx, pageRect)
	}
	pageClip := clip.Rect{Min: pageRect.Min, Max: pageRect.Max}.Push(gtx.Ops)
	semantic.DescriptionOp(fmt.Sprintf("Word 第 %d 页，共 %d 页", index+1, total)).Add(gtx.Ops)
	a.drawWordPageScene(gtx, state, page, pageRect.Min, pageScale)
	pageClip.Pop()
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
		drawWordPageCellBorder(gtx, origin, cell.Bounds, pixelsPerPoint)
	}
	for _, fragment := range page.Text {
		drawWordPageTextFragment(gtx, origin, fragment, pixelsPerPoint)
	}
	for _, border := range page.ParagraphBorders {
		drawWordPageParagraphBorder(gtx, origin, border, pixelsPerPoint)
	}
	for _, placement := range page.Images {
		decoded, ok := state.wordImageCache.Get(placement.ImageID)
		if !ok {
			if state.wordImageErrors[placement.ImageID] == "" {
				a.requestWordImageDecode(state, placement.ImageID)
			}
			drawWordPageImagePlaceholder(gtx, origin, placement.Bounds, pixelsPerPoint)
			continue
		}
		drawWordPageImage(gtx, origin, decoded, placement, pixelsPerPoint)
	}
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
