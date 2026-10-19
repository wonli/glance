package desktopui

import (
	"context"
	"glance/internal/i18n"
	"path/filepath"
	"strings"

	"glance/internal/document"
)

type spreadsheetPDFPickerRequest struct {
	state       *documentSessionState
	sheet       string
	orientation string
}

type spreadsheetPDFExportResult struct {
	state *documentSessionState
	sheet string
	path  string
	err   error
}

func (a *App) requestSpreadsheetPDFExport() {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.closed.Load() || state.formatRunning.Load() || a.isSaving() || a.fileCommandRunning || state.spreadsheetPDFExportRunning {
		return
	}
	if a.cellEdit.Editing {
		_ = a.commitGridCell(a.cellEdit.Draft)
	}
	a.openSpreadsheetPDFExportPreview(state, state.sheet)
}

func (a *App) startSpreadsheetPDFSavePicker(state *documentSessionState, sheet string, orientation string) {
	if state == nil || state.book == nil || a.fileCommandRunning {
		return
	}
	a.spreadsheetPDFPickerRequest = &spreadsheetPDFPickerRequest{state: state, sheet: sheet, orientation: orientation}
	a.startFileCommand(commandExportPDF, "pdf")
}

type spreadsheetDocumentExportPdfArguments struct {
	Sheet       string             `json:"sheet"`
	Bounds      document.CellRange `json:"bounds"`
	TargetPath  string             `json:"target_path"`
	Orientation string             `json:"orientation"`
}

func (a *App) startSpreadsheetPDFFileExport(state *documentSessionState, sheet string, path string, orientation string) {
	if state == nil || state.book == nil || state.closed.Load() {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.the_pdf_export_request_expired_675efe"}, feedbackInfo, notificationToast)
		return
	}
	if pathsReferToSameFile(path, state.path) {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.the_pdf_destination_cannot_overwrite_68d2dd"}, feedbackWarning, notificationToast)
		return
	}
	if strings.ToLower(filepath.Ext(path)) != ".pdf" {
		path += ".pdf"
	}
	if a.documentSessionState != state {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.the_active_workbook_changed_pdf_0da394"}, feedbackInfo, notificationToast)
		return
	}
	dimensions, ok := state.book.(interface {
		Dimension(string) (int, int, error)
	})
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.this_spreadsheet_cannot_read_its_3128d6"}, feedbackWarning, notificationToast)
		return
	}
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
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.the_worksheet_range_exceeds_the_f33a09"}, feedbackError, notificationToast)
		return
	}
	bounds := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: maxRow, EndColumn: maxColumn}
	if !a.applicationSessionRegistered(state.documentID) {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.the_mcp_application_runtime_is_898677"}, feedbackWarning, notificationToast)
		return
	}
	token, ok := state.beginOperation(sessionOperationSnapshot)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.the_spreadsheet_is_busy_try_f8f66c"}, feedbackWarning, notificationToast)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	state.spreadsheetPDFExportCancel = cancel
	state.spreadsheetPDFExportRunning = true
	results, shutdown, appWindow := a.spreadsheetPDFExportResults, a.shutdown, a.window
	if results == nil {
		state.finishOperation(token)
		cancel()
		state.spreadsheetPDFExportCancel = nil
		state.spreadsheetPDFExportRunning = false
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.pdf_export_is_unavailable_42fdd1"}, feedbackWarning, notificationToast)
		return
	}

	stateCopy := state
	a.launchManagedBackground(func() {
		defer cancel()
		defer stateCopy.finishOperation(token)
		result := spreadsheetPDFExportResult{state: stateCopy, sheet: sheet, path: path}
		_, result.err = a.executeRibbonApplicationCommandWithContext(ctx, token,
			"spreadsheet.document.export_pdf", "spreadsheet_sheet", sheet, sheet, spreadsheetDocumentExportPdfArguments{Sheet: sheet, Bounds: bounds, TargetPath: path, Orientation: orientation},
		)
		select {
		case results <- result:
			if appWindow != nil {
				appWindow.Invalidate()
			}
		case <-shutdown:
		}
	})
	a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.generating_the_spreadsheet_pdf_0e7f93"}, feedbackInfo, notificationSuppressed)
}

func (a *App) applySpreadsheetPDFExportResult(result spreadsheetPDFExportResult) {
	state := result.state
	if state == nil {
		return
	}
	state.spreadsheetPDFExportRunning = false
	state.spreadsheetPDFExportCancel = nil
	if state.closed.Load() {
		return
	}
	if result.err != nil {
		if state == a.documentSessionState {
			a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.error.operation_failed"}, result.err, notificationToast)
		}
	} else if state == a.documentSessionState {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.pdf_exported", Args: i18n.Args{"File": filepath.Base(result.path)}}, feedbackSuccess, notificationToast)
	}
}
