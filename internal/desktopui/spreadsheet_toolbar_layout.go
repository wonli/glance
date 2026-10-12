package desktopui

import (
	"glance/internal/document"
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func (a *App) spreadsheetToolbarVisible() bool {
	if a == nil || a.book == nil || !a.book.Editable() {
		return false
	}
	_, ok := a.book.(document.GridReader)
	return ok
}

func (a *App) selectedCellStyle() document.CellStyle {
	state := a.documentSessionState
	if state == nil || state.virtualGrid == nil || !state.selectionSet {
		return document.CellStyle{}
	}
	pos := state.selection.Active
	return windowCell(state.virtualGrid.Window, pos.Row+1, pos.Column+1).Style
}

func (a *App) layoutSpreadsheetToolbar(gtx layout.Context) layout.Dimensions {
	if !a.spreadsheetToolbarVisible() {
		return layout.Dimensions{}
	}
	height := gtx.Dp(unit.Dp(102))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	state := a.documentSessionState
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, contentSurface, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutRibbonCategoryTabs(gtx, a.theme, toolbarCategoryLabelsForState(state), state.toolbarTabs[:], int(state.toolbarTab))
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					stripHeight := gtx.Dp(unit.Dp(68))
					gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = stripHeight, stripHeight
					actions := toolbarActionsForTab(state.toolbarTab)
					iconSize := toolbarIconSize
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: unit.Dp(11), Right: unit.Dp(5)}.Layout(gtx, a.layoutAgentRibbonEntry)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return state.toolbarActionList.Layout(gtx, len(actions), func(gtx layout.Context, index int) layout.Dimensions {
									action := actions[index]
									style := a.selectedCellStyle()
									mixed := action.slot >= 0 && formatStyleSlot(action.slot) && a.selectionFormatMixed(action.slot)
									formatActive := formatStyleActive(action.slot, style) ||
										(action.slot == toolbarActionFormatPainter && state != nil && state.excelFormatPainterActive) ||
										((action.slot == toolbarActionViewGridlines || action.slot == toolbarActionLayoutGridlines) && (state == nil || !state.spreadsheetGridlinesHidden)) ||
										(action.slot == toolbarActionNormalView && (state == nil || state.spreadsheetViewMode == "normal" || state.spreadsheetViewMode == "")) ||
										(action.slot == toolbarActionPageBreakPreview && state != nil && state.spreadsheetViewMode == "pageBreakPreview") ||
										(action.slot == toolbarActionRuler && state != nil && state.spreadsheetRulerVisible) ||
										(action.slot == toolbarActionProtectSheet && state != nil && state.spreadsheetSheetProtected) ||
										(action.slot == toolbarActionProtectWorkbook && state != nil && state.spreadsheetWorkbookProtected)
									active := action.slot >= 0 && !mixed && formatActive
									enabled := action.slot >= 0 && a.toolbarActionEnabled(action.slot)
									var button *widget.Clickable
									if action.slot >= 0 {
										button = &state.toolbarActionButtons[action.slot]
									}
									iconColor, background := toolbarIconColor(true), contentSurface
									if active {
										background = toolbarActiveBackground
									}
									if mixed {
										background = mixedToolbarBackground
									}
									if !enabled {
										background = contentSurface
									}
									icon, _ := tintedToolbarIcon(action.icon, iconColor)
									return layoutToolbarActionItem(gtx, action, index, actions, func(gtx layout.Context) layout.Dimensions {
										btnGtx := gtx
										btnGtx.Constraints.Min = image.Pt(gtx.Dp(excelToolbarButtonWidth), gtx.Dp(excelToolbarButtonHeight))
										btnGtx.Constraints.Max = btnGtx.Constraints.Min
										return toolbarIconButton(btnGtx, a.theme, button, icon, iconSize, action.label, action.label, enabled, mixed, background)
									})
								})
							})
						}),
					)
				}),
			)
		},
	)
}

