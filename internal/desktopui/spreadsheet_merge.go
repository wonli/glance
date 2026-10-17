package desktopui

import (
	"encoding/json"
	"fmt"

	"glance/internal/application"
	"glance/internal/document"
)

type spreadsheetMergeArguments struct {
	Sheet               string             `json:"sheet"`
	Bounds              document.CellRange `json:"bounds"`
	ConfirmDiscardToken string             `json:"confirm_discard_token,omitempty"`
}

func (a *App) mergeSpreadsheetSelection() error {
	state := a.documentSessionState
	if state == nil || state.book == nil || !state.selectionSet || state.selection.Scope != selectionCells {
		return fmt.Errorf("请先选择要合并的单元格区域")
	}
	return a.mergeSpreadsheetRange(state, state.sheet, state.selection.Bounds(), "")
}

func (a *App) mergeSpreadsheetRange(state *documentSessionState, sheet string, bounds document.CellRange, confirmationToken string) error {
	if state == nil || state.book == nil || state != a.documentSessionState || state.formatRunning.Load() || a.isSaving() {
		return fmt.Errorf("表格正在处理其他操作")
	}
	if sheet == "" || bounds.StartRow < 1 || bounds.StartColumn < 1 || bounds.EndRow < bounds.StartRow || bounds.EndColumn < bounds.StartColumn || bounds.EndRow > xlsxRows || bounds.EndColumn > xlsxColumns {
		return fmt.Errorf("所选区域超出工作表范围")
	}
	if selectionCellCount(bounds) > maxClipboardCellsUI {
		return fmt.Errorf("合并一次最多处理 %d 个单元格", maxClipboardCellsUI)
	}
	editor, ok := state.book.(document.SpreadsheetMergeCellsEditor)
	if !ok {
		return fmt.Errorf("当前工作簿不支持合并单元格")
	}
	if state.valueFilter != nil {
		a.clearValueFilter(state)
	}
	opToken, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return fmt.Errorf("表格正在处理其他操作")
	}
	generation, window := a.documentGeneration, state.virtualGrid
	mergeBounds := bounds
	arguments := spreadsheetMergeArguments{Sheet: sheet, Bounds: bounds, ConfirmDiscardToken: confirmationToken}
	if !a.launchManagedBackground(func() {
		supportedFormat := state.sourceFormat == "" || state.sourceFormat == application.FormatXLSX
		commandRouted := supportedFormat && a.applicationSessionRegistered(opToken.documentID)
		var outcome document.SpreadsheetMergeResult
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(opToken, "spreadsheet.cells.merge", "spreadsheet_cell_range", "selection", sheet, arguments)
			err = commandErr
			if err == nil {
				err = json.Unmarshal(result.Data, &outcome)
			}
		} else {
			unlock := lockSessionAdapterWrite(state)
			outcome, err = editor.MergeCellsChanged(sheet, bounds, confirmationToken)
			unlock()
		}
		a.completeSessionOperation(state, opToken, err, err == nil && outcome.Changed && !commandRouted)
		select {
		case a.formatResults <- spreadsheetFormatResult{
			state: state, token: opToken, revision: state.wordRevisionAtomic.Load(), generation: generation,
			sheet: sheet, window: window, err: err, operation: "合并后居中", applied: outcome.Changed,
			noOp:    err == nil && !outcome.Changed && !outcome.ConfirmationRequired,
			details: "未发生变更", merge: &outcome, mergeBounds: &mergeBounds,
		}:
		case <-a.shutdown:
		}
	}) {
		state.finishOperation(opToken)
		return fmt.Errorf("合并任务无法启动")
	}
	return nil
}

func (a *App) showSpreadsheetMergeConfirmation(state *documentSessionState, generation uint64, sheet string, bounds document.CellRange, outcome document.SpreadsheetMergeResult) {
	if state == nil || outcome.ConfirmationToken == "" || outcome.DiscardedCellCount < 1 {
		return
	}
	message := fmt.Sprintf("选区中有 %d 个非左上角单元格包含内容。合并后只保留左上角单元格的内容，其他内容将被删除。可使用撤销恢复。是否继续？", outcome.DiscardedCellCount)
	a.modal.Show("合并单元格", message, func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		if state != a.documentSessionState || generation != a.documentGeneration || sheet != a.sheet {
			a.setStatus("文档已切换，已取消合并")
			if a.window != nil {
				a.window.Invalidate()
			}
			return
		}
		if err := a.mergeSpreadsheetRange(state, sheet, bounds, outcome.ConfirmationToken); err != nil {
			a.setStatus("合并失败：" + err.Error())
		}
	}, modalDialogActionSpec{ID: "confirm", Label: "合并并居中"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}
