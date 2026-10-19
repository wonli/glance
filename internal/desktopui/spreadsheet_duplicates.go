package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/document"
	"glance/internal/i18n"
)

func (a *App) runSpreadsheetDuplicatesCommand(operation string, arguments any, edit func(document.SpreadsheetDuplicatesEditor) (document.SpreadsheetDuplicatesResult, error)) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetDuplicatesEditor)
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
		var res document.SpreadsheetDuplicatesResult
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.duplicates.remove", "spreadsheet_cell_range", "selection", sheet, arguments)
			err = commandErr
			if err == nil {
				err = json.Unmarshal(result.Data, &res)
			}
		} else {
			unlock := lockSessionAdapterWrite(state)
			res, err = edit(editor)
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && res.RemovedCount > 0 && !commandRouted)
		if err == nil {
			a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.duplicates.removed", Args: i18n.Args{"Removed": res.RemovedCount, "Remaining": res.RemainingCount}}, feedbackSuccess, notificationToast)
		}
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation}:
		case <-a.shutdown:
		}
	})
}

type spreadsheetduplicatesGoSheetBoundsArguments struct {
	Sheet     string             `json:"sheet"`
	Bounds    document.CellRange `json:"bounds"`
	KeyColumn int                `json:"key_column"`
}

func (a *App) showSpreadsheetRemoveDuplicatesDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	if !state.selectionSet {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.select_a_cell_range_containing_33d185"}, feedbackWarning, notificationToast)
		return
	}
	bounds := state.selection.Bounds()
	rowCount := bounds.EndRow - bounds.StartRow + 1
	if rowCount <= 1 {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.select_at_least_two_rows_17c8ab"}, feedbackInfo, notificationSuppressed)
		return
	}
	sheet := state.sheet
	rangeRef := fmt.Sprintf("%s%d:%s%d", columnName(bounds.StartColumn), bounds.StartRow, columnName(bounds.EndColumn), bounds.EndRow)
	keyCol := state.selection.Active.Column + 1
	activeColName := columnName(keyCol)

	a.modal.ShowMessage("spreadsheet.duplicates", i18n.Message{ID: "spreadsheet.duplicates.dialog.title"}, i18n.Message{ID: "spreadsheet.duplicates.dialog.body", Args: i18n.Args{"Range": rangeRef, "Rows": rowCount, "Column": activeColName}}, func(action string) {
		a.modal.Hide()
		switch action {
		case "col":
			a.runSpreadsheetDuplicatesCommand("按列删除重复项", spreadsheetduplicatesGoSheetBoundsArguments{Sheet: sheet, Bounds: bounds, KeyColumn: keyCol}, func(ed document.SpreadsheetDuplicatesEditor) (document.SpreadsheetDuplicatesResult, error) {
				return ed.RemoveDuplicates(sheet, bounds, keyCol)
			})
		case "all":
			a.runSpreadsheetDuplicatesCommand("按整行删除重复项", spreadsheetduplicatesGoSheetBoundsArguments{Sheet: sheet, Bounds: bounds, KeyColumn: 0}, func(ed document.SpreadsheetDuplicatesEditor) (document.SpreadsheetDuplicatesResult, error) {
				return ed.RemoveDuplicates(sheet, bounds, 0)
			})
		}
	},
		modalDialogActionSpec{ID: "col", LabelRef: i18n.Message{ID: "spreadsheet.duplicates.action.by_column", Args: i18n.Args{"Column": activeColName}}},
		modalDialogActionSpec{ID: "all", LabelRef: i18n.Message{ID: "spreadsheet.duplicates.action.all_columns"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}
