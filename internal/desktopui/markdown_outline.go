package desktopui

import (
	"fmt"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"image"
)

func (a *App) layoutMarkdownOutline(gtx layout.Context) layout.Dimensions {
	v := a.ensureMarkdownView()
	return jsonPanel(gtx, jsonSidebarColor, false, false, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx = jsonFixedHeight(gtx, 48)
				return layout.Inset{Left: 18, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return jsonLabel(gtx, a, "大纲", 13, homeText, true, false)
					}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.markdownTextButton(gtx, "outline", "‹", 28, 32, false)
					}))
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: 12, Bottom: 12, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx = jsonFixedHeight(gtx, 32)
					return homeRoundedPanel(gtx, contentSurface, jsonBorderColor, 4, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: 10, Right: 10, Top: 5, Bottom: 5}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							style := material.Editor(a.theme, &v.search, "搜索标题")
							style.TextSize = 11
							return style.Layout(gtx)
						})
					})
				})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				headings := v.headings()
				return v.outline.Layout(gtx, len(headings), func(gtx layout.Context, index int) layout.Dimensions {
					h := headings[index]
					start, _ := v.editor.Selection()
					active := start >= h.Offset && (index == len(headings)-1 || start < headings[index+1].Offset)
					return layout.Inset{Left: 10, Right: 10, Bottom: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx = jsonFixedHeight(gtx, 34)
						return v.button(fmt.Sprintf("heading-%d", h.Offset)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							if active {
								paint.FillShape(gtx.Ops, homeHoverFill, clip.Rect{Max: gtx.Constraints.Max}.Op())
								paint.FillShape(gtx.Ops, jsonBlue, clip.Rect{Max: image.Pt(gtx.Dp(3), gtx.Constraints.Max.Y)}.Op())
							}
							return layout.Inset{Left: unit.Dp(12 + (h.Level-1)*14), Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return jsonLabel(gtx, a, h.Title, 12, homeText, active, false)
								})
							})
						})
					})
				})
			}),
		)
	})
}
