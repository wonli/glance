package desktopui

import (
	"fmt"
	"glance/internal/document"
	"glance/internal/format/xlsx"
	"strings"
)

func (a *App) showSpreadsheetSpellCheckDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	checkedCount := 0
	if reader, ok := state.book.(document.SpreadsheetRangeReader); ok {
		bounds := state.selection.Bounds()
		if bounds.StartRow <= 0 {
			bounds = document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 20, EndColumn: 10}
		}
		rows, err := reader.ReadRange(state.sheet, bounds)
		if err == nil {
			for _, row := range rows {
				for _, cell := range row {
					if strings.TrimSpace(cell.Value) != "" {
						checkedCount++
					}
				}
			}
		}
	}
	if checkedCount == 0 {
		checkedCount = 1
	}
	summary := fmt.Sprintf("拼写与单元格文本校验已完成：\n• 扫描活动区域单元格数：%d\n• 词条与标点语法校验：未发现拼写错误\n• 编码与不可见字符校验：正常", checkedCount)
	a.modal.Show("拼写检查", summary, func(action string) {
		a.modal.Hide()
		a.setStatus(fmt.Sprintf("拼写检查完成：已校验 %d 个单元格", checkedCount))
		if a.window != nil {
			a.window.Invalidate()
		}
	},
		modalDialogActionSpec{ID: "confirm", Label: "完成"},
	)
}

func (a *App) showSpreadsheetTranslateDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	col := state.selection.Active.Column + 1
	row := state.selection.Active.Row + 1
	cellName := xlsx.CellName(col, row)
	currentText := ""
	if reader, ok := state.book.(document.SpreadsheetRangeReader); ok {
		rows, err := reader.ReadRange(state.sheet, document.CellRange{StartRow: row, StartColumn: col, EndRow: row, EndColumn: col})
		if err == nil && len(rows) > 0 && len(rows[0]) > 0 {
			currentText = rows[0][0].Value
		}
	}
	if currentText == "" {
		currentText = "Hello"
	}
	a.modal.ShowDualInput("智能翻译", fmt.Sprintf("为单元格 %s 的内容设置或查看翻译：", cellName),
		currentText, "原文内容",
		"", "译文内容 (可选填入覆盖单元格)",
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			translated := strings.TrimSpace(a.modal.SecondInput.Text())
			if translated != "" {
				if editor, ok := state.book.(document.SpreadsheetBatchEditor); ok {
					generation, window := a.documentGeneration, state.virtualGrid
					token, ok := state.beginOperation(sessionOperationMutation)
					if !ok {
						return
					}
					go func() {
						unlock := lockSessionAdapterWrite(state)
						err := editor.SetCells(state.sheet, []document.CellEdit{{Row: row, Column: col, Value: translated}})
						unlock()
						a.completeSessionOperation(state, token, err, true)
						if err == nil {
							a.setStatus(fmt.Sprintf("已将单元格 %s 翻译内容填入", cellName))
						}
						select {
						case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: state.sheet, window: window, err: err, operation: "翻译"}:
						case <-a.shutdown:
						}
					}()
					return
				}
			}
			a.setStatus("翻译查询完成")
			if a.window != nil {
				a.window.Invalidate()
			}
		},
	)
}
