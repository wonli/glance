package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/document"
	"strings"
)

type spreadsheetDataValidationArguments struct {
	Sheet     string             `json:"sheet"`
	Bounds    document.CellRange `json:"bounds"`
	Operation string             `json:"operation"`
	Items     []string           `json:"items,omitempty"`
}

func (a *App) showSpreadsheetDataValidationDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetDataValidationEditor)
	if !ok {
		a.setStatus("当前工作簿不支持数据验证")
		return
	}
	bounds := state.selection.Bounds()
	prompt := fmt.Sprintf("为选区 %s%d:%s%d 设置下拉列表选项（用逗号分隔，留空则清除验证）：",
		columnName(bounds.StartColumn), bounds.StartRow, columnName(bounds.EndColumn), bounds.EndRow)
	a.modal.ShowInput("数据验证", prompt, "男,女", "例如: 是,否 或 优秀,良好,及格", func(action string) {
		input := strings.TrimSpace(a.modal.Input.Text())
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
		clear := input == ""
		var items []string
		if !clear {
			normalized := strings.ReplaceAll(input, "，", ",")
			for _, part := range strings.Split(normalized, ",") {
				if item := strings.TrimSpace(part); item != "" {
					items = append(items, item)
				}
			}
			clear = len(items) == 0
		}
		token, ok := state.beginOperation(sessionOperationMutation)
		if !ok {
			return
		}
		a.launchManagedBackground(func() {
			commandRouted := a.applicationSessionRegistered(token.documentID)
			changed := true
			var err error
			if commandRouted {
				operation := "list"
				arguments := spreadsheetDataValidationArguments{Sheet: sheet, Bounds: bounds}
				if clear {
					operation = "clear"
				} else {
					arguments.Items = items
				}
				arguments.Operation = operation
				result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.data_validation.set", "spreadsheet_cell_range", "selection", sheet, arguments)
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
				if clear {
					changed, err = editor.DeleteDataValidation(sheet, bounds)
				} else {
					changed, err = editor.AddDataValidation(document.SpreadsheetDataValidationSpec{
						Sheet: sheet, Range: bounds, Type: document.DataValidationTypeList, DropList: items,
					})
				}
				unlock()
			}
			a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
			if err == nil {
				if clear {
					a.setStatus("已清除数据验证")
				} else {
					a.setStatus("已设置数据验证下拉列表")
				}
			}
			select {
			case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "数据验证", applied: changed}:
			case <-a.shutdown:
			}
		})
	}, modalDialogActionSpec{ID: "confirm", Label: "确定"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}
