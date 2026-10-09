package desktopui

import (
	"fmt"
	"glance/internal/document"
	"glance/internal/format/xlsx"
	"strings"
)

func (a *App) showSpreadsheetInsertShapeDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetShapeEditor)
	if !ok {
		a.setStatus("当前工作簿不支持插入形状")
		return
	}
	col := state.selection.Active.Column + 1
	row := state.selection.Active.Row + 1
	cellName := xlsx.CellName(col, row)
	sheet := state.sheet

	a.modal.ShowInput("插入形状", fmt.Sprintf("在单元格 %s 处插入形状：", cellName), "roundRect", "类型: rect(矩形), roundRect(圆角矩形), ellipse(椭圆), rightArrow(箭头), star5(五角星)", func(action string) {
		a.modal.Hide()
		var shapeType string
		switch action {
		case "confirm":
			shapeType = strings.TrimSpace(a.modal.Input.Text())
		case "rect":
			shapeType = "rect"
		case "roundRect":
			shapeType = "roundRect"
		case "ellipse":
			shapeType = "ellipse"
		case "rightArrow":
			shapeType = "rightArrow"
		case "star5":
			shapeType = "star5"
		default:
			return
		}
		if shapeType == "" {
			shapeType = "roundRect"
		}

		generation, window := a.documentGeneration, state.virtualGrid
		token, ok := state.beginOperation(sessionOperationMutation)
		if !ok {
			return
		}
		go func() {
			unlock := lockSessionAdapterWrite(state)
			err := editor.AddShape(document.SpreadsheetShapeSpec{
				Sheet:  sheet,
				Cell:   cellName,
				Type:   shapeType,
				Width:  180,
				Height: 90,
			})
			unlock()
			a.completeSessionOperation(state, token, err, true)
			if err == nil {
				a.setStatus("已成功插入形状")
			}
			select {
			case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "插入形状"}:
			case <-a.shutdown:
			}
		}()
	},
		modalDialogActionSpec{ID: "roundRect", Label: "圆角矩形"},
		modalDialogActionSpec{ID: "rect", Label: "标准矩形"},
		modalDialogActionSpec{ID: "ellipse", Label: "椭圆"},
		modalDialogActionSpec{ID: "rightArrow", Label: "箭头"},
		modalDialogActionSpec{ID: "star5", Label: "五角星"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showSpreadsheetInsertTextBoxDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetShapeEditor)
	if !ok {
		a.setStatus("当前工作簿不支持插入文本框")
		return
	}
	col := state.selection.Active.Column + 1
	row := state.selection.Active.Row + 1
	cellName := xlsx.CellName(col, row)
	sheet := state.sheet

	a.modal.ShowInput("插入文本框", fmt.Sprintf("在单元格 %s 处插入文本框：", cellName), "文本框内容", "输入要在文本框中显示的文字", func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		text := strings.TrimSpace(a.modal.Input.Text())
		if text == "" {
			text = "文本框内容"
		}

		generation, window := a.documentGeneration, state.virtualGrid
		token, ok := state.beginOperation(sessionOperationMutation)
		if !ok {
			return
		}
		go func() {
			unlock := lockSessionAdapterWrite(state)
			err := editor.AddShape(document.SpreadsheetShapeSpec{
				Sheet:  sheet,
				Cell:   cellName,
				Type:   "rect",
				Width:  200,
				Height: 80,
				Text:   text,
			})
			unlock()
			a.completeSessionOperation(state, token, err, true)
			if err == nil {
				a.setStatus("已成功插入文本框")
			}
			select {
			case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "插入文本框"}:
			case <-a.shutdown:
			}
		}()
	},
		modalDialogActionSpec{ID: "confirm", Label: "插入文本框"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showSpreadsheetInsertChartDialog(kind document.SpreadsheetChartKind) {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetChartEditor)
	if !ok {
		a.setStatus("当前工作簿不支持插入图表")
		return
	}
	bounds := state.selection.Bounds()
	if bounds.StartRow <= 0 {
		bounds = document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 4, EndColumn: 2}
	}
	sheet := state.sheet
	chartCol := bounds.EndColumn + 2
	chartRow := bounds.StartRow
	cellName := xlsx.CellName(chartCol, chartRow)

	var defaultTitle, chartName string
	switch kind {
	case document.SpreadsheetChartLine:
		defaultTitle, chartName = "趋势折线图", "折线图"
	case document.SpreadsheetChartPie:
		defaultTitle, chartName = "占比饼图", "饼图"
	default:
		defaultTitle, chartName = "数据柱状图", "柱状图"
	}

	rangeStr := fmt.Sprintf("%s%d:%s%d",
		columnName(bounds.StartColumn), bounds.StartRow,
		columnName(bounds.EndColumn), bounds.EndRow)

	a.modal.ShowDualInput("插入"+chartName, fmt.Sprintf("基于选区创建并在 %s 处插入%s：", cellName, chartName),
		defaultTitle, "图表标题",
		rangeStr, "数据范围 (例如 A1:B5)",
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			title := strings.TrimSpace(a.modal.Input.Text())
			if title == "" {
				title = defaultTitle
			}

			generation, window := a.documentGeneration, state.virtualGrid
			token, ok := state.beginOperation(sessionOperationMutation)
			if !ok {
				return
			}
			go func() {
				unlock := lockSessionAdapterWrite(state)
				err := editor.AddChart(document.SpreadsheetChartSpec{
					Sheet: sheet,
					Cell:  cellName,
					Kind:  kind,
					Title: title,
					Range: bounds,
				})
				unlock()
				a.completeSessionOperation(state, token, err, true)
				if err == nil {
					a.setStatus("已成功插入" + chartName)
				}
				select {
				case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "插入" + chartName}:
				case <-a.shutdown:
				}
			}()
		},
	)
}
