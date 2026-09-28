package desktopui

import (
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func (a *App) layoutJSON(gtx layout.Context) layout.Dimensions {
	text := a.jsonDoc.Text()
	if a.jsonEditor.Text() != text {
		a.jsonEditor.SetText(text)
	}
	return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		style := material.Editor(a.theme, &a.jsonEditor, "编辑 JSON 文本")
		dims := style.Layout(gtx)
		updated := a.jsonEditor.Text()
		if updated != text {
			if err := a.jsonDoc.SetText(updated); err != nil {
				a.status = err.Error()
			} else {
				a.persistActiveSession()
			}
		}
		return dims
	})
}
