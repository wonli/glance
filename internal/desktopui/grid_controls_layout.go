package desktopui

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"glance/internal/document"
)

const formulaEditorOpticalCenterTopInset unit.Dp = 4

func (a *App) layoutGridControls(gtx layout.Context) layout.Dimensions {
	for ev, ok := a.nameBox.Update(gtx); ok; ev, ok = a.nameBox.Update(gtx) {
		if _, submit := ev.(widget.SubmitEvent); submit && a.goToCell(gtx) {
			a.focusSpreadsheetGrid(gtx)
		}
	}
	if !editorHasActiveComposition(&a.nameBox) {
		for ev, ok := gtx.Event(key.Filter{Focus: &a.nameBox, Name: key.NameEscape}); ok; ev, ok = gtx.Event(key.Filter{Focus: &a.nameBox, Name: key.NameEscape}) {
			if pressed, yes := ev.(key.Event); yes && pressed.State == key.Press {
				a.nameBox.SetText(a.selectedCell)
				a.focusSpreadsheetGrid(gtx)
				break
			}
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
	formulaCommitted := a.handleSpreadsheetEditorNavigation(gtx, a.virtualGrid, &a.formulaEditor)
	if !formulaCommitted {
		for ev, ok := a.formulaEditor.Update(gtx); ok; ev, ok = a.formulaEditor.Update(gtx) {
			if _, submit := ev.(widget.SubmitEvent); submit {
				if err := a.commitGridCell(a.formulaEditor.Text()); err != nil {
					a.setStatus("修改失败：" + err.Error())
					a.window.Invalidate()
				} else {
					formulaCommitted = true
					a.moveSpreadsheetSelectionAfterEntry(a.virtualGrid, key.NameDownArrow)
					if a.virtualGrid != nil {
						gtx.Execute(key.FocusCmd{Tag: &a.virtualGrid.PointerTag})
					}
				}
			}
		}
	}
	if !formulaCommitted {
		a.syncFormulaReferenceInput(gtx)
	}
	if a.goToCellButton.Clicked(gtx) {
		if a.goToCell(gtx) {
			a.focusSpreadsheetGrid(gtx)
		}
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

func (a *App) focusSpreadsheetNameBox(gtx layout.Context) {
	if a.selectedCell == "" {
		return
	}
	a.nameBox.SetText(a.selectedCell)
	a.nameBox.SetCaret(0, a.nameBox.Len())
	gtx.Execute(key.FocusCmd{Tag: &a.nameBox})
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) focusSpreadsheetGrid(gtx layout.Context) {
	if a.virtualGrid == nil {
		return
	}
	gtx.Execute(key.FocusCmd{Tag: &a.virtualGrid.PointerTag})
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) goToCell(gtx layout.Context) bool {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		a.setStatus("当前工作簿不可用")
		return false
	}
	reference := strings.TrimSpace(a.nameBox.Text())
	targetSheet, start, end, ok := parseSpreadsheetGoToReference(reference, state.sheet, nil)
	if !ok {
		if reader, supported := state.book.(document.SpreadsheetDefinedNameEditor); supported {
			names, err := reader.GetDefinedNames()
			if err != nil {
				a.setStatus("读取定义名称失败：" + err.Error())
				return false
			}
			targetSheet, start, end, ok = parseSpreadsheetGoToReference(reference, state.sheet, names)
		}
	}
	if !ok {
		a.setStatus("单元格、区域地址或定义名称无效")
		return false
	}
	if strings.EqualFold(targetSheet, state.sheet) {
		targetSheet = state.sheet
	} else {
		canonicalSheet := ""
		for _, sheet := range state.sheets {
			if strings.EqualFold(sheet, targetSheet) {
				canonicalSheet = sheet
				break
			}
		}
		if canonicalSheet == "" {
			a.setStatus("目标工作表不存在：" + targetSheet)
			return false
		}
		targetSheet = canonicalSheet
	}
	if targetSheet != state.sheet {
		if !a.activateSpreadsheetSheet(gtx, targetSheet) {
			return false
		}
	}
	a.navigateToRange(start, end)
	return true
}

func (a *App) navigateToCell(row, column int) {
	if row < 1 || column < 1 || row > xlsxRows || column > xlsxColumns {
		return
	}
	position := CellPosition{Row: row - 1, Column: column - 1}
	a.navigateToRange(position, position)
}

func (a *App) navigateToRange(first, last CellPosition) {
	if a.virtualGrid == nil || first.Row < 0 || first.Column < 0 || last.Row < 0 || last.Column < 0 ||
		first.Row >= xlsxRows || first.Column >= xlsxColumns || last.Row >= xlsxRows || last.Column >= xlsxColumns {
		return
	}
	state, v := a.documentSessionState, a.virtualGrid
	if state == nil {
		return
	}
	// Keep the first typed reference as Excel's active cell while the opposite
	// endpoint provides the fixed selection anchor.
	a.updateGridSelection(v, first, false, false)
	state.selection = CellSelection{Anchor: last, Active: first}
	state.selectionSet = true
	a.persistViewState()
	revealGridCell(v, first, gtxGridContentWidth(a), gtxGridContentHeight(a))
	v.PendingSelectionLoad = true
	v.invalidateWindow()
	if a.window != nil {
		a.window.Invalidate()
	}
}
