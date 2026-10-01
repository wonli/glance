package desktopui

import (
	"image"
	"image/color"
	"path/filepath"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

const wordStatusBarHeightDP = 34

var (
	wordStatusBackground   = color.NRGBA{R: 250, G: 251, B: 252, A: 255}
	rulerTickColor         = color.NRGBA{R: 146, G: 154, B: 163, A: 255}
	rulerLabelColor        = color.NRGBA{R: 91, G: 99, B: 109, A: 255}
	rulerMarkerColor       = color.NRGBA{R: 74, G: 119, B: 177, A: 255}
	rulerResizeHandleColor = color.NRGBA{R: 92, G: 111, B: 136, A: 255}
)

func (a *App) layoutWordStatusBar(gtx layout.Context, state *documentSessionState) layout.Dimensions {
	height := gtx.Dp(unit.Dp(wordStatusBarHeightDP))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	dirty := state.word.Dirty() || wordHasPendingTextDrafts(state) || state.wordParagraphSplitIntent != nil || state.wordParagraphJoinIntent != nil
	status := "已保存"
	if dirty {
		status = "有未保存的更改"
	}
	name := filepath.Base(state.path)
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "Word 文档"
	}
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, wordStatusBackground, clip.Rect{Max: gtx.Constraints.Max}.Op())
			paint.FillShape(gtx.Ops, ribbonDividerColor, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(1)))}.Op())
			bounds := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
			semantic.DescriptionOp("Word 状态栏").Add(gtx.Ops)
			bounds.Pop()
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						label := material.Caption(a.theme, name)
						label.TextSize = unit.Sp(10)
						label.Color = rulerLabelColor
						return label.Layout(gtx)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						label := material.Caption(a.theme, status)
						label.TextSize = unit.Sp(10)
						label.Color = rulerLabelColor
						return label.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							label := material.Caption(a.theme, "连续视图")
							label.TextSize = unit.Sp(10)
							label.Color = rulerLabelColor
							return label.Layout(gtx)
						})
					}),
				)
			})
		},
	)
}
