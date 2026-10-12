package desktopui

import (
	"fmt"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget/material"
	"glance/internal/document"
	"image"
	"image/color"
	"math"
)

var pdfToolbarActions = []ribbonAction{
	{id: "open", label: "打开", group: "file", icon: "folder-open"},
	{id: "save", label: "保存", group: "file", icon: "save"},
	{id: "save-as", label: "另存为", group: "file", icon: "file-down"},

	{id: "thumbnails", label: "缩略图", group: "view", icon: "panel-left"},
	{id: "search", label: "搜索", group: "view", icon: "search"},

	{id: "previous", label: "上一页", group: "page", icon: "chevron-left"},
	{id: "page-indicator", label: "页码", group: "page", icon: ""},
	{id: "next", label: "下一页", group: "page", icon: "chevron-right"},

	{id: "zoom-out", label: "缩小", group: "zoom", icon: "minus"},
	{id: "zoom-indicator", label: "缩放", group: "zoom", icon: ""},
	{id: "zoom-in", label: "放大", group: "zoom", icon: "plus"},
	{id: "fit-width", label: "适合宽度", group: "zoom", icon: "view-multiple-pages"},
	{id: "fit-page", label: "适合页面", group: "zoom", icon: "view-single-page"},
	{id: "rotate", label: "旋转", group: "zoom", icon: "rotate-cw"},

	{id: "highlight", label: "高亮", group: "annotate", icon: "text-highlight"},
	{id: "note", label: "添加批注", group: "annotate", icon: "message-square-plus"},
	{id: "ink", label: "自由绘制", group: "annotate", icon: "pencil"},

	{id: "undo", label: "撤销", group: "history", icon: "undo"},
	{id: "redo", label: "重做", group: "history", icon: "redo"},
}

var pdfIconAliases = map[string]string{
	"open":       "folder-open",
	"save-as":    "file-down",
	"thumbnails": "panel-left",
	"previous":   "chevron-left",
	"next":       "chevron-right",
	"zoom-out":   "minus",
	"zoom-in":    "plus",
	"fit-width":  "view-multiple-pages",
	"fit-page":   "view-single-page",
	"rotate":     "rotate-cw",
	"highlight":  "text-highlight",
	"note":       "message-square-plus",
	"ink":        "pencil",
	"pencil":     "pencil",
	"download":   "file-down",
	"print":      "printer",
}

func pdfToolbarIcon(name string, tint color.NRGBA) (image.Image, bool) {
	if alias, ok := pdfIconAliases[name]; ok {
		name = alias
	}
	if img, ok := tintedToolbarIcon(name, tint); ok {
		return img, true
	}
	if img, ok := svgIconTintAtSize("word", name, 96, tint); ok {
		return img, true
	}
	if img, ok := svgIconTintAtSize("excel", name, 96, tint); ok {
		return img, true
	}
	return nil, false
}

func (a *App) layoutPDFDocument(gtx layout.Context) layout.Dimensions {
	a.handlePDFEvents(gtx)
	v := a.ensurePDFView()
	if r, ok := v.cache[v.page]; ok {
		v.scene = r.page
	} else {
		v.scene = document.PDFPage{}
	}
	a.updatePDFScale(gtx)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(a.layoutPDFRibbon),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return a.layoutAgentSidebarWorkspace(gtx, a.layoutPDFWorkspace)
		}),
		layout.Rigid(a.layoutPDFStatus),
	)
}

