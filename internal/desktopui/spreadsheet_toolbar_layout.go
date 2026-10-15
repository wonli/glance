package desktopui

import (
	"glance/internal/document"
	"image"

	"gioui.org/io/semantic"
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
	height := gtx.Dp(unit.Dp(wordRibbonHeightDP))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	state := a.documentSessionState
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, a.tokens().Colors.Surface.Surface, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutRibbonCategoryTabs(gtx, a.theme, toolbarCategoryLabelsForState(state), state.toolbarTabs[:], int(state.toolbarTab), a.tokens().Colors)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					stripHeight := gtx.Dp(unit.Dp(wordRibbonCommandHeightDP))
					gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = stripHeight, stripHeight
					actions := toolbarActionsForTab(state.toolbarTab)
					if state.toolbarTab == toolbarTabHome {
						return a.layoutSpreadsheetHomeRibbon(gtx, state, actions)
					}
					return layout.Background{}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							paint.FillShape(gtx.Ops, a.tokens().Colors.Toolbar.Background, clip.Rect{Max: gtx.Constraints.Max}.Op())
							return layout.Dimensions{Size: gtx.Constraints.Max}
						},
						func(gtx layout.Context) layout.Dimensions {
							return a.layoutSpreadsheetToolbarActionList(gtx, state, actions)
						},
					)
				}),
			)
		},
	)
}

type spreadsheetRibbonCell struct {
	slot         int
	hasAction    bool
	label        string
	displayLabel string
	icon         string
	iconFamily   string
	symbol       string
}

func (a *App) layoutSpreadsheetRibbonRow(gtx layout.Context, state *documentSessionState, actions []spreadsheetToolbarAction, items []spreadsheetRibbonCell, widthDP, heightDP int, mode spreadsheetRibbonActionMode) layout.Dimensions {
	if len(items) == 0 {
		return layout.Dimensions{}
	}
	cellWidthDP := max(1, widthDP/len(items))
	children := make([]layout.FlexChild, 0, len(items))
	for _, item := range items {
		item := item
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if item.hasAction {
				for _, action := range actions {
					if action.slot != item.slot {
						continue
					}
					if item.displayLabel != "" {
						action.displayLabel = item.displayLabel
					}
					return a.layoutSpreadsheetRibbonAction(gtx, state, action, cellWidthDP, heightDP, mode)
				}
			}
			return a.layoutSpreadsheetRibbonPlaceholder(gtx, item, cellWidthDP, heightDP, mode)
		}))
	}
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx, children...)
}

func (a *App) layoutSpreadsheetRibbonPlaceholder(gtx layout.Context, item spreadsheetRibbonCell, widthDP, heightDP int, mode spreadsheetRibbonActionMode) layout.Dimensions {
	width, height := gtx.Dp(unit.Dp(widthDP)), gtx.Dp(unit.Dp(heightDP))
	gtx.Constraints = layout.Exact(image.Pt(width, height))
	colors := a.tokens().Colors
	bounds := clip.Rect{Max: image.Pt(width, height)}.Push(gtx.Ops)
	semantic.Button.Add(gtx.Ops)
	semantic.DescriptionOp(item.label).Add(gtx.Ops)
	semantic.EnabledOp(false).Add(gtx.Ops)
	bounds.Pop()
	paint.FillShape(gtx.Ops, colors.Toolbar.Background, clip.UniformRRect(image.Rectangle{Max: image.Pt(width, height)}, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
	var icon image.Image
	if item.icon != "" {
		iconSize := max(1, gtx.Dp(unit.Dp(20)))
		if item.iconFamily != "" {
			icon, _ = svgIconTintAtSize(item.iconFamily, item.icon, iconSize, colors.Toolbar.DisabledText)
		} else {
			icon, _ = svgIconTintAtSize("ribbon", item.icon, iconSize, colors.Toolbar.DisabledText)
		}
	}
	drawMark := func(gtx layout.Context, size int) layout.Dimensions {
		size = min(size, min(width, height))
		if icon != nil {
			gtx.Constraints = layout.Exact(image.Pt(size, size))
			return widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center}.Layout(gtx)
		}
		if item.symbol != "" {
			style := material.Body2(a.theme, item.symbol)
			style.TextSize = unit.Sp(12)
			style.Color = colors.Text.PureBlack
			gtx.Constraints.Min.Y = 0
			return layout.Center.Layout(gtx, style.Layout)
		}
		return layout.Dimensions{Size: image.Pt(size, size)}
	}
	switch mode {
	case spreadsheetRibbonActionIconOnly:
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return drawMark(gtx, gtx.Dp(unit.Dp(20)))
		})
	case spreadsheetRibbonActionVertical:
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return drawMark(gtx, gtx.Dp(unit.Dp(20))) }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				caption := item.displayLabel
				if caption == "" {
					caption = item.label
				}
				label := material.Caption(a.theme, caption)
				label.TextSize = unit.Sp(9)
				label.Color = colors.Text.PureBlack
				label.MaxLines = 1
				return layout.Center.Layout(gtx, label.Layout)
			}),
		)
	default:
		return layout.Inset{Left: unit.Dp(3), Right: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return drawMark(gtx, gtx.Dp(unit.Dp(20))) }),
				layout.Rigid(layout.Spacer{Width: unit.Dp(3)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					caption := item.displayLabel
					if caption == "" {
						caption = item.label
					}
					label := material.Caption(a.theme, caption)
					label.TextSize = unit.Sp(9)
					label.Color = colors.Text.PureBlack
					label.MaxLines = 1
					gtx.Constraints.Min.Y = 0
					return label.Layout(gtx)
				}),
			)
		})
	}
}

