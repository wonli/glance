package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"unicode"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
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
	height := gtx.Dp(unit.Dp(wordStatusBarHeightDP))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	paint.FillShape(gtx.Ops, wordStatusBackground, clip.Rect{Max: gtx.Constraints.Max}.Op())
	paint.FillShape(gtx.Ops, wordStatusTopRule, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(1)))}.Op())
	barBounds := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	semantic.DescriptionOp("Word 状态栏").Add(gtx.Ops)
	barBounds.Pop()

	a.drawWordStatusText(gtx, 18, 11, 79, 16, "Word 页数", "第 1 页")
	a.drawWordStatusText(gtx, 142, 11, 46, 16, "Word 字数", wordStatusWordCount(state))
	a.drawWordStatusText(gtx, 250, 11, 53, 16, "Word 语言", "中文(中国)")
	a.drawWordStatusIcon(gtx, 350, 9, 15, 15, "tX3tP", "Word 辅助功能图标")
	a.drawWordStatusText(gtx, 371, 11, 77, 16, "Word 辅助功能", "辅助功能：正常")

	widthDP := int(float32(gtx.Constraints.Max.X) / gtx.Metric.PxPerDp)
	rightShift := widthDP - 1440
	a.drawWordStatusIcon(gtx, 1112+rightShift, 9, 15, 15, "X8Le51", "Word 专注模式")
	a.drawWordStatusIcon(gtx, 1145+rightShift, 9, 15, 15, "rTjWY", "Word 打印布局")
	a.drawWordStatusIcon(gtx, 1178+rightShift, 9, 15, 15, "RWUft", "Word Web版式")
	a.drawWordStatusIcon(gtx, 1230+rightShift, 9, 15, 15, "x6Trh", "Word 缩小")
	a.drawWordStatusRect(gtx, 1257+rightShift, 16, 92, 2, "Word 缩放轨道", wordStatusTrack, 0)
	a.drawWordStatusRect(gtx, 1304+rightShift, 11, 11, 11, "Word 缩放滑块", wordStatusThumb, 6)
	a.drawWordStatusIcon(gtx, 1361+rightShift, 9, 15, 15, "kqOIJ", "Word 放大")
	a.drawWordStatusText(gtx, 1390+rightShift, 11, 30, 13, "Word 缩放百分比", "100%")
	return layout.Dimensions{Size: gtx.Constraints.Max}
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
	count := 0
	for _, paragraph := range allWordParagraphs(wordBlocksForState(state)) {
		for _, inline := range paragraph.Inlines {
			if inline.Text == nil {
				continue
			}
			for _, r := range inline.Text.Text {
				if !unicode.IsSpace(r) {
					count++
				}
			}
		}
	}
	return fmt.Sprintf("%d 个字", count)
}
