package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"strconv"
	"strings"

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
	pageBackground           = color.NRGBA{R: 247, G: 248, B: 250, A: 255}
	chromeSurface            = color.NRGBA{R: 241, G: 243, B: 245, A: 255}
	workspaceTabStripSurface = color.NRGBA{R: 238, G: 241, B: 244, A: 255}
	tabSurface               = color.NRGBA{R: 238, G: 241, B: 244, A: 255}
	contentSurface           = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	gridHeaderFill           = color.NRGBA{R: 242, G: 244, B: 246, A: 255}
	gridLine                 = color.NRGBA{R: 216, G: 221, B: 227, A: 255}
	selectedFill             = color.NRGBA{R: 231, G: 240, B: 251, A: 255}
	mutedText                = color.NRGBA{R: 104, G: 113, B: 123, A: 255}
)

func (a *App) layout(gtx layout.Context) layout.Dimensions {
	a.processAsyncResults()
	gtx.Constraints.Min = gtx.Constraints.Max
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions { return fill(gtx, pageBackground) },
		func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{Alignment: layout.Center}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
						layout.Rigid(a.layoutWorkspaceTabs),
						layout.Flexed(1, a.layoutWorkspaceBody),
					)
				}),
				layout.Stacked(a.layoutModalDialog),
			)
		},
	)
}

// layoutWorkspaceBody keeps the document as the only content directly below
// the tabs. Transient actions float over it and do not change its geometry.
func (a *App) layoutWorkspaceBody(gtx layout.Context) layout.Dimensions {
	return layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
				layout.Flexed(1, a.layoutDocument),
				layout.Rigid(a.layoutEditor),
			)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if !a.showNewMenu {
				return layout.Dimensions{}
			}
			return layout.Inset{Left: unit.Dp(12)}.Layout(gtx, a.layoutCreateActions)
		}),
	)
}

func (a *App) processAsyncResults() {
	select {
	case result := <-a.formatResults:
		result.state.formatRunning = false
		if result.err == nil && result.filter != nil {
			if result.state.valueFilter != nil {
				a.clearValueFilter(result.state)
			}
			result.filter.OriginalHeights = make(map[int]float64)
			if result.state.virtualGrid != nil {
				for row := result.filter.Bounds.StartRow; row <= result.filter.Bounds.EndRow; row++ {
					index := row - 1
					base := result.state.virtualGrid.Rows.sizeAt(index)
					if offset := row - result.filter.Bounds.StartRow; offset >= 0 && offset < len(result.filter.RowHeightsPoints) && result.filter.RowHeightsPoints[offset] > 0 {
						base = pointsToGridPixels(result.filter.RowHeightsPoints[offset]) * result.state.virtualGrid.Scale
					}
					result.filter.OriginalHeights[index] = base
				}
			}
			result.state.valueFilter = result.filter
			a.applyValueFilterWindow(result.state, result.state.virtualGrid)
		}
		if result.err == nil && result.structure != nil {
			a.refreshAfterStructure(result.state, *result.structure)
		}
		if result.generation == a.documentGeneration && result.state == a.documentSessionState && result.sheet == a.sheet {
			if result.err != nil {
				a.status = "格式应用失败：" + result.err.Error()
				if result.operation != "" {
					a.status = result.operation + "失败：" + result.err.Error()
				}
			} else if !result.history || result.applied {
				a.status = "已应用单元格格式"
				if result.operation != "" {
					a.status = "已完成" + result.operation
				}
				if result.history {
					if result.undo {
						a.status = "已撤销"
					} else {
						a.status = "已重做"
					}
				}
				if result.window != nil {
					result.window.invalidateWindow()
				}
				a.persistActiveSession()
			}
			if a.window != nil {
				a.window.Invalidate()
			}
		}
	default:
	}
	select {
	case result := <-a.fileCommandResults:
		a.fileCommandRunning = false
		if result.err != nil {
			a.status = result.err.Error()
			if result.kind == commandSaveAs && a.pendingCloseTabID != "" {
				a.showCloseConfirmation(a.pendingCloseTabID, "保存失败", "无法选择另存为位置。可以重试保存、不保存并关闭，或取消关闭。")
			}
		} else {
			switch result.kind {
			case commandOpenFile:
				if result.path != "" {
					a.startOpen(result.path)
				} else {
					a.status = "已取消打开文件"
				}
			case commandOpenFolder:
				if result.path == "" {
					a.status = "已取消打开文件夹"
				} else if a.collection == nil {
					a.status = "工作区数据库不可用，无法记录文件夹内容"
				} else if len(result.recents) == 0 {
					a.status = "所选文件夹中没有支持的文档"
				} else {
					a.collection.StageRecents(result.recents)
					a.persistRecentsAsync()
					a.status = fmt.Sprintf("已添加 %d 个文档到最近打开", len(result.recents))
				}
			case commandSaveAs:
				if result.path != "" {
					a.startSaveAs(result.path)
				} else {
					a.status = "已取消另存为"
					if a.pendingCloseTabID != "" {
						a.pendingCloseTabID = ""
					}
				}
			}
		}
	default:
	}
	select {
	case err := <-a.workspaceWriteResults:
		if err != nil {
			a.workspaceWriteError = "工作区保存失败：" + err.Error()
		} else {
			a.workspaceWriteError = ""
		}
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
			if result.tabID == a.pendingCloseTabID && result.tabID != "" {
				a.showCloseConfirmation(result.tabID, "保存失败", "保存没有完成。可以重试保存、不保存并关闭，或取消关闭。")
			}
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
			if result.tabID != "" && result.tabID == a.pendingCloseTabID {
				a.pendingCloseTabID = ""
				a.closeWorkspaceTab(result.tabID)
			}
		}
	default:
	}
}

