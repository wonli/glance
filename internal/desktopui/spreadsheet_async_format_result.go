package desktopui

import (
	"strconv"

	"glance/internal/document"
)

func (a *App) processSpreadsheetFormatResult() bool {
	select {
	case result := <-a.formatResults:
		if result.state == nil || !result.state.acceptsResult(result.token) || result.state.wordRevisionAtomic.Load() != result.revision {
			return true
		}
		if result.replaceAll != nil {
			a.applySpreadsheetReplaceAllPlan(result)
			return true
		}
		if result.err == nil && result.filter != nil {
			a.installSpreadsheetValueFilter(result.state, result.filter)
		}
		if result.err == nil && result.structure != nil {
			a.refreshAfterStructure(result.state, *result.structure)
		}
		gridStillShowsResultSheet := result.window != nil && (result.window.WindowSheet == "" || result.window.WindowSheet == result.sheet)
		if result.err == nil && len(result.dimensions) > 0 && result.window != nil && result.state.virtualGrid == result.window && result.state.sheet == result.sheet && gridStillShowsResultSheet {
			for _, change := range result.dimensions {
				if change.row {
					if change.autoFit {
						if result.state.spreadsheetAutoFitRowBasePoints == nil {
							result.state.spreadsheetAutoFitRowBasePoints = make(map[gridAutoFitRow]float64)
						}
						result.state.spreadsheetAutoFitRowBasePoints[gridAutoFitRow{Sheet: result.sheet, Row: change.index - 1}] = change.value
					} else {
						result.state.markGridRowHeightManual(result.sheet, change.index-1)
					}
					setGridRowSizePreservingViewport(result.window, change.index-1, pointsToGridPixels(change.value)*result.window.Scale)
				} else {
					setGridColumnSizePreservingViewport(result.window, change.index-1, columnCharsToGridPixels(change.value)*result.window.Scale)
				}
			}
		}
		if result.err == nil && result.panes != nil && result.window != nil && result.state.virtualGrid == result.window && result.state.sheet == result.sheet && gridStillShowsResultSheet {
			result.window.setPanes(*result.panes)
		}
		if result.generation == a.documentGeneration && result.state == a.documentSessionState && result.sheet == a.sheet {
			if result.err == nil && result.rangeEntry != nil && result.state.cellEdit.Editing && result.state.cellEdit.Draft == *result.rangeEntry {
				result.state.cellEdit.selectCell(result.state.cellEdit.Address, *result.rangeEntry)
				a.cellEditor.SetText(*result.rangeEntry)
				a.formulaEditor.SetText(*result.rangeEntry)
				if result.window != nil {
					result.window.KeyboardFocusRequested = true
					result.window.PendingSelectionLoad = true
				}
			}
			if result.err != nil {
				a.setStatus("格式应用失败：" + result.err.Error())
				if result.operation != "" {
					a.setStatus(result.operation + "失败：" + result.err.Error())
				}
			} else if result.noOp {
				a.setStatus(result.details)
			} else if !result.history || result.applied {
				a.setStatus("已应用单元格格式")
				if result.operation != "" {
					a.setStatus("已完成" + result.operation)
				}
				switch result.operation {
				case "求和", "平均值", "计数", "最大值", "最小值":
					a.setStatus(result.details)
				case "调整行高", "调整列宽":
					if len(result.dimensions) > 0 {
						change := result.dimensions[0]
						unitName := "行"
						if !change.row {
							unitName = "列"
						}
						a.setStatus("第 " + strconv.Itoa(change.index) + " " + unitName + "已调整；点击保存写入原文件")
					}
				case "自动调整行高", "自动调整列宽":
					a.setStatus(result.details)
				case "替换单元格":
					a.setStatus("已替换当前单元格")
					if a.modal.Visible && a.modal.SearchReplace {
						a.modal.SearchReplaceSummary = "已替换当前单元格"
						a.modal.FocusInputRequested = true
					}
				case "替换全部":
					a.setStatus(result.details)
					if a.modal.Visible && a.modal.SearchReplace {
						a.modal.SearchReplaceSummary = result.details
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
					if result.applied {
						// Undo/redo can restore row dimensions from any workbook edit.
						// Rebase automatic-row sizing on the refreshed grid geometry.
						result.state.spreadsheetAutoFitRowBasePoints = nil
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