func (a *App) layoutSpreadsheetActionGrid(gtx layout.Context, state *documentSessionState, actions []spreadsheetToolbarAction, rows int, labels bool) layout.Dimensions {
	if rows <= 0 {
		rows = 1
	}
	cols := (len(actions) + rows - 1) / rows
	if cols <= 0 {
		return layout.Dimensions{}
	}
	cellWidth := max(24, pxToRibbonDP(gtx, gtx.Constraints.Max.X)/cols)
	rowHeight := 34
	children := make([]layout.FlexChild, 0, rows)
	for row := 0; row < rows; row++ {
		row := row
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var cells []layout.FlexChild
			for col := 0; col < cols; col++ {
				index := row*cols + col
				if index >= len(actions) {
					break
				}
				action := actions[index]
				cells = append(cells, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					mode := spreadsheetRibbonActionIconOnly
					if labels {
						mode = spreadsheetRibbonActionCompact
					}
					return a.layoutSpreadsheetRibbonAction(gtx, state, action, cellWidth, rowHeight, mode)
				}))
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, cells...)
		}))
	}
	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx, children...)
}

func pxToRibbonDP(gtx layout.Context, pixels int) int {
	if gtx.Metric.PxPerDp <= 0 {
		return pixels
	}
	return int(float32(pixels) / gtx.Metric.PxPerDp)
}

type spreadsheetRibbonActionMode uint8

const (
	spreadsheetRibbonActionIconOnly spreadsheetRibbonActionMode = iota
	spreadsheetRibbonActionCompact
	spreadsheetRibbonActionVertical
	spreadsheetRibbonActionField
)

func spreadsheetRibbonActionDisplayLabel(action spreadsheetToolbarAction) string {
	if action.displayLabel != "" {
		return action.displayLabel
	}
	label := action.label
	switch action.slot {
	case toolbarActionSum:
		label = "自动求和"
	case toolbarActionFind:
		label = "查找和选择"
	case toolbarActionTableFormat:
		label = "套用表格格式"
	case toolbarActionZoomIn:
		label = "缩放"
	}
	switch action.slot {
	case toolbarActionOpenFile, 21, toolbarActionConditionalFormat, toolbarActionTableFormat, toolbarActionCellStyles, 12, 13, toolbarActionFormat, toolbarActionSum, toolbarActionFind, toolbarActionInsertTable, toolbarActionLayoutMargins, toolbarActionRefreshAll, toolbarActionSpellCheck, toolbarActionNormalView:
		label += " ▾"
	case 18:
		if action.label == "排序和筛选" {
			label += " ▾"
		}
	}
	return label
}