func (a *App) refreshAfterStructure(state *documentSessionState, edit document.StructureEdit) {
	oldSheet := state.sheet
	state.sheets = state.book.SheetNames()
	switch edit.Kind {
	case document.StructureCreateSheet:
		if len(state.sheets) > 0 {
			state.sheet = state.sheets[len(state.sheets)-1]
		}
	case document.StructureRenameSheet:
		if strings.EqualFold(oldSheet, edit.Sheet) {
			state.sheet = strings.TrimSpace(edit.NewName)
		}
	case document.StructureDeleteSheet:
		if strings.EqualFold(oldSheet, edit.Sheet) && len(state.sheets) > 0 {
			state.sheet = state.sheets[0]
		}
	}
	if edit.Kind == document.StructureInsertRows || edit.Kind == document.StructureDeleteRows || edit.Kind == document.StructureInsertColumns || edit.Kind == document.StructureDeleteColumns {
		state.selection.Anchor = rebasePosition(state.selection.Anchor, edit)
		state.selection.Active = rebasePosition(state.selection.Active, edit)
	} else {
		state.selection = CellSelection{}
	}
	state.selectionSet = true
	active := state.selection.Active
	state.selectedCell = xlsx.CellName(active.Column+1, active.Row+1)
	state.cellEdit.selectCell(state.selectedCell, "")
	state.nameBox.SetText(state.selectedCell)
	state.formulaEditor.SetText("")
	if state.virtualGrid != nil {
		state.virtualGrid.SelectedRow, state.virtualGrid.SelectedColumn = active.Row+1, active.Column+1
		state.virtualGrid.PendingSelectionLoad = true
		state.virtualGrid.invalidateWindow()
	}
	if state == a.documentSessionState {
		a.sheet = state.sheet
		a.sheets = state.sheets
	}
	state.sheetNameEditor.SetText("")
}

func rebasePosition(position CellPosition, edit document.StructureEdit) CellPosition {
	coordinate := position.Row
	if edit.Kind == document.StructureInsertColumns || edit.Kind == document.StructureDeleteColumns {
		coordinate = position.Column
	}
	index := edit.Index - 1
	switch edit.Kind {
	case document.StructureInsertRows, document.StructureInsertColumns:
		if coordinate >= index {
			coordinate += edit.Count
		}
	case document.StructureDeleteRows, document.StructureDeleteColumns:
		if coordinate >= index+edit.Count {
			coordinate -= edit.Count
		} else if coordinate >= index {
			coordinate = index
		}
	}
	if coordinate < 0 {
		coordinate = 0
	}
	if edit.Kind == document.StructureInsertColumns || edit.Kind == document.StructureDeleteColumns {
		position.Column = coordinate
	} else {
		position.Row = coordinate
	}
	return position
}

