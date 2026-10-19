package desktopui

import (
	"glance/internal/i18n"
	"runtime"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
	"glance/internal/application"
	"glance/internal/document"
)

// handleOfficeShortcuts routes the common Office shortcuts through the same
// command methods used by the Ribbon. A nil Focus filter lets the focused
// document editor keep focus while the non-text command is handled here.
func (a *App) handleOfficeShortcuts(gtx layout.Context) {
	state := a.documentSessionState
	if state == nil || (state.word == nil && state.book == nil) || a.modal.Visible || a.fileCommandRunning || a.wordVersionHistoryVisible || a.wordSymbolPickerVisible {
		return
	}

	replaceModifier := key.ModShortcut
	if runtime.GOOS == "darwin" {
		// Cmd+H belongs to macOS' Hide Application command. Office for Mac
		// uses Control+H for Replace.
		replaceModifier = key.ModCtrl
	}
	filters := []event.Filter{
		key.Filter{Name: key.Name("O"), Required: key.ModShortcut},
		key.Filter{Name: key.Name("S"), Required: key.ModShortcut, Optional: key.ModShift},
		key.Filter{Name: key.Name("F"), Required: key.ModShortcut},
		key.Filter{Name: key.Name("P"), Required: key.ModShortcut},
		key.Filter{Name: key.Name("H"), Required: replaceModifier},
	}
	spreadsheetGridShortcuts := state.book != nil && !state.cellEdit.Editing &&
		!gtx.Focused(&a.cellEditor) && !gtx.Focused(&a.formulaEditor) && !gtx.Focused(&a.nameBox) &&
		!state.formulaReference.Active && state.selectedGridPictureID == "" &&
		state.toolbarPicker == pickerNone && state.officeRibbonMenu.kind == officeRibbonMenuClosed && !a.showNewMenu
	spreadsheetFormatCellsShortcut := spreadsheetGridShortcuts && a.toolbarActionEnabled(4)
	spreadsheetNumberFormatShortcuts := spreadsheetGridShortcuts && a.toolbarActionEnabled(10)
	spreadsheetHistoryShortcuts := spreadsheetGridShortcuts
	if spreadsheetHistoryShortcuts {
		filters = append(filters,
			key.Filter{Name: key.Name("Z"), Required: key.ModShortcut, Optional: key.ModShift},
			key.Filter{Name: key.Name("Y"), Required: key.ModShortcut},
		)
	}
	if spreadsheetGridShortcuts {
		filters = append(filters,
			key.Filter{Name: key.Name("G"), Required: key.ModCtrl},
			key.Filter{Name: key.NameF5},
		)
	}
	if spreadsheetNumberFormatShortcuts {
		for _, name := range []key.Name{"!", "@", "#", "$", "%", "^", "~"} {
			filters = append(filters, key.Filter{Name: name, Required: key.ModCtrl | key.ModShift})
		}
	}
	if spreadsheetFormatCellsShortcut {
		filters = append(filters, key.Filter{Name: key.Name("1"), Required: key.ModShortcut})
	}
	spreadsheetAutoSumShortcut := spreadsheetGridShortcuts && state.selectionSet && a.toolbarActionEnabled(toolbarActionSum)
	spreadsheetAutoSumName, spreadsheetAutoSumModifiers := spreadsheetAutoSumShortcutBinding(runtime.GOOS)
	if spreadsheetAutoSumShortcut {
		filters = append(filters, key.Filter{Name: spreadsheetAutoSumName, Required: spreadsheetAutoSumModifiers})
	}
	spreadsheetCopyShortcut := spreadsheetGridShortcuts && a.toolbarActionEnabled(2)
	spreadsheetCutShortcut := spreadsheetGridShortcuts && a.toolbarActionEnabled(3)
	spreadsheetPasteShortcut := spreadsheetGridShortcuts && a.toolbarActionEnabled(21)
	spreadsheetFormatShortcuts := spreadsheetGridShortcuts && a.toolbarActionEnabled(4)
	spreadsheetLinkShortcut := spreadsheetGridShortcuts && a.toolbarActionEnabled(toolbarActionInsertLink)
	wordHyperlinkShortcut := false
	if state.word != nil {
		_, wordHyperlinkShortcut = any(state.word).(document.WordHyperlinkEditor)
		if wordHyperlinkShortcut && wordPageTextSelectionActive(state.wordPageTextSelection) {
			_, wordHyperlinkShortcut = any(state.word).(document.WordTextSelectionHyperlinkEditor)
		}
	}
	wordCommentShortcut := false
	if state.word != nil {
		_, wordCommentShortcut = any(state.word).(document.WordCommentEditor)
	}
	wordCommentShortcutName, wordCommentShortcutModifiers := wordNewCommentShortcutBinding(runtime.GOOS)
	wordSelectionFormatShortcuts := state.word != nil && wordPageTextSelectionActive(state.wordPageTextSelection)
	if wordSelectionFormatShortcuts {
		_, wordSelectionFormatShortcuts = any(state.word).(document.WordTextSelectionFormatter)
	}
	wordParagraphFormattingShortcuts := state.word != nil && len(wordParagraphIDsForSelection(state)) > 0 &&
		!state.formatRunning.Load() && !a.isSaving() && state.wordParagraphSplitIntent == nil &&
		state.wordParagraphJoinIntent == nil && state.wordSelectionEditIntent == nil && state.wordPageBreakIntent == nil
	if wordParagraphFormattingShortcuts {
		runID := state.wordFocusedRunID
		if runID == "" {
			runID = state.selectedWordRunID
		}
		if editor := state.wordRunEditors[runID]; editor != nil && gtx.Focused(editor) && editorHasActiveComposition(editor) {
			wordParagraphFormattingShortcuts = false
		}
	}
	wordBreakRunID, _, wordBreakShortcuts := a.focusedWordBreakShortcutEditor(gtx, state)
	if wordBreakShortcuts {
		for _, name := range []key.Name{key.NameReturn, key.NameEnter} {
			filters = append(filters,
				key.Filter{Name: name, Required: key.ModShortcut},
				key.Filter{Name: name, Required: key.ModShortcut | key.ModShift},
			)
		}
	}
	wordSelectionCutShortcut := state.word != nil && wordPageTextSelectionActive(state.wordPageTextSelection)
	if wordSelectionCutShortcut {
		_, wordSelectionCutShortcut = any(state.word).(document.WordTextSelectionEditor)
	}
	if spreadsheetCopyShortcut {
		filters = append(filters, key.Filter{Name: key.Name("C"), Required: key.ModShortcut})
	}
	if spreadsheetCutShortcut {
		filters = append(filters, key.Filter{Name: key.Name("X"), Required: key.ModShortcut})
	}
	if wordSelectionCutShortcut {
		filters = append(filters, key.Filter{Name: key.Name("X"), Required: key.ModShortcut})
	}
	if spreadsheetPasteShortcut {
		filters = append(filters, key.Filter{Name: key.Name("V"), Required: key.ModShortcut})
	}
	if spreadsheetLinkShortcut || wordHyperlinkShortcut {
		filters = append(filters, key.Filter{Name: key.Name("K"), Required: key.ModShortcut})
	}
	if wordCommentShortcut {
		filters = append(filters, key.Filter{Name: wordCommentShortcutName, Required: wordCommentShortcutModifiers})
	}
	spreadsheetCellTextSelectionShortcuts := a.spreadsheetCellTextSelectionActive()
	if spreadsheetFormatShortcuts || spreadsheetCellTextSelectionShortcuts {
		for _, name := range []key.Name{"B", "I", "U"} {
			filters = append(filters, key.Filter{Name: name, Required: key.ModShortcut})
			if key.ModShortcut != key.ModCtrl {
				filters = append(filters, key.Filter{Name: name, Required: key.ModCtrl})
			}
		}
	}
	if wordSelectionFormatShortcuts {
		for _, name := range []key.Name{"B", "I", "U"} {
			filters = append(filters, key.Filter{Name: name, Required: key.ModShortcut})
			if key.ModShortcut != key.ModCtrl {
				filters = append(filters, key.Filter{Name: name, Required: key.ModCtrl})
			}
		}
	}
	if wordParagraphFormattingShortcuts {
		filters = append(filters,
			key.Filter{Name: key.Name("1"), Required: key.ModShortcut | key.ModAlt},
			key.Filter{Name: key.Name("2"), Required: key.ModShortcut | key.ModAlt},
			key.Filter{Name: key.Name("N"), Required: key.ModShortcut | key.ModShift},
			key.Filter{Name: key.Name("L"), Required: key.ModShortcut},
			key.Filter{Name: key.Name("E"), Required: key.ModShortcut},
			key.Filter{Name: key.Name("R"), Required: key.ModShortcut},
			key.Filter{Name: key.Name("J"), Required: key.ModShortcut},
		)
	}
	for event, ok := gtx.Event(filters...); ok; event, ok = gtx.Event(filters...) {
		pressed, ok := event.(key.Event)
		if !ok || pressed.State != key.Press {
			continue
		}
		switch pressed.Name {
		case key.NameReturn, key.NameEnter:
			if wordBreakShortcuts {
				a.updateFocusedWordEditorBeforeCommandWithSubmit(gtx, state, false)
				if state.wordParagraphSplitIntent == nil {
					breakType := document.WordBreakPage
					if pressed.Modifiers.Contain(key.ModShift) {
						breakType = document.WordBreakColumn
					}
					if wordBreakRunID != "" && state.selectedWordRunID == wordBreakRunID {
						a.queueWordBreak(breakType)
					}
				}
			}
		case key.Name("O"):
			a.startOpenFilePicker()
		case key.Name("G"), key.NameF5:
			if spreadsheetGridShortcuts {
				a.focusSpreadsheetNameBox(gtx)
			}
		case key.Name("S"):
			if state.word != nil {
				a.updateFocusedWordEditorBeforeCommand(gtx, state)
				if pressed.Modifiers.Contain(key.ModShift) {
					a.startOfficeWordSaveAs(state)
				} else {
					a.startSave()
				}
			} else if state.book != nil && !a.commitSpreadsheetEditorBeforeCommand(gtx, state) {
				return
			} else if state.book != nil && pressed.Modifiers.Contain(key.ModShift) {
				ext := "xlsx"
				if state.sourceFormat == application.FormatCSV {
					ext = "csv"
				}
				a.startSaveAsPicker(ext)
			} else {
				a.startSave()
			}
		case key.Name("F"):
			if state.word != nil {
				a.updateFocusedWordEditorBeforeCommand(gtx, state)
				a.showWordFindDialog()
			} else if state.book != nil {
				if !a.commitSpreadsheetEditorBeforeCommand(gtx, state) {
					return
				}
				a.showSpreadsheetFindDialog()
			}
		case key.Name("P"):
			if state.word != nil {
				a.updateFocusedWordEditorBeforeCommand(gtx, state)
				a.requestWordPrint()
			} else if state.book != nil {
				if !a.commitSpreadsheetEditorBeforeCommand(gtx, state) {
					return
				}
				a.requestSpreadsheetPrint()
			}
		case key.Name("H"):
			if state.word != nil {
				a.updateFocusedWordEditorBeforeCommand(gtx, state)
				a.showWordReplaceDialog()
			} else if state.book != nil {
				if !a.commitSpreadsheetEditorBeforeCommand(gtx, state) {
					return
				}
				a.showSpreadsheetReplaceDialog()
			}
		case key.Name("Z"):
			if spreadsheetHistoryShortcuts {
				a.applyHistory(!pressed.Modifiers.Contain(key.ModShift))
			}
		case key.Name("Y"):
			if spreadsheetHistoryShortcuts {
				a.applyHistory(false)
			}
		case key.Name("C"):
			if spreadsheetCopyShortcut {
				a.writeSpreadsheetSelectionToClipboard(gtx, false)
			}
		case key.Name("X"):
			if spreadsheetCutShortcut {
				a.writeSpreadsheetSelectionToClipboard(gtx, true)
			} else if wordSelectionCutShortcut {
				a.updateFocusedWordEditorBeforeCommand(gtx, state)
				if state.wordParagraphSplitIntent == nil {
					a.cutSelectedWordText(gtx, state)
				}
			}
		case key.Name("V"):
			if spreadsheetPasteShortcut {
				a.requestSpreadsheetClipboardPaste(gtx, state)
			}
		case key.Name("K"):
			if wordHyperlinkShortcut {
				a.updateFocusedWordEditorBeforeCommand(gtx, state)
				if a.isSaving() {
					a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.the_document_is_saving_try_inserting_the_h_e0b2b9"}, feedbackInfo, notificationToast)
					continue
				}
				if state.formatRunning.Load() && !state.wordFlushRequested.Load() {
					a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.the_document_is_busy_try_inserting_the_hyp_fa1aa3"}, feedbackInfo, notificationToast)
					continue
				}
				if state.wordDeferredToolbarAction != "" && state.wordDeferredToolbarAction != "insert-link" {
					a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.finish_the_current_text_edit_before_insert_771284"}, feedbackWarning, notificationSuppressed)
					continue
				}
				if state.wordFlushRequested.Load() || wordHasPendingTextDrafts(state) {
					state.wordForceDraftFlush.Store(true)
					state.wordFlushRequested.Store(true)
					state.wordFlushAllDrafts.Store(true)
					state.wordDeferredToolbarAction = "insert-link"
					if !state.formatRunning.Load() {
						a.startNextWordTextCommitFor(state)
					}
					continue
				}
				a.showWordHyperlinkDialog()
			} else if spreadsheetLinkShortcut {
				a.showSpreadsheetLinkDialog()
			}
		case key.Name("A"), key.Name("M"):
			if wordCommentShortcut && pressed.Name == wordCommentShortcutName {
				a.updateFocusedWordEditorBeforeCommand(gtx, state)
				if a.isSaving() {
					a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.the_document_is_saving_try_adding_the_comm_1284d5"}, feedbackInfo, notificationToast)
					continue
				}
				if state.formatRunning.Load() && !state.wordFlushRequested.Load() {
					a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.the_document_is_busy_try_adding_the_commen_a766fc"}, feedbackInfo, notificationToast)
					continue
				}
				if state.wordDeferredToolbarAction != "" && state.wordDeferredToolbarAction != "review-new-comment" {
					a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.finish_the_current_text_edit_before_adding_f603d2"}, feedbackWarning, notificationSuppressed)
					continue
				}
				if state.wordFlushRequested.Load() || wordHasPendingTextDrafts(state) {
					state.wordForceDraftFlush.Store(true)
					state.wordFlushRequested.Store(true)
					state.wordFlushAllDrafts.Store(true)
					state.wordDeferredToolbarAction = "review-new-comment"
					if !state.formatRunning.Load() {
						a.startNextWordTextCommitFor(state)
					}
					continue
				}
				a.showWordNewCommentDialog()
			}
		case key.Name("B"):
			if a.spreadsheetCellTextSelectionActive() {
				a.applySpreadsheetCellTextFormat(4)
			} else if spreadsheetFormatShortcuts {
				a.applySpreadsheetFormatAction(4)
			} else if wordSelectionFormatShortcuts {
				a.updateFocusedWordEditorBeforeCommand(gtx, state)
				if state.wordParagraphSplitIntent == nil {
					a.toggleWordTextStyle("bold")
				}
			}
		case key.Name("I"):
			if a.spreadsheetCellTextSelectionActive() {
				a.applySpreadsheetCellTextFormat(5)
			} else if spreadsheetFormatShortcuts {
				a.applySpreadsheetFormatAction(5)
			} else if wordSelectionFormatShortcuts {
				a.updateFocusedWordEditorBeforeCommand(gtx, state)
				if state.wordParagraphSplitIntent == nil {
					a.toggleWordTextStyle("italic")
				}
			}
		case key.Name("U"):
			if a.spreadsheetCellTextSelectionActive() {
				a.applySpreadsheetCellTextFormat(6)
			} else if spreadsheetFormatShortcuts {
				a.applySpreadsheetFormatAction(6)
			} else if wordSelectionFormatShortcuts {
				a.updateFocusedWordEditorBeforeCommand(gtx, state)
				if state.wordParagraphSplitIntent == nil {
					a.toggleWordTextStyle("underline")
				}
			}
		case key.Name("1"):
			if spreadsheetFormatCellsShortcut {
				a.showSpreadsheetFormatCellsDialog()
			} else if wordParagraphFormattingShortcuts {
				a.updateFocusedWordEditorBeforeCommand(gtx, state)
				if state.wordParagraphSplitIntent == nil {
					a.applyWordParagraphStyleAction("style-heading")
				}
			}
		case key.Name("2"):
			if wordParagraphFormattingShortcuts {
				a.updateFocusedWordEditorBeforeCommand(gtx, state)
				if state.wordParagraphSplitIntent == nil {
					a.applyWordParagraphStyleAction("style-heading-2")
				}
			}
		case key.Name("N"):
			if wordParagraphFormattingShortcuts {
				a.updateFocusedWordEditorBeforeCommand(gtx, state)
				if state.wordParagraphSplitIntent == nil {
					a.applyWordParagraphStyleAction("style-body")
				}
			}
		case key.Name("L"), key.Name("E"), key.Name("R"), key.Name("J"):
			if wordParagraphFormattingShortcuts {
				a.updateFocusedWordEditorBeforeCommand(gtx, state)
				if state.wordParagraphSplitIntent == nil {
					a.setWordAlignment(map[key.Name]string{
						key.Name("L"): "align-left",
						key.Name("E"): "align-center",
						key.Name("R"): "align-right",
						key.Name("J"): "align-justify",
					}[pressed.Name])
				}
			}
		case key.Name("!"), key.Name("@"), key.Name("#"), key.Name("$"), key.Name("%"), key.Name("^"), key.Name("~"):
			if spreadsheetNumberFormatShortcuts {
				if format, ok := spreadsheetNumberFormatForShortcut(pressed.Name); ok {
					a.applyCellFormat(document.CellFormatPatch{NumberFormat: &format})
				}
			}
		case key.Name("="), key.Name("T"):
			if spreadsheetAutoSumShortcut && pressed.Name == spreadsheetAutoSumName {
				a.applySpreadsheetAutoSum()
			}
		}
		// Opening a dialog changes the active key target. Handle at most one
		// shortcut per frame so queued events cannot leak into the new surface.
		if a.modal.Visible {
			return
		}
	}
}

