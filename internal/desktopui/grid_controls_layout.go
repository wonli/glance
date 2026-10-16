package desktopui

import (
	"image"
	"image/color"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

const formulaEditorOpticalCenterTopInset unit.Dp = 4

func (a *App) layoutGridControls(gtx layout.Context) layout.Dimensions {
	for ev, ok := a.nameBox.Update(gtx); ok; ev, ok = a.nameBox.Update(gtx) {
		if _, submit := ev.(widget.SubmitEvent); submit {
			a.goToCell()
		}
	}
	if ev, ok := gtx.Event(key.Filter{Focus: &a.formulaEditor, Name: key.NameEscape}); ok {
		if ke, yes := ev.(key.Event); yes && ke.State == key.Press && a.documentSessionState.formulaReference.Active {
			a.restoreFormulaReferenceTarget()
			if a.virtualGrid != nil {
				gtx.Execute(key.FocusCmd{Tag: &a.virtualGrid.PointerTag})
			}
		}
	}
	formulaCommitted := false
	for ev, ok := a.formulaEditor.Update(gtx); ok; ev, ok = a.formulaEditor.Update(gtx) {
		if _, submit := ev.(widget.SubmitEvent); submit {
			if err := a.commitGridCell(a.formulaEditor.Text()); err != nil {
				a.setStatus("修改失败：" + err.Error())
				a.window.Invalidate()
			} else {
				formulaCommitted = true
				if a.virtualGrid != nil {
					gtx.Execute(key.FocusCmd{Tag: &a.virtualGrid.PointerTag})
				}
			}
		}
	}
	if !formulaCommitted {
		a.syncFormulaReferenceInput(gtx)
	}
	if a.goToCellButton.Clicked(gtx) {
		a.goToCell()
	}
	if a.applyButton.Clicked(gtx) {
		if err := a.commitGridCell(a.formulaEditor.Text()); err != nil {
			a.setStatus("修改失败：" + err.Error())
			a.window.Invalidate()
		} else if a.virtualGrid != nil {
			gtx.Execute(key.FocusCmd{Tag: &a.virtualGrid.PointerTag})
		}
	}
	height := gtx.Dp(unit.Dp(56))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	outerSize := gtx.Constraints.Max
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			barHeight := gtx.Dp(unit.Dp(44))
			paint.FillShape(gtx.Ops, color.NRGBA{R: 244, G: 245, B: 246, A: 255}, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, barHeight)}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			transform := op.Offset(image.Pt(0, gtx.Dp(unit.Dp(19)))).Push(gtx.Ops)
			defer transform.Pop()
			gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = gtx.Dp(unit.Dp(30)), gtx.Dp(unit.Dp(30))
			layout.Inset{Left: unit.Dp(20), Right: unit.Dp(28)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layoutFormulaEditorField(gtx, a.theme, &a.nameBox, "A1", unit.Dp(110))
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(12)), 0)}
					}),
					layout.Rigid(a.layoutFormulaFunctionIcon),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(10)), 0)}
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return a.formulaParagraphSelection.layout(gtx, &a.formulaEditor, func(gtx layout.Context) layout.Dimensions {
							return layoutFormulaEditorField(gtx, a.theme, &a.formulaEditor, "", 0)
						})
					}),
				)
			})
			return layout.Dimensions{Size: outerSize}
		},
	)
}

func layoutFormulaEditorField(gtx layout.Context, theme *material.Theme, editor *widget.Editor, hint string, width unit.Dp) layout.Dimensions {
	height := gtx.Dp(unit.Dp(30))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	if width > 0 {
		fixedWidth := gtx.Dp(width)
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = fixedWidth, fixedWidth
	}
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			bounds := image.Rectangle{Max: gtx.Constraints.Max}
			shape := clip.UniformRRect(bounds, gtx.Dp(unit.Dp(4)))
			paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, shape.Op(gtx.Ops))
			paint.FillShape(gtx.Ops, gridLine, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
			return layout.Dimensions{Size: bounds.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			settingsFieldFocus(gtx, editor)
			return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10), Top: formulaEditorOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = 0
				style := material.Editor(theme, editor, hint)
				style.TextSize = unit.Sp(13)
				style.Color = color.NRGBA{R: 76, G: 85, B: 96, A: 255}
				style.HintColor = color.NRGBA{R: 104, G: 113, B: 123, A: 255}
				return style.Layout(gtx)
			})
		},
	)
}

func (a *App) layoutFormulaFunctionIcon(gtx layout.Context) layout.Dimensions {
	size := gtx.Dp(unit.Dp(20))
	gtx.Constraints.Min = image.Pt(size, size)
	gtx.Constraints.Max = image.Pt(size, size)
	icon, _ := tintedToolbarIcon("sigma", color.NRGBA{R: 104, G: 113, B: 125, A: 255})
	return widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: 20.0 / 96.0}.Layout(gtx)
}

func (a *App) goToCell() {
	r, c, ok := parseCellAddress(a.nameBox.Text())
	if !ok || r > xlsxRows || c > xlsxColumns {
		a.setStatus("单元格地址无效")
		return
	}
	a.navigateToCell(r, c)
}

func (a *App) navigateToCell(row, column int) {
	if a.virtualGrid == nil || row < 1 || column < 1 || row > xlsxRows || column > xlsxColumns {
		return
	}
	v := a.virtualGrid
	a.updateGridSelection(v, CellPosition{Row: row - 1, Column: column - 1}, false, false)
	a.persistViewState()
	x, _ := v.Columns.Boundary(column - 1)
	y, _ := v.Rows.Boundary(row - 1)
	v.scroll(x-v.X, y-v.Y, gtxGridContentWidth(a), gtxGridContentHeight(a))
	v.invalidateWindow()
	v.PendingSelectionLoad = true
	if a.window != nil {
		a.window.Invalidate()
	}
}
