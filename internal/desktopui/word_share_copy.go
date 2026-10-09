package desktopui

import (
	"errors"

	"glance/internal/format/docx"
)

type wordShareCopyPickerRequest struct {
	state      *documentSessionState
	revision   uint64
	draftEpoch uint64
}

type wordShareCopyResult struct {
	state *documentSessionState
	path  string
	err   error
}

func (a *App) requestWordShareCopy() {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.closed.Load() || state.formatRunning.Load() || a.isSaving() || a.fileCommandRunning {
		return
	}
	if state.wordFlushRequested.Load() || state.wordDeferredToolbarAction != "" {
		a.setStatus("正文正在提交，请稍后再导出共享副本")
		return
	}
	if wordHasPendingTextDrafts(state) {
		state.wordForceDraftFlush.Store(true)
		state.wordFlushRequested.Store(true)
		state.wordFlushAllDrafts.Store(true)
		state.wordDeferredToolbarAction = "share"
		a.setStatus("正在提交正文编辑，然后准备共享 DOCX 副本…")
		a.startNextWordTextCommit()
		return
	}
	a.beginWordShareCopy(state)
}

func (a *App) beginWordShareCopy(state *documentSessionState) {
	if state == nil || state.word == nil || state.closed.Load() || state.formatRunning.Load() || a.isSaving() || a.fileCommandRunning {
		return
	}
	if a.documentSessionState != state {
		a.setStatus("Word 文档已切换，共享副本导出已取消")
		return
	}
	if state.wordFlushRequested.Load() || wordHasPendingTextDrafts(state) {
		a.setStatus("正文编辑尚未提交，未导出共享副本")
		return
	}
	request := &wordShareCopyPickerRequest{
		state:      state,
		revision:   state.wordRevisionAtomic.Load(),
		draftEpoch: state.wordDraftEpoch.Load(),
	}
	a.wordShareCopyRequest = request
	a.startFileCommand(commandShareCopy, "docx")
}

func (a *App) startWordShareCopyFile(request *wordShareCopyPickerRequest, path string) {
	if request == nil || request.state == nil || request.state.word == nil || request.state.closed.Load() {
		a.setStatus("共享副本状态已过期，请重新导出")
		return
	}
	state := request.state
	if a.documentSessionState != state {
		a.setStatus("Word 文档已切换，共享副本导出已取消")
		return
	}
	if state.formatRunning.Load() || state.wordRevisionAtomic.Load() != request.revision || state.wordDraftEpoch.Load() != request.draftEpoch || state.wordFlushRequested.Load() || wordHasPendingTextDrafts(state) {
		a.setStatus("文档在选择位置期间已更改，未导出共享副本；请重试")
		return
	}
	path = ensureShareCopyExtension(path, "docx")
	if sameDocumentPath(path, state.path) {
		a.setStatus("共享副本不能覆盖当前 DOCX")
		return
	}

	word := state.word
	revision, draftEpoch := request.revision, request.draftEpoch
	token, ok := state.beginOperation(sessionOperationSnapshot)
	if !ok {
		a.setStatus("文档正在处理其他操作，请稍后重试")
		return
	}
	results, shutdown, window := a.wordShareCopyResults, a.shutdown, a.window
	if results == nil {
		state.finishOperation(token)
		a.setStatus("共享副本导出不可用")
		return
	}
	go func() {
		defer state.finishOperation(token)
		result := wordShareCopyResult{state: state, path: path}
		result.err = saveShareCopySnapshot(
			path,
			"DOCX",
			func() ([]byte, error) {
				unlock := lockSessionAdapterRead(state)
				snapshot, err := word.Snapshot()
				unlock()
				return snapshot, err
			},
			func() error {
				if state.closed.Load() || state.wordRevisionAtomic.Load() != revision || state.wordDraftEpoch.Load() != draftEpoch {
					return errors.New("Word 文档在准备共享副本时已更改")
				}
				return nil
			},
			docx.OpenSnapshot,
			func() (func(), error) {
				state.wordExportCommitMu.Lock()
				release := state.wordExportCommitMu.Unlock
				if state.closed.Load() || state.wordRevisionAtomic.Load() != revision || state.wordDraftEpoch.Load() != draftEpoch {
					release()
					return nil, errors.New("Word 文档在导出期间已更改，未覆盖目标文件")
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
	}()
	a.setStatus("正在导出共享 DOCX 副本…")
}

func (a *App) applyWordShareCopyResult(result wordShareCopyResult) {
	a.applyShareCopyResult(result.state, result.path, result.err, "DOCX")
}
