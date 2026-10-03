package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"unicode"

	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
)

const wordStatusBarHeightDP = 34

var (
	wordStatusBackground   = color.NRGBA{R: 247, G: 248, B: 250, A: 255}
	wordStatusTopRule      = color.NRGBA{R: 212, G: 216, B: 222, A: 255}
	wordStatusText         = color.NRGBA{R: 92, G: 100, B: 109, A: 255}
	wordStatusTrack        = color.NRGBA{R: 193, G: 199, B: 207, A: 255}
	wordStatusThumb        = color.NRGBA{R: 83, G: 120, B: 171, A: 255}
	rulerTickColor         = color.NRGBA{R: 146, G: 154, B: 163, A: 255}
	rulerLabelColor        = color.NRGBA{R: 91, G: 99, B: 109, A: 255}
	rulerMarkerColor       = color.NRGBA{R: 74, G: 119, B: 177, A: 255}
	rulerResizeHandleColor = color.NRGBA{R: 92, G: 111, B: 136, A: 255}
)

func (a *App) layoutWordStatusBar(gtx layout.Context, state *documentSessionState) layout.Dimensions {
	if state.wordStatusPrintLayoutButton.Clicked(gtx) {
		a.toggleWordPrintLayout()
	}
	if state.wordStatusWebLayoutButton.Clicked(gtx) {
		a.toggleWordWebLayout()
	}
	if state.wordStatusZoomOutButton.Clicked(gtx) {
		setWordManualZoom(state, state.wordLayoutZoom-0.1)
	}
	if state.wordStatusZoomInButton.Clicked(gtx) {
		setWordManualZoom(state, state.wordLayoutZoom+0.1)
	}
	height := gtx.Dp(unit.Dp(wordStatusBarHeightDP))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	paint.FillShape(gtx.Ops, wordStatusBackground, clip.Rect{Max: gtx.Constraints.Max}.Op())
	paint.FillShape(gtx.Ops, wordStatusTopRule, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(1)))}.Op())
	barBounds := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	semantic.DescriptionOp("Word 状态栏").Add(gtx.Ops)
	barBounds.Pop()

	a.drawWordStatusText(gtx, 18, 11, 118, 16, "Word 页码", wordPaginationStatusText(state))
	a.drawWordStatusText(gtx, 142, 11, 46, 16, "Word 字数", wordStatusWordCount(state))
	a.drawWordStatusText(gtx, 250, 11, 53, 16, "Word 语言", "中文(中国)")
	a.drawWordStatusIcon(gtx, 350, 9, 15, 15, "tX3tP", "Word 辅助功能图标")
	a.drawWordStatusText(gtx, 371, 11, 77, 16, "Word 辅助功能", "辅助功能：正常")

	widthDP := int(float32(gtx.Constraints.Max.X) / gtx.Metric.PxPerDp)
	rightShift := widthDP - 1440
	zoom := wordStatusZoomClamp(state.wordLayoutZoom)
	a.layoutWordStatusZoomSlider(gtx, state, rightShift)
	zoom = wordStatusZoomClamp(state.wordLayoutZoom)
	a.drawWordStatusIcon(gtx, 1112+rightShift, 9, 15, 15, "X8Le51", "Word 专注模式")
	a.drawWordStatusClickableIcon(gtx, 1145+rightShift, 9, "rTjWY", "Word 打印布局", &state.wordStatusPrintLayoutButton, state.wordViewMode == wordViewPrintLayout)
	a.drawWordStatusClickableIcon(gtx, 1178+rightShift, 9, "RWUft", "Word Web版式", &state.wordStatusWebLayoutButton, state.wordViewMode == wordViewWebLayout)
	a.drawWordStatusClickableIcon(gtx, 1230+rightShift, 9, "x6Trh", "Word 缩小", &state.wordStatusZoomOutButton, false)
	a.drawWordStatusRect(gtx, 1257+rightShift, 16, 92, 2, "Word 缩放轨道", wordStatusTrack, 0)
	thumbX := wordStatusZoomThumbLeftDP(zoom)
	a.drawWordStatusRect(gtx, thumbX+rightShift, 11, 11, 11, "Word 缩放滑块", wordStatusThumb, 6)
	a.drawWordStatusClickableIcon(gtx, 1361+rightShift, 9, "kqOIJ", "Word 放大", &state.wordStatusZoomInButton, false)
	a.drawWordStatusText(gtx, 1390+rightShift, 11, 30, 13, "Word 缩放百分比", fmt.Sprintf("%.0f%%", zoom*100))
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func setWordManualZoom(state *documentSessionState, zoom float64) {
	if state == nil {
		return
	}
	currentPage := wordCurrentPageIndex(state)
	setWordPageListFirst(state, currentPage)
	state.wordLayoutZoom = wordStatusZoomClamp(zoom)
	state.wordPageFitMode = false
	state.wordMultiplePagesMode = false
}