func wordNewCommentShortcutBinding(goos string) (key.Name, key.Modifiers) {
	if goos == "darwin" {
		return key.Name("A"), key.ModShortcut | key.ModAlt
	}
	return key.Name("M"), key.ModCtrl | key.ModAlt
}

func spreadsheetNumberFormatForShortcut(name key.Name) (string, bool) {
	switch name {
	case "!":
		return "#,##0.00", true
	case "@":
		return "h:mm AM/PM", true
	case "#":
		return "m/d/yy", true
	case "$":
		return spreadsheetCurrencyNumberFormat, true
	case "%":
		return "0%", true
	case "^":
		return "0.00E+00", true
	case "~":
		return "General", true
	default:
		return "", false
	}
}

func spreadsheetAutoSumShortcutBinding(goos string) (key.Name, key.Modifiers) {
	if goos == "darwin" {
		return key.Name("T"), key.ModShortcut | key.ModShift
	}
	return key.Name("="), key.ModAlt
}

func (a *App) startOfficeWordSaveAs(state *documentSessionState) {
	if state == nil || state.word == nil || a.isSaving() {
		return
	}
	if state.formatRunning.Load() {
		if state.wordFlushRequested.Load() && state.wordDeferredToolbarAction == "" {
			state.wordDeferredToolbarAction = "save-as"
		}
		return
	}
	if !wordHasPendingTextDrafts(state) && state.wordParagraphSplitIntent == nil && state.wordParagraphJoinIntent == nil && state.wordPageBreakIntent == nil {
		a.executeWordToolbarAction("save-as")
		return
	}
	if state.wordDeferredToolbarAction != "" {
		return
	}
	state.wordDeferredToolbarAction = "save-as"
	state.wordForceDraftFlush.Store(true)
	state.wordFlushRequested.Store(true)
	state.wordFlushAllDrafts.Store(true)
	a.startNextWordTextCommitFor(state)
}

