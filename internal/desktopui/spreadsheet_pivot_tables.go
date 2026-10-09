package desktopui

import (
	"fmt"
	"glance/internal/document"
)

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

	a.modal.Show("插入数据透视表", fmt.Sprintf("基于选区创建数据透视表：\n数据源范围：%s\n\n将在工作表右侧生成透视表报表。是否继续？", rangeStr), func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		generation, window := a.documentGeneration, state.virtualGrid
		token, ok := state.beginOperation(sessionOperationMutation)
		if !ok {
			return
		}
		go func() {
			unlock := lockSessionAdapterWrite(state)
			err := editor.AddPivotTable(document.SpreadsheetPivotTableSpec{
				Sheet:     sheet,
				DataRange: bounds,
			})
			unlock()
			a.completeSessionOperation(state, token, err, true)
			if err == nil {
				a.setStatus("已成功插入数据透视表")
			}
			select {
			case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "插入数据透视表"}:
			case <-a.shutdown:
			}
		}()
	},
		modalDialogActionSpec{ID: "confirm", Label: "创建透视表"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}