func (a *App) layoutSpreadsheetRibbonAction(gtx layout.Context, state *documentSessionState, action spreadsheetToolbarAction, widthDP, heightDP int, mode spreadsheetRibbonActionMode) layout.Dimensions {
	width, height := gtx.Dp(unit.Dp(widthDP)), gtx.Dp(unit.Dp(heightDP))
	gtx.Constraints = layout.Exact(image.Pt(width, height))
	enabled := a.spreadsheetToolbarActionButtonEnabled(action.slot)
	button := &state.toolbarActionButtons[action.slot]
	style := a.selectedCellStyle()
	mixed := formatStyleSlot(action.slot) && a.selectionFormatMixed(action.slot)
	active := !mixed && formatStyleActive(action.slot, style)
	draw := func(gtx layout.Context) layout.Dimensions {
		bounds := clip.Rect{Max: image.Pt(width, height)}.Push(gtx.Ops)
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(action.label).Add(gtx.Ops)
		semantic.EnabledOp(enabled).Add(gtx.Ops)
		bounds.Pop()
		colors := a.tokens().Colors
		paintRibbonControlState(gtx, image.Pt(width, height), enabled && active, mixed, enabled && (button.Hovered() || gtx.Focused(button)), colors.Toolbar)
		if mode == spreadsheetRibbonActionField {
			paint.FillShape(gtx.Ops, colors.Surface.Border, clip.UniformRRect(image.Rectangle{Max: image.Pt(width, height)}, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
			inner := image.Rectangle{Max: image.Pt(width, height)}.Inset(gtx.Dp(unit.Dp(1)))
			paint.FillShape(gtx.Ops, colors.Surface.Surface, clip.UniformRRect(inner, gtx.Dp(unit.Dp(2))).Op(gtx.Ops))
			value := spreadsheetRibbonActionDisplayLabel(action)
			if action.slot == toolbarActionFontFamily {
				value = a.fontPickerDisplayFamilyName(style.FontFamily)
				if value == "" {
					value = "字体"
				}
			} else if action.slot == toolbarActionFontSize {
				value = "11"
			}
			return layout.Inset{Left: unit.Dp(6), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				label := material.Caption(a.theme, value)
				label.TextSize = unit.Sp(11)
				label.Color = colors.Text.PureBlack
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.Y = 0
						if action.slot == toolbarActionFontFamily || action.slot == toolbarActionFontSize {
							return layout.Inset{Top: spreadsheetFontFieldTextOpticalTopInset}.Layout(gtx, label.Layout)
						}
						return label.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layoutPenRibbonGlyph(gtx, "chevron-down", 12, label.Color)
					}),
				)
			})
		}
		iconColorID := action.id
		if iconColorID == "" {
			iconColorID = action.icon
		}
		iconColor := a.toolbarActionIconColor(iconColorID, enabled)
		if enabled && active {
			iconColor = colors.Toolbar.ActiveGlyph
		}
		iconName := action.icon
		if name, ok := penSpreadsheetRibbonIcons[action.slot]; ok {
			iconName = name
		}
		if action.slot == 18 && state.toolbarTab == toolbarTabData {
			iconName = "arrow-up-down"
		}
		icon, _ := svgIconTintAtSize("ribbon", iconName, gtx.Dp(unit.Dp(20)), iconColor)
		if mode == spreadsheetRibbonActionIconOnly {
			if icon == nil {
				return layout.Dimensions{Size: image.Pt(width, height)}
			}
			iconSize := gtx.Dp(unit.Dp(20))
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints = layout.Exact(image.Pt(iconSize, iconSize))
				return widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: 20.0 / float32(iconSize)}.Layout(gtx)
			})
		}
		if mode == spreadsheetRibbonActionVertical {
			return layoutOfficeRibbonCommandContents(gtx, a.theme, icon, spreadsheetRibbonActionDisplayLabel(action))
		}
		return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					iconSize := gtx.Dp(unit.Dp(20))
					gtx.Constraints = layout.Exact(image.Pt(iconSize, iconSize))
					if icon == nil {
						return layout.Dimensions{Size: image.Pt(iconSize, iconSize)}
					}
					return widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center}.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					label := material.Caption(a.theme, spreadsheetRibbonActionDisplayLabel(action))
					gtx.Constraints.Min.Y = 0
					label.TextSize = unit.Sp(10)
					label.Color = colors.Text.PureBlack
					gtx.Constraints.Min.Y = 0
					return label.Layout(gtx)
				}),
			)
		})
	}
	if enabled {
		return button.Layout(gtx, draw)
	}
	return draw(gtx)
}

