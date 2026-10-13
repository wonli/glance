package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/document"
)

type spreadsheetPivotCreateArguments struct {
	Sheet  string             `json:"sheet"`
	Bounds document.CellRange `json:"bounds"`
}

func (a *App) showSpreadsheetPivotTableDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetPivotTableEditor)
	if !ok {
		a.setStatus("当前工作簿不支持插入数据透视表")
		return
	}
	bounds := state.selection.Bounds()
	if bounds.StartRow <= 0 {
		bounds = document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 5, EndColumn: 3}
	}
	sheet := state.sheet
	rangeStr := fmt.Sprintf("%s%d:%s%d",
		columnName(bounds.StartColumn), bounds.StartRow,
		columnName(bounds.EndColumn), bounds.EndRow)

	a.modal.Show("插入数据透视表", fmt.Sprintf("基于选区创建数据透视表：\n数据源范围：%s\n\n将在工作表右侧生成透视表报表。若标题行有空白单元格，会自动补上“字段N”名称。是否继续？", rangeStr), func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		generation, window := a.documentGeneration, state.virtualGrid
		token, ok := state.beginOperation(sessionOperationMutation)
		if !ok {
			return
		}
		a.launchManagedBackground(func() {
			spec := document.SpreadsheetPivotTableSpec{
				Sheet:     sheet,
				DataRange: bounds,
			}
			commandRouted := a.applicationSessionRegistered(token.documentID)
			changed := true
			var err error
			if commandRouted {
				result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.pivot.create", "spreadsheet_cell_range", "selection", sheet, spreadsheetPivotCreateArguments{Sheet: sheet, Bounds: bounds})
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
				err = editor.AddPivotTable(spec)
				unlock()
			}
			a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
			if err == nil {
				a.setStatus("已成功插入数据透视表")
			}
			if a.formatResults != nil {
				select {
				case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "插入数据透视表", applied: changed}:
				case <-a.shutdown:
				}
			}
		})
	},
		modalDialogActionSpec{ID: "confirm", Label: "创建透视表"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}
