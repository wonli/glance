package desktopui

import (
	"fmt"
	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"image"
	"image/color"
	"strings"
)

var (
	jsonSidebarColor   = color.NRGBA{R: 245, G: 247, B: 249, A: 255}
	jsonInspectorColor = color.NRGBA{R: 247, G: 248, B: 250, A: 255}
	jsonBorderColor    = color.NRGBA{R: 221, G: 226, B: 231, A: 255}
	jsonBlue           = color.NRGBA{R: 43, G: 108, B: 176, A: 255}
	jsonKeyColor       = color.NRGBA{R: 107, G: 63, B: 145, A: 255}
	jsonStringColor    = color.NRGBA{R: 177, G: 93, B: 40, A: 255}
	jsonMonoFace       = font.Typeface("JetBrains Mono, Menlo, monospace, " + string(preferredSystemTypeface))
)

func jsonLabel(gtx layout.Context, a *App, text string, size unit.Sp, fg color.NRGBA, bold, mono bool) layout.Dimensions {
	// Labels report their natural height so row alignment can center their ink.
	gtx.Constraints.Min.Y = 0
	text = jsonTextPreview(text, 4096)
	style := homeLabelStyle(a.theme, text, size, fg, font.Normal)
	if bold {
		style.Font.Weight = font.SemiBold
	}
	if mono {
		style.Font.Typeface = jsonMonoFace
	}
	return style.Layout(gtx)
}

func jsonFixedHeight(gtx layout.Context, h unit.Dp) layout.Context {
	height := gtx.Dp(h)
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return gtx
}

func jsonIcon(gtx layout.Context, name string, size unit.Dp, fg color.NRGBA) layout.Dimensions {
	pixels := gtx.Dp(size)
	gtx.Constraints = layout.Exact(image.Pt(pixels, pixels))
	img, ok := svgIconTint("json", name, fg)
	if !ok {
		img, ok = tintedToolbarIcon(name, fg)
	}
	if !ok {
		return layout.Dimensions{Size: image.Pt(pixels, pixels)}
	}
	return widget.Image{Src: paint.NewImageOp(img), Fit: widget.Contain, Position: layout.Center}.Layout(gtx)
}

func jsonPanel(gtx layout.Context, fillColor color.NRGBA, leftBorder, bottomBorder bool, child layout.Widget) layout.Dimensions {
	return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, fillColor, clip.Rect{Max: gtx.Constraints.Max}.Op())
		if leftBorder {
			paint.FillShape(gtx.Ops, jsonBorderColor, clip.Rect{Max: image.Pt(gtx.Dp(1), gtx.Constraints.Max.Y)}.Op())
		}
		if bottomBorder {
			paint.FillShape(gtx.Ops, jsonBorderColor, clip.Rect{Min: image.Pt(0, gtx.Constraints.Max.Y-gtx.Dp(1)), Max: gtx.Constraints.Max}.Op())
		}
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}, child)
}

func (a *App) jsonTextButton(gtx layout.Context, id, label string, width, height unit.Dp, selected bool) layout.Dimensions {
	if width > 0 {
		w := gtx.Dp(width)
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
	}
	gtx = jsonFixedHeight(gtx, height)
	button := jsonToolbarButton(a.documentSessionState, a, id)
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		jsonButtonSemantics(gtx, label, true)
		bg := color.NRGBA{}
		if selected {
			bg = contentSurface
		} else if button.Hovered() {
			bg = homeHoverFill
		}
		paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(4)).Op(gtx.Ops))
		fg := mutedText
		if selected {
			fg = jsonBlue
		}
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return jsonLabel(gtx, a, label, 11, fg, selected, false) })
	})
}

