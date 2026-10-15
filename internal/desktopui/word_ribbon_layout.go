package desktopui

import (
	"image"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

const (
	wordRibbonCategoryHeightDP    = 34
	wordRibbonCategoryTabWidthDP  = 31
	wordRibbonCategoryTabHeightDP = 30
	wordRibbonCommandHeightDP     = 68
	wordRibbonHeightDP            = wordRibbonCategoryHeightDP + wordRibbonCommandHeightDP
	spreadsheetRibbonLeftInsetDP  = 16
)

type wordRibbonGroup struct {
	label   string
	actions []ribbonAction
}

func (a *App) layoutWordRibbon(gtx layout.Context) layout.Dimensions {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return layout.Dimensions{}
	}
	if state.wordToolbarTab == wordTabPicture && state.selectedWordImageID == "" {
		state.wordToolbarTab = wordTabHome
	}
	state.wordToolbarActionList.List.Axis = layout.Horizontal
	state.wordToolbarTabList.List.Axis = layout.Horizontal
	height := gtx.Dp(unit.Dp(wordRibbonHeightDP))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, a.tokens().Colors.Surface.Surface, clip.Rect{Max: gtx.Constraints.Max}.Op())
			bounds := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
			semantic.DescriptionOp("Word 功能区").Add(gtx.Ops)
			bounds.Pop()
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.layoutWordRibbonCategories(gtx, state)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.layoutWordRibbonCommands(gtx, state)
				}),
			)
		},
	)
}

func (a *App) layoutWordRibbonCategories(gtx layout.Context, state *documentSessionState) layout.Dimensions {
	height := gtx.Dp(unit.Dp(wordRibbonCategoryHeightDP))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	tabs := wordToolbarTabs(state)
	paint.FillShape(gtx.Ops, a.tokens().Colors.Surface.Surface, clip.Rect{Max: gtx.Constraints.Max}.Op())
	for index, tab := range tabs {
		x := 22 + index*(wordRibbonCategoryTabWidthDP+28)
		a.layoutWordRibbonCategoryTabAt(gtx, state, index, x, 2, wordToolbarTabLabels[tab], tab == state.wordToolbarTab)
	}
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func (a *App) layoutWordRibbonCategoryTabAt(gtx layout.Context, state *documentSessionState, index int, xDP, yDP int, label string, selected bool) {
	width, height := gtx.Dp(unit.Dp(wordRibbonCategoryTabWidthDP)), gtx.Dp(unit.Dp(wordRibbonCategoryTabHeightDP))
	x, y := gtx.Dp(unit.Dp(xDP)), gtx.Dp(unit.Dp(yDP))
	transform := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	buttonGtx := gtx
	buttonGtx.Constraints = layout.Constraints{Min: image.Pt(width, height), Max: image.Pt(width, height)}
	state.wordToolbarTabs[index].Layout(buttonGtx, func(gtx layout.Context) layout.Dimensions {
		bounds := clip.Rect{Max: image.Pt(width, height)}.Push(gtx.Ops)
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(label).Add(gtx.Ops)
		bounds.Pop()
		style := material.Body2(a.theme, label)
		style.TextSize = unit.Sp(13)
		style.Color = a.tokens().Colors.Text.Secondary
		if selected {
			style.Color = a.tokens().Colors.Text.Brand
		}
		shift := op.Offset(image.Pt(0, gtx.Dp(unit.Dp(3)))).Push(gtx.Ops)
		layout.Center.Layout(gtx, style.Layout)
		shift.Pop()
		return layout.Dimensions{Size: image.Pt(width, height)}
	})
	transform.Pop()
	if selected {
		lineX, lineY := gtx.Dp(unit.Dp(xDP)), gtx.Dp(unit.Dp(yDP+wordRibbonCategoryTabHeightDP-1))
		line := op.Offset(image.Pt(lineX, lineY)).Push(gtx.Ops)
		paint.FillShape(gtx.Ops, a.tokens().Colors.Text.Brand, clip.Rect{Max: image.Pt(width, gtx.Dp(unit.Dp(1)))}.Op())
		line.Pop()
	}
}

func (a *App) layoutWordRibbonCommands(gtx layout.Context, state *documentSessionState) layout.Dimensions {
	height := gtx.Dp(unit.Dp(wordRibbonCommandHeightDP))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, contentSurface, clip.Rect{Max: gtx.Constraints.Max}.Op())
			paint.FillShape(gtx.Ops, wordStatusTopRule, clip.Rect{Min: image.Pt(0, height-gtx.Dp(unit.Dp(1))), Max: image.Pt(gtx.Constraints.Max.X, height)}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return a.layoutWordRibbonActions(gtx, state, state.wordToolbarTab)
		},
	)
}

func (a *App) layoutWordHomeRibbonContent(gtx layout.Context, state *documentSessionState) layout.Dimensions {
	return a.layoutWordRibbonActions(gtx, state, wordTabHome)
}

func (a *App) layoutWordHomeRibbonControls(gtx layout.Context, state *documentSessionState) layout.Dimensions {
	return a.layoutWordRibbonActions(gtx, state, wordTabHome)
}

var (
	wordPenActionPositions = make(map[wordToolbarTab][]int)
	wordPenGroupDividers   = make(map[wordToolbarTab][]int)
)

func init() {
	allTabs := []wordToolbarTab{
		wordTabFile,
		wordTabHome,
		wordTabInsert,
		wordTabDesign,
		wordTabLayout,
		wordTabReferences,
		wordTabReview,
		wordTabView,
		wordTabPicture,
	}
	buttonWidth := int(toolbarButtonWidth)
	gap := int(toolbarItemGap)
	dividerMargin := 12
	dividerWidth := 1
	for _, tab := range allTabs {
		actions := wordToolbarActions(tab)
		if len(actions) == 0 {
			continue
		}
		positions := make([]int, len(actions))
		var dividers []int
		x := 34
		for i, action := range actions {
			if i > 0 && action.group != actions[i-1].group {
				dividerX := x - gap + dividerMargin
				dividers = append(dividers, dividerX)
				x = dividerX + dividerWidth + dividerMargin
			}
			positions[i] = x
			x += buttonWidth + gap
		}
		wordPenActionPositions[tab] = positions
		wordPenGroupDividers[tab] = dividers
	}
}