const (
	wordStatusZoomMinimum      = 0.5
	wordStatusZoomMaximum      = 1.5
	wordStatusZoomTrackLeftDP  = 1257
	wordStatusZoomReferenceDP  = 1304
	wordStatusZoomThumbLastDP  = 1338
	wordStatusZoomTrackEndDP   = 1349
	wordStatusZoomInteractionY = 5
	wordStatusZoomInteractionH = 23
	wordStatusZoomInteractionX = 8
)

func wordStatusZoomClamp(zoom float64) float64 {
	if math.IsNaN(zoom) || math.IsInf(zoom, 0) {
		return 1
	}
	return math.Max(wordStatusZoomMinimum, math.Min(wordStatusZoomMaximum, zoom))
}

func wordStatusZoomThumbLeftDP(zoom float64) int {
	zoom = wordStatusZoomClamp(zoom)
	if zoom <= 1 {
		return wordStatusZoomTrackLeftDP + int(math.Round((zoom-wordStatusZoomMinimum)/(1-wordStatusZoomMinimum)*float64(wordStatusZoomReferenceDP-wordStatusZoomTrackLeftDP)))
	}
	return wordStatusZoomReferenceDP + int(math.Round((zoom-1)/(wordStatusZoomMaximum-1)*float64(wordStatusZoomThumbLastDP-wordStatusZoomReferenceDP)))
}

func wordStatusZoomAtDesignX(xDP float64) float64 {
	xDP = math.Max(float64(wordStatusZoomTrackLeftDP), math.Min(float64(wordStatusZoomThumbLastDP), xDP))
	if xDP <= float64(wordStatusZoomReferenceDP) {
		return wordStatusZoomMinimum + (xDP-float64(wordStatusZoomTrackLeftDP))/float64(wordStatusZoomReferenceDP-wordStatusZoomTrackLeftDP)*(1-wordStatusZoomMinimum)
	}
	return 1 + (xDP-float64(wordStatusZoomReferenceDP))/float64(wordStatusZoomThumbLastDP-wordStatusZoomReferenceDP)*(wordStatusZoomMaximum-1)
}

func wordStatusZoomForPointerX(positionX float32, metric unit.Metric, rightShiftDP int) float64 {
	pixelsPerDP := max32(0.1, metric.PxPerDp)
	positionDP := float64(positionX) / float64(pixelsPerDP)
	return wordStatusZoomAtDesignX(positionDP - float64(rightShiftDP))
}

func (a *App) layoutWordStatusZoomSlider(gtx layout.Context, state *documentSessionState, rightShiftDP int) {
	leftDP := wordStatusZoomTrackLeftDP - wordStatusZoomInteractionX + rightShiftDP
	rightDP := wordStatusZoomTrackEndDP + wordStatusZoomInteractionX + rightShiftDP
	topDP := wordStatusZoomInteractionY
	bottomDP := wordStatusZoomInteractionY + wordStatusZoomInteractionH
	hitArea := clip.Rect{
		Min: image.Pt(gtx.Dp(unit.Dp(leftDP)), gtx.Dp(unit.Dp(topDP))),
		Max: image.Pt(gtx.Dp(unit.Dp(rightDP)), gtx.Dp(unit.Dp(bottomDP))),
	}.Push(gtx.Ops)
	semantic.Button.Add(gtx.Ops)
	semantic.DescriptionOp("Word 缩放控制").Add(gtx.Ops)
	state.wordStatusZoomDrag.Add(gtx.Ops)

	for event, ok := state.wordStatusZoomDrag.Update(gtx.Metric, gtx.Source, gesture.Horizontal); ok; event, ok = state.wordStatusZoomDrag.Update(gtx.Metric, gtx.Source, gesture.Horizontal) {
		switch event.Kind {
		case pointer.Press, pointer.Drag, pointer.Release:
			setWordManualZoom(state, wordStatusZoomForPointerX(event.Position.X, gtx.Metric, rightShiftDP))
		}
	}
	semantic.LabelOp(fmt.Sprintf("%.0f%%", wordStatusZoomClamp(state.wordLayoutZoom)*100)).Add(gtx.Ops)
	hitArea.Pop()
}

func (a *App) drawWordStatusText(gtx layout.Context, xDP, yDP, widthDP, heightDP int, description, value string) {
	x, y := gtx.Dp(unit.Dp(xDP)), gtx.Dp(unit.Dp(yDP))
	width, height := gtx.Dp(unit.Dp(widthDP)), gtx.Dp(unit.Dp(heightDP))
	transform := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	textGtx := gtx
	textGtx.Constraints = layout.Constraints{Min: image.Pt(width, height), Max: image.Pt(width, height)}
	bounds := clip.Rect{Max: image.Pt(width, height)}.Push(gtx.Ops)
	semantic.DescriptionOp(description).Add(gtx.Ops)
	label := material.Caption(a.theme, value)
	label.TextSize = unit.Sp(10)
	label.Color = wordStatusText
	label.Layout(textGtx)
	bounds.Pop()
	transform.Pop()
}