func (a *App) updatePDFScale(gtx layout.Context) {
	v := a.ensurePDFView()
	refW, refH := 595.0, 842.0
	if r, ok := v.cache[v.page]; ok && r.page.Width > 0 {
		refW = r.page.Width
		refH = r.page.Height
	} else {
		for _, cached := range v.cache {
			if cached.page.Width > 0 {
				refW = cached.page.Width
				refH = cached.page.Height
				break
			}
		}
	}

	wide := gtx.Constraints.Max.X >= gtx.Dp(1000)
	thumbW := 0
	if v.thumbnails {
		thumbW = gtx.Dp(140)
		if wide {
			thumbW = gtx.Dp(220)
		}
	}
	notesW := 0
	if wide || v.mode != "read" || v.showSearch {
		notesW = gtx.Dp(220)
		if wide {
			notesW = gtx.Dp(300)
		}
	}
	sidebarW := 0
	if a.agentUI.sidebarVisible {
		sidebarW = a.agentSidebarWidth(gtx)
	}
	canvasW := max(1, gtx.Constraints.Max.X-sidebarW-thumbW-notesW)
	canvasH := max(1, gtx.Constraints.Max.Y-gtx.Dp(54)-gtx.Dp(34))

	width, height := canvasW-gtx.Dp(48), canvasH-gtx.Dp(48)
	baseScale := float64(gtx.Metric.PxPerDp) * 4 / 3
	scale := baseScale * v.zoom
	if v.fit != "" {
		scale = float64(max(1, width)) / refW
		if v.fit == "page" {
			scale = math.Min(scale, float64(max(1, height))/refH)
		}
		if baseScale > 0 {
			v.zoom = scale / baseScale
		}
	}
	if v.scale != scale {
		gtx.Execute(op.InvalidateCmd{})
	}
	v.scale = scale
}

func (a *App) layoutPDFRibbon(gtx layout.Context) layout.Dimensions {
	v := a.ensurePDFView()
	gtx = jsonFixedHeight(gtx, 54)
	return jsonPanel(gtx, contentSurface, false, true, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(a.layoutAgentRibbonEntry),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return v.toolbar.Layout(gtx, len(pdfToolbarActions), func(gtx layout.Context, i int) layout.Dimensions {
					action := pdfToolbarActions[i]
					if action.id == "page-indicator" {
						return layoutToolbarActionItem(gtx, action, i, pdfToolbarActions, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(82), gtx.Dp(34)))
							paint.FillShape(gtx.Ops, color.NRGBA{R: 221, G: 226, B: 231, A: 255}, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(5)).Op(gtx.Ops))
							paint.FillShape(gtx.Ops, color.NRGBA{R: 247, G: 248, B: 250, A: 255}, clip.UniformRRect(image.Rect(1, 1, gtx.Constraints.Max.X-1, gtx.Constraints.Max.Y-1), gtx.Dp(4)).Op(gtx.Ops))
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										l := material.Label(a.theme, 12, fmt.Sprintf("%d", v.page+1))
										l.Color = color.NRGBA{R: 52, G: 64, B: 75, A: 255}
										l.Font.Weight = font.SemiBold
										return l.Layout(gtx)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return layout.Inset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											l := material.Label(a.theme, 12, fmt.Sprintf("/ %d", max(1, v.info.Pages)))
											l.Color = color.NRGBA{R: 122, G: 131, B: 142, A: 255}
											return l.Layout(gtx)
										})
									}),
								)
							})
						})
					}
					if action.id == "zoom-indicator" {
						return layoutToolbarActionItem(gtx, action, i, pdfToolbarActions, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(64), gtx.Dp(34)))
							paint.FillShape(gtx.Ops, color.NRGBA{R: 221, G: 226, B: 231, A: 255}, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(5)).Op(gtx.Ops))
							btn := v.button("zoom-indicator")
							bg := color.NRGBA{R: 247, G: 248, B: 250, A: 255}
							if btn.Hovered() {
								bg = color.NRGBA{R: 238, G: 242, B: 246, A: 255}
							}
							paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rect(1, 1, gtx.Constraints.Max.X-1, gtx.Constraints.Max.Y-1), gtx.Dp(4)).Op(gtx.Ops))
							return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									l := material.Label(a.theme, 11, fmt.Sprintf("%.0f%%", v.currentZoom(gtx.Metric)*100))
									l.Color = color.NRGBA{R: 78, G: 88, B: 99, A: 255}
									l.Font.Weight = font.SemiBold
									return l.Layout(gtx)
								})
							})
						})
					}
					enabled := !a.isSaving()
					if action.id == "save" {
						enabled = enabled && a.isDirty()
					}
					if action.id == "undo" {
						enabled = enabled && v.info.CanUndo
					}
					if action.id == "redo" {
						enabled = enabled && v.info.CanRedo
					}
					active := false
					if action.id == "thumbnails" && v.thumbnails {
						active = true
					}
					if action.id == "fit-width" && v.fit == "width" {
						active = true
					}
					if action.id == "fit-page" && v.fit == "page" {
						active = true
					}
					if action.id == "highlight" && v.tool == "highlight" {
						active = true
					}
					if action.id == "ink" && v.tool == "ink" {
						active = true
					}
					if action.id == "note" && v.tool == "note" {
						active = true
					}

					iconCol := toolbarIconColor(enabled)
					background := contentSurface
					if active {
						iconCol = color.NRGBA{R: 43, G: 108, B: 176, A: 255}
						background = toolbarActiveBackground
					}
					icon, ok := pdfToolbarIcon(action.icon, iconCol)
					if !ok {
						icon, _ = markdownToolbarIcon(action, enabled)
					}
					return layoutToolbarActionItem(gtx, action, i, pdfToolbarActions, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(excelToolbarButtonWidth), gtx.Dp(excelToolbarButtonHeight)))
						return toolbarIconButton(gtx, a.theme, v.button(action.id), icon, toolbarIconSize, action.label, action.label, enabled, false, background)
					})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: 8, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(a.layoutPDFModeSwitch),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								enabled := !a.isSaving()
								icon, _ := pdfToolbarIcon("printer", toolbarIconColor(enabled))
								gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(excelToolbarButtonWidth), gtx.Dp(excelToolbarButtonHeight)))
								return toolbarIconButton(gtx, a.theme, v.button("print"), icon, toolbarIconSize, "打印", "打印", enabled, false, contentSurface)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								enabled := !a.isSaving()
								icon, _ := pdfToolbarIcon("file-down", toolbarIconColor(enabled))
								gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(excelToolbarButtonWidth), gtx.Dp(excelToolbarButtonHeight)))
								return toolbarIconButton(gtx, a.theme, v.button("download"), icon, toolbarIconSize, "下载", "下载", enabled, false, contentSurface)
							})
						}),
					)
				})
			}),
		)
	})
}

