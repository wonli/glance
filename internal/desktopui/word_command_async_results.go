package desktopui

import "glance/internal/document"

func (a *App) processWordCommandAsyncResult() bool {
	select {
	case result := <-a.wordCommandResults:
		if result.state == nil {
			return true
		}
		if result.operationToken.generation != 0 && (!result.state.acceptsResult(result.operationToken) || result.state.wordRevisionAtomic.Load() != result.revision) {
			return true
		}
		if result.state != nil {
			if result.operation == "调整段落缩进" {
				result.state.wordRulerIndentIntent = nil
			}
			if result.bodyParagraphInsert {
				result.state.wordInsertionRunning = false
			}
		}
		activeResult := result.generation == a.documentGeneration && result.state == a.documentSessionState
		if result.wordReplaceAll != nil {
			a.completeWordTextReplaceAll(result, activeResult)
			return false
		}
		if result.wordCommentList {
			if activeResult {
				if result.err != nil {
					a.setStatus(wordCommandError(result.operation, result.err))
				} else if len(result.wordComments) == 0 {
					a.setStatus("当前文档没有可删除的批注")
				} else {
					a.showWordDeleteCommentPrompt(result.wordComments, result.wordCommentTotal, result.revision)
				}
				if a.window != nil {
					a.window.Invalidate()
				}
			}
			return false
		}
		if activeResult && result.applicationCommand && !result.viewOnly && result.err == nil && result.blocks != nil {
			result.state.setWordBlocks(result.blocks)
			reconcileWordSelection(result.state, false)
		}
		if result.selectionEdit {
			a.completeWordSelectionEdit(result, activeResult)
			if activeResult && result.err == nil && !result.state.formatRunning.Load() && !result.state.wordFlushRequested.Load() {
				a.maybeAutoSaveWord(result.state)
			}
			if a.window != nil {
				a.window.Invalidate()
			}
		} else if result.paragraphSplit {
			// A split owns view-local draft and focus state even after a tab switch.
			a.completeWordParagraphSplit(result, activeResult)
			if activeResult && result.err == nil && !result.state.formatRunning.Load() && !result.state.wordFlushRequested.Load() {
				a.maybeAutoSaveWord(result.state)
			}
			if a.window != nil {
				a.window.Invalidate()
			}
		} else if result.pageBreak {
			a.completeWordPageBreak(result, activeResult)
		} else if result.paragraphJoin {
			a.completeWordParagraphJoin(result, activeResult)
		} else if result.textDraft && result.state.wordSelectionEditIntent != nil && !activeResult {
			a.completeInactiveWordSelectionEditDraft(result)
		} else if result.textDraft && result.state.pendingWordRestoreAfterDrafts() != 0 {
			if result.err != nil {
				message := "恢复历史版本已取消，" + wordCommandError(result.operation, result.err)
				a.publishWordRestoreNotice(result.state, message)
				result.state.wordFlushRequested.Store(false)
				result.state.wordFlushAllDrafts.Store(false)
				result.state.wordForceDraftFlush.Store(false)
				result.state.clearPendingWordRestore()
			} else {
				result.state.setCommittedWordTextAndClearDraft(result.runID, result.runText)
				reconcileWordSelection(result.state, false)
				if activeResult {
					a.setStatus("正在编辑 Word 正文")
					a.persistActiveSession()
				}
				if result.state.wordFlushRequested.Load() {
					a.startNextWordTextCommitFor(result.state)
				}
			}
		} else if activeResult && result.viewOnly {
			if result.err != nil {
				a.setStatus(wordCommandError(result.operation, result.err))
			} else if result.operation == "设置 Word 自动保存" {
				a.maybeAutoSaveWord(result.state)
			}
		} else if activeResult {
			if result.textDraft {
				if result.err != nil {
					a.setStatus(wordCommandError(result.operation, result.err))
					result.state.wordFlushRequested.Store(false)
					result.state.wordFlushAllDrafts.Store(false)
					result.state.wordForceDraftFlush.Store(false)
					result.state.clearPendingWordRestore()
					if intent := result.state.wordSelectionEditIntent; intent != nil && intent.replaces {
						restoreWordSelectionReplacementDraft(result.state, *intent)
					}
					result.state.wordSelectionEditIntent = nil
					result.state.wordParagraphSplitIntent = nil
					result.state.wordParagraphJoinIntent = nil
					result.state.wordPageBreakIntent = nil
					result.state.wordPDFExportAfterDrafts = false
					result.state.wordPrintAfterDrafts = false
					if result.state.wordDeferredToolbarAction == "ruler-indents" {
						result.state.wordDeferredToolbarAction = ""
						result.state.wordRulerIndentIntent = nil
					} else if result.state.wordDeferredToolbarAction == "export-pdf" || result.state.wordDeferredToolbarAction == "share" || result.state.wordDeferredToolbarAction == "save-as" {
						result.state.wordDeferredToolbarAction = ""
					} else if result.state.wordDeferredToolbarAction == "insert-symbol" || result.state.wordDeferredToolbarAction == "layout-before" || result.state.wordDeferredToolbarAction == "layout-after" {
						result.state.wordDeferredToolbarAction = ""
					} else if result.state.wordDeferredToolbarAction == "apply-format-painter" {
						result.state.wordDeferredToolbarAction = ""
						result.state.wordFormatPainterSourceRunID = ""
						clearWordFormatPainterTarget(result.state)
					} else if result.state.wordDeferredToolbarAction == "align-left" || result.state.wordDeferredToolbarAction == "align-center" || result.state.wordDeferredToolbarAction == "align-right" || result.state.wordDeferredToolbarAction == "align-justify" {
						result.state.wordDeferredToolbarAction = ""
					}
				} else {
					result.state.setCommittedWordTextAndClearDraft(result.runID, result.runText)
					if result.blocks != nil && result.revision == 0 {
						result.state.setWordBlocks(result.blocks)
						a.ensureDesktopRuntime().publishSessionRevision(result.state.documentID, result.state.wordRevisionAtomic.Load())
					}
					reconcileWordSelection(result.state, false)
					a.setStatus("正在编辑 Word 正文")
					a.persistActiveSession()
				}
				if result.err == nil && result.state.wordFlushRequested.Load() {
					a.startNextWordTextCommit()
				}
			} else if result.wordRangeReplacement {
				a.completeWordRangeReplacement(result)
			} else if result.wordSymbolInsertion {
				a.completeWordSymbolInsertion(result)
			} else if result.confirmationPending {
				a.setStatus("操作已送交人工确认；请在 AI 助手中检查目标并批准或拒绝。")
			} else if result.err != nil {
				a.setStatus(wordCommandError(result.operation, result.err))
			} else if !result.history || result.applied {
				if result.wordTrackRevisions != nil {
					result.state.wordTrackRevisions = *result.wordTrackRevisions
				}
				if result.blocks != nil && result.revision == 0 {
					result.state.setWordBlocks(result.blocks)
					a.ensureDesktopRuntime().publishSessionRevision(result.state.documentID, result.state.wordRevisionAtomic.Load())
				}
				if result.history {
					invalidateAllWordImagePreviews(result.state)
				} else if result.imageID != "" && (result.operation == "替换图片" || result.operation == "删除图片") {
					invalidateWordImagePreview(result.state, result.imageID)
				}
				reconcileWordSelection(result.state, result.history)
				if result.operation == "开启限制编辑" {
					result.state.wordProtected = true
				} else if result.operation == "解除限制编辑" {
					result.state.wordProtected = false
				}
				if result.history {
					if result.undo {
						a.setStatus("已撤销 Word 编辑")
					} else {
						a.setStatus("已重做 Word 编辑")
					}
					if ed, ok := any(result.state.word).(document.WordDocumentProtectionEditor); ok {
						result.state.wordProtected, _ = ed.GetWordDocumentProtection()
					}
				} else {
					a.setStatus(wordCommandError(result.operation, nil))
				}
				if result.clearImageSelection && result.state.selectedWordImageID == result.imageID {
					result.state.selectedWordImageID = ""
					result.state.wordImageFocusRequestID = ""
				}
				if result.selectInsertedImage && result.imageID != "" {
					result.state.selectedWordRunID = ""
					result.state.selectedWordImageID = result.imageID
					result.state.wordImageFocusRequestID = result.imageID
					result.state.selectedBlock = -1
					result.state.selectedBlockEditable = false
					result.state.wordToolbarTab = wordTabPicture
					for _, paragraph := range allWordParagraphs(wordBlocksForState(result.state)) {
						for _, inline := range paragraph.Inlines {
							if inline.Image != nil && inline.Image.ID == result.imageID {
								result.state.selectedWordParagraphID = paragraph.ID
							}
						}
					}
				}
				a.persistActiveSession()
				if result.insertedParagraphID != "" {
					a.focusInsertedWordParagraph(result.state, result.insertedParagraphID)
				}
			}
			if result.imageID != "" && (result.operation == "调整图片" || result.operation == "旋转图片") {
				finishWordImagePreviewCommand(result.state, result.imageID)
			}
			if result.state.wordFlushRequested.Load() && !result.state.formatRunning.Load() {
				a.startNextWordTextCommit()
			}
			if activeResult && !result.viewOnly && result.err == nil && !result.confirmationPending && !result.state.formatRunning.Load() && !result.state.wordFlushRequested.Load() {
				a.maybeAutoSaveWord(result.state)
			}
			if a.window != nil {
				a.window.Invalidate()
			}
		}
	default:
	}
	return false
}

// completeQueuedWordSplitsOnClose runs after managed workers stop and before
// persistence drains: no more frames will acknowledge buffered split results.
func (a *App) completeQueuedWordSplitsOnClose() {
	for remaining := len(a.wordCommandResults); remaining > 0; remaining-- {
		result := <-a.wordCommandResults
		if result.selectionEdit {
			a.completeWordSelectionEdit(result, false)
		} else if result.paragraphSplit {
			a.completeWordParagraphSplit(result, false)
		} else {
			a.wordCommandResults <- result
		}
	}
}