func (a *App) visibleStatus() string {
	if a.workspaceWriteError != "" {
		return a.workspaceWriteError
	}
	return a.status
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
		a.handleSpreadsheetToolbarEvents(gtx)
		return layout.Stack{Alignment: layout.NW}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(a.layoutSpreadsheetToolbar),
					layout.Rigid(a.layoutGridControls),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, a.layoutVirtualGrid)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, a.layoutSheetTabs)
					}),
					layout.Rigid(a.layoutSpreadsheetStatusBar),
				)
			}),
			layout.Stacked(a.layoutSpreadsheetToolbarPickerOverlay),
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
						a.documentSessionState.selection = CellSelection{}
						a.documentSessionState.selectionSet = true
						a.documentSessionState.selectionDragging = false
						a.documentSessionState.cellEdit.selectCell("A1", "")
						a.nameBox.SetText("A1")
						a.formulaEditor.SetText("")
					} else if err := a.loadSheet(); err != nil {
						a.status = err.Error()
					}
					a.persistViewState()
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
	return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceEnd}.Layout(gtx, children...)
}

func layoutSheetTab(gtx layout.Context, theme *material.Theme, button *widget.Clickable, name string, selected bool) layout.Dimensions {
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		height := gtx.Dp(unit.Dp(26))
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		bg := chromeSurface
		fg := color.NRGBA{R: 104, G: 113, B: 123, A: 255}
		if selected {
			bg = contentSurface
			fg = color.NRGBA{R: 62, G: 111, B: 169, A: 255}
		}
		label := material.Body2(theme, name)
		label.Color = fg
		label.TextSize = unit.Sp(12)
		return layout.Stack{}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				size := gtx.Constraints.Min
				paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(unit.Dp(4))).Op(gtx.Ops))
				return layout.Dimensions{Size: size}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Center.Layout(gtx, label.Layout)
				})
			}),
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

func (a *App) layoutSpreadsheetStatusBar(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(50))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, chromeSurface, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			status := a.status
			if status == "" {
				status = "就绪"
			}
			if runes := []rune(status); len(runes) > 18 {
				status = string(runes[:17]) + "…"
			}
			return layout.Inset{Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return spreadsheetStatusText(gtx, a.theme, status)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(22)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							index := 0
							for i, name := range a.sheets {
								if name == a.sheet {
									index = i + 1
									break
								}
							}
							if index == 0 {
								index = 1
							}
							return spreadsheetStatusText(gtx, a.theme, fmt.Sprintf("工作表 %d", index))
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions { return spreadsheetStatusIcon(gtx, "minus") }),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Left: unit.Dp(6), Right: unit.Dp(13)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										railWidth, railHeight := gtx.Dp(unit.Dp(110)), gtx.Dp(unit.Dp(4))
										paint.FillShape(gtx.Ops, color.NRGBA{R: 205, G: 211, B: 218, A: 255}, clip.UniformRRect(image.Rect(0, 0, railWidth, railHeight), railHeight/2).Op(gtx.Ops))
										return layout.Dimensions{Size: image.Pt(railWidth, railHeight)}
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions { return spreadsheetStatusIcon(gtx, "plus") }),
							)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(32)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return spreadsheetStatusText(gtx, a.theme, "100%")
						})
					}),
				)
			})
		},
	)
}

func spreadsheetStatusText(gtx layout.Context, theme *material.Theme, text string) layout.Dimensions {
	style := material.Body2(theme, text)
	style.TextSize = unit.Sp(12)
	style.Color = mutedText
	return style.Layout(gtx)
}

func spreadsheetStatusIcon(gtx layout.Context, name string) layout.Dimensions {
	icon, _ := tintedToolbarIcon(name, color.NRGBA{R: 105, G: 114, B: 125, A: 255})
	return widget.Image{Src: paint.NewImageOp(icon), Fit: widget.Contain, Position: layout.Center, Scale: 18.0 / 96.0}.Layout(gtx)
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
			return fill(gtx, gridHeaderFill)
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
		return contentSurface
	}
	return contentSurface
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
					return fill(gtx, contentSurface)
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
