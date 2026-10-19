package desktopui

import (
	"errors"

	"glance/internal/format/docx"
	"glance/internal/i18n"
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
		a.setStatusMessage(i18n.Message{ID: "word.share.error.flush_in_progress"}, feedbackWarning, notificationSuppressed)
		return
	}
	if wordHasPendingTextDrafts(state) {
		state.wordForceDraftFlush.Store(true)
		state.wordFlushRequested.Store(true)
		state.wordFlushAllDrafts.Store(true)
		state.wordDeferredToolbarAction = "share"
		a.setStatusMessage(i18n.Message{ID: "word.share.status.commit_before_export"}, feedbackInfo, notificationSuppressed)
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
		a.setStatusMessage(i18n.Message{ID: "word.share.error.document_switched"}, feedbackWarning, notificationToast)
		return
	}
	if state.wordFlushRequested.Load() || wordHasPendingTextDrafts(state) {
		a.setStatusMessage(i18n.Message{ID: "word.share.error.pending_draft"}, feedbackWarning, notificationToast)
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
		a.setStatusMessage(i18n.Message{ID: "share.copy.error.expired"}, feedbackWarning, notificationToast)
		return
	}
	state := request.state
	if a.documentSessionState != state {
		a.setStatusMessage(i18n.Message{ID: "word.share.error.document_switched"}, feedbackWarning, notificationToast)
		return
	}
	if state.formatRunning.Load() || state.wordRevisionAtomic.Load() != request.revision || state.wordDraftEpoch.Load() != request.draftEpoch || state.wordFlushRequested.Load() || wordHasPendingTextDrafts(state) {
		a.setStatusMessage(i18n.Message{ID: "word.share.error.changed_before_export"}, feedbackWarning, notificationToast)
		return
	}
	path = ensureShareCopyExtension(path, "docx")
	if sameDocumentPath(path, state.path) {
		a.setStatusMessage(i18n.Message{ID: "share.copy.error.overwrite_source", Args: i18n.Args{"Format": "DOCX"}}, feedbackError, notificationToast)
		return
	}
	if a.applicationSessionRegistered(state.documentID) {
		results, shutdown := a.wordShareCopyResults, a.shutdown
		if results == nil {
			a.setStatusMessage(i18n.Message{ID: "share.copy.error.unavailable"}, feedbackError, notificationToast)
			return
		}
		if a.startDocumentExportCopy(state, path, request.revision, func(outputPath string, err error) {
			select {
			case results <- wordShareCopyResult{state: state, path: outputPath, err: err}:
			case <-shutdown:
			}
		}) {
			a.setStatusMessage(i18n.Message{ID: "share.copy.status.exporting", Args: i18n.Args{"Format": "DOCX"}}, feedbackInfo, notificationSuppressed)
			return
		}
	}

	word := state.word
	revision, draftEpoch := request.revision, request.draftEpoch
	token, ok := state.beginOperation(sessionOperationSnapshot)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "share.copy.error.operation_busy"}, feedbackWarning, notificationToast)
		return
	}
	results, shutdown, window := a.wordShareCopyResults, a.shutdown, a.window
	if results == nil {
		state.finishOperation(token)
		a.setStatusMessage(i18n.Message{ID: "share.copy.error.unavailable"}, feedbackError, notificationToast)
		return
	}
	a.launchManagedBackground(func() {
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
	})
	a.setStatusMessage(i18n.Message{ID: "share.copy.status.exporting", Args: i18n.Args{"Format": "DOCX"}}, feedbackInfo, notificationSuppressed)
}

func (a *App) applyWordShareCopyResult(result wordShareCopyResult) {
	a.applyShareCopyResult(result.state, result.path, result.err, "DOCX")
}
