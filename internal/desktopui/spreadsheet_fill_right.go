package desktopui

import (
	"encoding/json"
	"fmt"

	"glance/internal/application"
	"glance/internal/document"
)

type spreadsheetFillRightArguments struct {
	Sheet  string             `json:"sheet"`
	Bounds document.CellRange `json:"bounds"`
}

func (a *App) fillRightSpreadsheetSelection() error {
	state := a.documentSessionState
	if state == nil || state.book == nil || !state.selectionSet || state.selection.Scope != selectionCells {
		return fmt.Errorf("请先选择至少两列的单元格区域")
	}
	if state.formatRunning.Load() || a.isSaving() {
		return fmt.Errorf("表格正在处理其他操作")
	}
	bounds := state.selection.Bounds()
	if bounds.StartRow < 1 || bounds.StartColumn < 1 || bounds.EndRow < bounds.StartRow || bounds.EndColumn < bounds.StartColumn || bounds.EndRow > xlsxRows || bounds.EndColumn > xlsxColumns {
		return fmt.Errorf("所选区域超出工作表范围")
	}
	rows, columns := bounds.EndRow-bounds.StartRow+1, bounds.EndColumn-bounds.StartColumn+1
	if columns < 2 {
		return fmt.Errorf("向右填充至少需要选择两列")
	}
	if rows > maxSpreadsheetFillDownCells/columns {
		return fmt.Errorf("向右填充一次最多处理 %d 个单元格", maxSpreadsheetFillDownCells)
	}
	editor, ok := state.book.(document.SpreadsheetFillRightEditor)
	if !ok {
		return fmt.Errorf("当前工作簿不支持向右填充")
	}
	if state.valueFilter != nil {
		a.clearValueFilter(state)
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return fmt.Errorf("表格正在处理其他操作")
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	arguments := spreadsheetFillRightArguments{Sheet: sheet, Bounds: bounds}
	if !a.launchManagedBackground(func() {
		supportedFormat := state.sourceFormat == "" || state.sourceFormat == application.FormatXLSX
		commandRouted := supportedFormat && a.applicationSessionRegistered(token.documentID)
		changed := false
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.range.fill_right", "spreadsheet_cell_range", "selection", sheet, arguments)
			err = commandErr
			if err == nil {
				var outcome struct {
					Changed bool `json:"changed"`
				}
				err = json.Unmarshal(result.Data, &outcome)
				changed = outcome.Changed
			}
		} else {
			unlock := lockSessionAdapterWrite(state)
			changed, err = editor.FillRightChanged(sheet, bounds)
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		select {
		case a.formatResults <- spreadsheetFormatResult{
			state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation,
			sheet: sheet, window: window, err: err, operation: "向右填充", applied: changed,
			noOp: err == nil && !changed, details: "选区内容已与首列一致",
		}:
		case <-a.shutdown:
		}
	}) {
		state.finishOperation(token)
		return fmt.Errorf("向右填充任务无法启动")
	}
	return nil
}