func (a *App) layoutSpreadsheetToolbarPickerOverlay(gtx layout.Context) layout.Dimensions {
	state := a.documentSessionState
	if state == nil || state.toolbarPicker == pickerNone {
		return layout.Dimensions{}
	}
	options := toolbarPickerOptions(state.toolbarPicker)
	if len(options) == 0 {
		return layout.Dimensions{}
	}
	actions := toolbarActionsForTab(state.toolbarTab)
	panelWidth := gtx.Dp(unit.Dp(276))
	if panelWidth > gtx.Constraints.Max.X {
		panelWidth = gtx.Constraints.Max.X
	}
	normalWidth := gtx.Dp(unit.Dp(51))
	groupWidth := gtx.Dp(unit.Dp(55))
	panelX := toolbarPickerPanelX(state.toolbarPickerSlot, actions, state.toolbarActionList.List.Position, normalWidth, groupWidth, gtx.Constraints.Max.X, panelWidth)
	panelX += gtx.Dp(unit.Dp(8))
	maxX := gtx.Constraints.Max.X - panelWidth
	if maxX < 0 {
		maxX = 0
	}
	if panelX > maxX {
		panelX = maxX
	}
	left := unit.Dp(0)
	if gtx.Metric.PxPerDp > 0 {
		left = unit.Dp(float32(panelX) / gtx.Metric.PxPerDp)
	}
	return layout.Inset{Left: left, Top: unit.Dp(106)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		width := panelWidth
		if width > gtx.Constraints.Max.X {
			width = gtx.Constraints.Max.X
		}
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
		return a.layoutToolbarPickerPanel(gtx, options)
	})
}

func (a *App) layoutToolbarPickerPanel(gtx layout.Context, options []string) layout.Dimensions {
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			bounds := image.Rectangle{Max: gtx.Constraints.Min}
			shadowBounds := bounds.Add(image.Pt(1, 2))
			paint.FillShape(gtx.Ops, color.NRGBA{R: 28, G: 38, B: 54, A: 28}, clip.UniformRRect(shadowBounds, gtx.Dp(unit.Dp(9))).Op(gtx.Ops))
			paint.FillShape(gtx.Ops, gridLine, clip.UniformRRect(bounds, gtx.Dp(unit.Dp(9))).Op(gtx.Ops))
			inner := bounds.Inset(1)
			paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.UniformRRect(inner, gtx.Dp(unit.Dp(8))).Op(gtx.Ops))
			return layout.Dimensions{Size: bounds.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Top: unit.Dp(6), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				rows := make([]layout.FlexChild, 0, (len(options)+1)/2+2)
				title := material.Body2(a.theme, toolbarPickerTitle(a.documentSessionState.toolbarPicker))
				title.Color = mutedText
				rows = append(rows, layout.Rigid(title.Layout))
				rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(0, gtx.Dp(unit.Dp(4)))}
				}))
				for row := 0; row < (len(options)+1)/2; row++ {
					rowIndex := row
					rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						children := make([]layout.FlexChild, 0, 2)
						for column := 0; column < 2; column++ {
							index := rowIndex*2 + column
							if index >= len(options) {
								children = append(children, layout.Flexed(1, func(layout.Context) layout.Dimensions { return layout.Dimensions{} }))
								continue
							}
							children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								button := material.Button(a.theme, &a.documentSessionState.toolbarPickerButtons[index], options[index])
								button.TextSize = a.theme.TextSize * 12 / 16
								button.Background = contentSurface
								button.Color = mutedText
								button.CornerRadius = 6
								button.Inset = layout.Inset{Left: unit.Dp(5), Right: unit.Dp(5), Top: unit.Dp(4), Bottom: unit.Dp(4)}
								return layout.Inset{Left: unit.Dp(2), Right: unit.Dp(2), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, button.Layout)
							}))
						}
						return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
					}))
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
			})
		},
	)
}
