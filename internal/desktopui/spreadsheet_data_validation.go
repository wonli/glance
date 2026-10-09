package desktopui

import (
	"fmt"
	"glance/internal/document"
	"strings"
)

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
		token, ok := state.beginOperation(sessionOperationMutation)
		if !ok {
			return
		}
		go func() {
			unlock := lockSessionAdapterWrite(state)
			var err error
			if input == "" {
				err = editor.DeleteDataValidation(sheet, bounds)
			} else {
				normalized := strings.ReplaceAll(input, "，", ",")
				parts := strings.Split(normalized, ",")
				var items []string
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if p != "" {
						items = append(items, p)
					}
				}
				if len(items) > 0 {
					err = editor.AddDataValidation(document.SpreadsheetDataValidationSpec{
						Sheet:    sheet,
						Range:    bounds,
						Type:     document.DataValidationTypeList,
						DropList: items,
					})
				} else {
					err = editor.DeleteDataValidation(sheet, bounds)
				}
			}
			unlock()
			a.completeSessionOperation(state, token, err, true)
			if err == nil {
				if input == "" {
					a.setStatus("已清除数据验证")
				} else {
					a.setStatus("已设置数据验证下拉列表")
				}
			}
			select {
			case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "数据验证"}:
			case <-a.shutdown:
			}
		}()
	}, modalDialogActionSpec{ID: "confirm", Label: "确定"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}
