package desktopui

import "glance/internal/document"

func (a *App) processSpreadsheetFormatResult() bool {
	select {
	case result := <-a.formatResults:
		if result.state == nil || !result.state.acceptsResult(result.token) || result.state.wordRevisionAtomic.Load() != result.revision {
			return true
		}
		if result.err == nil && result.filter != nil {
			a.installSpreadsheetValueFilter(result.state, result.filter)
		}
		if result.err == nil && result.structure != nil {
			a.refreshAfterStructure(result.state, *result.structure)
		}
		if result.generation == a.documentGeneration && result.state == a.documentSessionState && result.sheet == a.sheet {
			if result.err != nil {
				a.setStatus("格式应用失败：" + result.err.Error())
				if result.operation != "" {
					a.setStatus(result.operation + "失败：" + result.err.Error())
				}
			} else if !result.history || result.applied {
				a.setStatus("已应用单元格格式")
				if result.operation != "" {
					a.setStatus("已完成" + result.operation)
				}
				switch result.operation {
				case "替换单元格":
					a.setStatus("已替换当前单元格")
					if a.modal.Visible && a.modal.SearchReplace {
						a.modal.SearchReplaceSummary = "已替换当前单元格"
						a.modal.FocusInputRequested = true
					}
				case "重新计算公式":
					a.setStatus("已刷新公式结果（不包含外部数据连接）")
				case "刷新全部工作表":
					result.state.sheets = result.state.book.SheetNames()
					a.setStatus("已刷新全部工作表公式结果（不包含外部数据连接）")
				}
				switch result.operation {
				case "保护工作表":
					result.state.spreadsheetSheetProtected = true
				case "撤销保护工作表":
					result.state.spreadsheetSheetProtected = false
				case "保护工作簿":
					result.state.spreadsheetWorkbookProtected = true
				case "撤销保护工作簿":
					result.state.spreadsheetWorkbookProtected = false
				}
				if result.history {
					if result.undo {
						a.setStatus("已撤销")
					} else {
						a.setStatus("已重做")
					}
					if ed, ok := result.state.book.(document.SpreadsheetProtectionEditor); ok {
						result.state.spreadsheetSheetProtected, _ = ed.GetSheetProtection(result.sheet)
					}
					if ed, ok := result.state.book.(document.SpreadsheetWorkbookProtectionEditor); ok {
						result.state.spreadsheetWorkbookProtected, _ = ed.GetWorkbookProtection()
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
			if result.err == nil && result.operation == "拼写检查" {
				a.modal.Show("拼写检查", result.details, func(action string) {
					a.modal.Hide()
					a.setStatus("文本检查完成；未使用词典判断拼写")
					if a.window != nil {
						a.window.Invalidate()
					}
				}, modalDialogActionSpec{ID: "confirm", Label: "完成"})
			}
		}
	default:
	}
	return false
}

func (a *App) installSpreadsheetValueFilter(state *documentSessionState, filter *ValueFilter) {
	if state == nil || filter == nil {
		return
	}
	if state.valueFilter != nil {
		a.clearValueFilter(state)
	}
	filter.OriginalHeights = make(map[int]float64)
	if state.virtualGrid != nil {
		for row := filter.Bounds.StartRow; row <= filter.Bounds.EndRow; row++ {
			index := row - 1
			base := state.virtualGrid.Rows.sizeAt(index)
			if offset := row - filter.Bounds.StartRow; offset >= 0 && offset < len(filter.RowHeightsPoints) && filter.RowHeightsPoints[offset] > 0 {
				base = pointsToGridPixels(filter.RowHeightsPoints[offset]) * state.virtualGrid.Scale
			}
			filter.OriginalHeights[index] = base
		}
	}
	state.valueFilter = filter
	a.applyValueFilterWindow(state, state.virtualGrid)
}