func (a *App) layoutJSONWorkspace(gtx layout.Context) layout.Dimensions {
	v := a.ensureJSONView()
	return layout.Stack{Alignment: layout.NE}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					width := gtx.Dp(248)
					if gtx.Constraints.Max.X < gtx.Dp(1100) {
						width = gtx.Dp(200)
					}
					if gtx.Constraints.Max.X < gtx.Dp(680) {
						return layout.Dimensions{}
					}
					gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
					return a.layoutJSONNavigator(gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(a.layoutJSONBreadcrumb),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							if v.mode == jsonModeSource {
								return a.layoutJSONEditor(gtx)
							}
							if v.mode == jsonModeCompare {
								return a.layoutJSONComparison(gtx)
							}
							return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
								layout.Flexed(1, a.layoutJSONTree),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if gtx.Constraints.Max.X < gtx.Dp(620) {
										return layout.Dimensions{}
									}
									width := gtx.Dp(372)
									if gtx.Constraints.Max.X < gtx.Dp(1000) {
										width = gtx.Dp(300)
									}
									gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
									return a.layoutJSONInspector(gtx)
								}),
							)
						}),
					)
				}),
			)
		}),
	)
}

func (a *App) layoutJSONNavigator(gtx layout.Context) layout.Dimensions {
	v := a.ensureJSONView()
	return jsonPanel(gtx, jsonSidebarColor, false, false, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx = jsonFixedHeight(gtx, 49)
				return jsonPanel(gtx, jsonSidebarColor, false, true, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: 18}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return jsonLabel(gtx, a, "JSON 结构", 12, homeText, true, false)
						})
					})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: 12, Right: 12, Top: 11, Bottom: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx = jsonFixedHeight(gtx, 32)
					return homeRoundedPanel(gtx, contentSurface, jsonBorderColor, 5, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: 10, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions { return jsonIcon(gtx, "search", 14, homeMuted) }),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.Y = 0
									style := material.Editor(a.theme, &v.search, "搜索键或值")
									style.TextSize = 11
									style.HintColor = homeMuted
									return layout.Inset{Left: 8}.Layout(gtx, style.Layout)
								}),
							)
						})
					})
				})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return v.outlineList.Layout(gtx, len(v.outline), func(gtx layout.Context, index int) layout.Dimensions {
					id := v.outline[index]
					n := v.tree.Nodes[id]
					return layout.Inset{Left: 10, Right: 10, Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx = jsonFixedHeight(gtx, 34)
						button := jsonViewButton(v.outlineButtons, id)
						return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							fg := homeText
							if v.selected == id {
								fg = jsonBlue
								paint.FillShape(gtx.Ops, color.NRGBA{R: 229, G: 237, B: 247, A: 255}, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(4)).Op(gtx.Ops))
								paint.FillShape(gtx.Ops, jsonBlue, clip.Rect{Max: image.Pt(gtx.Dp(3), gtx.Constraints.Max.Y)}.Op())
							}
							count := fmt.Sprintf("{%d}", v.tree.ChildCount(id))
							if n.Kind == "array" {
								count = fmt.Sprintf("[%d]", v.tree.ChildCount(id))
							}
							inset := unit.Dp(12 + min(n.Depth, 6)*16)
							return layout.Inset{Left: inset, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return jsonLabel(gtx, a, jsonTextPreview(n.Key, 1024)+"  "+count, 12, fg, v.selected == id, false)
								})
							})
						})
					})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: 12, Right: 12, Top: 12, Bottom: 24}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return a.jsonActionButton(gtx, "paste", "粘贴或导入 JSON", "clipboard-paste", 42, false)
				})
			}),
		)
	})
}

