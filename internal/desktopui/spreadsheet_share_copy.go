package desktopui

import (
	"errors"

	"glance/internal/format/xlsx"
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
		a.setStatus("工作簿已切换，共享副本导出已取消")
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
		a.setStatus("共享副本状态已过期，请重新导出")
		return
	}
	state := request.state
	if a.documentSessionState != state {
		a.setStatus("工作簿已切换，共享副本导出已取消")
		return
	}
	if state.formatRunning.Load() {
		a.setStatus("工作簿在选择位置期间正在处理操作，未导出共享副本；请重试")
		return
	}
	if h, ok := state.book.(interface{ HistoryState() uint64 }); ok && h.HistoryState() != request.historyState {
		a.setStatus("工作簿在选择位置期间已更改，未导出共享副本；请重试")
		return
	}
	path = ensureShareCopyExtension(path, "xlsx")
	if sameDocumentPath(path, state.path) {
		a.setStatus("共享副本不能覆盖当前 XLSX")
		return
	}
	if a.applicationSessionRegistered(state.documentID) {
		results, shutdown := a.spreadsheetShareCopyResults, a.shutdown
		if results == nil {
			a.setStatus("共享副本导出不可用")
			return
		}
		if a.startDocumentExportCopy(state, path, state.wordRevisionAtomic.Load(), func(outputPath string, err error) {
			select {
			case results <- spreadsheetShareCopyResult{state: state, path: outputPath, err: err}:
			case <-shutdown:
			}
		}) {
			a.setStatus("正在导出共享 XLSX 副本…")
			return
		}
	}

	book := state.book
	snapshooter, ok := book.(interface{ Snapshot() ([]byte, error) })
	if !ok {
		a.setStatus("当前表格格式不支持导出共享副本")
		return
	}
	histState := request.historyState
	token, ok := state.beginOperation(sessionOperationSnapshot)
	if !ok {
		a.setStatus("文档正在处理其他操作，请稍后重试")
		return
	}
	results, shutdown, window := a.spreadsheetShareCopyResults, a.shutdown, a.window
	if results == nil {
		state.finishOperation(token)
		a.setStatus("共享副本导出不可用")
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
	a.setStatus("正在导出共享 XLSX 副本…")
}

func (a *App) applySpreadsheetShareCopyResult(result spreadsheetShareCopyResult) {
	a.applyShareCopyResult(result.state, result.path, result.err, "XLSX")
}
