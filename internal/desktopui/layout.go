package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/format/docx"
	"glance/internal/format/xlsx"
)

var (
	pageBackground = color.NRGBA{R: 247, G: 249, B: 252, A: 255}
	gridLine       = color.NRGBA{R: 220, G: 225, B: 232, A: 255}
	selectedFill   = color.NRGBA{R: 221, G: 237, B: 255, A: 255}
	mutedText      = color.NRGBA{R: 91, G: 101, B: 116, A: 255}
)

func (a *App) layout(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min = gtx.Constraints.Max
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions { return fill(gtx, pageBackground) },
		func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
					layout.Rigid(a.layoutToolbar),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return material.Caption(a.theme, a.status).Layout(gtx)
						})
					}),
					layout.Flexed(1, a.layoutDocument),
					layout.Rigid(a.layoutEditor),
				)
			})
		},
	)
}

func (a *App) layoutToolbar(gtx layout.Context) layout.Dimensions {
	if a.chooseButton.Clicked(gtx) {
		path, err := chooseOfficeFile()
		if err != nil {
			a.status = err.Error()
		} else if path != "" {
			if err := a.open(path); err != nil {
				a.status = err.Error()
			}
		}
	}
	if a.openButton.Clicked(gtx) {
		if err := a.open(a.pathEditor.Text()); err != nil {
			a.status = err.Error()
		}
	}
	if a.saveButton.Clicked(gtx) && a.isDirty() {
		if err := a.save(); err != nil {
			a.status = "保存失败：" + err.Error()
		} else {
			a.status = "已保存 " + a.path
		}
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Button(a.theme, &a.chooseButton, "打开…").Layout(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return material.Editor(a.theme, &a.pathEditor, "输入 .xlsx 或 .docx 路径").Layout(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return material.Button(a.theme, &a.openButton, "打开路径").Layout(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			label := "保存"
			if a.isDirty() {
				label = "保存 *"
			}
			return material.Button(a.theme, &a.saveButton, label).Layout(gtx)
		}),
	)
}

func (a *App) layoutDocument(gtx layout.Context) layout.Dimensions {
	switch {
	case a.book != nil:
		return a.layoutWorkbook(gtx)
	case a.word != nil:
		return a.layoutWord(gtx)
	default:
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return material.H6(a.theme, "Glance Office").Layout(gtx)
		})
	}
}

func (a *App) layoutWorkbook(gtx layout.Context) layout.Dimensions {
	if _, ok := a.book.(document.GridReader); ok {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(a.layoutGridControls),
			layout.Flexed(1, a.layoutVirtualGrid),
			layout.Rigid(a.layoutSheetTabs),
		)
	}
	if len(a.grid) == 0 {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(a.layoutSheetTabs),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return material.H6(a.theme, "当前工作表为空，没有读到单元格内容").Layout(gtx)
				})
			}),
		)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutSheetTabs(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutColumnHeader(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return material.List(a.theme, &a.gridList).Layout(gtx, len(a.grid), func(gtx layout.Context, row int) layout.Dimensions {
				return a.layoutGridRow(gtx, row)
			})
		}),
	)
}

func (a *App) layoutGridControls(gtx layout.Context) layout.Dimensions {
	for ev, ok := a.nameBox.Update(gtx); ok; ev, ok = a.nameBox.Update(gtx) {
		if _, submit := ev.(widget.SubmitEvent); submit {
			a.goToCell()
		}
	}
	for ev, ok := a.formulaEditor.Update(gtx); ok; ev, ok = a.formulaEditor.Update(gtx) {
		if _, submit := ev.(widget.SubmitEvent); submit {
			if err := a.commitGridCell(a.formulaEditor.Text()); err != nil {
				a.status = "修改失败：" + err.Error()
			}
		}
	}
	if a.goToCellButton.Clicked(gtx) {
		a.goToCell()
	}
	if a.applyButton.Clicked(gtx) {
		if err := a.commitGridCell(a.formulaEditor.Text()); err != nil {
			a.status = "修改失败：" + err.Error()
		}
	}
	return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return material.Editor(a.theme, &a.nameBox, "A1").Layout(gtx)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return material.Button(a.theme, &a.goToCellButton, "前往").Layout(gtx)
				})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return material.Editor(a.theme, &a.formulaEditor, "单元格内容").Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return material.Button(a.theme, &a.applyButton, "✓").Layout(gtx)
				})
			}),
		)
	})
}

