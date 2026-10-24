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
	actions := a.localizedJSONToolbarActions()
	return jsonPanel(gtx, contentSurface, false, true, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: 8, Right: 8, Top: 10, Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(a.layoutAgentRibbonEntry),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					a.jsonToolbarActionList.List.Axis = layout.Horizontal
					return a.jsonToolbarActionList.Layout(gtx, len(actions), func(gtx layout.Context, index int) layout.Dimensions {
						action := actions[index]
						enabled := a.jsonToolbarActionEnabled(action)
						icon, ok := svgIconTint("json", action.icon, toolbarIconColor(enabled))
						if !ok {
							icon, _ = tintedToolbarIcon(action.icon, toolbarIconColor(enabled))
						}
						return layoutToolbarActionItem(gtx, action, index, actions, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(excelToolbarButtonWidth), gtx.Dp(excelToolbarButtonHeight)))
							return toolbarIconButton(gtx, a.theme, jsonToolbarButton(a.documentSessionState, a, action.id), icon, toolbarIconSize, action.label, action.label, enabled, false, contentSurface)
						})
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gap := unit.Dp(float32(width)/gtx.Metric.PxPerDp - 1316)
					if gap < 16 {
						gap = 16
					}
					if gap > 124 {
						gap = 124
					}
					return layout.Inset{Right: gap}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						// Keep room for the mode switch and a scrollable command strip.
						budget := max(gtx.Dp(96), width-gtx.Dp(16+202+160+gap))
						gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, max(0, min(budget, width-gtx.Dp(16+202+80+gap))))
						gtx.Constraints.Min.X = 0
						dims := a.layoutJSONLevelControl(gtx)
						x := width - gtx.Dp(8+202+gap) - dims.Size.X
						y := (gtx.Dp(jsonToolbarHeightDP) - dims.Size.Y) / 2
						a.ensureJSONView().levelControl.anchor = image.Rectangle{Min: image.Pt(x, y), Max: image.Pt(x, y).Add(dims.Size)}
						return dims
					})
				}),
				layout.Rigid(a.layoutJSONModeSwitch),
			)
		})
	})
}

func (a *App) jsonLevelOptions() []adaptiveChoiceOption {
	return []adaptiveChoiceOption{
		{ID: "level-1", Label: "1"}, {ID: "level-2", Label: "2"},
		{ID: "level-3", Label: "3"}, {ID: "level-all", Label: a.textID("json.ribbon.level.all")},
	}
}

func (a *App) jsonLevelSelection() string {
	switch a.ensureJSONView().expandedLevel {
	case 1:
		return "level-1"
	case 2:
		return "level-2"
	case 3:
		return "level-3"
	case -1:
		return "level-all"
	default:
		return ""
	}
}

func (a *App) layoutJSONLevelControl(gtx layout.Context) layout.Dimensions {
	return a.ensureJSONView().levelControl.Layout(gtx, a.theme,
		a.textID("json.workspace.expand_hierarchy"), a.jsonLevelOptions(), a.jsonLevelSelection())
}

func (a *App) layoutJSONModeSwitch(gtx layout.Context) layout.Dimensions {
	v := a.ensureJSONView()
	gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(202), gtx.Dp(34)))
	paint.FillShape(gtx.Ops, chromeSurface, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(6)).Op(gtx.Ops))
	return layout.UniformInset(3).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, 3)
		for i, item := range []struct{ id, message string }{{"mode-tree", "json.ribbon.mode.tree"}, {"mode-source", "json.ribbon.mode.source"}, {"mode-compare", "json.ribbon.mode.compare"}} {
			index, it := i, item
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				right := unit.Dp(2)
				if index == 2 {
					right = 0
				}
				return layout.Inset{Right: right}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return a.jsonTextButton(gtx, it.id, a.textID(it.message), 64, 28, int(v.mode) == index)
				})
			}))
		}
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
	})
}

func (a *App) layoutJSONSaveIndicator(gtx layout.Context) layout.Dimensions {
	if tab := a.proofreadingTab(a.selectedProofreadingTabID); tab != nil {
		return layout.Inset{Left: 12, Right: 18}.Layout(gtx, tab.editor.layoutProofreadingHistoryAction)
	}
	if (a.jsonDoc == nil && a.markdownDoc == nil) || a.activeTabID == "recent" {
		return layout.Dimensions{}
	}
	gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(110), gtx.Dp(37)))
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		label := a.textID("document.save_state.saved")
		fg := color.NRGBA{R: 95, G: 138, B: 105, A: 255}
		if a.isDirty() || a.path == "" {
			label = a.textID("document.save_state.unsaved")
			fg = homeMuted
		}
		if a.isSaving() {
			label = a.textID("document.save_state.saving")
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