func (a *App) drawWordStatusIcon(gtx layout.Context, xDP, yDP, widthDP, heightDP int, asset, description string) {
	x, y := gtx.Dp(unit.Dp(xDP)), gtx.Dp(unit.Dp(yDP))
	width, height := gtx.Dp(unit.Dp(widthDP)), gtx.Dp(unit.Dp(heightDP))
	transform := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	iconGtx := gtx
	iconGtx.Constraints = layout.Constraints{Min: image.Pt(width, height), Max: image.Pt(width, height)}
	bounds := clip.Rect{Max: image.Pt(width, height)}.Push(gtx.Ops)
	semantic.DescriptionOp(description).Add(gtx.Ops)
	if icon := wordPenAsset(asset); icon != nil {
		maxEdge := max(icon.Bounds().Dx(), icon.Bounds().Dy())
		widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: float32(width) / float32(maxEdge)}.Layout(iconGtx)
	}
	bounds.Pop()
	transform.Pop()
}

func (a *App) drawWordStatusClickableIcon(gtx layout.Context, xDP, yDP int, asset, description string, button *widget.Clickable, active bool) {
	xDP -= 4
	yDP -= 4
	x, y := gtx.Dp(unit.Dp(xDP)), gtx.Dp(unit.Dp(yDP))
	size := gtx.Dp(unit.Dp(23))
	transform := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Pt(size, size)
		gtx.Constraints.Max = gtx.Constraints.Min
		if active {
			paint.FillShape(gtx.Ops, color.NRGBA{R: 224, G: 235, B: 248, A: 255}, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
		}
		if icon := wordPenAsset(asset); icon != nil {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				size := gtx.Dp(unit.Dp(15))
				gtx.Constraints = layout.Constraints{Min: image.Pt(size, size), Max: image.Pt(size, size)}
				bounds := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
				semantic.DescriptionOp(description).Add(gtx.Ops)
				bounds.Pop()
				maxEdge := max(icon.Bounds().Dx(), icon.Bounds().Dy())
				return widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: float32(size) / float32(max(1, maxEdge))}.Layout(gtx)
			})
		}
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
	transform.Pop()
}

func (a *App) drawWordStatusRect(gtx layout.Context, xDP, yDP, widthDP, heightDP int, description string, fill color.NRGBA, radiusDP int) {
	x, y := gtx.Dp(unit.Dp(xDP)), gtx.Dp(unit.Dp(yDP))
	width, height := gtx.Dp(unit.Dp(widthDP)), gtx.Dp(unit.Dp(heightDP))
	transform := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	bounds := clip.Rect{Max: image.Pt(width, height)}.Push(gtx.Ops)
	semantic.DescriptionOp(description).Add(gtx.Ops)
	roundness := gtx.Dp(unit.Dp(radiusDP))
	paint.FillShape(gtx.Ops, fill, clip.UniformRRect(image.Rectangle{Max: image.Pt(width, height)}, roundness).Op(gtx.Ops))
	bounds.Pop()
	transform.Pop()
}

func wordStatusWordCount(state *documentSessionState) string {
	return fmt.Sprintf("%d 个字", wordTextStatisticsForState(state).charactersWithoutWhitespace)
}

type wordTextStatistics struct {
	charactersWithWhitespace    int
	charactersWithoutWhitespace int
	paragraphs                  int
	tables                      int
}

func wordTextStatisticsForState(state *documentSessionState) wordTextStatistics {
	var statistics wordTextStatistics
	blocks := wordBlocksForState(state)
	for _, block := range blocks {
		if block.Kind == document.WordTableBlock && block.Table != nil {
			statistics.tables++
		}
	}
	paragraphs := allWordParagraphs(blocks)
	statistics.paragraphs = len(paragraphs)
	for _, paragraph := range paragraphs {
		for _, inline := range paragraph.Inlines {
			if inline.Text == nil {
				continue
			}
			value := inline.Text.Text
			if state != nil {
				if draft, ok := state.wordDraft(inline.Text.ID); ok {
					value = draft
				} else if editor := state.wordRunEditors[inline.Text.ID]; editor != nil {
					value = editor.Text()
				}
			}
			for _, r := range value {
				statistics.charactersWithWhitespace++
				if !unicode.IsSpace(r) {
					statistics.charactersWithoutWhitespace++
				}
			}
		}
	}
	return statistics
}