func (a *App) goToCell() {
	r, c, ok := parseCellAddress(a.nameBox.Text())
	if !ok || r > xlsxRows || c > xlsxColumns {
		a.status = "单元格地址无效"
		return
	}
	if a.virtualGrid == nil {
		return
	}
	v := a.virtualGrid
	v.SelectedRow, v.SelectedColumn = r, c
	a.selectedCell = xlsx.CellName(c, r)
	a.cellEdit.selectCell(a.selectedCell, "")
	a.nameBox.SetText(a.selectedCell)
	a.formulaEditor.SetText("")
	x, _ := v.Columns.Boundary(c - 1)
	y, _ := v.Rows.Boundary(r - 1)
	v.scroll(x-v.X, y-v.Y, gtxGridContentWidth(a), gtxGridContentHeight(a))
	v.invalidateWindow()
	v.PendingSelectionLoad = true
	a.window.Invalidate()
}

func gtxGridContentWidth(a *App) float64  { return a.lastGridWidth }
func gtxGridContentHeight(a *App) float64 { return a.lastGridHeight }

func (a *App) layoutVirtualGrid(gtx layout.Context) layout.Dimensions {
	reader := a.book.(document.GridReader)
	if a.virtualGrid == nil {
		a.virtualGrid = newGridViewport()
	}
	v := a.virtualGrid
	width, height := float64(gtx.Constraints.Max.X), float64(gtx.Constraints.Max.Y)
	contentW, contentH := width-gridRowHeader, height-gridColumnHeader
	a.lastGridWidth, a.lastGridHeight = contentW, contentH
	visible := visibleCellWindow(v.Rows, v.Columns, v.X, v.Y, contentW, contentH)
	previousSheet := v.WindowSheet
	if err := v.ensureWindow(reader, a.sheet, visible); err != nil {
		a.status = gridWindowError(a.sheet, err)
	} else if previousSheet != a.sheet {
		start := xlsx.CellName(v.WindowRange.StartColumn, v.WindowRange.StartRow)
		end := xlsx.CellName(v.WindowRange.EndColumn, v.WindowRange.EndRow)
		a.status = fmt.Sprintf("工作表「%s」已读取 %s:%s（当前窗口）", a.sheet, start, end)
	}
	if v.PendingSelectionLoad && v.SelectedRow >= v.WindowRange.StartRow && v.SelectedRow <= v.WindowRange.EndRow && v.SelectedColumn >= v.WindowRange.StartColumn && v.SelectedColumn <= v.WindowRange.EndColumn {
		cell := windowCell(v.Window, v.SelectedRow, v.SelectedColumn)
		a.cellEdit.selectCell(a.selectedCell, cell.Value)
		a.formulaEditor.SetText(cell.Value)
		v.PendingSelectionLoad = false
	}
	for {
		if a.cellEdit.Editing {
			if ev, ok := gtx.Event(key.Filter{Focus: &a.cellEditor, Name: key.NameEscape}); ok {
				if ke, yes := ev.(key.Event); yes && ke.State == key.Press {
					a.cellEdit.cancel()
					a.cellEditor.SetText(a.cellEdit.Original)
					a.formulaEditor.SetText(a.cellEdit.Original)
					gtx.Execute(key.FocusCmd{Tag: &v.PointerTag})
				}
			}
			for ev, ok := a.cellEditor.Update(gtx); ok; ev, ok = a.cellEditor.Update(gtx) {
				if _, submit := ev.(widget.SubmitEvent); submit {
					if err := a.commitGridCell(a.cellEditor.Text()); err != nil {
						a.status = "修改失败：" + err.Error()
					}
					gtx.Execute(key.FocusCmd{Tag: &v.PointerTag})
				}
			}
		}
		ev, ok := gtx.Event(key.Filter{Focus: &v.PointerTag})
		if !ok {
			break
		}
		if ke, yes := ev.(key.Event); yes && ke.State == key.Press && ke.Name == key.NameF2 && a.selectedCell != "" {
			if a.cellEdit.begin() {
				a.cellEditor.SetText(a.cellEdit.Original)
				gtx.Execute(key.FocusCmd{Tag: &a.cellEditor})
			}
		}
	}
	for {
		e, ok := gtx.Event(pointer.Filter{Target: &v.PointerTag, Kinds: pointer.Press | pointer.Scroll, ScrollX: pointer.ScrollRange{Min: -1e9, Max: 1e9}, ScrollY: pointer.ScrollRange{Min: -1e9, Max: 1e9}})
		if !ok {
			break
		}
		pe, ok := e.(pointer.Event)
		if !ok {
			continue
		}
		switch pe.Kind {
		case pointer.Scroll:
			dx, dy := float64(pe.Scroll.X), float64(pe.Scroll.Y)
			if pe.Modifiers.Contain(key.ModShift) {
				dx += dy
				dy = 0
			}
			v.scroll(-dx, -dy, contentW, contentH)
			a.window.Invalidate()
		case pointer.Press:
			row, col, hit := cellAtPoint(v.Rows, v.Columns, v.X, v.Y, image.Pt(int(pe.Position.X), int(pe.Position.Y)), gtx.Dp(unit.Dp(gridRowHeader)), gtx.Dp(unit.Dp(gridColumnHeader)))
			if hit {
				row, col, _ = mergeAnchorAt(v.Window.Merges, row, col)
				v.SelectedRow, v.SelectedColumn = row, col
				a.selectedCell = xlsx.CellName(col, row)
				cell := windowCell(v.Window, row, col)
				if a.cellEdit.Editing {
					a.cellEdit.cancel()
				}
				a.cellEdit.selectCell(a.selectedCell, cell.Value)
				a.nameBox.SetText(a.selectedCell)
				a.formulaEditor.SetText(cell.Value)
				if pe.Time-v.LastClickTime <= 500*time.Millisecond && v.LastClickAddress == a.selectedCell {
					a.cellEdit.begin()
					a.cellEditor.SetText(cell.Value)
					gtx.Execute(key.FocusCmd{Tag: &a.cellEditor})
				} else {
					gtx.Execute(key.FocusCmd{Tag: &v.PointerTag})
				}
				v.LastClickAddress, v.LastClickTime = a.selectedCell, pe.Time
			}
		}
	}
	event.Op(gtx.Ops, &v.PointerTag)
	key.InputHintOp{Tag: &v.PointerTag, Hint: key.HintText}.Add(gtx.Ops)
	pointer.PassOp{}.Push(gtx.Ops)
	return paintVirtualGrid(gtx, a, v)
}

