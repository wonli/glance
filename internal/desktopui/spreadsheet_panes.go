package desktopui

import (
	"fmt"
	"glance/internal/document"
	"glance/internal/format/xlsx"
)

func (a *App) runSpreadsheetPanesCommand(operation string, edit func(document.SpreadsheetPanesEditor) error) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetPanesEditor)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	go func() {
		unlock := lockSessionAdapterWrite(state)
		err := edit(editor)
		unlock()
		a.completeSessionOperation(state, token, err, true)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation}:
		case <-a.shutdown:
		}
	}()
}

func (a *App) showSpreadsheetFreezePanesDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetPanesEditor)
	if !ok {
		return
	}
	sheet := state.sheet
	panes, _ := editor.GetSheetPanes(sheet)

	var activeCol, activeRow int
	if state.selectionSet {
		activeCol = state.selection.Active.Column
		activeRow = state.selection.Active.Row
	}

	actions := []modalDialogActionSpec{
		{ID: "freeze_row", Label: "冻结首行"},
		{ID: "freeze_col", Label: "冻结首列"},
	}
	if activeCol > 0 || activeRow > 0 {
		cellRef := xlsx.CellName(activeCol+1, activeRow+1)
		actions = append(actions, modalDialogActionSpec{ID: "freeze_sel", Label: "冻结至选区 (" + cellRef + ")"})
	}
	if panes.Frozen {
		actions = append(actions, modalDialogActionSpec{ID: "unfreeze", Label: "取消冻结"})
	}
	actions = append(actions, modalDialogActionSpec{ID: "cancel", Label: "取消"})

	var msg string
	if panes.Frozen {
		msg = fmt.Sprintf("当前工作表已冻结（行: %d，列: %d）。\n请选择调整或取消冻结：", panes.YSplit, panes.XSplit)
	} else {
		msg = "冻结工作表可以在滚动时保持特定行或列始终可见。\n请选择冻结方式："
	}

	a.modal.Show("冻结窗格", msg, func(action string) {
		a.modal.Hide()
		switch action {
		case "freeze_row":
			a.runSpreadsheetPanesCommand("冻结首行", func(ed document.SpreadsheetPanesEditor) error {
				return ed.SetSheetPanes(sheet, document.SpreadsheetPaneFreeze{Frozen: true, XSplit: 0, YSplit: 1})
			})
			a.setStatus("已冻结首行")
		case "freeze_col":
			a.runSpreadsheetPanesCommand("冻结首列", func(ed document.SpreadsheetPanesEditor) error {
				return ed.SetSheetPanes(sheet, document.SpreadsheetPaneFreeze{Frozen: true, XSplit: 1, YSplit: 0})
			})
			a.setStatus("已冻结首列")
		case "freeze_sel":
			a.runSpreadsheetPanesCommand("冻结选区", func(ed document.SpreadsheetPanesEditor) error {
				return ed.SetSheetPanes(sheet, document.SpreadsheetPaneFreeze{Frozen: true, XSplit: activeCol, YSplit: activeRow})
			})
			a.setStatus(fmt.Sprintf("已冻结至 %s", xlsx.CellName(activeCol+1, activeRow+1)))
		case "unfreeze":
			a.runSpreadsheetPanesCommand("取消冻结", func(ed document.SpreadsheetPanesEditor) error {
				return ed.SetSheetPanes(sheet, document.SpreadsheetPaneFreeze{Frozen: false})
			})
			a.setStatus("已取消冻结窗格")
		}
	}, actions...)
}
