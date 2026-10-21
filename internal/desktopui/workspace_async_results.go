package desktopui

import (
	"path/filepath"

	"glance/internal/i18n"
)

func (a *App) processAsyncResults() {
	a.ensureDesktopRuntime()
	a.processPDFWorkspaceRestoreResults()
	a.pollFontResults()
	a.processAgentSettingsResult()
	a.processAgentModelLookupResult()
	a.processAgentChatHistoryResults()
	a.processAgentSettingsKeyPreviewResult()
	a.processAgentLocalAccessUI()
	a.processAgentApplicationEvents()
	a.processAgentChatEvents()
	a.processPrimaryPromotion()
	a.surfacePendingWordRestoreError(a.documentSessionState)
	select {
	case result := <-a.wordEditorReviewResults:
		a.applyWordEditorReviewResult(result)
	default:
	}
	select {
	case result := <-a.wordCompareResults:
		a.applyWordCompareResult(result)
	default:
	}
	select {
	case result := <-a.wordVersionHistoryResults:
		a.applyWordVersionHistoryResult(result)
	default:
	}
	select {
	case err := <-a.windowOpenResults:
		if err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "workspace.error.create_window"}, err, notificationToast)
		}
	default:
	}
	a.pollGridPictureResults()
	a.processSpreadsheetSelectionStatsResults()
	a.processWordLayoutResults()
	select {
	case result := <-a.wordPDFExportResults:
		a.applyWordPDFExportResult(result)
	default:
	}
	select {
	case result := <-a.spreadsheetPDFExportResults:
		a.applySpreadsheetPDFExportResult(result)
	case result := <-a.spreadsheetPDFPreviewResults:
		a.applySpreadsheetPDFPreviewResult(result)
	case result := <-a.wordPDFPreviewResults:
		a.applyWordPDFPreviewResult(result)
	case result := <-a.presentationPDFPreviewResults:
		a.applyPresentationPDFPreviewResult(result)
	case result := <-a.presentationPDFExportResults:
		a.applyPresentationPDFExportResult(result)
	default:
	}
	select {
	case result := <-a.spreadsheetPrintResults:
		a.applySpreadsheetPrintResult(result)
	default:
	}
	select {
	case result := <-a.wordShareCopyResults:
		a.applyWordShareCopyResult(result)
	default:
	}
	select {
	case result := <-a.spreadsheetShareCopyResults:
		a.applySpreadsheetShareCopyResult(result)
	default:
	}
	select {
	case result := <-a.wordPrintResults:
		a.applyWordPrintResult(result)
	default:
	}
	select {
	case result := <-a.homeMetadataResults:
		if result.generation == a.homeMetadataGeneration {
			a.homeMetadata = result.entries
			if a.homeCanonicalPaths == nil {
				a.homeCanonicalPaths = make(map[string]string)
			}
			for path, canonical := range result.canonicalPaths {
				a.homeCanonicalPaths[path] = canonical
			}
			if a.window != nil {
				a.window.Invalidate()
			}
		}
	default:
	}
	select {
	case result := <-a.homeFolderContentsResults:
		a.applyHomeFolderContentsResult(result)
	default:
	}
	if a.processSpreadsheetFormatResult() {
		return
	}
	select {
	case result := <-a.fileCommandResults:
		a.fileCommandRunning = false
		if result.err != nil {
			a.setStatusErrorMessage(fileCommandFailureMessage(result.kind), result.err, notificationToast)
			if result.kind == commandExportPDF {
				a.wordPDFPickerRequest = nil
				a.spreadsheetPDFPickerRequest = nil
				a.presentationPDFPickerRequest = nil
			}
			if result.kind == commandShareCopy {
				a.wordShareCopyRequest = nil
				a.spreadsheetShareCopyRequest = nil
			}
			if result.kind == commandSaveAs && a.pendingCloseTabID != "" {
				a.showCloseConfirmationMessage(a.pendingCloseTabID,
					i18n.Message{ID: "document.save.failure.title"},
					i18n.Message{ID: "document.save.close_retry_body"})
			}
		} else {
			switch result.kind {
			case commandOpenFile:
				if result.path != "" {
					a.startOpen(result.path)
				} else {
					a.setStatusMessage(i18n.Message{ID: "file.command.open_cancelled"}, feedbackInfo, notificationSuppressed)
				}
			case commandOpenFolder:
				if result.path == "" {
					a.setStatusMessage(i18n.Message{ID: "file.command.folder_cancelled"}, feedbackInfo, notificationSuppressed)
				} else if a.collection == nil {
					a.setStatusMessage(i18n.Message{ID: "file.command.workspace_unavailable"}, feedbackError, notificationToast)
				} else if len(result.recents) == 0 {
					a.setStatusMessage(i18n.Message{ID: "file.command.folder_no_supported_files"}, feedbackWarning, notificationToast)
				} else {
					a.collection.StageRecents(result.recents)
					a.refreshHomeMetadata()
					a.persistRecentsAsync()
					a.setStatusMessage(i18n.Message{ID: "file.command.folder_opened", Args: i18n.Args{"Count": len(result.recents)}}, feedbackSuccess, notificationToast)
				}
			case commandRegisterHomeFolder:
				a.handleHomeFolderPickerResult(result)
			case commandRevealFile:
				if result.path == a.homeRecentMenuPath {
					a.dismissHomeRecentMenu()
				}
				a.setStatusMessage(i18n.Message{ID: "file.command.revealed"}, feedbackInfo, notificationSuppressed)
			case commandSaveAs:
				if result.path != "" {
					a.startSaveAs(result.path)
				} else {
					a.setStatusMessage(i18n.Message{ID: "file.command.save_as_cancelled"}, feedbackInfo, notificationSuppressed)
					if a.pendingCloseTabID != "" {
						a.pendingCloseTabID = ""
					}
				}
			case commandExportPDF:
				wordReq := a.wordPDFPickerRequest
				sheetReq := a.spreadsheetPDFPickerRequest
				presentationReq := a.presentationPDFPickerRequest
				a.wordPDFPickerRequest = nil
				a.spreadsheetPDFPickerRequest = nil
				a.presentationPDFPickerRequest = nil
				if result.path == "" {
					a.setStatusMessage(i18n.Message{ID: "file.command.pdf_cancelled"}, feedbackInfo, notificationSuppressed)
				} else if wordReq != nil {
					a.startWordPDFFileExport(wordReq.state, wordReq.key, result.path, wordReq.orientation)
				} else if sheetReq != nil {
					a.startSpreadsheetPDFFileExport(sheetReq.state, sheetReq.sheet, result.path, sheetReq.orientation)
				} else if presentationReq != nil {
					a.startPresentationPDFFileExport(presentationReq, result.path)
				} else {
					a.setStatusMessage(i18n.Message{ID: "file.command.pdf_request_expired"}, feedbackWarning, notificationToast)
				}
			case commandShareCopy:
				wordReq := a.wordShareCopyRequest
				sheetReq := a.spreadsheetShareCopyRequest
				a.wordShareCopyRequest = nil
				a.spreadsheetShareCopyRequest = nil
				if result.path == "" {
					a.setStatusMessage(i18n.Message{ID: "file.command.share_cancelled"}, feedbackInfo, notificationSuppressed)
				} else if wordReq != nil {
					a.startWordShareCopyFile(wordReq, result.path)
				} else if sheetReq != nil {
					a.startSpreadsheetShareCopyFile(sheetReq, result.path)
				} else {
					a.setStatusMessage(i18n.Message{ID: "file.command.share_request_expired"}, feedbackWarning, notificationToast)
				}
			}
		}
	default:
	}
	select {
	case result := <-a.wordImagePickerResults:
		a.handleWordImagePickerResult(result)
		if a.window != nil {
			a.window.Invalidate()
		}
	default:
	}
	select {
	case result := <-a.pptxPicPickerResults:
		a.handlePresentationPicturePickerResult(result)
		if a.window != nil {
			a.window.Invalidate()
		}
	default:
	}
	select {
	case result := <-a.pptxMediaPickerResults:
		a.handlePresentationMediaPickerResult(result)
		if a.window != nil {
			a.window.Invalidate()
		}
	default:
	}
	select {
	case result := <-a.pptxPicCommandResults:
		a.applyPresentationPictureCommandResult(result)
	default:
	}
	select {
	case result := <-a.xlsxImagePickerResults:
		a.handleSpreadsheetImagePickerResult(result)
		if a.window != nil {
			a.window.Invalidate()
		}
	default:
	}
	select {
	case result := <-a.wordDocumentInfoResults:
		if result.state == nil || !result.state.acceptsResult(result.token) {
			return
		}
		if result.generation == a.documentGeneration && result.state == a.documentSessionState {
			if result.err != nil {
				a.setStatusErrorMessage(i18n.Message{ID: "word.document_info.error.read"}, result.err, notificationToast)
			} else {
				a.showWordDocumentInfoDialog(result.properties)
			}
			if a.window != nil {
				a.window.Invalidate()
			}
		}
	default:
	}
	select {
	case result := <-a.wordMacrosInspectionResults:
		a.applyWordMacrosInspectionResult(result)
	default:
	}
	select {
	case result := <-a.wordVersionRestoreResults:
		if result.state == nil {
			return
		}
		if result.err != nil {
			a.publishWordRestoreNoticeMessage(result.state, i18n.Message{ID: "word.history.error.restore"})
		}
		if !result.state.acceptsResult(result.token) || result.state.wordRevisionAtomic.Load() != result.revision {
			return
		}
		activeResult := result.generation == a.documentGeneration && result.state == a.documentSessionState
		if result.err == nil && activeResult && result.applied {
			invalidateAllWordImagePreviews(result.state)
			reconcileWordSelection(result.state, true)
			a.setStatusMessage(i18n.Message{ID: "word.history.status.restored"}, feedbackSuccess, notificationToast)
			a.startWordPagination(result.state)
			a.persistActiveSession()
		}
		if activeResult {
			if a.window != nil {
				a.window.Invalidate()
			}
		}
	default:
	}
	if a.processWordCommandAsyncResult() {
		return
	}

	select {
	case result := <-a.wordImageResults:
		state := result.state
		if state != nil {
			if result.decodeSlot {
				if active := state.wordImageDecodeActive.Add(-1); active < 0 {
					state.wordImageDecodeActive.Store(0)
				}
			}
			if pendingVersion, pending := state.wordImagePending[result.id]; pending && pendingVersion == result.version {
				delete(state.wordImagePending, result.id)
			}
			if !state.closed.Load() && state.wordImageVersions[result.id] == result.version {
				if result.err != nil {
					state.wordImageErrors[result.id] = result.err.Error()
					if state == a.documentSessionState {
						a.setStatusErrorMessage(i18n.Message{ID: "word.image.error.preview"}, result.err, notificationToast)
					}
				} else if !state.wordImageCache.Put(result.id, result.image) {
					state.wordImageErrors[result.id] = a.text(i18n.Message{ID: "word.image.error.cache"})
					if state == a.documentSessionState {
						a.setStatusMessage(i18n.Message{ID: "word.image.error.cache"}, feedbackError, notificationToast)
					}
				} else {
					delete(state.wordImageErrors, result.id)
				}
			}
			if state == a.documentSessionState && a.window != nil {
				a.window.Invalidate()
			}
		}
	default:
	}
	select {
	case err := <-a.workspaceWriteResults:
		if err != nil {
			a.workspaceWriteErrorRef = i18n.Message{ID: "workspace.error.save_state"}
			a.setStatusErrorMessage(a.workspaceWriteErrorRef, err, notificationToast)
		} else {
			a.workspaceWriteErrorRef = i18n.Message{}
		}
	default:
	}
	select {
	case result := <-a.homeFolderWriteResults:
		a.handleHomeFolderWriteResult(result)
	default:
	}
	select {
	case result := <-a.homeRecentWriteResults:
		a.handleHomeRecentWriteResult(result)
	default:
	}
	select {
	case result := <-a.openResults:
		a.opening = false
		openSucceeded := false
		if result.err != nil {
			a.showDocumentOpenFailure(result.err)
		} else if err := a.installDocument(result.loaded); err != nil {
			a.showDocumentOpenFailure(err)
		} else {
			openSucceeded = true
			if result.historyErr != nil && result.loaded.word != nil {
				a.setStatusDetailMessage(i18n.Message{ID: "word.history.warning.unavailable"}, feedbackWarning, notificationToast, result.historyErr.Error())
			}
		}
		if openSucceeded && a.homeRecentOpenPath != "" && a.path == a.homeRecentOpenPath {
			a.dismissHomeRecentMenu()
		}
	default:
	}
	select {
	case result := <-a.saveResults:
		a.saving = false
		if result.err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "document.save.error.failed"}, result.err, notificationToast)
			if result.tabID == a.pendingCloseTabID && result.tabID != "" {
				a.showCloseConfirmationMessage(result.tabID,
					i18n.Message{ID: "document.save.failure.title"},
					i18n.Message{ID: "document.save.close_retry_body"})
			}
		} else {
			if result.saveAs {
				if state := a.sessions[result.tabID]; state != nil {
					a.updateSessionPath(state.sharedDocumentSession, result.path)
					unlock := lockSessionAdapterWrite(state)
					if state.book != nil {
						if saver, ok := state.book.(interface{ SetSourcePath(string) }); ok {
							saver.SetSourcePath(result.path)
						}
					}
					if state.word != nil {
						state.word.SetSourcePath(result.path)
					}
					if state.markdownDoc != nil {
						if state.markdownView != nil {
							unlock := lockSessionAdapterWrite(state)
							state.markdownView.draftEpoch.Add(1)
							unlock()
						}
						state.markdownDoc.SetSourcePath(result.path)
					}
					if state.pdfDoc != nil {
						state.pdfDoc.SetSourcePath(result.path)
					}
					if state.jsonDoc != nil {
						state.jsonDoc.SetSourcePath(result.path)
					}
					unlock()
				}
				if meta, ok := a.tabMeta[result.tabID]; ok {
					meta.SourcePath = result.path
					meta.Title = filepath.Base(result.path)
					a.tabMeta[result.tabID] = meta
				}
				if result.tabID == a.activeTabID {
					a.path = result.path
				}
			}
			if state := a.sessions[result.tabID]; state != nil && result.token.documentID != "" && state.wordRevisionAtomic.Load() != result.token.revision {
				if result.historyErr != nil {
					a.setStatusDetailMessage(i18n.Message{ID: "document.save.warning.saved_and_modified_history"}, feedbackWarning, notificationToast, result.historyErr.Error())
				} else {
					a.setStatusMessage(i18n.Message{ID: "document.save.status.saved_and_modified"}, feedbackWarning, notificationToast)
				}
				a.updatePersistedTab(a.tabMeta[result.tabID])
				a.maybeAutoSaveWord(a.documentSessionState)
				if a.window != nil {
					a.window.Invalidate()
				}
				return
			}
			if result.historyErr != nil {
				a.setStatusDetailMessage(i18n.Message{ID: "word.history.warning.save_failed"}, feedbackWarning, notificationToast, result.historyErr.Error())
			} else {
				a.setStatusMessage(i18n.Message{ID: "document.save.status.saved", Args: i18n.Args{"Path": result.path}}, feedbackSuccess, notificationToast)
			}
			if state := a.sessions[result.tabID]; state != nil {
				a.updatePersistedTab(a.tabMeta[result.tabID])
				if result.tabID == a.activeTabID {
					a.persistViewState()
				}
			}
			if result.tabID != "" && result.tabID == a.pendingCloseTabID {
				a.pendingCloseTabID = ""
				a.closeWorkspaceTab(result.tabID)
			}
			a.maybeAutoSaveWord(a.documentSessionState)
		}
	default:
	}
}

// A background paginator may enqueue several partials plus completion before
// Gio handles its coalesced invalidation. Consume a bounded batch before layout
// so the latest ready scene never waits for another user input event.
func (a *App) processWordLayoutResults() {
	for range max(1, cap(a.wordLayoutResults)) {
		select {
		case result := <-a.wordLayoutResults:
			a.applyWordLayoutResult(result)
		default:
			return
		}
	}
	if len(a.wordLayoutResults) > 0 && a.window != nil {
		a.window.Invalidate()
	}
}
