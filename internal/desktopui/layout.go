package desktopui

import (
	"image"
	"image/color"
	"path/filepath"
	"strconv"

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
	a.processAsyncResults()
	gtx.Constraints.Min = gtx.Constraints.Max
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions { return fill(gtx, pageBackground) },
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
				layout.Rigid(a.layoutWorkspaceTabs),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if a.showNewMenu {
						return a.layoutCreateActions(gtx)
					}
					return layout.Dimensions{}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return material.Caption(a.theme, a.status).Layout(gtx)
					})
				}),
				layout.Flexed(1, a.layoutDocument),
				layout.Rigid(a.layoutEditor),
			)
		},
	)
}

func (a *App) processAsyncResults() {
	select {
	case err := <-a.workspaceWriteResults:
		a.status = "工作区保存失败：" + err.Error()
	default:
	}
	select {
	case result := <-a.openResults:
		a.opening = false
		if result.err != nil {
			a.status = result.err.Error()
		} else if err := a.installDocument(result.loaded); err != nil {
			a.status = err.Error()
		}
	default:
	}
	select {
	case result := <-a.saveResults:
		a.saving = false
		if result.err != nil {
			a.status = "保存失败：" + result.err.Error()
		} else {
			if result.saveAs {
				if state := a.sessions[result.tabID]; state != nil {
					state.path = result.path
					if state.book != nil {
						if saver, ok := state.book.(interface{ SetSourcePath(string) }); ok {
							saver.SetSourcePath(result.path)
						}
					}
					if state.word != nil {
						state.word.SetSourcePath(result.path)
					}
					if state.jsonDoc != nil {
						state.jsonDoc.SetSourcePath(result.path)
					}
				}
				if meta, ok := a.tabMeta[result.tabID]; ok {
					meta.SourcePath = result.path
					meta.Title = filepath.Base(result.path)
					a.tabMeta[result.tabID] = meta
				}
				if result.tabID == a.activeTabID {
					a.path = result.path
					a.documentSessionState.path = result.path
				}
			}
			a.status = "已保存 " + result.path
			if state := a.sessions[result.tabID]; state != nil {
				a.updatePersistedTab(a.tabMeta[result.tabID])
				if result.tabID == a.activeTabID {
					a.persistViewState()
				}
			}
		}
	default:
	}
}

func (a *App) layoutDocument(gtx layout.Context) layout.Dimensions {
	if a.activeTabID == "recent" || (a.book == nil && a.word == nil && a.jsonDoc == nil) {
		return a.layoutRecent(gtx)
	}
	switch {
	case a.book != nil:
		return a.layoutWorkbook(gtx)
	case a.word != nil:
		return a.layoutWord(gtx)
	case a.jsonDoc != nil:
		return a.layoutJSON(gtx)
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

func (a *App) layoutSheetTabs(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = gtx.Dp(unit.Dp(34)), gtx.Dp(unit.Dp(34))
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return fill(gtx, color.NRGBA{R: 239, G: 242, B: 246, A: 255})
		},
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, color.NRGBA{R: 202, G: 208, B: 216, A: 255}, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(1)))}.Op())
			children := make([]layout.FlexChild, 0, len(a.sheets))
			for _, name := range a.sheets {
				sheetName := name
				button := a.sheetButton(sheetName)
				if button.Clicked(gtx) {
					if a.cellEdit.Editing {
						if err := a.commitGridCell(a.cellEditor.Text()); err != nil {
							a.status = "修改失败：" + err.Error()
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
						a.selectedCell = "A1"
						a.cellEdit.selectCell("A1", "")
						a.nameBox.SetText("A1")
						a.formulaEditor.SetText("")
					} else if err := a.loadSheet(); err != nil {
						a.status = err.Error()
					}
					a.persistViewState()
				}
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(1), Left: unit.Dp(3), Right: unit.Dp(3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layoutSheetTab(gtx, a.theme, button, sheetName, sheetName == a.sheet)
					})
				}))
			}
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
		},
	)
}

func layoutSheetTab(gtx layout.Context, theme *material.Theme, button *widget.Clickable, name string, selected bool) layout.Dimensions {
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bg := color.NRGBA{R: 230, G: 234, B: 240, A: 255}
		fg := color.NRGBA{R: 70, G: 78, B: 88, A: 255}
		if selected {
			bg = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			fg = color.NRGBA{R: 35, G: 43, B: 53, A: 255}
		}
		label := material.Body2(theme, name)
		label.Color = fg
		return layout.Stack{}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				size := gtx.Constraints.Min
				paint.FillShape(gtx.Ops, bg, clip.Rect{Max: size}.Op())
				if selected {
					paint.FillShape(gtx.Ops, color.NRGBA{R: 35, G: 125, B: 85, A: 255}, clip.Rect{Max: image.Pt(size.X, gtx.Dp(unit.Dp(2)))}.Op())
				}
				return layout.Dimensions{Size: size}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(31))
				return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Center.Layout(gtx, label.Layout)
				})
			}),
		)
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
			a.persistViewState()
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
	if a.jsonDoc != nil {
		return layout.Dimensions{}
	}
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
