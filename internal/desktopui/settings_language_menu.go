package desktopui

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"glance/internal/i18n"
)

type languageMenuOption struct {
	Selection i18n.Selection
	MessageID string
}

func supportedLanguageOptions() [3]languageMenuOption {
	return [3]languageMenuOption{
		{Selection: i18n.SelectionSystem, MessageID: "settings.basic.language.system"},
		{Selection: i18n.SelectionChinese, MessageID: "settings.basic.language.chinese"},
		{Selection: i18n.SelectionEnglish, MessageID: "settings.basic.language.english"},
	}
}

func (a *App) processLanguageMenuClicks(gtx layout.Context) {
	if a.agentUI.languageSelector.Clicked(gtx) {
		a.agentUI.languageMenuOpen = !a.agentUI.languageMenuOpen
	}
	if !a.agentUI.languageMenuOpen {
		return
	}
	for index, option := range supportedLanguageOptions() {
		if a.agentUI.languageOptions[index].Clicked(gtx) {
			a.selectLocale(option.Selection)
			a.agentUI.languageMenuOpen = false
			return
		}
	}
}

func (a *App) layoutLanguageSelector(gtx layout.Context) layout.Dimensions {
	state := a.localeSnapshot()
	selection := state.Selection
	selectedLabel := a.languageOptionLabel(selection)
	width := gtx.Dp(unit.Dp(220))
	if available := gtx.Constraints.Max.X; width > available {
		width = available
	}
	height := gtx.Dp(unit.Dp(40))
	gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(width, height), image.Pt(width, height)
	return a.agentUI.languageSelector.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return homeRoundedPanel(gtx, homeSurface, homeBorder, unit.Dp(7), func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.Y = 0
						return layout.Inset{Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return homeTextLabel(gtx, a.theme, selectedLabel, 14, homeText, false)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return homeIcon(gtx, "chevron-down", 16, homeMuted)
					}),
				)
			})
		})
	})
}

func (a *App) layoutLanguageOption(gtx layout.Context, index int, option languageMenuOption, width int) layout.Dimensions {
	button := &a.agentUI.languageOptions[index]
	height := gtx.Dp(unit.Dp(38))
	gtx.Constraints.Min, gtx.Constraints.Max = image.Pt(width, height), image.Pt(width, height)
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				background := color.NRGBA{}
				if a.localeSnapshot().Selection == option.Selection {
					background = homeSelectedFill
				} else if button.Hovered() {
					background = homeHoverFill
				}
				shape := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(5)))
				paint.FillShape(gtx.Ops, background, shape.Op(gtx.Ops))
				return layout.Dimensions{Size: gtx.Constraints.Max}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.Y = 0
							return layout.Inset{Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return homeTextLabel(gtx, a.theme, a.text(i18n.Message{ID: option.MessageID}), 14, homeText, false)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if a.localeSnapshot().Selection != option.Selection {
								return layout.Dimensions{}
							}
							return homeIcon(gtx, "check", 16, settingsBlack)
						}),
					)
				})
			},
		)
	})
}

func (a *App) languageOptionLabel(selection i18n.Selection) string {
	for _, option := range supportedLanguageOptions() {
		if option.Selection == selection {
			return a.text(i18n.Message{ID: option.MessageID})
		}
	}
	return a.text(i18n.Message{ID: "settings.basic.language.english"})
}
