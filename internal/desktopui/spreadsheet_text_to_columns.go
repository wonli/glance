package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/document"
	"glance/internal/i18n"
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
			a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.text_to_columns.completed", Args: i18n.Args{"Rows": res.RowsSplit, "Columns": res.MaxCols}}, feedbackSuccess, notificationToast)
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
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.select_a_cell_range_containing_33d185"}, feedbackWarning, notificationToast)
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

	splitWith := func(delim, label string) {
		a.runSpreadsheetTextToColumnsCommand("文本分列 ("+label+")", spreadsheettextSheetBoundsArguments{Sheet: sheet, Bounds: bounds, KeyColumn: keyCol, Delimiter: delim}, func(ed document.SpreadsheetTextToColumnsEditor) (document.SpreadsheetTextToColumnsResult, error) {
			return ed.SplitTextToColumns(sheet, bounds, keyCol, delim)
		})
	}

	a.modal.ShowMessage("spreadsheet.text_to_columns", i18n.Message{ID: "spreadsheet.text_to_columns.dialog.title"}, i18n.Message{ID: "spreadsheet.text_to_columns.dialog.body", Args: i18n.Args{"Range": rangeRef, "Column": activeColName}}, func(action string) {
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
			a.modal.ShowInputMessage("spreadsheet.text_to_columns.custom", i18n.Message{ID: "spreadsheet.text_to_columns.custom.title"}, i18n.Message{ID: "spreadsheet.text_to_columns.custom.body"}, ",", i18n.Message{ID: "spreadsheet.text_to_columns.custom.hint"}, func(act string) {
				delim := a.modal.Input.Text()
				a.modal.Hide()
				if act != "confirm" {
					return
				}
				if delim == "\\t" {
					delim = "\t"
				}
				if delim == "" {
					a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.the_delimiter_cannot_be_empty_18c4fc"}, feedbackError, notificationToast)
					return
				}
				splitWith(delim, "自定义")
			}, modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "common.action.confirm"}}, modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}})
		}
	},
		modalDialogActionSpec{ID: "comma", LabelRef: i18n.Message{ID: "spreadsheet.text_to_columns.delimiter.comma"}},
		modalDialogActionSpec{ID: "semicolon", LabelRef: i18n.Message{ID: "spreadsheet.text_to_columns.delimiter.semicolon"}},
		modalDialogActionSpec{ID: "space", LabelRef: i18n.Message{ID: "spreadsheet.text_to_columns.delimiter.space"}},
		modalDialogActionSpec{ID: "tab", LabelRef: i18n.Message{ID: "spreadsheet.text_to_columns.delimiter.tab"}},
		modalDialogActionSpec{ID: "custom", LabelRef: i18n.Message{ID: "spreadsheet.text_to_columns.delimiter.custom"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}
