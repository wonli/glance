package desktopui

import (
	"image"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

const wordReadingBarHeightDP = 40

func (a *App) toggleWordReadingMode() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	state.wordReadingMode = !state.wordReadingMode
	state.wordToolbarColorPicker = false
	state.wordToolbarHighlightPicker = false
	if state.wordReadingMode {
		a.setStatus("已进入阅读模式")
	} else {
		a.setStatus("已退出阅读模式")
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) layoutWordReadingBar(gtx layout.Context, state *documentSessionState) layout.Dimensions {
	height := gtx.Dp(unit.Dp(wordReadingBarHeightDP))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	if state.wordReadingExitButton.Clicked(gtx) {
		a.toggleWordReadingMode()
	}
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, wordRulerBackground, clip.Rect{Max: gtx.Constraints.Max}.Op())
			paint.FillShape(gtx.Ops, ribbonDividerColor, clip.Rect{Min: image.Pt(0, height-gtx.Dp(unit.Dp(1))), Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						label := material.Caption(a.theme, "阅读模式")
						label.Color = wordStatusText
						semantic.DescriptionOp("Word 阅读模式").Add(gtx.Ops)
						return label.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						semantic.Button.Add(gtx.Ops)
						semantic.DescriptionOp("退出阅读模式").Add(gtx.Ops)
						return material.Button(a.theme, &state.wordReadingExitButton, "退出阅读模式").Layout(gtx)
					}),
				)
			})
		},
	)
}
