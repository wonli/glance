package desktopui

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

func (a *App) layoutPDFExportPaperCanvas(gtx layout.Context) layout.Dimensions {
	prev := &a.pdfExportPreview

	boxHeight := gtx.Constraints.Max.Y
	if boxHeight > gtx.Dp(unit.Dp(250)) {
		boxHeight = gtx.Dp(unit.Dp(250))
	}
	gtx.Constraints.Min.Y = boxHeight
	gtx.Constraints.Max.Y = boxHeight

	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		// Gray background board
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			bounds := image.Rectangle{Max: gtx.Constraints.Max}
			radius := gtx.Dp(unit.Dp(8))
			shape := clip.RRect{Rect: bounds, NW: radius, NE: radius, SW: radius, SE: radius}
			paint.FillShape(gtx.Ops, color.NRGBA{R: 248, G: 250, B: 252, A: 255}, shape.Op(gtx.Ops))
			paint.FillShape(gtx.Ops, color.NRGBA{R: 226, G: 232, B: 240, A: 255}, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
			return layout.Dimensions{Size: bounds.Max}
		}),
		// Centered Miniature Paper Sheet
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			paperW := gtx.Dp(unit.Dp(150))
			paperH := gtx.Dp(unit.Dp(212))
			if prev.Orientation == "landscape" {
				paperW = gtx.Dp(unit.Dp(212))
				paperH = gtx.Dp(unit.Dp(150))
			}
			maxW := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(16))
			maxH := gtx.Constraints.Max.Y - gtx.Dp(unit.Dp(16))
			if maxW > 0 && paperW > maxW {
				paperH = paperH * maxW / paperW
				paperW = maxW
			}
			if maxH > 0 && paperH > maxH {
				paperW = paperW * maxH / paperH
				paperH = maxH
			}

			gtx.Constraints.Min = image.Pt(paperW, paperH)
			gtx.Constraints.Max = image.Pt(paperW, paperH)

			return layout.Stack{Alignment: layout.Center}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					bounds := image.Rectangle{Max: image.Pt(paperW, paperH)}
					radius := gtx.Dp(unit.Dp(3))
					// Subtle shadow
					shadowBounds := bounds.Add(image.Pt(0, gtx.Dp(unit.Dp(2))))
					paint.FillShape(gtx.Ops, color.NRGBA{R: 30, G: 41, B: 59, A: 20}, clip.RRect{Rect: shadowBounds, NW: radius, NE: radius, SW: radius, SE: radius}.Op(gtx.Ops))
					// White Paper
					paperShape := clip.RRect{Rect: bounds, NW: radius, NE: radius, SW: radius, SE: radius}
					paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, paperShape.Op(gtx.Ops))
					paint.FillShape(gtx.Ops, color.NRGBA{R: 203, G: 213, B: 225, A: 255}, clip.Stroke{Path: paperShape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
					return layout.Dimensions{Size: bounds.Max}
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						if prev.Format == "spreadsheet" {
							return a.layoutMiniatureSpreadsheetPaper(gtx)
						}
						if prev.Format == "presentation" {
							return a.layoutMiniaturePresentationPaper(gtx)
						}
						return a.layoutMiniatureWordPaper(gtx)
					})
				}),
			)
		}),
	)
}

func (a *App) layoutMiniatureSpreadsheetPaper(gtx layout.Context) layout.Dimensions {
	prev := &a.pdfExportPreview
	size := gtx.Constraints.Max

	p := prev.sheetPagination
	colBandIdx := 0
	rowBandIdx := 0
	if len(p.ColBands) > 0 && len(p.RowBands) > 0 {
		numRowBands := len(p.RowBands)
		bandIdx := prev.CurrentPageIndex
		colBandIdx = bandIdx / numRowBands
		rowBandIdx = bandIdx % numRowBands
		if colBandIdx >= len(p.ColBands) {
			colBandIdx = len(p.ColBands) - 1
		}
		if rowBandIdx >= len(p.RowBands) {
			rowBandIdx = len(p.RowBands) - 1
		}
	}

	startCol, endCol := 0, len(p.ColWidths)
	if colBandIdx < len(p.ColBands) {
		startCol = p.ColBands[colBandIdx].Start
		endCol = p.ColBands[colBandIdx].End
	}
	visibleColCount := endCol - startCol
	if visibleColCount <= 0 {
		visibleColCount = 1
	}

	startRow, endRow := 0, len(p.RowHeights)
	if rowBandIdx < len(p.RowBands) {
		startRow = p.RowBands[rowBandIdx].Start
		endRow = p.RowBands[rowBandIdx].End
	}
	visibleRowCount := endRow - startRow
	if visibleRowCount <= 0 {
		visibleRowCount = 1
	}
	if visibleRowCount > 12 {
		visibleRowCount = 12
	}

	colW := float64(size.X) / float64(visibleColCount)
	rowH := float64(size.Y) / float64(visibleRowCount)
	if rowH > float64(gtx.Dp(unit.Dp(14))) {
		rowH = float64(gtx.Dp(unit.Dp(14)))
	}

	// 1. Draw header background
	headerRect := image.Rect(0, 0, size.X, int(rowH))
	paint.FillShape(gtx.Ops, color.NRGBA{R: 219, G: 234, B: 254, A: 255}, clip.Rect{Max: headerRect.Max}.Op())

	// 2. Draw horizontal grid lines
	for r := 0; r <= visibleRowCount; r++ {
		y := int(float64(r) * rowH)
		if y > size.Y {
			y = size.Y
		}
		lineRect := image.Rect(0, y, size.X, y+1)
		paint.FillShape(gtx.Ops, color.NRGBA{R: 226, G: 232, B: 240, A: 255}, clip.Rect{Min: lineRect.Min, Max: lineRect.Max}.Op())
	}

	// 3. Draw vertical grid lines
	for c := 0; c <= visibleColCount; c++ {
		x := int(float64(c) * colW)
		if x > size.X {
			x = size.X
		}
		totalH := int(float64(visibleRowCount) * rowH)
		lineRect := image.Rect(x, 0, x+1, totalH)
		paint.FillShape(gtx.Ops, color.NRGBA{R: 226, G: 232, B: 240, A: 255}, clip.Rect{Min: lineRect.Min, Max: lineRect.Max}.Op())
	}

	// 4. Subtle data bars inside cells
	for r := 0; r < visibleRowCount; r++ {
		for c := 0; c < visibleColCount; c++ {
			x := int(float64(c)*colW) + gtx.Dp(unit.Dp(2))
			y := int(float64(r)*rowH) + int(rowH/2) - 1
			w := int(colW*0.6) - gtx.Dp(unit.Dp(4))
			if w < 4 {
				w = 4
			}
			barColor := color.NRGBA{R: 203, G: 213, B: 225, A: 255}
			if r == 0 {
				barColor = color.NRGBA{R: 147, G: 197, B: 253, A: 255}
			}
			paint.FillShape(gtx.Ops, barColor, clip.Rect{Min: image.Pt(x, y), Max: image.Pt(x+w, y+2)}.Op())
		}
	}

	return layout.Dimensions{Size: size}
}

