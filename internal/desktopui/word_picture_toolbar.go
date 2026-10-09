package desktopui

import (
	"glance/internal/document"
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

func (a *App) showWordImageResizeDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	imageID := state.selectedWordImageID
	imageData := findWordImage(wordBlocksForState(state), imageID)
	if imageData == nil {
		return
	}
	defaultWidth := strconv.FormatFloat(float64(imageData.WidthEMU)/914400, 'f', 2, 64)
	a.modal.ShowInput("调整图片大小", "输入宽度（英寸，保持宽高比）", defaultWidth, "宽度", func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		widthInches, err := strconv.ParseFloat(strings.TrimSpace(a.modal.Input.Text()), 64)
		if err != nil || math.IsNaN(widthInches) || math.IsInf(widthInches, 0) || widthInches <= 0 || widthInches > 100 {
			a.setStatus("图片宽度必须大于 0 且不超过 100 英寸")
			return
		}
		widthEMU := int64(math.Round(widthInches * 914400))
		a.runWordImageCommand("调整图片", imageID, func(editor document.WordImageEditor) error { return editor.ResizeInlineImage(imageID, widthEMU) }, false)
	}, modalDialogActionSpec{ID: "confirm", Label: "应用"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}

func (a *App) layoutWordToolbarColorPicker(gtx layout.Context) layout.Dimensions {
	state := a.documentSessionState
	if state == nil || !state.wordToolbarColorPicker && !state.wordToolbarHighlightPicker {
		return layout.Dimensions{}
	}
	colors := wordFontColors
	if state.wordToolbarHighlightPicker {
		colors = wordHighlightColors
	}
	return layout.Inset{Top: unit.Dp(wordRibbonHeightDP + 8), Left: unit.Dp(440)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		width := gtx.Dp(unit.Dp(len(colors)*28 + (len(colors)-1)*6 + 16))
		height := gtx.Dp(unit.Dp(44))
		gtx.Constraints.Min = image.Pt(width, height)
		gtx.Constraints.Max = gtx.Constraints.Min
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				size := gtx.Constraints.Max
				shadow := image.Rectangle{Max: size}.Add(image.Pt(1, 2))
				paint.FillShape(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 22}, clip.UniformRRect(shadow, gtx.Dp(unit.Dp(5))).Op(gtx.Ops))
				paint.FillShape(gtx.Ops, contentSurface, clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(unit.Dp(5))).Op(gtx.Ops))
				return layout.Dimensions{Size: size}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					children := make([]layout.FlexChild, 0, len(colors))
					for index, item := range colors {
						index, item := index, item
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							button := &state.wordToolbarColorButtons[index]
							return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								size := gtx.Dp(unit.Dp(28))
								bounds := image.Rect(0, 0, size, size)
								paint.FillShape(gtx.Ops, item.color, clip.UniformRRect(bounds, gtx.Dp(unit.Dp(5))).Op(gtx.Ops))
								if state.wordToolbarHighlightPicker && item.value == "" {
									border := clip.UniformRRect(bounds, gtx.Dp(unit.Dp(5)))
									paint.FillShape(gtx.Ops, color.NRGBA{R: 170, G: 184, B: 199, A: 255}, clip.Stroke{Path: border.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
								}
								semantic.Button.Add(gtx.Ops)
								semantic.DescriptionOp(item.label).Add(gtx.Ops)
								return layout.Dimensions{Size: image.Pt(size, size)}
							})
						}))
					}
					return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx, children...)
				})
			},
		)
	})
}
