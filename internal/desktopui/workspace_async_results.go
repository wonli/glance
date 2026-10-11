package desktopui

import (
	"fmt"
	"path/filepath"
)

func (a *App) processAsyncResults() {
	a.ensureDesktopRuntime()
	a.processAgentSettingsResult()
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
			a.setStatus("创建新窗口失败：" + err.Error())
		}
	default:
	}
	a.pollGridPictureResults()
	select {
	case result := <-a.wordLayoutResults:
		a.applyWordLayoutResult(result)
	default:
	}
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
			if a.window != nil {
				a.window.Invalidate()
			}
		}
	default:
	}
	if a.processSpreadsheetFormatResult() {
		return
	}
	select {
	case result := <-a.fileCommandResults:
		a.fileCommandRunning = false
		if result.err != nil {
			a.setStatus(result.err.Error())
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
				a.showCloseConfirmation(a.pendingCloseTabID, "保存失败", "无法选择另存为位置。可以重试保存、不保存并关闭，或取消关闭。")
			}
		} else {
			switch result.kind {
			case commandOpenFile:
				if result.path != "" {
					a.startOpen(result.path)
				} else {
					a.setStatus("已取消打开文件")
				}
			case commandOpenFolder:
				if result.path == "" {
					a.setStatus("已取消打开文件夹")
				} else if a.collection == nil {
					a.setStatus("工作区数据库不可用，无法记录文件夹内容")
				} else if len(result.recents) == 0 {
					a.setStatus("所选文件夹中没有支持的文档")
				} else {
					a.collection.StageRecents(result.recents)
					a.refreshHomeMetadata()
					a.persistRecentsAsync()
					a.setStatus(fmt.Sprintf("已添加 %d 个文档到最近打开", len(result.recents)))
				}
			case commandSaveAs:
				if result.path != "" {
					a.startSaveAs(result.path)
				} else {
					a.setStatus("已取消另存为")
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
					a.setStatus("已取消导出 PDF")
				} else if wordReq != nil {
					a.startWordPDFFileExport(wordReq.state, wordReq.key, result.path, wordReq.orientation)
				} else if sheetReq != nil {
					a.startSpreadsheetPDFFileExport(sheetReq.state, sheetReq.sheet, result.path, sheetReq.orientation)
				} else if presentationReq != nil {
					a.startPresentationPDFFileExport(presentationReq, result.path)
				} else {
					a.setStatus("PDF 导出状态已过期，请重新导出")
				}
			case commandShareCopy:
				wordReq := a.wordShareCopyRequest
				sheetReq := a.spreadsheetShareCopyRequest
				a.wordShareCopyRequest = nil
				a.spreadsheetShareCopyRequest = nil
				if result.path == "" {
					a.setStatus("已取消共享副本导出")
				} else if wordReq != nil {
					a.startWordShareCopyFile(wordReq, result.path)
				} else if sheetReq != nil {
					a.startSpreadsheetShareCopyFile(sheetReq, result.path)
				} else {
					a.setStatus("共享副本状态已过期，请重新导出")
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
				a.setStatus("读取文档信息失败：" + result.err.Error())
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
			a.publishWordRestoreNotice(result.state, "恢复历史版本失败："+result.err.Error())
		}
		if !result.state.acceptsResult(result.token) || result.state.wordRevisionAtomic.Load() != result.revision {
			return
		}
		activeResult := result.generation == a.documentGeneration && result.state == a.documentSessionState
		if result.err == nil && activeResult && result.applied {
			invalidateAllWordImagePreviews(result.state)
			reconcileWordSelection(result.state, true)
			a.setStatus("已恢复历史版本")
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
						a.setStatus("图片预览失败：" + result.err.Error())
					}
				} else if !state.wordImageCache.Put(result.id, result.image) {
					state.wordImageErrors[result.id] = "图片无法放入文档预览缓存"
					if state == a.documentSessionState {
						a.setStatus("图片预览失败：图片无法放入文档预览缓存")
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
			a.workspaceWriteError = "工作区保存失败：" + err.Error()
		} else {
			a.workspaceWriteError = ""
		}
	default:
	}
	select {
	case result := <-a.openResults:
		a.opening = false
		if result.err != nil {
			a.setStatus(result.err.Error())
		} else if err := a.installDocument(result.loaded); err != nil {
			a.setStatus(err.Error())
		} else if result.historyErr != nil && result.loaded.word != nil {
			a.setStatus("文档已打开，但历史版本暂不可用：" + result.historyErr.Error())
		}
	default:
	}
	select {
	case result := <-a.saveResults:
		a.saving = false
		if result.err != nil {
			a.setStatus("保存失败：" + result.err.Error())
			if result.tabID == a.pendingCloseTabID && result.tabID != "" {
				a.showCloseConfirmation(result.tabID, "保存失败", "保存没有完成。可以重试保存、不保存并关闭，或取消关闭。")
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
			historyWarning := ""
			if result.historyErr != nil {
				historyWarning = "文件已保存，但历史版本未记录：" + result.historyErr.Error()
			}
			if state := a.sessions[result.tabID]; state != nil && result.token.documentID != "" && state.wordRevisionAtomic.Load() != result.token.revision {
				a.setStatus("保存已完成，但保存后文档又有修改")
				if historyWarning != "" {
					a.setStatus(historyWarning + "；保存后文档又有修改")
				}
				a.updatePersistedTab(a.tabMeta[result.tabID])
				a.maybeAutoSaveWord(a.documentSessionState)
				if a.window != nil {
					a.window.Invalidate()
				}
				return
			}
			if historyWarning != "" {
				a.setStatus(historyWarning)
			} else {
				a.setStatus("已保存 " + result.path)
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
			if historyWarning != "" {
				a.setStatus(historyWarning)
			}
			a.maybeAutoSaveWord(a.documentSessionState)
		}
	default:
	}
}