func (a *App) commitSpreadsheetEditorBeforeCommand(gtx layout.Context, state *documentSessionState) bool {
	if state == nil || state.book == nil {
		return true
	}
	var editor *widget.Editor
	switch {
	case gtx.Focused(&a.formulaEditor):
		editor = &a.formulaEditor
	case state.cellEdit.Editing:
		editor = &a.cellEditor
	default:
		return true
	}
	for event, ok := editor.Update(gtx); ok; event, ok = editor.Update(gtx) {
		if _, changed := event.(widget.ChangeEvent); changed {
			state.cellEdit.setDraft(editor.Text())
		}
	}
	value := editor.Text()
	if value == state.cellEdit.Original {
		state.cellEdit.setEditing(false)
		return true
	}
	if err := a.commitGridCell(value); err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "common.feedback.operation_failed"}, err, notificationToast)
		if a.window != nil {
			a.window.Invalidate()
		}
		return false
	}
	return true
}

func (a *App) updateFocusedWordEditorBeforeCommand(gtx layout.Context, state *documentSessionState) {
	a.updateFocusedWordEditorBeforeCommandWithSubmit(gtx, state, true)
}

func (a *App) updateFocusedWordEditorBeforeCommandWithSubmit(gtx layout.Context, state *documentSessionState, handleSubmit bool) {
	if state == nil || state.word == nil {
		return
	}
	runID := state.wordFocusedRunID
	if runID == "" {
		runID = state.selectedWordRunID
	}
	editor := state.wordRunEditors[runID]
	if editor == nil || !gtx.Focused(editor) {
		return
	}
	for event, ok := editor.Update(gtx); ok; event, ok = editor.Update(gtx) {
		if _, submit := event.(widget.SubmitEvent); submit && handleSubmit {
			start, end := editor.Selection()
			a.queueWordParagraphSplit(runID, editor.Text(), start, end)
			break
		}
	}
	captureWordEditorDraft(state, runID, editor.Text())
}

func (a *App) focusedWordBreakShortcutEditor(gtx layout.Context, state *documentSessionState) (string, *widget.Editor, bool) {
	if state == nil || state.word == nil || a.isSaving() || state.formatRunning.Load() ||
		state.wordFlushRequested.Load() || state.wordParagraphSplitIntent != nil ||
		state.wordParagraphJoinIntent != nil || state.wordSelectionEditIntent != nil ||
		state.wordPageBreakIntent != nil || wordPageTextSelectionActive(state.wordPageTextSelection) {
		return "", nil, false
	}
	if _, ok := any(state.word).(document.WordPageBreakEditor); !ok {
		return "", nil, false
	}
	runID := state.wordFocusedRunID
	if runID == "" || runID != state.selectedWordRunID {
		return "", nil, false
	}
	editor := state.wordRunEditors[runID]
	if editor == nil || editor.ReadOnly || !gtx.Focused(editor) || editorHasActiveComposition(editor) {
		return "", nil, false
	}
	run, ok := selectedWordRun(wordBlocksForState(state), runID)
	if !ok || !run.Editable || !wordBodyRunEditable(wordBlocksForState(state), runID) {
		return "", nil, false
	}
	return runID, editor, true
}
