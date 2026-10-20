package desktopui

import (
	"glance/internal/document"
	"glance/internal/i18n"
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
	a.modal.ShowInputMessage("word.image.resize", i18n.Message{ID: "word.image.resize.dialog.title"}, i18n.Message{ID: "word.image.resize.dialog.body"}, defaultWidth, i18n.Message{ID: "word.image.resize.dialog.hint"}, func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		widthInches, err := strconv.ParseFloat(strings.TrimSpace(a.modal.Input.Text()), 64)
		if err != nil || math.IsNaN(widthInches) || math.IsInf(widthInches, 0) || widthInches <= 0 || widthInches > 100 {
			a.setStatusMessage(i18n.Message{ID: "word.status.inline.image_width_must_be_greater_777b95"}, feedbackError, notificationToast)
			return
		}
		widthEMU := int64(math.Round(widthInches * 914400))
		a.runWordImageCommand("调整图片", imageID, func(editor document.WordImageEditor) error { return editor.ResizeInlineImage(imageID, widthEMU) }, false)
	}, modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "common.action.apply"}}, modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}})
}

func (a *App) layoutWordToolbarColorPicker(gtx layout.Context) layout.Dimensions {
	state := a.documentSessionState
	if state == nil || !state.wordToolbarColorPicker && !state.wordToolbarHighlightPicker {
		return layout.Dimensions{}
	}
	colors := wordFontColors
	selectedValue := ""
	if run, ok := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID); ok {
		selectedValue = run.Style.FontColor
	}
	if state.wordToolbarHighlightPicker {
		colors = wordHighlightColors
		if run, ok := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID); ok {
			selectedValue = run.Style.Highlight
		}
	}
	selectedValue = strings.TrimPrefix(strings.TrimSpace(selectedValue), "#")
	if !state.wordToolbarHighlightPicker && selectedValue == "" {
		selectedValue = "000000"
	}
	selectedIndex := -1
	for index, item := range colors {
		if strings.EqualFold(selectedValue, item.value) || selectedValue == "" && item.value == "" {
			selectedIndex = index
			break
		}
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
				paint.FillShape(gtx.Ops, a.tokens().Colors.Menu.Shadow, clip.UniformRRect(shadow, gtx.Dp(unit.Dp(5))).Op(gtx.Ops))
				paint.FillShape(gtx.Ops, a.tokens().Colors.Menu.Border, clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(unit.Dp(5))).Op(gtx.Ops))
				paint.FillShape(gtx.Ops, a.tokens().Colors.Menu.Surface, clip.UniformRRect(image.Rectangle{Max: size}.Inset(1), gtx.Dp(unit.Dp(4))).Op(gtx.Ops))
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
								background := a.tokens().Colors.Menu.Surface
								if selectedIndex == index {
									background = a.tokens().Colors.Menu.Selected
								} else if button.Hovered() {
									background = a.tokens().Colors.Menu.Hover
								}
								paint.FillShape(gtx.Ops, background, clip.UniformRRect(bounds, gtx.Dp(unit.Dp(5))).Op(gtx.Ops))
								swatch := bounds.Inset(gtx.Dp(unit.Dp(2)))
								paint.FillShape(gtx.Ops, item.color, clip.UniformRRect(swatch, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
								if state.wordToolbarHighlightPicker && item.value == "" {
									border := clip.UniformRRect(swatch, gtx.Dp(unit.Dp(3)))
									paint.FillShape(gtx.Ops, color.NRGBA{R: 170, G: 184, B: 199, A: 255}, clip.Stroke{Path: border.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
								}
								if selectedIndex == index {
									border := clip.UniformRRect(bounds, gtx.Dp(unit.Dp(5)))
									paint.FillShape(gtx.Ops, a.tokens().Colors.OfficeAccent.Word.Accent, clip.Stroke{Path: border.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
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
