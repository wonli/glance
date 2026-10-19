package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/document"
	"glance/internal/i18n"
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
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.this_workbook_does_not_support_dcaf18"}, feedbackWarning, notificationToast)
		return
	}
	bounds := state.selection.Bounds()
	rangeRef := fmt.Sprintf("%s%d:%s%d", columnName(bounds.StartColumn), bounds.StartRow, columnName(bounds.EndColumn), bounds.EndRow)
	a.modal.ShowInputMessage("spreadsheet.data_validation", i18n.Message{ID: "spreadsheet.data_validation.dialog.title"}, i18n.Message{ID: "spreadsheet.data_validation.dialog.body", Args: i18n.Args{"Range": rangeRef}}, a.text(i18n.Message{ID: "spreadsheet.data_validation.dialog.example_values"}), i18n.Message{ID: "spreadsheet.data_validation.dialog.hint"}, func(action string) {
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
					a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.data_validation_cleared_ec5de8"}, feedbackSuccess, notificationToast)
				} else {
					a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.data_validation_dropdown_set_dd8a63"}, feedbackSuccess, notificationToast)
				}
			}
			select {
			case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "数据验证", applied: changed}:
			case <-a.shutdown:
			}
		})
	}, modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "common.action.confirm"}}, modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}})
}