func (a *App) layoutMiniatureWordPaper(gtx layout.Context) layout.Dimensions {
	prev := &a.pdfExportPreview
	size := gtx.Constraints.Max

	if prev.wordResult == nil || len(prev.wordResult.Pages) == 0 {
		// Draw default mock text lines
		return a.layoutMockWordLines(gtx, size)
	}

	pageIdx := prev.CurrentPageIndex
	if pageIdx >= len(prev.wordResult.Pages) {
		pageIdx = len(prev.wordResult.Pages) - 1
	}
	pageScene := prev.wordResult.Pages[pageIdx]

	clipW := pageScene.Clip.WidthPT
	clipH := pageScene.Clip.HeightPT
	if clipW <= 0 {
		clipW = 595.28
	}
	if clipH <= 0 {
		clipH = 841.89
	}

	scaleX := float64(size.X) / clipW
	scaleY := float64(size.Y) / clipH

	// Draw text lines
	lineColor := color.NRGBA{R: 148, G: 163, B: 184, A: 255}
	for _, f := range pageScene.Text {
		x := int(f.XPT * scaleX)
		y := int(f.BaselineYPT*scaleY) - 1
		w := int(float64(len([]rune(f.Text))) * 6.0 * scaleX)
		if w < 6 {
			w = 6
		}
		if x+w > size.X {
			w = size.X - x
		}
		if y >= 0 && y+2 <= size.Y && w > 0 {
			paint.FillShape(gtx.Ops, lineColor, clip.Rect{Min: image.Pt(x, y), Max: image.Pt(x+w, y+2)}.Op())
		}
	}

	// Draw image placements
	imgColor := color.NRGBA{R: 191, G: 219, B: 254, A: 255}
	for _, img := range pageScene.Images {
		x := int(img.Bounds.XPT * scaleX)
		y := int(img.Bounds.YPT * scaleY)
		w := int(img.Bounds.WidthPT * scaleX)
		h := int(img.Bounds.HeightPT * scaleY)
		if x >= 0 && y >= 0 && w > 0 && h > 0 {
			paint.FillShape(gtx.Ops, imgColor, clip.Rect{Min: image.Pt(x, y), Max: image.Pt(x+w, y+h)}.Op())
		}
	}

	return layout.Dimensions{Size: size}
}

func (a *App) layoutMockWordLines(gtx layout.Context, size image.Point) layout.Dimensions {
	lineColor := color.NRGBA{R: 148, G: 163, B: 184, A: 255}
	// Title
	paint.FillShape(gtx.Ops, color.NRGBA{R: 71, G: 85, B: 105, A: 255}, clip.Rect{Min: image.Pt(size.X/4, gtx.Dp(unit.Dp(6))), Max: image.Pt(size.X*3/4, gtx.Dp(unit.Dp(10)))}.Op())
	// Body paragraphs
	y := gtx.Dp(unit.Dp(16))
	step := gtx.Dp(unit.Dp(6))
	for y < size.Y-gtx.Dp(unit.Dp(10)) {
		paint.FillShape(gtx.Ops, lineColor, clip.Rect{Min: image.Pt(gtx.Dp(unit.Dp(4)), y), Max: image.Pt(size.X-gtx.Dp(unit.Dp(4)), y+2)}.Op())
		y += step
	}
	return layout.Dimensions{Size: size}
}
