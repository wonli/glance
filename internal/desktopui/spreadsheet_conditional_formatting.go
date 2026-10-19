package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/document"
	"glance/internal/i18n"
)

type spreadsheetConditionalFormatArguments struct {
	Sheet     string             `json:"sheet"`
	Bounds    document.CellRange `json:"bounds"`
	Operation string             `json:"operation"`
	Type      string             `json:"type,omitempty"`
}

func (a *App) showSpreadsheetConditionalFormatDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetConditionalFormatEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.this_workbook_does_not_support_205be4"}, feedbackWarning, notificationToast)
		return
	}
	bounds := state.selection.Bounds()
	rangeLabel := fmt.Sprintf("%s%d:%s%d", columnName(bounds.StartColumn), bounds.StartRow, columnName(bounds.EndColumn), bounds.EndRow)
	a.modal.ShowMessage("spreadsheet.conditional_format", i18n.Message{ID: "spreadsheet.conditional_format.dialog.title"}, i18n.Message{ID: "spreadsheet.conditional_format.dialog.body", Args: i18n.Args{"Range": rangeLabel}}, func(action string) {
		a.modal.Hide()
		var ruleType document.SpreadsheetConditionalFormatType
		clear := false
		switch action {
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
				args := spreadsheetConditionalFormatArguments{Sheet: sheet, Bounds: bounds, Operation: "set"}
				if clear {
					args.Operation = "clear"
				} else {
					args.Type = string(ruleType)
				}
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
					a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.conditional_formatting_cleared_90994a"}, feedbackSuccess, notificationToast)
				} else {
					a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.conditional_formatting_applied_bf7dae"}, feedbackSuccess, notificationSuppressed)
				}
			}
			select {
			case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "条件格式", applied: changed}:
			case <-a.shutdown:
			}
		})
	},
		modalDialogActionSpec{ID: "data_bar", LabelRef: i18n.Message{ID: "spreadsheet.conditional_format.action.data_bar"}},
		modalDialogActionSpec{ID: "3_color", LabelRef: i18n.Message{ID: "spreadsheet.conditional_format.action.color_scale"}},
		modalDialogActionSpec{ID: "duplicate", LabelRef: i18n.Message{ID: "spreadsheet.conditional_format.action.duplicate"}},
		modalDialogActionSpec{ID: "above_avg", LabelRef: i18n.Message{ID: "spreadsheet.conditional_format.action.above_average"}},
		modalDialogActionSpec{ID: "clear", LabelRef: i18n.Message{ID: "spreadsheet.conditional_format.action.clear"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}