func (a *App) layoutPDFModeSwitch(gtx layout.Context) layout.Dimensions {
	v := a.ensurePDFView()
	gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(202), gtx.Dp(34)))
	paint.FillShape(gtx.Ops, color.NRGBA{R: 241, G: 243, B: 245, A: 255}, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(6)).Op(gtx.Ops))
	return layout.UniformInset(3).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		var children []layout.FlexChild
		modes := []struct{ id, label string }{
			{"read", "阅读"},
			{"annotate", "批注"},
			{"signature", "签名"},
		}
		for _, item := range modes {
			item := item
			active := v.mode == item.id
			children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, gtx.Dp(28)))
				return v.button(item.id).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					if active {
						shadowRect := image.Rect(0, gtx.Dp(1), gtx.Constraints.Max.X, gtx.Constraints.Max.Y+gtx.Dp(1))
						paint.FillShape(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 20}, clip.UniformRRect(shadowRect, gtx.Dp(4)).Op(gtx.Ops))
						paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(4)).Op(gtx.Ops))
					}
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						l := material.Label(a.theme, 11, item.label)
						if active {
							l.Color = color.NRGBA{R: 43, G: 108, B: 176, A: 255}
							l.Font.Weight = font.SemiBold
						} else {
							l.Color = color.NRGBA{R: 104, G: 113, B: 123, A: 255}
						}
						return l.Layout(gtx)
					})
				})
			}))
		}
		return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx, children...)
	})
}
