package desktopui

import (
	"glance/internal/i18n"

	"glance/internal/document"
)

func (a *App) processSpreadsheetFormatResult() bool {
	select {
	case result := <-a.formatResults:
		if result.state == nil || !result.state.acceptsResult(result.token) || result.state.wordRevisionAtomic.Load() != result.revision {
			return true
		}
		if result.err == nil && result.merge != nil && result.merge.ConfirmationRequired {
			if result.mergeBounds != nil && result.generation == a.documentGeneration && result.state == a.documentSessionState && result.sheet == a.sheet {
				a.showSpreadsheetMergeConfirmation(result.state, result.generation, result.sheet, *result.mergeBounds, *result.merge)
			}
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
				a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.error.operation_failed"}, result.err, notificationToast)
				if result.operation != "" {
					a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.error.operation_failed"}, result.err, notificationToast)
				}
			} else if result.noOp {
				if result.details.ID != "" {
					a.setStatusMessage(result.details, feedbackInfo, notificationSuppressed)
				} else {
					a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.no_changes"}, feedbackInfo, notificationSuppressed)
				}
			} else if !result.history || result.applied {
				if result.details.ID != "" {
					a.setStatusDetailMessage(result.details, feedbackSuccess, notificationToast, result.detail)
				} else if result.operation != "" {
					a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.operation_completed", Args: i18n.Args{"Operation": a.localizedSpreadsheetToolbarText(result.operation)}}, feedbackSuccess, notificationToast)
				} else {
					a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.cell_formatting_applied_209933"}, feedbackSuccess, notificationSuppressed)
				}
				switch result.operation {
				case "向下填充":
					if result.window != nil && result.state.virtualGrid == result.window && result.state.sheet == result.sheet {
						result.window.PendingSelectionLoad = true
					}
				case "合并后居中":
					if result.merge != nil && result.merge.Unmerged {
						a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.cells_unmerged_b88b83"}, feedbackSuccess, notificationToast)
					} else {
						a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.cells_merged_and_centered_65ffe2"}, feedbackSuccess, notificationToast)
					}
					if result.window != nil && result.state.virtualGrid == result.window && result.state.sheet == result.sheet {
						result.window.PendingSelectionLoad = true
					}
				case "插入日期":
					if result.window != nil && result.state.virtualGrid == result.window && result.state.sheet == result.sheet {
						result.window.PendingSelectionLoad = true
					}
				case "插入时间":
					if result.window != nil && result.state.virtualGrid == result.window && result.state.sheet == result.sheet {
						result.window.PendingSelectionLoad = true
					}
				case "求和", "平均值", "计数", "最大值", "最小值":
					if result.details.ID != "" {
						a.setStatusDetailMessage(result.details, feedbackSuccess, notificationToast, result.detail)
					}
				case "调整行高", "调整列宽":
					if len(result.dimensions) > 0 {
						change := result.dimensions[0]
						unitName := "行"
						if !change.row {
							unitName = "列"
						}
						a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.row_column_resized", Args: i18n.Args{"Index": change.index, "Unit": a.localizedSpreadsheetToolbarText(unitName)}}, feedbackSuccess, notificationToast)
					}
				case "自动调整行高", "自动调整列宽":
					a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.dimensions.autofit"}, feedbackSuccess, notificationToast)
				case "替换单元格":
					a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.current_cell_replaced_cbc1b4"}, feedbackSuccess, notificationToast)
					if a.modal.Visible && a.modal.SearchReplace {
						a.modal.SetSearchReplaceSummaryRef(i18n.Message{ID: "spreadsheet.replace.status.replaced_current"})
						a.modal.FocusInputRequested = true
					}
				case "替换全部":
					if result.details.ID != "" {
						a.setStatusMessage(result.details, feedbackSuccess, notificationToast)
					}
					if a.modal.Visible && a.modal.SearchReplace {
						a.modal.SetSearchReplaceSummaryRef(result.details)
						a.modal.FocusInputRequested = true
					}
				case "重新计算公式":
					a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.formula_results_refreshed_external_data_db9373"}, feedbackSuccess, notificationToast)
				case "刷新全部工作表":
					result.state.sheets = result.state.book.SheetNames()
					a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.formula_results_refreshed_on_all_6971a2"}, feedbackSuccess, notificationToast)
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
						a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.undid_the_last_action_fef96e"}, feedbackSuccess, notificationToast)
					} else {
						a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.redid_the_last_action_12998e"}, feedbackSuccess, notificationToast)
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
				if result.details.ID != "" {
					a.setStatusDetailMessage(result.details, feedbackInfo, notificationToast, result.detail)
				}
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
