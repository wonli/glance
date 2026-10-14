package desktopui

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/widget/material"
)

func settingsBodyLabel(gtx layout.Context, theme *material.Theme, typography Typography, text string, fg color.NRGBA, bold bool) layout.Dimensions {
	return homeTextLabel(gtx, theme, text, typography.SettingsBody, fg, bold)
}

func settingsSecondaryLabel(theme *material.Theme, typography Typography, text string) material.LabelStyle {
	style := material.Caption(theme, text)
	style.TextSize = typography.SettingsSecondary
	return style
}
