package desktopui

import (
	"encoding/json"
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
	_, ok := state.book.(document.SpreadsheetShapeEditor)
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

		a.runSpreadsheetShapeCommand(state, "插入形状", document.SpreadsheetShapeSpec{
			Sheet: sheet, Cell: cellName, Type: shapeType, Width: 180, Height: 90,
		})
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
	_, ok := state.book.(document.SpreadsheetShapeEditor)
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

		a.runSpreadsheetShapeCommand(state, "插入文本框", document.SpreadsheetShapeSpec{
			Sheet: sheet, Cell: cellName, Type: "rect", Width: 200, Height: 80, Text: text,
		})
	},
		modalDialogActionSpec{ID: "confirm", Label: "插入文本框"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) runSpreadsheetShapeCommand(state *documentSessionState, operation string, spec document.SpreadsheetShapeSpec) {
	if state == nil || state != a.documentSessionState || state.book == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetShapeEditor)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, spec.Sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	a.launchManagedBackground(func() {
		commandRouted := a.applicationSessionRegistered(token.documentID)
		changed := true
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.shape.create", "spreadsheet_cell", spec.Cell, spec.Sheet, map[string]any{
				"sheet": spec.Sheet, "cell": spec.Cell, "type": spec.Type,
				"width_px": spec.Width, "height_px": spec.Height, "text": spec.Text,
			})
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
			err = editor.AddShape(spec)
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		if err == nil {
			a.setStatus("已成功" + operation)
		}
		if a.formatResults != nil {
			select {
			case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation, applied: changed}:
			case <-a.shutdown:
			}
		}
	})
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
			a.launchManagedBackground(func() {
				spec := document.SpreadsheetChartSpec{
					Sheet: sheet,
					Cell:  cellName,
					Kind:  kind,
					Title: title,
					Range: bounds,
				}
				commandRouted := a.applicationSessionRegistered(token.documentID)
				changed := true
				var err error
				if commandRouted {
					result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.chart.create", "spreadsheet_cell_range", "chart-source", sheet, map[string]any{
						"sheet": sheet, "kind": kind, "bounds": bounds, "cell": cellName, "title": title,
					})
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
					err = editor.AddChart(spec)
					unlock()
				}
				a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
				if err == nil {
					a.setStatus("已成功插入" + chartName)
				}
				if a.formatResults != nil {
					select {
					case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "插入" + chartName, applied: changed}:
					case <-a.shutdown:
					}
				}
			})
		},
	)
}