func (a *App) layoutJSONBreadcrumb(gtx layout.Context) layout.Dimensions {
	v := a.ensureJSONView()
	gtx = jsonFixedHeight(gtx, 50)
	return jsonPanel(gtx, contentSurface, true, true, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: 18, Right: 18}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					text := "root"
					if v.tree != nil && v.selected < len(v.tree.Nodes) {
						target := v.selected
						n := v.tree.Nodes[target]
						if !n.Container() && n.Parent >= 0 {
							target = n.Parent
						}
						text = strings.ReplaceAll(jsonTextPreview(v.tree.NodeDotPath(target), 1024), ".", "  ›  ")
					}
					gtx.Constraints.Min.Y = 0
					return jsonLabel(gtx, a, text, 12, mutedText, false, true)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if v.pending {
						return jsonLabel(gtx, a, "正在解析…", 11, homeMuted, false, false)
					}
					if v.err != nil {
						return jsonLabel(gtx, a, "JSON 无效", 11, color.NRGBA{R: 188, G: 63, B: 60, A: 255}, true, false)
					}
					return layout.UniformInset(0).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(98), gtx.Dp(26)))
						return homeRoundedPanel(gtx, color.NRGBA{R: 237, G: 247, B: 240, A: 255}, color.NRGBA{}, 13, func(gtx layout.Context) layout.Dimensions {
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return jsonLabel(gtx, a, "✓ 有效 JSON", 11, color.NRGBA{R: 71, G: 122, B: 83, A: 255}, true, false)
							})
						})
					})
				}),
			)
		})
	})
}

func (a *App) layoutJSONComparison(gtx layout.Context) layout.Dimensions {
	v := a.ensureJSONView()
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return jsonPanel(gtx, jsonSidebarColor, true, false, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(16).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return jsonLabel(gtx, a, "打开时的源码", 11, mutedText, true, false)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							if len(v.baseline) > jsonSourceEditLimit {
								return a.layoutJSONSourcePreview(gtx, v.baseline, v.baselineSegments, &v.baselineList)
							}
							return layout.Inset{Top: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								style := material.Editor(a.theme, &v.original, "")
								style.Font.Typeface = jsonMonoFace
								style.TextSize = 13
								return style.Layout(gtx)
							})
						}),
					)
				})
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return jsonPanel(gtx, contentSurface, true, false, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: 20, Top: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return jsonLabel(gtx, a, "当前源码", 11, mutedText, true, false)
						})
					}),
					layout.Flexed(1, a.layoutJSONEditor),
				)
			})
		}),
	)
}

func (a *App) layoutJSONStatus(gtx layout.Context) layout.Dimensions {
	v := a.ensureJSONView()
	gtx = jsonFixedHeight(gtx, 34)
	return jsonPanel(gtx, color.NRGBA{R: 248, G: 249, B: 250, A: 255}, false, false, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, jsonBorderColor, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(1))}.Op())
		return layout.Inset{Left: 20, Right: 28}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					text := fmt.Sprintf("%s", formatHomeFileSize(int64(len(a.jsonDoc.Text()))))
					if v.tree != nil && v.tree.Source == a.jsonDoc.Text() {
						text = fmt.Sprintf("%d 个键    %d 层 · %s", v.tree.KeyCount, v.tree.MaxDepth+1, formatHomeFileSize(int64(len(v.tree.Source))))
					}
					if v.toolPending {
						text = "正在处理 JSON…"
					} else if v.toolMessage != "" {
						text = v.toolMessage
					}
					return jsonLabel(gtx, a, text, 11, mutedText, false, false)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					text, fg := "有效 JSON", color.NRGBA{R: 77, G: 128, B: 90, A: 255}
					if v.pending || v.parsed != a.jsonDoc.Text() {
						text, fg = "正在解析…", homeMuted
					} else if v.err != nil {
						text, fg = "JSON 无效", color.NRGBA{R: 188, G: 63, B: 60, A: 255}
					}
					return jsonLabel(gtx, a, text, 11, fg, false, false)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: 32}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return jsonLabel(gtx, a, "UTF-8", 11, mutedText, false, false)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: 32}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return jsonLabel(gtx, a, "JSON", 11, jsonBlue, true, false)
					})
				}),
			)
		})
	})
}

func jsonButtonSemantics(gtx layout.Context, label string, enabled bool) {
	bounds := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	semantic.Button.Add(gtx.Ops)
	semantic.DescriptionOp(label).Add(gtx.Ops)
	semantic.EnabledOp(enabled).Add(gtx.Ops)
	bounds.Pop()
}
