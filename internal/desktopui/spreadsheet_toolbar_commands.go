package desktopui

import "glance/internal/document"

func (a *App) recalculateSpreadsheet() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	if ed, ok := state.book.(document.SpreadsheetRecalculateEditor); ok {
		_ = ed.RecalculateFormulas()
	}
	if state.virtualGrid != nil {
		state.virtualGrid.invalidateWindow()
	}
	a.setStatus("已重新计算工作簿公式")
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) refreshAllSpreadsheet() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	if ed, ok := state.book.(document.SpreadsheetRecalculateEditor); ok {
		_ = ed.RecalculateFormulas()
	}
	if state.virtualGrid != nil {
		state.virtualGrid.invalidateWindow()
	}
	state.sheets = state.book.SheetNames()
	a.setStatus("已刷新全部工作表数据")
	if a.window != nil {
		a.window.Invalidate()
	}
}
