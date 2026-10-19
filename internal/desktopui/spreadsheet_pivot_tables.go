package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/document"
	"glance/internal/i18n"
)

type spreadsheetPivotCreateArguments struct {
	Sheet  string             `json:"sheet"`
	Bounds document.CellRange `json:"bounds"`
}

func (a *App) showSpreadsheetPivotTableDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetPivotTableEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.this_workbook_does_not_support_57b4f1"}, feedbackWarning, notificationToast)
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

	a.modal.ShowMessage("spreadsheet.pivot_table", i18n.Message{ID: "spreadsheet.pivot_table.dialog.title"}, i18n.Message{ID: "spreadsheet.pivot_table.dialog.body", Args: i18n.Args{"Range": rangeStr}}, func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		generation, window := a.documentGeneration, state.virtualGrid
		token, ok := state.beginOperation(sessionOperationMutation)
		if !ok {
			return
		}
		a.launchManagedBackground(func() {
			spec := document.SpreadsheetPivotTableSpec{
				Sheet:     sheet,
				DataRange: bounds,
			}
			commandRouted := a.applicationSessionRegistered(token.documentID)
			changed := true
			var err error
			if commandRouted {
				result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.pivot.create", "spreadsheet_cell_range", "selection", sheet, spreadsheetPivotCreateArguments{Sheet: sheet, Bounds: bounds})
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
				err = editor.AddPivotTable(spec)
				unlock()
			}
			a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
			if err == nil {
				a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.pivot_table_inserted_584aa5"}, feedbackSuccess, notificationToast)
			}
			if a.formatResults != nil {
				select {
				case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "插入数据透视表", applied: changed}:
				case <-a.shutdown:
				}
			}
		})
	},
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "spreadsheet.pivot_table.action.create"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}
