package desktopui

import (
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"image"
	"image/color"
	"strings"
)

func (a *App) jsonActionButton(gtx layout.Context, id, label, icon string, height unit.Dp, primary bool) layout.Dimensions {
	gtx = jsonFixedHeight(gtx, height)
	button := jsonToolbarButton(a.documentSessionState, a, id)
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		jsonButtonSemantics(gtx, label, true)
		bg, fg := contentSurface, mutedText
		if primary {
			bg, fg = jsonBlue, contentSurface
		}
		return homeRoundedPanel(gtx, bg, jsonBorderColor, 5, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return jsonIcon(gtx, icon, 13, fg) }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: 5}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return jsonLabel(gtx, a, label, 10, fg, primary, false) })
					}),
				)
			})
		})
	})
}

func (a *App) layoutJSONInspector(gtx layout.Context) layout.Dimensions {
	v := a.ensureJSONView()
	return jsonPanel(gtx, jsonInspectorColor, true, false, func(gtx layout.Context) layout.Dimensions {
		if v.tree == nil {
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}
		n := v.tree.Nodes[v.selected]
		return layout.UniformInset(20).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return v.inspectorList.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Bottom: 18}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return jsonLabel(gtx, a, a.textID("json.inspector.current_node"), 13, homeText, true, false)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(62), gtx.Dp(24)))
									return homeRoundedPanel(gtx, color.NRGBA{R: 255, G: 240, B: 231, A: 255}, color.NRGBA{}, 12, func(gtx layout.Context) layout.Dimensions {
										return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return jsonLabel(gtx, a, strings.ToUpper(n.Kind), 9, jsonNodeColor(n), true, false)
										})
									})
								}),
							)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Bottom: 18}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return jsonLabel(gtx, a, a.textID("json.inspector.key"), 10, homeMuted, false, false)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Top: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return jsonLabel(gtx, a, n.Key, 18, homeText, true, true) })
								}),
							)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutJSONPathField(gtx, "点路径", v.tree.NodeDotPath(v.selected), "copy-dot")
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutJSONPathField(gtx, "JSONPath", v.tree.NodeJSONPath(v.selected), "copy-path")
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return a.layoutJSONPathField(gtx, "JSON Pointer", v.tree.NodePointer(v.selected), "copy-pointer")
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Bottom: 18}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return jsonLabel(gtx, a, a.textID("json.inspector.value"), 10, mutedText, true, false)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Top: 7}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										gtx = jsonFixedHeight(gtx, 72)
										return homeRoundedPanel(gtx, contentSurface, jsonBorderColor, 6, func(gtx layout.Context) layout.Dimensions {
											return layout.UniformInset(10).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												style := material.Editor(a.theme, &v.value, "")
												style.TextSize = 11
												style.Font.Typeface = jsonMonoFace
												style.Color = jsonNodeColor(n)
												return style.Layout(gtx)
											})
										})
									})
								}),
							)
						})
					}),

					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if v.valueError == "" {
							return layout.Dimensions{}
						}
						return layout.Inset{Bottom: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							style := material.Caption(a.theme, v.valueError)
							style.TextSize = 11
							style.Color = color.NRGBA{R: 188, G: 63, B: 60, A: 255}
							return style.Layout(gtx)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if v.value.ReadOnly {
							return layout.Dimensions{}
						}
						original := n.Raw
						if n.Kind == "string" {
							original = n.Display
						}
						if original == v.value.Text() {
							return layout.Dimensions{}
						}
						return layout.Inset{Bottom: 18}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return a.jsonActionButton(gtx, "apply-value", a.textID("json.action.apply_value"), "circle-check", 34, true)
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return a.jsonActionButton(gtx, "cancel-value", a.textID("json.action.cancel_value"), "fold-vertical", 34, false)
									})
								}),
							)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return a.jsonActionButton(gtx, "copy-value", a.textID("json.action.copy_value"), "copy", 34, true)
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return a.jsonActionButton(gtx, "copy-key", a.textID("json.action.copy_key"), "key-round", 34, false)
								})
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return a.jsonActionButton(gtx, "copy-object", a.textID("json.action.copy_object"), "braces", 34, false)
								})
							}),
						)
					}),
				)
			})
		})
	})
}

func (a *App) layoutJSONPathField(gtx layout.Context, label, value, id string) layout.Dimensions {
	return layout.Inset{Bottom: 18}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return jsonLabel(gtx, a, label, 10, mutedText, true, false)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: 7}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx = jsonFixedHeight(gtx, 42)
					return homeRoundedPanel(gtx, contentSurface, jsonBorderColor, 6, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: 10, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.Y = 0
									return jsonLabel(gtx, a, value, 11, homeText, false, true)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.jsonCopyButton(gtx, id, "copy") }),
							)
						})
					})
				})
			}),
		)
	})
}
