package desktopui

import "glance/internal/document"

func (a *App) processSpreadsheetFormatResult() bool {
	select {
	case result := <-a.formatResults:
		if result.state == nil || !result.state.acceptsResult(result.token) || result.state.wordRevisionAtomic.Load() != result.revision {
			return true
		}
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
		}
	default:
	}
	return false
}
