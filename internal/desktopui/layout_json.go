package desktopui

import (
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func (a *App) layoutJSON(gtx layout.Context) layout.Dimensions {
	unlock := lockSessionAdapterRead(a.documentSessionState)
	text := a.jsonDoc.Text()
	unlock()
	if a.jsonEditor.Text() != text {
		a.jsonEditor.SetText(text)
	}
	return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		style := material.Editor(a.theme, &a.jsonEditor, "编辑 JSON 文本")
		dims := style.Layout(gtx)
		updated := a.jsonEditor.Text()
		if updated != text {
			token, ok := a.documentSessionState.beginOperation(sessionOperationMutation)
			if !ok {
				a.status = "JSON 文档正在处理其他操作"
				return dims
			}
			unlock := lockSessionAdapterWrite(a.documentSessionState)
			err := a.jsonDoc.SetText(updated)
			unlock()
			a.completeSessionOperation(a.documentSessionState, token, err, err == nil)
			if err != nil {
				a.status = err.Error()
			} else {
				a.persistActiveSession()
			}
		}
		return dims
	})
}
