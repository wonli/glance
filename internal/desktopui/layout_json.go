package desktopui

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func (a *App) layoutJSON(gtx layout.Context) layout.Dimensions {
	if a.actionSave.Clicked(gtx) && !a.isSaving() && a.isDirty() {
		if a.path == "" {
			a.startSaveAsPicker("json")
		} else {
			a.startSave()
		}
	}
	if a.actionSaveAs.Clicked(gtx) && !a.isSaving() {
		a.startSaveAsPicker("json")
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(a.layoutJSONSaveToolbar),
		layout.Flexed(1, a.layoutJSONEditor),
	)
}

func (a *App) layoutJSONSaveToolbar(gtx layout.Context) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutJSONSaveButton(gtx, a.theme, &a.actionSave, "保存", !a.isSaving() && a.isDirty(), true)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gap := gtx.Dp(unit.Dp(8))
				return layout.Dimensions{Size: image.Pt(gap, 0)}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutJSONSaveButton(gtx, a.theme, &a.actionSaveAs, "另存为", !a.isSaving(), false)
			}),
		)
	})
}

func layoutJSONSaveButton(gtx layout.Context, theme *material.Theme, click *widget.Clickable, label string, enabled, primary bool) layout.Dimensions {
	height := gtx.Dp(unit.Dp(32))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	semantic.Button.Add(gtx.Ops)
	semantic.DescriptionOp(label).Add(gtx.Ops)
	semantic.EnabledOp(enabled).Add(gtx.Ops)
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				bounds := image.Rectangle{Max: gtx.Constraints.Max}
				radius := gtx.Dp(unit.Dp(6))
				shape := clip.UniformRRect(bounds, radius)
				fill := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
				border := homeBorder
				if primary && enabled {
					fill = color.NRGBA{R: 52, G: 120, B: 209, A: 255}
					border = fill
				} else if !enabled {
					fill = color.NRGBA{R: 245, G: 246, B: 248, A: 255}
					border = color.NRGBA{R: 231, G: 234, B: 237, A: 255}
				} else if click.Hovered() {
					fill = homeHoverFill
				}
				paint.FillShape(gtx.Ops, fill, shape.Op(gtx.Ops))
				if !primary || !enabled {
					paint.FillShape(gtx.Ops, border, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
				}
				return layout.Dimensions{Size: bounds.Max}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					fg := homeText
					if primary && enabled {
						fg = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
					} else if !enabled {
						fg = color.NRGBA{R: 155, G: 162, B: 170, A: 255}
					}
					return homeLabelStyle(theme, label, 12, fg, font.SemiBold).Layout(gtx)
				})
			},
		)
	})
}

func (a *App) layoutJSONEditor(gtx layout.Context) layout.Dimensions {
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
