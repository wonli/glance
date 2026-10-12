package desktopui

import (
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"image"
	"image/color"
)

func (a *App) layoutJSONToolbar(gtx layout.Context) layout.Dimensions {
	if a.documentSessionState == nil || a.jsonDoc == nil {
		return layout.Dimensions{}
	}
	a.ensureJSONView()
	gtx = jsonFixedHeight(gtx, jsonToolbarHeightDP)
	width := gtx.Constraints.Max.X
	return jsonPanel(gtx, contentSurface, false, true, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: 8, Right: 8, Top: 10, Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(a.layoutAgentRibbonEntry),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					a.jsonToolbarActionList.List.Axis = layout.Horizontal
					return a.jsonToolbarActionList.Layout(gtx, len(jsonToolbarActions), func(gtx layout.Context, index int) layout.Dimensions {
						action := jsonToolbarActions[index]
						enabled := a.jsonToolbarActionEnabled(action)
						icon, ok := svgIconTint("json", action.icon, toolbarIconColor(enabled))
						if !ok {
							icon, _ = tintedToolbarIcon(action.icon, toolbarIconColor(enabled))
						}
						return layoutToolbarActionItem(gtx, action, index, jsonToolbarActions, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(excelToolbarButtonWidth), gtx.Dp(excelToolbarButtonHeight)))
							return toolbarIconButton(gtx, a.theme, jsonToolbarButton(a.documentSessionState, a, action.id), icon, toolbarIconSize, action.label, action.label, enabled, false, contentSurface)
						})
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if width < gtx.Dp(1180) {
						return layout.Dimensions{}
					}
					gap := unit.Dp(float32(width)/gtx.Metric.PxPerDp - 1316)
					if gap < 16 {
						gap = 16
					}
					if gap > 124 {
						gap = 124
					}
					return layout.Inset{Right: gap}.Layout(gtx, a.layoutJSONLevelControl)
				}),
				layout.Rigid(a.layoutJSONModeSwitch),
			)
		})
	})
}

func (a *App) layoutJSONLevelControl(gtx layout.Context) layout.Dimensions {
	v := a.ensureJSONView()
	gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(202), gtx.Dp(34)))
	return homeRoundedPanel(gtx, jsonSidebarColor, jsonBorderColor, 6, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: 10, Right: 10, Top: 5, Bottom: 5}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return jsonLabel(gtx, a, "展开层级", 11, mutedText, false, false)
					})
				})
			})}
			for i, item := range []struct {
				id, label string
				level     int
			}{{"level-1", "1", 1}, {"level-2", "2", 2}, {"level-3", "3", 3}, {"level-all", "全部", -1}} {
				index, it := i, item
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					right := unit.Dp(5)
					if index == 3 {
						right = 0
					}
					return layout.Inset{Right: right}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						width := unit.Dp(22)
						if it.level == -1 {
							width = 38
						}
						return a.jsonTextButton(gtx, it.id, it.label, width, 24, v.expandedLevel == it.level)
					})
				}))
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
		})
	})
}

func (a *App) layoutJSONModeSwitch(gtx layout.Context) layout.Dimensions {
	v := a.ensureJSONView()
	gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(202), gtx.Dp(34)))
	paint.FillShape(gtx.Ops, chromeSurface, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(6)).Op(gtx.Ops))
	return layout.UniformInset(3).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, 3)
		for i, item := range []struct{ id, label string }{{"mode-tree", "树形"}, {"mode-source", "源码"}, {"mode-compare", "对比"}} {
			index, it := i, item
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				right := unit.Dp(2)
				if index == 2 {
					right = 0
				}
				return layout.Inset{Right: right}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return a.jsonTextButton(gtx, it.id, it.label, 64, 28, int(v.mode) == index)
				})
			}))
		}
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
	})
}

func (a *App) layoutJSONSaveIndicator(gtx layout.Context) layout.Dimensions {
	if (a.jsonDoc == nil && a.markdownDoc == nil) || a.activeTabID == "recent" {
		return layout.Dimensions{}
	}
	gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(110), gtx.Dp(37)))
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		label := "已保存"
		fg := color.NRGBA{R: 95, G: 138, B: 105, A: 255}
		if a.isDirty() || a.path == "" {
			label = "未保存"
			fg = homeMuted
		}
		if a.isSaving() {
			label = "保存中…"
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return jsonIcon(gtx, "cloud-check", 14, fg) }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: 6, Right: 18}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return jsonLabel(gtx, a, label, 11, mutedText, false, false)
				})
			}),
		)
	})
}
