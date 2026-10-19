package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/document"
	"glance/internal/format/xlsx"
	"glance/internal/i18n"
	"strings"
)

func (a *App) showSpreadsheetInsertShapeDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	_, ok := state.book.(document.SpreadsheetShapeEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.this_workbook_does_not_support_549d00"}, feedbackWarning, notificationToast)
		return
	}
	col := state.selection.Active.Column + 1
	row := state.selection.Active.Row + 1
	cellName := xlsx.CellName(col, row)
	sheet := state.sheet

	a.modal.ShowMessage("spreadsheet.shape.insert", i18n.Message{ID: "spreadsheet.shape.dialog.title"}, i18n.Message{ID: "spreadsheet.shape.dialog.body", Args: i18n.Args{"Cell": cellName}}, func(action string) {
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
		modalDialogActionSpec{ID: "roundRect", LabelRef: i18n.Message{ID: "spreadsheet.shape.type.round_rectangle"}},
		modalDialogActionSpec{ID: "rect", LabelRef: i18n.Message{ID: "spreadsheet.shape.type.rectangle"}},
		modalDialogActionSpec{ID: "ellipse", LabelRef: i18n.Message{ID: "spreadsheet.shape.type.ellipse"}},
		modalDialogActionSpec{ID: "rightArrow", LabelRef: i18n.Message{ID: "spreadsheet.shape.type.arrow"}},
		modalDialogActionSpec{ID: "star5", LabelRef: i18n.Message{ID: "spreadsheet.shape.type.star"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

func (a *App) showSpreadsheetInsertTextBoxDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	_, ok := state.book.(document.SpreadsheetShapeEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.this_workbook_does_not_support_2a0f8b"}, feedbackWarning, notificationToast)
		return
	}
	col := state.selection.Active.Column + 1
	row := state.selection.Active.Row + 1
	cellName := xlsx.CellName(col, row)
	sheet := state.sheet

	a.modal.ShowInputMessage("spreadsheet.text_box", i18n.Message{ID: "spreadsheet.text_box.dialog.title"}, i18n.Message{ID: "spreadsheet.text_box.dialog.body", Args: i18n.Args{"Cell": cellName}}, a.text(i18n.Message{ID: "spreadsheet.text_box.default_text"}), i18n.Message{ID: "spreadsheet.text_box.dialog.hint"}, func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		text := strings.TrimSpace(a.modal.Input.Text())
		if text == "" {
			text = a.text(i18n.Message{ID: "spreadsheet.text_box.default_text"})
		}

		a.runSpreadsheetShapeCommand(state, "插入文本框", document.SpreadsheetShapeSpec{
			Sheet: sheet, Cell: cellName, Type: "rect", Width: 200, Height: 80, Text: text,
		})
	},
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "spreadsheet.text_box.action.insert"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

type spreadsheetShapeCreateArguments struct {
	Sheet    string `json:"sheet"`
	Cell     string `json:"cell"`
	Type     string `json:"type"`
	WidthPx  uint   `json:"width_px"`
	HeightPx uint   `json:"height_px"`
	Text     string `json:"text"`
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
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.shape.create", "spreadsheet_cell", spec.Cell, spec.Sheet, spreadsheetShapeCreateArguments{Sheet: spec.Sheet, Cell: spec.Cell, Type: spec.Type, WidthPx: spec.Width, HeightPx: spec.Height, Text: spec.Text})
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
			a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inserted_operation", Args: i18n.Args{"Operation": a.localizedSpreadsheetToolbarText(operation)}}, feedbackSuccess, notificationToast)
		}
		if a.formatResults != nil {
			select {
			case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation, applied: changed}:
			case <-a.shutdown:
			}
		}
	})
}

type spreadsheetChartCreateArguments struct {
	Sheet  string                        `json:"sheet"`
	Kind   document.SpreadsheetChartKind `json:"kind"`
	Bounds document.CellRange            `json:"bounds"`
	Cell   string                        `json:"cell"`
	Title  string                        `json:"title"`
}

func (a *App) showSpreadsheetInsertChartDialog(kind document.SpreadsheetChartKind) {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetChartEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.this_workbook_does_not_support_9477c0"}, feedbackWarning, notificationToast)
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

	var defaultTitle, chartName i18n.Message
	switch kind {
	case document.SpreadsheetChartLine:
		defaultTitle, chartName = i18n.Message{ID: "spreadsheet.chart.default_title.line"}, i18n.Message{ID: "spreadsheet.chart.type.line"}
	case document.SpreadsheetChartPie:
		defaultTitle, chartName = i18n.Message{ID: "spreadsheet.chart.default_title.pie"}, i18n.Message{ID: "spreadsheet.chart.type.pie"}
	default:
		defaultTitle, chartName = i18n.Message{ID: "spreadsheet.chart.default_title.column"}, i18n.Message{ID: "spreadsheet.chart.type.column"}
	}

	rangeStr := fmt.Sprintf("%s%d:%s%d",
		columnName(bounds.StartColumn), bounds.StartRow,
		columnName(bounds.EndColumn), bounds.EndRow)
	chartNameText, defaultTitleText := a.text(chartName), a.text(defaultTitle)

	a.modal.ShowDualInputMessage("spreadsheet.chart.insert", i18n.Message{ID: "spreadsheet.chart.dialog.title", Args: i18n.Args{"Chart": a.text(chartName)}}, i18n.Message{ID: "spreadsheet.chart.dialog.body", Args: i18n.Args{"Cell": cellName, "Chart": a.text(chartName)}},
		i18n.Message{ID: "spreadsheet.chart.dialog.title_hint"}, i18n.Message{ID: "spreadsheet.chart.dialog.range_hint"},
		defaultTitleText, rangeStr,
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			title := strings.TrimSpace(a.modal.Input.Text())
			if title == "" {
				title = defaultTitleText
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
					result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.chart.create", "spreadsheet_cell_range", "chart-source", sheet, spreadsheetChartCreateArguments{Sheet: sheet, Kind: kind, Bounds: bounds, Cell: cellName, Title: title})
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
					a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inserted_chart", Args: i18n.Args{"Chart": chartNameText}}, feedbackSuccess, notificationToast)
				}
				if a.formatResults != nil {
					select {
					case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: a.text(i18n.Message{ID: "spreadsheet.status.operation.insert_chart", Args: i18n.Args{"Chart": chartNameText}}), applied: changed}:
					case <-a.shutdown:
					}
				}
			})
		},
	)
}
