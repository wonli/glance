package desktopui

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func layoutPenRibbonGlyph(gtx layout.Context, name string, size unit.Dp, tint color.NRGBA) layout.Dimensions {
	pixels := gtx.Dp(size)
	gtx.Constraints = layout.Exact(image.Pt(pixels, pixels))
	icon, ok := svgIconTintAtSize("ribbon", name, pixels, tint)
	if !ok {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	return widget.Image{Src: iconImageOp(icon), Fit: widget.Contain, Position: layout.Center}.Layout(gtx)
}

// The overview boards use 80×68 commands, 4dp gaps and 8dp strip padding.
// Keep the label at its natural height so the icon and visible text share an axis.
func layoutOfficeRibbonCommandContents(gtx layout.Context, theme *material.Theme, icon image.Image, label string) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Point{}
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := gtx.Dp(20)
				gtx.Constraints = layout.Exact(image.Pt(size, size))
				if icon == nil {
					return layout.Dimensions{Size: gtx.Constraints.Max}
				}
				return widget.Image{Src: iconImageOp(icon), Fit: widget.Contain, Position: layout.Center}.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				style := material.Caption(theme, label)
				style.TextSize = unit.Sp(12)
				style.Color = color.NRGBA{R: 0x35, G: 0x41, B: 0x4f, A: 255}
				return layoutDropdownLabel(gtx, style)
			}),
		)
	})
}

func (a *App) layoutWordPenCommandList(gtx layout.Context, state *documentSessionState, tab wordToolbarTab, actions []ribbonAction) layout.Dimensions {
	left := unit.Dp(12)
	if tab == wordTabInsert {
		left = 6
	}
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	state.wordToolbarActionList.List.Axis = layout.Horizontal
	return layout.Inset{Left: left, Right: unit.Dp(6), Top: unit.Dp(10), Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layoutHorizontalRibbonList(gtx, &state.wordToolbarActionScroll, &state.wordToolbarActionList, len(actions), func(gtx layout.Context, index int) layout.Dimensions {
			return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutWordRibbonAction(gtx, state, actions[index], 80, 68, true)
			})
		})
	})
}

func (a *App) layoutSpreadsheetPenCommandList(gtx layout.Context, state *documentSessionState, actions []spreadsheetToolbarAction) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	state.toolbarActionList.List.Axis = layout.Horizontal
	visible := make([]spreadsheetToolbarAction, 0, len(actions))
	for _, action := range actions {
		if action.slot == toolbarActionInsertFunction || action.slot == toolbarActionZoom100 {
			continue
		}
		visible = append(visible, action)
	}
	return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12), Top: unit.Dp(10), Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layoutHorizontalRibbonList(gtx, &state.toolbarActionScroll, &state.toolbarActionList, len(visible), func(gtx layout.Context, index int) layout.Dimensions {
			return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				action := visible[index]

				return a.layoutSpreadsheetRibbonAction(gtx, state, action, 80, 68, spreadsheetRibbonActionVertical)
			})
		})
	})
}

func wordPenCommandX(gtx layout.Context, state *documentSessionState, id string) int {
	if state.wordToolbarTab == wordTabHome {
		groups := wordRibbonGroupsForTab(wordTabHome, wordToolbarActions(wordTabHome))
		position := state.wordToolbarGroupList.List.Position
		scroll := 0
		for i := 0; i < min(position.First, len(groups)); i++ {
			scroll += groups[i].widthDP + 4
		}
		return gtx.Dp(unit.Dp(96+wordRibbonActionAnchorOffset(groups, id)-scroll)) - position.Offset
	}
	left := 12
	if state.wordToolbarTab == wordTabInsert {
		left = 6
	}
	position := state.wordToolbarActionList.List.Position
	for index, action := range wordToolbarActions(state.wordToolbarTab) {
		if action.id == id {
			return gtx.Dp(unit.Dp(left+84*(index-position.First))) - position.Offset
		}
	}
	return gtx.Dp(unit.Dp(left))
}

func spreadsheetPenCommandX(gtx layout.Context, state *documentSessionState, slot int) int {
	position := state.toolbarActionList.List.Position
	index := 0
	for _, action := range toolbarActionsForTab(state.toolbarTab) {
		if action.slot == toolbarActionInsertFunction || action.slot == toolbarActionZoom100 {
			continue
		}
		if action.slot == slot {
			return gtx.Dp(unit.Dp(12+84*(index-position.First))) - position.Offset
		}
		index++
	}
	return gtx.Dp(12)
}
