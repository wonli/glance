package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/document"
	"strings"
)

func (a *App) showSpreadsheetConditionalFormatDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetConditionalFormatEditor)
	if !ok {
		a.setStatus("当前工作簿不支持条件格式")
		return
	}
	bounds := state.selection.Bounds()
	prompt := fmt.Sprintf("为选区 %s%d:%s%d 选择条件格式规则：",
		columnName(bounds.StartColumn), bounds.StartRow, columnName(bounds.EndColumn), bounds.EndRow)
	a.modal.ShowInput("条件格式", prompt, "data_bar", "规则: data_bar(数据条), 3_color(色阶), duplicate(重复值), above_avg(高于平均值), clear(清除)", func(action string) {
		a.modal.Hide()
		var ruleType document.SpreadsheetConditionalFormatType
		clear := false
		switch action {
		case "confirm":
			val := strings.TrimSpace(a.modal.Input.Text())
			switch val {
			case "3_color", "color", "scale":
				ruleType = document.ConditionalFormat3ColorScale
			case "duplicate", "dup":
				ruleType = document.ConditionalFormatDuplicate
			case "above_avg", "avg":
				ruleType = document.ConditionalFormatAboveAvg
			case "clear", "none", "":
				clear = true
			default:
				ruleType = document.ConditionalFormatDataBar
			}
		case "data_bar":
			ruleType = document.ConditionalFormatDataBar
		case "3_color":
			ruleType = document.ConditionalFormat3ColorScale
		case "duplicate":
			ruleType = document.ConditionalFormatDuplicate
		case "above_avg":
			ruleType = document.ConditionalFormatAboveAvg
		case "clear":
			clear = true
		default:
			return
		}

		generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
		token, ok := state.beginOperation(sessionOperationMutation)
		if !ok {
			return
		}
		a.launchManagedBackground(func() {
			commandRouted := a.applicationSessionRegistered(token.documentID)
			changed := true
			var err error
			if commandRouted {
				operation, args := "set", map[string]any{"sheet": sheet, "bounds": bounds}
				if clear {
					operation = "clear"
				} else {
					args["type"] = string(ruleType)
				}
				args["operation"] = operation
				result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.conditional_format.set", "spreadsheet_cell_range", "selection", sheet, args)
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
					changed, err = editor.ClearConditionalFormat(sheet, bounds)
				} else {
					changed, err = editor.SetConditionalFormat(document.SpreadsheetConditionalFormatSpec{Sheet: sheet, Range: bounds, Type: ruleType})
				}
				unlock()
			}
			a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
			if err == nil {
				if clear {
					a.setStatus("已清除条件格式")
				} else {
					a.setStatus("已应用条件格式")
				}
			}
			select {
			case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "条件格式", applied: changed}:
			case <-a.shutdown:
			}
		})
	},
		modalDialogActionSpec{ID: "data_bar", Label: "渐变数据条"},
		modalDialogActionSpec{ID: "3_color", Label: "三色色阶"},
		modalDialogActionSpec{ID: "duplicate", Label: "高亮重复值"},
		modalDialogActionSpec{ID: "above_avg", Label: "高于平均值"},
		modalDialogActionSpec{ID: "clear", Label: "清除规则"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}