func windowCell(w document.SheetWindow, row, col int) document.Cell {
	for _, m := range w.Merges {
		if row == m.Range.StartRow && col == m.Range.StartColumn {
			return m.Anchor
		}
	}
	r, c := row-w.StartRow, col-w.StartColumn
	if r < 0 || r >= len(w.Cells) || c < 0 || c >= len(w.Cells[r]) {
		return document.Cell{}
	}
	return w.Cells[r][c]
}

func (a *App) layoutSheetTabs(gtx layout.Context) layout.Dimensions {
	return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, len(a.sheets))
		for _, name := range a.sheets {
			sheetName := name
			button := a.sheetButton(sheetName)
			if button.Clicked(gtx) {
				a.sheet = sheetName
				if _, virtual := a.book.(document.GridReader); virtual {
					if a.virtualGrid != nil {
						a.virtualGrid.invalidateWindow()
					}
				} else if err := a.loadSheet(); err != nil {
					a.status = err.Error()
				}
			}
			label := sheetName
			if sheetName == a.sheet {
				label = "• " + label
			}
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return material.Button(a.theme, button, label).Layout(gtx)
				})
			}))
		}
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
	})
}

func (a *App) layoutColumnHeader(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(36)
	gtx.Constraints.Max.Y = gtx.Dp(36)
	cols := previewColumns(a.grid)
	children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return gridHeaderCell(gtx, a.theme, "", unit.Dp(44))
	})}
	children = append(children, columnChildren(a, cols, func(gtx layout.Context, label string) layout.Dimensions {
		return gridHeaderCell(gtx, a.theme, label, unit.Dp(110))
	})...)
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}

func (a *App) layoutGridRow(gtx layout.Context, row int) layout.Dimensions {
	// A List gives each item the remaining viewport as Max.Y. Constrain the
	// item before painting its background, or fill() expands it to that height
	// and the first row hides every later row below a huge blank block.
	gtx.Constraints.Min.Y = gtx.Dp(36)
	gtx.Constraints.Max.Y = gtx.Dp(36)
	cols := previewColumns(a.grid)
	children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return gridHeaderCell(gtx, a.theme, strconv.Itoa(row+1), unit.Dp(44))
	})}
	for col := 0; col < cols; col++ {
		column := col
		ref := xlsx.CellName(column+1, row+1)
		cell := document.Cell{}
		if row < len(a.grid) && column < len(a.grid[row]) {
			cell = a.grid[row][column]
		}
		button := a.cellButton(ref)
		if button.Clicked(gtx) {
			a.selectedCell = ref
			a.cellEditor.SetText(cell.Value)
		}
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(unit.Dp(90))
			return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				label := material.Body2(a.theme, cell.Value)
				if ref == a.selectedCell {
					label.Color = color.NRGBA{R: 25, G: 80, B: 145, A: 255}
				}
				return layout.Background{}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return fill(gtx, cellBackground(ref == a.selectedCell, cell.StyleID))
					},
					func(gtx layout.Context) layout.Dimensions {
						return layout.UniformInset(unit.Dp(5)).Layout(gtx, func(gtx layout.Context) layout.Dimensions { return label.Layout(gtx) })
					},
				)
			})
		}))
	}
	return layout.Inset{Bottom: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions { return fill(gtx, gridLine) },
			func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
			},
		)
	})
}

