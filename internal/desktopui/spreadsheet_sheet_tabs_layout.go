package desktopui

import (
	"glance/internal/document"
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func (a *App) layoutSheetTabs(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = gtx.Dp(unit.Dp(32)), gtx.Dp(unit.Dp(32))
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, chromeSurface, clip.Rect{Max: gtx.Constraints.Max}.Op())
			paint.FillShape(gtx.Ops, gridLine, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(1)))}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			children := make([]layout.FlexChild, 0, len(a.sheets)+1)
			for _, name := range a.sheets {
				sheetName := name
				button := a.sheetButton(sheetName)
				if button.Clicked(gtx) {
					if a.documentSessionState.valueFilter != nil {
						a.clearValueFilter(a.documentSessionState)
					}
					formulaReferenceActive := a.documentSessionState.formulaReference.Active
					if a.cellEdit.Editing && !formulaReferenceActive {
						if err := a.commitGridCell(a.cellEditor.Text()); err != nil {
							a.setStatus("修改失败：" + err.Error())
							a.window.Invalidate()
							continue
						}
					}
					a.sheet = sheetName
					if _, virtual := a.book.(document.GridReader); virtual {
						if a.virtualGrid != nil {
							a.virtualGrid.invalidateWindow()
							a.virtualGrid.SelectedRow, a.virtualGrid.SelectedColumn = 1, 1
							a.virtualGrid.PendingSelectionLoad = true
						}
						a.documentSessionState.selection = CellSelection{}
						if formulaReferenceActive {
							a.documentSessionState.selection = CellSelection{Anchor: CellPosition{}, Active: CellPosition{}}
						} else {
							a.selectedCell = "A1"
							a.documentSessionState.selection = CellSelection{}
						}
						a.documentSessionState.selectionSet = true
						a.documentSessionState.selectionDragging = false
						if !formulaReferenceActive {
							a.documentSessionState.cellEdit.selectCell("A1", "")
							a.nameBox.SetText("A1")
							a.formulaEditor.SetText("")
						} else if editor := a.formulaReferenceEditor(); editor != nil {
							gtx.Execute(key.FocusCmd{Tag: editor})
						}
					} else if err := a.loadSheet(); err != nil {
						a.setStatus(err.Error())
					}
					if !formulaReferenceActive {
						a.persistViewState()
					}
				}
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layoutSheetTab(gtx, a.theme, button, sheetName, sheetName == a.sheet)
					})
				}))
			}
			if a.documentSessionState.sheetCreateButton.Clicked(gtx) {
				a.applyStructure(document.StructureEdit{Kind: document.StructureCreateSheet})
			}
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutSheetPlusTab(gtx, a.theme, &a.documentSessionState.sheetCreateButton)
			}))
			return layout.Inset{Left: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layoutSheetTabRow(gtx, children...)
			})
		},
	)
}

func layoutSheetTabRow(gtx layout.Context, children ...layout.FlexChild) layout.Dimensions {
	// The document body is laid out as an Expanded child of a Stack, which
	// gives it a zero minimum width. Fill the strip so Background doesn't
	// center this intrinsic-width row of sheet tabs.
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceEnd}.Layout(gtx, children...)
}

func layoutSheetTab(gtx layout.Context, theme *material.Theme, button *widget.Clickable, name string, selected bool) layout.Dimensions {
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		tabHeight := gtx.Dp(unit.Dp(26))
		bg := chromeSurface
		fg := color.NRGBA{R: 104, G: 113, B: 123, A: 255}
		weight := font.Normal
		if selected {
			bg = contentSurface
			fg = color.NRGBA{R: 62, G: 111, B: 169, A: 255}
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

func layoutSheetPlusTab(gtx layout.Context, theme *material.Theme, button *widget.Clickable) layout.Dimensions {
	gtx.Constraints.Min = image.Pt(gtx.Dp(unit.Dp(28)), gtx.Dp(unit.Dp(26)))
	gtx.Constraints.Max = gtx.Constraints.Min
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		label := material.Body1(theme, "+")
		label.TextSize = unit.Sp(17)
		label.Color = color.NRGBA{R: 89, G: 99, B: 110, A: 255}
		return layout.Center.Layout(gtx, label.Layout)
	})
}
