package desktopui

import (
	"fmt"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"glance/internal/document"
	"image"
	"image/color"
)

func (a *App) layoutJSONTree(gtx layout.Context) layout.Dimensions {
	v := a.ensureJSONView()
	return jsonPanel(gtx, contentSurface, true, false, func(gtx layout.Context) layout.Dimensions {
		if v.tree == nil || len(v.rows) == 0 {
			text := "正在解析 JSON…"
			if v.err != nil {
				text = "JSON 暂时无法解析，请切换源码修正\n" + v.err.Error()
			} else if v.tree != nil {
				text = "没有匹配的键或值"
			}
			return layout.UniformInset(24).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				style := material.Body2(a.theme, text)
				style.TextSize = 13
				style.Color = mutedText
				return style.Layout(gtx)
			})
		}
		return v.treeList.Layout(gtx, len(v.rows), func(gtx layout.Context, index int) layout.Dimensions {
			row := v.rows[index]
			n := v.tree.Nodes[row.node]
			gtx = jsonFixedHeight(gtx, 38)
			button := jsonViewButton(v.rowButtons, row.node)
			if row.closing {
				return a.layoutJSONTreeRow(gtx, index, row, n)
			}
			return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return a.layoutJSONTreeRow(gtx, index, row, n) })
		})
	})
}

func (a *App) layoutJSONTreeRow(gtx layout.Context, index int, row jsonTreeRow, n document.JSONNode) layout.Dimensions {
	v := a.ensureJSONView()
	bg := contentSurface
	if index%2 == 1 {
		bg = color.NRGBA{R: 252, G: 252, B: 253, A: 255}
	}
	if v.selected == row.node && !row.closing {
		bg = color.NRGBA{R: 234, G: 242, B: 251, A: 255}
	}
	return jsonPanel(gtx, bg, false, false, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(14 + min(n.Depth, 12)*22), Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(15), gtx.Dp(24)))
					if !n.Container() || row.closing || len(n.Children) == 0 {
						return layout.Dimensions{Size: gtx.Constraints.Max}
					}
					button := jsonViewButton(v.toggleButtons, row.node)
					return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						icon := "chevron-down"
						if v.collapsed[row.node] && v.search.Text() == "" {
							icon = "chevron-right"
						}
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return jsonIcon(gtx, icon, 15, mutedText) })
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return jsonLabel(gtx, a, fmt.Sprintf("%02d", index+1), 10, color.NRGBA{R: 176, G: 182, B: 189, A: 255}, false, true)
					})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if row.closing {
						mark := "}"
						if n.Kind == "array" {
							mark = "]"
						}
						return jsonLabel(gtx, a, mark, 13, jsonKeyColor, false, true)
					}
					key := fmt.Sprintf("%q", jsonTextPreview(n.Key, 1024)) + " : "
					if n.Parent >= 0 && v.tree.Nodes[n.Parent].Kind == "array" {
						key = jsonTextPreview(n.Key, 1024) + " : "
					}
					value := jsonTextPreview(n.Raw, 4096)
					if n.Container() {
						if n.Kind == "object" {
							value = "{"
						} else {
							value = "["
						}
						if v.collapsed[row.node] && v.search.Text() == "" || len(n.Children) == 0 {
							if n.Kind == "object" {
								value = fmt.Sprintf("{ %d keys }", len(n.Children))
							} else {
								value = fmt.Sprintf("[ %d items ]", len(n.Children))
							}
						}
					}
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return jsonLabel(gtx, a, key, 13, jsonKeyColor, v.selected == row.node, true)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return jsonLabel(gtx, a, value, 13, jsonNodeColor(n), false, true)
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if v.selected != row.node || row.closing || gtx.Constraints.Max.X < gtx.Dp(60) {
						return layout.Dimensions{}
					}
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.jsonCopyButton(gtx, "row-copy-value", "copy") }),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return a.jsonCopyButton(gtx, "row-copy-path", "route") })
						}),
					)
				}),
			)
		})
	})
}

func jsonNodeColor(n document.JSONNode) color.NRGBA {
	switch n.Kind {
	case "string":
		return jsonStringColor
	case "number":
		return jsonBlue
	case "boolean":
		return jsonKeyColor
	case "null":
		return homeMuted
	default:
		return homeText
	}
}

func (a *App) jsonCopyButton(gtx layout.Context, id, icon string) layout.Dimensions {
	gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(26), gtx.Dp(26)))
	button := jsonToolbarButton(a.documentSessionState, a, id)
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return homeRoundedPanel(gtx, contentSurface, jsonBorderColor, 4, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return jsonIcon(gtx, icon, 14, jsonBlue) })
		})
	})
}
