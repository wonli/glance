package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/document"
)

func (a *App) runSpreadsheetDuplicatesCommand(operation string, arguments any, edit func(document.SpreadsheetDuplicatesEditor) (document.SpreadsheetDuplicatesResult, error)) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetDuplicatesEditor)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	a.launchManagedBackground(func() {
		commandRouted := a.applicationSessionRegistered(token.documentID)
		var res document.SpreadsheetDuplicatesResult
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.duplicates.remove", "spreadsheet_cell_range", "selection", sheet, arguments)
			err = commandErr
			if err == nil {
				err = json.Unmarshal(result.Data, &res)
			}
		} else {
			unlock := lockSessionAdapterWrite(state)
			res, err = edit(editor)
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && res.RemovedCount > 0 && !commandRouted)
		if err == nil {
			a.setStatus(fmt.Sprintf("已删除 %d 个重复项，保留 %d 个唯一项", res.RemovedCount, res.RemainingCount))
		}
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation}:
		case <-a.shutdown:
		}
	})
}

func (a *App) showSpreadsheetRemoveDuplicatesDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	if !state.selectionSet {
		a.setStatus("请先选择包含数据的单元格区域")
		return
	}
	bounds := state.selection.Bounds()
	rowCount := bounds.EndRow - bounds.StartRow + 1
	if rowCount <= 1 {
		a.setStatus("选区至少需要 2 行才能执行删除重复项")
		return
	}
	sheet := state.sheet
	rangeRef := fmt.Sprintf("%s%d:%s%d", columnName(bounds.StartColumn), bounds.StartRow, columnName(bounds.EndColumn), bounds.EndRow)
	keyCol := state.selection.Active.Column + 1
	activeColName := columnName(keyCol)

	msg := fmt.Sprintf("将在选区 %s（共 %d 行）中查找并删除重复数据。\n默认按当前活动列（%s 列）排重，或按整行全部列排重。", rangeRef, rowCount, activeColName)

	a.modal.Show("删除重复项", msg, func(action string) {
		a.modal.Hide()
		switch action {
		case "col":
			a.runSpreadsheetDuplicatesCommand("按列删除重复项", map[string]any{"sheet": sheet, "bounds": bounds, "key_column": keyCol}, func(ed document.SpreadsheetDuplicatesEditor) (document.SpreadsheetDuplicatesResult, error) {
				return ed.RemoveDuplicates(sheet, bounds, keyCol)
			})
		case "all":
			a.runSpreadsheetDuplicatesCommand("按整行删除重复项", map[string]any{"sheet": sheet, "bounds": bounds, "key_column": 0}, func(ed document.SpreadsheetDuplicatesEditor) (document.SpreadsheetDuplicatesResult, error) {
				return ed.RemoveDuplicates(sheet, bounds, 0)
			})
		}
	},
		modalDialogActionSpec{ID: "col", Label: fmt.Sprintf("按 %s 列排重", activeColName)},
		modalDialogActionSpec{ID: "all", Label: "按全部列排重"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}
