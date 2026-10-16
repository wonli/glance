package desktopui

import (
	"glance/internal/document"
	"image"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func (a *App) layoutSheetTabs(gtx layout.Context) layout.Dimensions {
	state := a.documentSessionState
	if state != nil {
		state.sheetTabList.Axis = layout.Horizontal
		state.sheetTabList.ScrollAnyAxis = false
		a.handleSpreadsheetSheetTabScroll(gtx, state)
		revealActiveSpreadsheetSheetTab(state)
	}
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = gtx.Dp(unit.Dp(32)), gtx.Dp(unit.Dp(32))
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, chromeSurface, clip.Rect{Max: gtx.Constraints.Max}.Op())
			paint.FillShape(gtx.Ops, gridLine, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(1)))}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			if state == nil {
				return layout.Dimensions{}
			}
			return layout.Inset{Left: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				dims := state.sheetTabList.Layout(gtx, len(a.sheets)+1, func(gtx layout.Context, index int) layout.Dimensions {
					if index == len(a.sheets) {
						if state.sheetCreateButton.Clicked(gtx) {
							a.applyStructure(document.StructureEdit{Kind: document.StructureCreateSheet})
						}
						return layoutSheetPlusTab(gtx, a.theme, &state.sheetCreateButton, a.tokens().Colors)
					}
					sheetName := a.sheets[index]
					button := a.sheetButton(sheetName)
					if button.Clicked(gtx) {
						a.activateSpreadsheetSheet(gtx, sheetName)
					}
					return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layoutSheetTab(gtx, a.theme, a.tokens().Colors, button, sheetName, sheetName == a.sheet)
					})
				})
				pass := pointer.PassOp{}.Push(gtx.Ops)
				area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
				event.Op(gtx.Ops, &state.sheetTabScrollTag)
				area.Pop()
				pass.Pop()
				return dims
			})
		},
	)
}

func (a *App) handleSpreadsheetSheetTabScroll(gtx layout.Context, state *documentSessionState) {
	if state == nil {
		return
	}
	const scrollLimit = 1 << 30
	filter := pointer.Filter{
		Target:  &state.sheetTabScrollTag,
		Kinds:   pointer.Scroll,
		ScrollX: pointer.ScrollRange{Min: -scrollLimit, Max: scrollLimit},
		ScrollY: pointer.ScrollRange{Min: -scrollLimit, Max: scrollLimit},
	}
	for eventValue, ok := gtx.Event(filter); ok; eventValue, ok = gtx.Event(filter) {
		pe, ok := eventValue.(pointer.Event)
		if !ok || pe.Kind != pointer.Scroll || pe.Scroll.X == 0 && pe.Scroll.Y == 0 {
			continue
		}
		delta := pe.Scroll.X
		if absFloat32(pe.Scroll.Y) >= absFloat32(pe.Scroll.X) {
			delta = pe.Scroll.Y
		}
		state.sheetTabScrollRemainder += delta
		pixels := int(state.sheetTabScrollRemainder)
		state.sheetTabScrollRemainder -= float32(pixels)
		if pixels == 0 {
			continue
		}
		state.sheetTabList.Position.Offset += pixels
		state.sheetTabList.Position.BeforeEnd = true
	}
}

func revealActiveSpreadsheetSheetTab(state *documentSessionState) {
	if state == nil || state.sheetTabList.Position.Count <= 0 {
		return
	}
	index := -1
	for i, name := range state.sheets {
		if name == state.sheet {
			index = i
			break
		}
	}
	if index < 0 || (state.sheetTabLastRevealed == state.sheet && state.sheetTabLastRevealedIndex == index) {
		return
	}
	position := state.sheetTabList.Position
	if index < position.First {
		state.sheetTabList.ScrollTo(index)
	} else if index >= position.First+position.Count {
		state.sheetTabList.ScrollTo(max(0, index-position.Count+1))
	}
	state.sheetTabLastRevealed = state.sheet
	state.sheetTabLastRevealedIndex = index
}

func layoutSheetTab(gtx layout.Context, theme *material.Theme, colors ColorTokens, button *widget.Clickable, name string, selected bool) layout.Dimensions {
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		tabHeight := gtx.Dp(unit.Dp(26))
		bg := chromeSurface
		fg := colors.Text.PureBlack
		weight := font.Normal
		if selected {
			bg = contentSurface
			weight = font.SemiBold
		}
		label := material.Body2(theme, name)
		label.Color = fg
		label.TextSize = unit.Sp(12)
		label.Font.Weight = weight
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(unit.Dp(4))).Op(gtx.Ops))
				return layout.Dimensions{Size: gtx.Constraints.Min}
			},
			func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = tabHeight, tabHeight
				return layout.Inset{Top: unit.Dp(3), Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Center.Layout(gtx, label.Layout)
				})
			},
		)
	})
}

func layoutSheetPlusTab(gtx layout.Context, theme *material.Theme, button *widget.Clickable, colors ColorTokens) layout.Dimensions {
	gtx.Constraints.Min = image.Pt(gtx.Dp(unit.Dp(28)), gtx.Dp(unit.Dp(26)))
	gtx.Constraints.Max = gtx.Constraints.Min
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		label := material.Body1(theme, "+")
		label.TextSize = unit.Sp(17)
		label.Color = colors.Text.PureBlack
		return layout.Center.Layout(gtx, label.Layout)
	})
}
