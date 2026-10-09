package desktopui

import (
	"glance/internal/document"
	"io"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/layout"
)

func (a *App) handleWordToolbarEvents(gtx layout.Context) {
	state := a.documentSessionState
	if state == nil || state.word == nil || a.modal.Visible || a.wordVersionHistoryVisible || a.wordSymbolPickerVisible {
		return
	}
	tabs := wordToolbarTabs(state)
	for index, tab := range tabs {
		if state.wordToolbarTabs[index].Clicked(gtx) {
			state.wordToolbarTab = tab
			state.wordToolbarActionList.List.Position = layout.Position{}
			state.wordToolbarColorPicker = false
			state.wordToolbarHighlightPicker = false
			break
		}
	}
	for _, action := range wordToolbarActions(state.wordToolbarTab) {
		button := state.wordToolbarActionButtons[action.id]
		if button == nil || !button.Clicked(gtx) || !a.wordToolbarActionEnabled(action) {
			continue
		}
		if action.id == "view-new-window" {
			a.requestWordNewWindow()
			continue
		}
		if wordHasPendingTextDrafts(state) && action.id == "paste" {
			state.wordClipboardRunID = state.selectedWordRunID
			state.wordForceDraftFlush.Store(true)
			state.wordFlushRequested.Store(true)
			state.wordFlushAllDrafts.Store(true)
			gtx.Execute(clipboard.ReadCmd{Tag: &state.wordClipboardTarget})
			continue
		}
		if wordHasPendingTextDrafts(state) && action.id == "cut" {
			if editor := state.wordRunEditors[state.selectedWordRunID]; editor != nil {
				value := editor.Text()
				if selected := editor.SelectedText(); selected != "" {
					value = selected
				}
				gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(value))})
			}
			state.wordForceDraftFlush.Store(true)
			state.wordFlushRequested.Store(true)
			state.wordFlushAllDrafts.Store(true)
			state.wordDeferredToolbarAction = action.id
			continue
		}
		if wordHasPendingTextDrafts(state) && action.id != "copy" {
			state.wordForceDraftFlush.Store(true)
			state.wordFlushRequested.Store(true)
			state.wordFlushAllDrafts.Store(true)
			if action.id == "save" {
				state.wordSaveAfterDrafts = true
				state.wordSaveAfterDraftsReason = wordSaveManual
			} else {
				state.wordDeferredToolbarAction = action.id
			}
			continue
		}
		if state.wordToolbarTab == wordTabFile && action.id != "save" {
			a.executeWordToolbarAction(action.id)
			continue
		}
		switch action.id {
		case "format-painter":
			a.toggleWordFormatPainter()
		case "undo":
			a.applyWordHistory(true)
		case "redo":
			a.applyWordHistory(false)
		case "cut", "copy":
			if action.id == "copy" {
				if selected, ok := wordPageSelectedText(state); ok {
					gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(selected))})
					continue
				}
			}
			if run, ok := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID); ok {
				value := run.Text
				if editor := state.wordRunEditors[run.ID]; editor != nil {
					value = editor.Text()
					if selected := editor.SelectedText(); selected != "" {
						value = selected
					}
				}
				gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(value))})
				if action.id == "cut" {
					runID := state.selectedWordRunID
					a.runWordCommand("剪切文字", func(editor document.WordTextEditor) error { return editor.ReplaceTextRun(runID, "") })
				}
			}
		case "paste":
			state.wordClipboardRunID = state.selectedWordRunID
			gtx.Execute(clipboard.ReadCmd{Tag: &state.wordClipboardTarget})
		case "bold", "italic", "underline":
			a.toggleWordTextStyle(action.id)
		case "font-size-decrease":
			a.adjustWordFontSize(-wordFontSizeStepPt)
		case "font-size-increase":
			a.adjustWordFontSize(wordFontSizeStepPt)
		case "font-family":
			a.showWordFontFamilyDialog()
		case "font-color":
			state.wordToolbarHighlightPicker = false
			state.wordToolbarColorPicker = !state.wordToolbarColorPicker
		case "text-highlight":
			state.wordToolbarColorPicker = false
			state.wordToolbarHighlightPicker = !state.wordToolbarHighlightPicker
		case "align-left", "align-center", "align-right", "align-justify":
			a.setWordAlignment(action.id)
		case "bullet-list", "numbered-list":
			a.setWordList(action.id)
		case "decrease-indent", "increase-indent":
			a.adjustWordParagraphIndent(action.id)
		case "layout-before", "layout-after":
			a.showWordParagraphSpacingDialog(action.id)
		case "layout-orientation":
			a.showWordOrientationDialog()
		case "layout-margins":
			a.showWordMarginsDialog()
		case "layout-size":
			a.showWordPageSizeDialog()
		case "layout-columns":
			a.showWordColumnsDialog()
		case "layout-breaks":
			a.showWordBreaksDialog()
		case "layout-line-numbers":
			a.showWordLineNumbersDialog()
		case "find":
			a.showWordFindDialog()
		case "replace":
			a.showWordReplaceDialog()
		case "insert-symbol":
			a.openWordSymbolPicker()
		case "select":
			a.selectAllWordText()
		case "design-page-color":
			a.showWordPageColorDialog()
		case "design-page-border":
			a.showWordPageBorderDialog()
		case "insert-table":
			a.showWordInsertTableDialog()
		case "cover":
			a.showWordCoverPageDialog()
		case "insert-bookmark":
			a.showWordBookmarkDialog()
		case "insert-link":
			a.showWordHyperlinkDialog()
		case "insert-textbox":
			a.showWordTextBoxDialog()
		case "insert-header":
			a.showWordHeaderDialog()
		case "insert-footer":
			a.showWordFooterDialog()
		case "insert-page-number":
			a.showWordPageNumberDialog()
		case "view-side-by-side":
			a.toggleWordSideBySideView()
		case "review-language":
			a.showWordLanguageDialog()
		case "review-word-count":
			a.showWordCountDetails()
		case "review-restrict":
			a.showWordDocumentProtectionDialog()
		case "design-watermark":
			a.showWordWatermarkDialog()
		case "review-track":
			a.toggleWordTrackRevisions()
		case "review-accept":
			a.acceptWordRevisions()
		case "review-reject":
			a.rejectWordRevisions()
		case "review-editor", "editor":
			a.showWordEditorDialog()
		case "review-translate":
			a.showWordTranslateDialog()
		case "review-compare":
			a.showWordCompareDialog()
		case "layout-position":
			a.showWordLayoutPositionDialog()
		case "layout-wrap":
			a.showWordLayoutWrapDialog()
		case "layout-align":
			a.showWordLayoutAlignDialog()
		case "insert-wordart":
			a.showWordWordArtDialog()
		case "insert-equation":
			a.showWordEquationDialog()
		case "insert-shapes":
			a.showWordShapesDialog()
		case "insert-chart":
			a.showWordChartDialog()
		case "review-new-comment":
			a.showWordNewCommentDialog()
		case "review-delete-comment":
			a.showWordDeleteCommentDialog()
		case "design-themes":
			a.showWordThemesDialog()
		case "design-colors":
			a.showWordColorsDialog()
		case "design-fonts":
			a.showWordFontsDialog()
		case "design-effects":
			a.showWordEffectsDialog()
		case "designer":
			a.showWordDesignerDialog()
		case "dictate":
			a.showWordDictateDialog()
		case "add-ins":
			a.showWordAddInsDialog()
		case "view-macros":
			a.showWordMacrosDialog()
		case "view-print-layout", "view-web-layout", "view-navigation", "view-single-page", "view-multiple-pages", "view-reading", "view-zoom", "view-ruler", "view-gridlines":
			a.executeWordToolbarAction(action.id)
		case "save":
			a.startSave()
		case "insert-image":
			a.startWordImagePicker(wordImageInsert)
		case "replace-image":
			a.startWordImagePicker(wordImageReplace)
		case "delete-image":
			imageID := state.selectedWordImageID
			a.runWordImageCommand("删除图片", imageID, func(editor document.WordImageEditor) error { return editor.DeleteInlineImage(imageID) }, true)
		case "resize-image":
			a.showWordImageResizeDialog()
		case "rotate-left":
			a.rotateSelectedWordImage(-90)
		case "rotate-right":
			a.rotateSelectedWordImage(90)
		}
	}
	if state.wordToolbarColorPicker || state.wordToolbarHighlightPicker {
		for index := range state.wordToolbarColorButtons {
			if state.wordToolbarColorButtons[index].Clicked(gtx) {
				if state.wordToolbarHighlightPicker {
					a.chooseWordHighlightColor(index)
				} else {
					colorValue := wordFontColors[index].value
					runID := state.selectedWordRunID
					state.wordToolbarColorPicker = false
					a.setWordFontColor(runID, colorValue)
				}
				break
			}
		}
	}
}
