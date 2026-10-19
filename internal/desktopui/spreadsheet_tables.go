package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/document"
	"glance/internal/i18n"
)

func (a *App) runSpreadsheetTableCommand(operation string, arguments any, edit func(document.SpreadsheetTableEditor) error) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetTableEditor)
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
		changed := true
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.table.create", "spreadsheet_cell_range", "selection", sheet, arguments)
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
			err = edit(editor)
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		if err == nil {
			a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.table_created_c91cb4"}, feedbackSuccess, notificationToast)
		}
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation, applied: changed}:
		case <-a.shutdown:
		}
	})
}

type showSpreadsheetInsertTableDialogArguments struct {
	Sheet     string             `json:"sheet"`
	Bounds    document.CellRange `json:"bounds"`
	StyleName string             `json:"style_name"`
}

func (a *App) showSpreadsheetInsertTableDialog(styleName string) {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	if !state.selectionSet {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.select_the_cell_range_for_35f60f"}, feedbackWarning, notificationToast)
		return
	}
	bounds := state.selection.Bounds()
	if bounds.EndRow <= bounds.StartRow {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.select_at_least_two_rows_8cad65"}, feedbackInfo, notificationSuppressed)
		return
	}
	sheet := state.sheet
	rangeRef := fmt.Sprintf("%s%d:%s%d", columnName(bounds.StartColumn), bounds.StartRow, columnName(bounds.EndColumn), bounds.EndRow)
	rowCount := bounds.EndRow - bounds.StartRow + 1
	colCount := bounds.EndColumn - bounds.StartColumn + 1

	createTable := func(style string) {
		a.runSpreadsheetTableCommand("创建表格", showSpreadsheetInsertTableDialogArguments{Sheet: sheet, Bounds: bounds, StyleName: style}, func(ed document.SpreadsheetTableEditor) error {
			return ed.AddTable(document.SpreadsheetTableSpec{
				Sheet:     sheet,
				Range:     bounds,
				StyleName: style,
			})
		})
	}

	if styleName != "" {
		createTable(styleName)
		return
	}

	a.modal.ShowMessage("spreadsheet.table.create", i18n.Message{ID: "spreadsheet.table.dialog.title"}, i18n.Message{ID: "spreadsheet.table.dialog.body", Args: i18n.Args{"Range": rangeRef, "Rows": rowCount, "Columns": colCount}}, func(action string) {
		a.modal.Hide()
		switch action {
		case "medium":
			createTable("TableStyleMedium2")
		case "light":
			createTable("TableStyleLight1")
		case "dark":
			createTable("TableStyleDark1")
		}
	},
		modalDialogActionSpec{ID: "medium", LabelRef: i18n.Message{ID: "spreadsheet.table.style.medium_recommended"}},
		modalDialogActionSpec{ID: "light", LabelRef: i18n.Message{ID: "spreadsheet.table.style.light"}},
		modalDialogActionSpec{ID: "dark", LabelRef: i18n.Message{ID: "spreadsheet.table.style.dark"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}
