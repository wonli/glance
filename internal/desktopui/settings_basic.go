package desktopui

import (
	"image"

	"gioui.org/layout"
	"gioui.org/unit"

	"glance/internal/i18n"
)

func (a *App) layoutBasicSettingsPage(gtx layout.Context) layout.Dimensions {
	a.processLanguageMenuClicks(gtx)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return homeTextLabel(gtx, a.theme, a.text(i18n.Message{ID: "settings.basic.title"}), 20, settingsBlack, true)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				style := settingsSecondaryLabel(a.theme, a.tokens().Typography, a.text(i18n.Message{ID: "settings.basic.subtitle"}))
				style.Color, style.MaxLines = settingsBlack, 2
				return style.Layout(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(36)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return homeTextLabel(gtx, a.theme, a.text(i18n.Message{ID: "settings.basic.language.title"}), 16, settingsBlack, true)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				style := settingsSecondaryLabel(a.theme, a.tokens().Typography, a.text(i18n.Message{ID: "settings.basic.language.description"}))
				style.Color, style.MaxLines = homeMuted, 2
				return style.Layout(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(14)}.Layout(gtx, a.layoutLanguageSettingCard)
		}),
	)
}

func (a *App) layoutLanguageSettingCard(gtx layout.Context) layout.Dimensions {
	rowHeight := gtx.Dp(unit.Dp(82))
	gtx.Constraints.Min.Y = 0
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = rowHeight, rowHeight
			return homeRoundedPanel(gtx, homeSurface, homeBorder, unit.Dp(8), func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(18), Right: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							// Keep the text group at its natural height so the row
							// can center it alongside the selector.
							gtx.Constraints.Min.Y = 0
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return homeTextLabel(gtx, a.theme, a.text(i18n.Message{ID: "settings.basic.language.row_label"}), 14, homeText, true)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										style := settingsSecondaryLabel(a.theme, a.tokens().Typography, a.text(i18n.Message{ID: "settings.basic.language.row_help"}))
										style.Color, style.MaxLines = homeMuted, 2
										return style.Layout(gtx)
									})
								}),
							)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: unit.Dp(18)}.Layout(gtx, a.layoutLanguageSelector)
						}),
					)
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !a.agentUI.languageMenuOpen {
				return layout.Dimensions{}
			}
			width := gtx.Dp(unit.Dp(220))
			available := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(54))
			if available < 0 {
				available = 0
			}
			if width > available {
				width = available
			}
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, 0)}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(5), Left: unit.Dp(18), Right: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return a.layoutLanguageOption(gtx, 0, supportedLanguageOptions()[0], width)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return a.layoutLanguageOption(gtx, 1, supportedLanguageOptions()[1], width)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return a.layoutLanguageOption(gtx, 2, supportedLanguageOptions()[2], width)
							}),
						)
					})
				}),
			)
		}),
	)
}