func (a *App) layoutWord(gtx layout.Context) layout.Dimensions {
	blocks := a.word.Blocks()
	list := a.wordList()
	return material.List(a.theme, list).Layout(gtx, len(blocks), func(gtx layout.Context, index int) layout.Dimensions {
		block := blocks[index]
		if block.Kind == docx.TableBlock {
			return layout.Inset{Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, tableRows(a, block.Rows)...)
			})
		}
		button := a.blockButton(index)
		if button.Clicked(gtx) {
			a.selectedBlock = index
			a.selectedBlockEditable = block.Editable
			a.wordEditor.SetText(block.Text)
		}
		return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return material.Body1(a.theme, block.Text).Layout(gtx)
				})
			})
		})
	})
}

func (a *App) layoutEditor(gtx layout.Context) layout.Dimensions {
	if _, virtual := a.book.(document.GridReader); virtual {
		return layout.Dimensions{}
	}
	if a.book != nil && a.book.Editable() && a.selectedCell != "" {
		if a.applyButton.Clicked(gtx) {
			if err := a.applyCellEdit(); err != nil {
				a.status = "修改失败：" + err.Error()
			} else {
				a.status = a.selectedCell + " 已修改；点击保存写入原文件"
			}
		}
		return editorPanel(gtx, a, a.selectedCell, &a.cellEditor, "应用单元格", &a.applyButton)
	}
	if a.word != nil && a.selectedBlock >= 0 {
		if a.applyButton.Clicked(gtx) {
			if err := a.applyWordEdit(); err != nil {
				a.status = "修改失败：" + err.Error()
			} else {
				a.status = "段落已修改；点击保存写入原文件"
			}
		}
		return editorPanel(gtx, a, "编辑所选段落", &a.wordEditor, "应用段落", &a.applyButton)
	}
	return layout.Dimensions{}
}

func editorPanel(gtx layout.Context, a *App, label string, editor *widget.Editor, action string, button *widget.Clickable) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return material.Caption(a.theme, label).Layout(gtx)
				})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return material.Editor(a.theme, editor, "文本").Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					style := material.Button(a.theme, button, action)
					if a.word != nil && !a.selectedBlockEditable {
						style.Background = color.NRGBA{R: 190, G: 194, B: 201, A: 255}
					}
					return style.Layout(gtx)
				})
			}),
		)
	})
}

func gridHeaderCell(gtx layout.Context, theme *material.Theme, text string, width unit.Dp) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Dp(width)
	gtx.Constraints.Max.X = gtx.Dp(width)
	gtx.Constraints.Min.Y = gtx.Dp(36)
	gtx.Constraints.Max.Y = gtx.Dp(36)
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return fill(gtx, color.NRGBA{R: 235, G: 239, B: 245, A: 255})
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return material.Caption(theme, text).Layout(gtx)
			})
		},
	)
}

func columnChildren(a *App, columns int, widget func(layout.Context, string) layout.Dimensions) []layout.FlexChild {
	children := make([]layout.FlexChild, 0, columns)
	for col := 0; col < columns; col++ {
		label := xlsx.CellName(col+1, 1)
		for i, r := range label {
			if r >= '0' && r <= '9' {
				label = label[:i]
				break
			}
		}
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return widget(gtx, label) }))
	}
	return children
}

func previewColumns(grid [][]document.Cell) int {
	max := 0
	for _, row := range grid {
		if len(row) > max {
			max = len(row)
		}
	}
	return max
}

func cellBackground(selected bool, styleID int) color.NRGBA {
	if selected {
		return selectedFill
	}
	if styleID != 0 {
		return color.NRGBA{R: 249, G: 251, B: 254, A: 255}
	}
	return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
}

func tableRows(a *App, rows [][]string) []layout.FlexChild {
	children := make([]layout.FlexChild, 0, len(rows))
	for _, cells := range rows {
		row := append([]string(nil), cells...)
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			items := make([]layout.FlexChild, 0, len(row))
			for _, text := range row {
				value := text
				items = append(items, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return material.Body2(a.theme, value).Layout(gtx)
					})
				}))
			}
			return layout.Background{}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return fill(gtx, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
				},
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, items...)
				},
			)
		}))
	}
	return children
}

func fill(gtx layout.Context, c color.NRGBA) layout.Dimensions {
	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, c, clip.Rect{Max: size}.Op())
	return layout.Dimensions{Size: size}
}
