package desktopui

import (
	"encoding/json"
	"fmt"
	"strings"

	"glance/internal/document"
)

const maxSpreadsheetRangeEntryCells = 10_000

func (a *App) enterSpreadsheetValueInSelection(value string) error {
	state := a.documentSessionState
	if state == nil || state.book == nil || !state.selectionSet || state.selection.Scope != selectionCells {
		return fmt.Errorf("请先选择单元格区域")
	}
	if state.formatRunning.Load() || a.isSaving() {
		return fmt.Errorf("表格正在处理其他操作")
	}
	bounds := state.selection.Bounds()
	if bounds.StartRow < 1 || bounds.StartColumn < 1 || bounds.EndRow < bounds.StartRow || bounds.EndColumn < bounds.StartColumn || bounds.EndRow > xlsxRows || bounds.EndColumn > xlsxColumns {
		return fmt.Errorf("所选区域超出工作表范围")
	}
	rows, columns := bounds.EndRow-bounds.StartRow+1, bounds.EndColumn-bounds.StartColumn+1
	if rows > maxSpreadsheetRangeEntryCells/columns {
		return fmt.Errorf("一次最多可向 %d 个单元格输入相同内容", maxSpreadsheetRangeEntryCells)
	}
	state.cellEdit.setDraft(value)
	if len(value) > 0 && strings.HasPrefix(strings.TrimSpace(value), "=") {
		active := document.CellCoordinate{Row: state.selection.Active.Row + 1, Column: state.selection.Active.Column + 1}
		if err := a.applySpreadsheetFormulaRangeEntry(active, bounds, value); err != nil {
			return err
		}
	} else {
		edits := make([]document.CellEdit, 0, rows*columns)
		for row := bounds.StartRow; row <= bounds.EndRow; row++ {
			for column := bounds.StartColumn; column <= bounds.EndColumn; column++ {
				edits = append(edits, document.CellEdit{Row: row, Column: column, Value: value})
			}
		}
		if err := a.applyBatchEditsForRangeEntry(edits, state.sheet, value); err != nil {
			return err
		}
	}
	if state.virtualGrid != nil {
		state.virtualGrid.PendingSelectionLoad = true
		state.virtualGrid.finishCellEditRowPreview(false)
	}
	return nil
}

type spreadsheetFormulaRangeEntryArguments struct {
	Sheet   string                  `json:"sheet"`
	Active  document.CellCoordinate `json:"active"`
	Bounds  document.CellRange      `json:"bounds"`
	Formula string                  `json:"formula"`
}

func (a *App) applySpreadsheetFormulaRangeEntry(active document.CellCoordinate, bounds document.CellRange, formula string) error {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return fmt.Errorf("表格正在处理其他操作")
	}
	editor, ok := state.book.(document.SpreadsheetFormulaRangeEntryEditor)
	if !ok {
		return fmt.Errorf("当前工作簿不支持在选区输入公式")
	}
	if state.valueFilter != nil {
		a.clearValueFilter(state)
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return fmt.Errorf("表格正在处理其他操作")
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	arguments := spreadsheetFormulaRangeEntryArguments{Sheet: sheet, Active: active, Bounds: bounds, Formula: formula}
	entryValue := formula
	if !a.launchManagedBackground(func() {
		commandRouted := a.applicationSessionRegistered(token.documentID)
		changed := false
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.formulas.fill", "spreadsheet_cell_range", "selection", sheet, arguments)
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
			changed, err = editor.SetCellFormulaRangeChanged(sheet, active, bounds, formula)
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		select {
		case a.formatResults <- spreadsheetFormatResult{
			state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation,
			sheet: sheet, window: window, err: err, operation: "填充选区", applied: changed,
			noOp: err == nil && !changed, details: "选区已包含相同公式", rangeEntry: &entryValue,
		}:
		case <-a.shutdown:
		}
	}) {
		state.finishOperation(token)
		return fmt.Errorf("公式填充任务无法启动")
	}
	return nil
}