const spreadsheetFontFieldTextOpticalTopInset unit.Dp = 5

func (a *App) layoutSpreadsheetToolbarActionList(gtx layout.Context, state *documentSessionState, actions []spreadsheetToolbarAction) layout.Dimensions {
	return a.layoutSpreadsheetPenCommandList(gtx, state, actions)
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
	panelX := 0
	if state.toolbarTab == toolbarTabHome {
		groups := spreadsheetHomeRibbonGroups(actions)
		first := max(0, min(state.toolbarGroupList.List.Position.First, len(groups)))
		scrollDP := 0
		for i := 0; i < first; i++ {
			scrollDP += groups[i].widthDP + 4
		}
		scrollPX := gtx.Dp(unit.Dp(scrollDP)) + state.toolbarGroupList.List.Position.Offset
		panelX = gtx.Dp(unit.Dp(80+spreadsheetHomeActionAnchorOffset(groups, state.toolbarPickerSlot))) - scrollPX
	} else {
		panelX = toolbarPickerPanelX(state.toolbarPickerSlot, actions, state.toolbarActionList.List.Position, normalWidth, groupWidth, gtx.Constraints.Max.X, panelWidth) + gtx.Dp(unit.Dp(8))
	}
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
	colors := a.tokens().Colors
	rows := (len(options) + 1) / 2
	height := gtx.Dp(unit.Dp(32 + rows*24))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			bounds := image.Rectangle{Max: gtx.Constraints.Min}
			shadowBounds := bounds.Add(image.Pt(1, 2))
			paint.FillShape(gtx.Ops, colors.Menu.Shadow, clip.UniformRRect(shadowBounds, gtx.Dp(unit.Dp(5))).Op(gtx.Ops))
			paint.FillShape(gtx.Ops, colors.Menu.Border, clip.UniformRRect(bounds, gtx.Dp(unit.Dp(5))).Op(gtx.Ops))
			paint.FillShape(gtx.Ops, colors.Menu.Surface, clip.UniformRRect(bounds.Inset(1), gtx.Dp(unit.Dp(4))).Op(gtx.Ops))
			return layout.Dimensions{Size: bounds.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(6), Right: unit.Dp(6), Top: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				rows := make([]layout.FlexChild, 0, (len(options)+1)/2+2)
				title := material.Body2(a.theme, toolbarPickerTitle(a.documentSessionState.toolbarPicker))
				title.Color = colors.Text.PureBlack
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
								button := &a.documentSessionState.toolbarPickerButtons[index]
								return layout.Inset{Left: unit.Dp(2), Right: unit.Dp(2), Top: unit.Dp(1), Bottom: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									height := gtx.Dp(unit.Dp(22))
									width := gtx.Constraints.Max.X
									gtx.Constraints = layout.Exact(image.Pt(width, height))
									selected := toolbarPickerOptionSelected(a.documentSessionState.toolbarPicker, index, a.selectedCellStyle())
									background := colors.Menu.Surface
									if selected {
										background = colors.Menu.Selected
									} else if button.Hovered() {
										background = colors.Menu.Hover
									}
									return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return layout.Background{}.Layout(gtx,
											func(gtx layout.Context) layout.Dimensions {
												paint.FillShape(gtx.Ops, background, clip.UniformRRect(image.Rectangle{Max: image.Pt(width, height)}, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
												return layout.Dimensions{Size: image.Pt(width, height)}
											},
											func(gtx layout.Context) layout.Dimensions {
												style := material.Body2(a.theme, options[index])
												style.TextSize = unit.Sp(12)
												style.Color = colors.Text.PureBlack
												gtx.Constraints.Min = image.Point{}
												gtx.Constraints.Min.Y = 0
												return layout.Center.Layout(gtx, style.Layout)
											},
										)
									})
								})
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
