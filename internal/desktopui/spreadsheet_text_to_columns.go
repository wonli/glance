package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/document"
)

func (a *App) runSpreadsheetTextToColumnsCommand(operation string, arguments any, edit func(document.SpreadsheetTextToColumnsEditor) (document.SpreadsheetTextToColumnsResult, error)) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetTextToColumnsEditor)
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
		var res document.SpreadsheetTextToColumnsResult
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.text_to_columns.split", "spreadsheet_cell_range", "selection", sheet, arguments)
			err = commandErr
			if err == nil {
				err = json.Unmarshal(result.Data, &res)
			}
		} else {
			unlock := lockSessionAdapterWrite(state)
			res, err = edit(editor)
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && res.RowsSplit > 0 && !commandRouted)
		if err == nil {
			a.setStatus(fmt.Sprintf("已完成分列：处理 %d 行，最多拆分为 %d 列", res.RowsSplit, res.MaxCols))
		}
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation}:
		case <-a.shutdown:
		}
	})
}

type spreadsheettextSheetBoundsArguments struct {
	Sheet     string             `json:"sheet"`
	Bounds    document.CellRange `json:"bounds"`
	KeyColumn int                `json:"key_column"`
	Delimiter string             `json:"delimiter"`
}

func (a *App) showSpreadsheetTextToColumnsDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	if !state.selectionSet {
		a.setStatus("请先选择包含数据的单元格区域")
		return
	}
	bounds := state.selection.Bounds()
	sheet := state.sheet
	keyCol := state.selection.Active.Column + 1
	if keyCol < bounds.StartColumn || keyCol > bounds.EndColumn {
		keyCol = bounds.StartColumn
	}
	activeColName := columnName(keyCol)
	rangeRef := fmt.Sprintf("%s%d:%s%d", columnName(bounds.StartColumn), bounds.StartRow, columnName(bounds.EndColumn), bounds.EndRow)
	msg := fmt.Sprintf("将在选区 %s 中对 %s 列进行文本分列。\n选择预设分隔符或输入自定义字符向右拆分数据：", rangeRef, activeColName)

	splitWith := func(delim, label string) {
		a.runSpreadsheetTextToColumnsCommand("文本分列 ("+label+")", spreadsheettextSheetBoundsArguments{Sheet: sheet, Bounds: bounds, KeyColumn: keyCol, Delimiter: delim}, func(ed document.SpreadsheetTextToColumnsEditor) (document.SpreadsheetTextToColumnsResult, error) {
			return ed.SplitTextToColumns(sheet, bounds, keyCol, delim)
		})
	}

	a.modal.Show("文本分列", msg, func(action string) {
		a.modal.Hide()
		switch action {
		case "comma":
			splitWith(",", "逗号")
		case "semicolon":
			splitWith(";", "分号")
		case "space":
			splitWith(" ", "空格")
		case "tab":
			splitWith("\t", "制表符")
		case "custom":
			a.modal.ShowInput("自定义分列", "输入分隔符（支持单个字符、单词或 \\t 制表符）：", ",", "分隔符", func(act string) {
				delim := a.modal.Input.Text()
				a.modal.Hide()
				if act != "confirm" {
					return
				}
				if delim == "\\t" {
					delim = "\t"
				}
				if delim == "" {
					a.setStatus("分隔符不能为空")
					return
				}
				splitWith(delim, "自定义")
			}, modalDialogActionSpec{ID: "confirm", Label: "确定"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
		}
	},
		modalDialogActionSpec{ID: "comma", Label: "逗号 (,)"},
		modalDialogActionSpec{ID: "semicolon", Label: "分号 (;)"},
		modalDialogActionSpec{ID: "space", Label: "空格 ( )"},
		modalDialogActionSpec{ID: "tab", Label: "制表符 (\\t)"},
		modalDialogActionSpec{ID: "custom", Label: "自定义..."},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}
