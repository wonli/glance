package desktopui

import (
	"context"
	"fmt"
	"glance/internal/i18n"

	"glance/internal/document"
)

type spreadsheetPrintResult struct {
	state      *documentSessionState
	generation uint64
	revision   uint64
	err        error
}

func (a *App) requestSpreadsheetPrint() {
	state := a.documentSessionState
	if state == nil || state.book == nil || a.nativePrinter == nil || !a.nativePrinter.Available() {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.native_printing_is_not_available_0eccba"}, feedbackWarning, notificationToast)
		return
	}
	if state.closed.Load() || state.formatRunning.Load() || a.isSaving() || a.fileCommandRunning || state.spreadsheetPrintRunning {
		return
	}
	if a.cellEdit.Editing {
		if err := a.commitGridCell(a.cellEdit.Draft); err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.error.operation_failed"}, err, notificationToast)
			return
		}
	}
	dimensions, ok := state.book.(interface {
		Dimension(string) (int, int, error)
	})
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.this_workbook_does_not_support_548458"}, feedbackWarning, notificationToast)
		return
	}
	sheet := state.sheet
	unlock := lockSessionAdapterRead(state)
	maxColumn, maxRow, err := dimensions.Dimension(sheet)
	unlock()
	if err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.error.operation_failed"}, err, notificationToast)
		return
	}
	if maxColumn <= 0 || maxRow <= 0 {
		maxColumn, maxRow = 1, 1
	}
	if maxColumn > 100 || maxRow > 5000 {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.the_worksheet_used_range_exceeds_798ae8"}, feedbackError, notificationToast)
		return
	}
	bounds := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: maxRow, EndColumn: maxColumn}
	revision := state.wordRevisionAtomic.Load()
	generation := a.documentGeneration
	rangeLabel := fmt.Sprintf("%s%d:%s%d", columnName(bounds.StartColumn), bounds.StartRow, columnName(bounds.EndColumn), bounds.EndRow)
	a.modal.ShowMessage("spreadsheet.print", i18n.Message{ID: "spreadsheet.print.dialog.title"}, i18n.Message{ID: "spreadsheet.print.dialog.body", Args: i18n.Args{"Sheet": sheet, "Range": rangeLabel, "Orientation": a.text(i18n.Message{ID: "spreadsheet.page_layout.orientation.portrait"})}}, func(action string) {
		a.modal.Hide()
		if action != "print" {
			return
		}
		if state != a.documentSessionState || state.closed.Load() || generation != a.documentGeneration || state.wordRevisionAtomic.Load() != revision {
			a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.the_workbook_changed_during_confirmation_8aa4b0"}, feedbackInfo, notificationToast)
			return
		}
		a.beginSpreadsheetPrint(state, sheet, bounds)
	},
		modalDialogActionSpec{ID: "print", LabelRef: i18n.Message{ID: "spreadsheet.print.action.confirm"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

type spreadsheetDocumentPrintArguments struct {
	Sheet       string             `json:"sheet"`
	Bounds      document.CellRange `json:"bounds"`
	Orientation string             `json:"orientation"`
}

func (a *App) beginSpreadsheetPrint(state *documentSessionState, sheet string, bounds document.CellRange) {
	if state == nil || state.book == nil || state.closed.Load() || a.documentSessionState != state {
		return
	}
	if !a.applicationSessionRegistered(state.documentID) {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.the_mcp_application_runtime_is_c4923c"}, feedbackWarning, notificationToast)
		return
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.the_workbook_is_busy_nothing_8afb04"}, feedbackInfo, notificationToast)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	state.spreadsheetPrintCancel = cancel
	state.spreadsheetPrintRunning = true
	results, shutdown, window := a.spreadsheetPrintResults, a.shutdown, a.window
	if results == nil {
		state.finishOperation(token)
		cancel()
		state.spreadsheetPrintCancel = nil
		state.spreadsheetPrintRunning = false
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.the_print_service_is_unavailable_cd0e6b"}, feedbackWarning, notificationToast)
		return
	}
	generation := a.documentGeneration
	a.launchManagedBackground(func() {
		result := spreadsheetPrintResult{state: state, generation: generation, revision: token.revision}
		command, err := a.executeRibbonApplicationCommandWithContext(ctx, token, "spreadsheet.document.print", "spreadsheet_range", sheet, sheet, spreadsheetDocumentPrintArguments{Sheet: sheet, Bounds: bounds, Orientation: "portrait"})
		result.err = err
		if err == nil {
			result.revision = command.Revision
		}
		state.finishOperation(token)
		cancel()
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	})
	a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.generating_and_sending_worksheet_print_d51eeb"}, feedbackInfo, notificationToast)
}

func (a *App) applySpreadsheetPrintResult(result spreadsheetPrintResult) {
	state := result.state
	if state == nil {
		return
	}
	state.spreadsheetPrintRunning = false
	if state.spreadsheetPrintCancel != nil {
		state.spreadsheetPrintCancel()
		state.spreadsheetPrintCancel = nil
	}
	if state.closed.Load() || state != a.documentSessionState || result.generation != a.documentGeneration {
		return
	}
	if result.err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.error.operation_failed"}, result.err, notificationToast)
	} else {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.print_submitted", Args: i18n.Args{"Revision": result.revision}}, feedbackSuccess, notificationToast)
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}
