package desktopui

import (
	"errors"

	"glance/internal/format/xlsx"
	"glance/internal/i18n"
)

type spreadsheetShareCopyPickerRequest struct {
	state        *documentSessionState
	historyState uint64
}

type spreadsheetShareCopyResult struct {
	state *documentSessionState
	path  string
	err   error
}

func (a *App) requestSpreadsheetShareCopy() {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.closed.Load() || state.formatRunning.Load() || a.isSaving() || a.fileCommandRunning {
		return
	}
	if a.cellEdit.Editing {
		_ = a.commitGridCell(a.cellEdit.Draft)
	}
	a.beginSpreadsheetShareCopy(state)
}

func (a *App) beginSpreadsheetShareCopy(state *documentSessionState) {
	if state == nil || state.book == nil || state.closed.Load() || state.formatRunning.Load() || a.isSaving() || a.fileCommandRunning {
		return
	}
	if a.documentSessionState != state {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.share.error.document_switched"}, feedbackWarning, notificationToast)
		return
	}
	var histState uint64
	if h, ok := state.book.(interface{ HistoryState() uint64 }); ok {
		histState = h.HistoryState()
	}
	request := &spreadsheetShareCopyPickerRequest{
		state:        state,
		historyState: histState,
	}
	a.spreadsheetShareCopyRequest = request
	a.startFileCommand(commandShareCopy, "xlsx")
}

func (a *App) startSpreadsheetShareCopyFile(request *spreadsheetShareCopyPickerRequest, path string) {
	if request == nil || request.state == nil || request.state.book == nil || request.state.closed.Load() {
		a.setStatusMessage(i18n.Message{ID: "share.copy.error.expired"}, feedbackWarning, notificationToast)
		return
	}
	state := request.state
	if a.documentSessionState != state {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.share.error.document_switched"}, feedbackWarning, notificationToast)
		return
	}
	if state.formatRunning.Load() {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.share.error.operation_busy_during_picker"}, feedbackWarning, notificationToast)
		return
	}
	if h, ok := state.book.(interface{ HistoryState() uint64 }); ok && h.HistoryState() != request.historyState {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.share.error.changed_during_picker"}, feedbackWarning, notificationToast)
		return
	}
	path = ensureShareCopyExtension(path, "xlsx")
	if sameDocumentPath(path, state.path) {
		a.setStatusMessage(i18n.Message{ID: "share.copy.error.overwrite_source", Args: i18n.Args{"Format": "XLSX"}}, feedbackError, notificationToast)
		return
	}
	if a.applicationSessionRegistered(state.documentID) {
		results, shutdown := a.spreadsheetShareCopyResults, a.shutdown
		if results == nil {
			a.setStatusMessage(i18n.Message{ID: "share.copy.error.unavailable"}, feedbackError, notificationToast)
			return
		}
		if a.startDocumentExportCopy(state, path, state.wordRevisionAtomic.Load(), func(outputPath string, err error) {
			select {
			case results <- spreadsheetShareCopyResult{state: state, path: outputPath, err: err}:
			case <-shutdown:
			}
		}) {
			a.setStatusMessage(i18n.Message{ID: "share.copy.status.exporting", Args: i18n.Args{"Format": "XLSX"}}, feedbackInfo, notificationSuppressed)
			return
		}
	}

	book := state.book
	snapshooter, ok := book.(interface{ Snapshot() ([]byte, error) })
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.share.error.snapshot_unsupported"}, feedbackError, notificationToast)
		return
	}
	histState := request.historyState
	token, ok := state.beginOperation(sessionOperationSnapshot)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "share.copy.error.operation_busy"}, feedbackWarning, notificationToast)
		return
	}
	results, shutdown, window := a.spreadsheetShareCopyResults, a.shutdown, a.window
	if results == nil {
		state.finishOperation(token)
		a.setStatusMessage(i18n.Message{ID: "share.copy.error.unavailable"}, feedbackError, notificationToast)
		return
	}
	a.launchManagedBackground(func() {
		defer state.finishOperation(token)
		result := spreadsheetShareCopyResult{state: state, path: path}
		result.err = saveShareCopySnapshot(
			path,
			"XLSX",
			func() ([]byte, error) {
				unlock := lockSessionAdapterRead(state)
				snapshot, err := snapshooter.Snapshot()
				unlock()
				return snapshot, err
			},
			func() error {
				if state.closed.Load() || (func() bool {
					if h, ok := book.(interface{ HistoryState() uint64 }); ok && h.HistoryState() != histState {
						return true
					}
					return false
				})() {
					return errors.New("工作簿在准备共享副本时已更改")
				}
				return nil
			},
			xlsx.OpenSnapshot,
			func() (func(), error) {
				state.spreadsheetExportCommitMu.Lock()
				release := state.spreadsheetExportCommitMu.Unlock
				if state.closed.Load() || (func() bool {
					if h, ok := book.(interface{ HistoryState() uint64 }); ok && h.HistoryState() != histState {
						return true
					}
					return false
				})() {
					release()
					return nil, errors.New("工作簿在导出期间已更改，未覆盖目标文件")
				}
				return release, nil
			},
		)
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	})
	a.setStatusMessage(i18n.Message{ID: "share.copy.status.exporting", Args: i18n.Args{"Format": "XLSX"}}, feedbackInfo, notificationSuppressed)
}

func (a *App) applySpreadsheetShareCopyResult(result spreadsheetShareCopyResult) {
	a.applyShareCopyResult(result.state, result.path, result.err, "XLSX")
}
