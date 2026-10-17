package desktopui

import (
	"glance/internal/document"
	"glance/internal/format/docx"
)

type wordParagraphSpacingAllSetArguments struct {
	BeforePoints int `json:"before_points"`
	AfterPoints  int `json:"after_points"`
}

func (a *App) executeWordToolbarAction(actionID string) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if a.runWordViewRibbonAction(actionID) {
		return
	}
	switch actionID {
	case "style-body", "style-heading", "style-heading-2":
		a.applyWordParagraphStyleAction(actionID)
	case "new":
		word, err := docx.NewBlank()
		if err != nil {
			a.setStatus("新建 Word 文档失败：" + err.Error())
			return
		}
		if err := a.installDocument(loadedDocument{word: word}); err != nil {
			a.setStatus("新建 Word 文档失败：" + err.Error())
		}
	case "open":
		a.startOpenFilePicker()
	case "recent":
		a.activateRecent()
	case "save-as":
		a.startSaveAsPicker("docx")
	case "export-pdf":
		a.requestWordPDFExport()
	case "share":
		a.requestWordShareCopy()
	case "print":
		a.requestWordPrint()
	case "doc-info":
		a.startWordDocumentInfoLoad()
	case "insert-symbol":
		a.openWordSymbolPicker()
	case "version-history":
		a.openWordVersionHistory()
	case "view-new-window":
		a.requestWordNewWindow()
	case "autosave":
		if !a.runWordViewRibbonAction("autosave") {
			a.setStatus("自动保存需要已连接的 Word 运行时")
		}
	case "format-painter":
		a.toggleWordFormatPainter()
	case "view-print-layout":
		a.toggleWordPrintLayout()
	case "view-web-layout":
		a.toggleWordWebLayout()
	case "view-navigation":
		a.toggleWordNavigationPane()
	case "view-zoom":
		a.showWordZoomDialog()
	case "view-single-page":
		a.selectWordSinglePageView()
	case "view-multiple-pages":
		a.selectWordMultiplePagesView()
	case "view-reading":
		a.toggleWordReadingMode()
	case "view-ruler":
		state.wordRulerHidden = !state.wordRulerHidden
		if state.wordRulerHidden {
			a.setStatus("已隐藏标尺")
		} else {
			a.setStatus("已显示标尺")
		}
	case "view-gridlines":
		state.wordGridlinesVisible = !state.wordGridlinesVisible
		if state.wordGridlinesVisible {
			a.setStatus("已显示网格线")
		} else {
			a.setStatus("已隐藏网格线")
		}
	case "apply-format-painter":
		a.startWordFormatPainter()
	case "undo":
		a.applyWordHistory(true)
	case "redo":
		a.applyWordHistory(false)
	case "cut":
		if wordPageTextSelectionActive(state.wordPageTextSelection) {
			if start, end, ok := wordPageOrderedSelection(state.wordPageTextSelection); ok {
				a.queueWordSelectionDeletion(state, start, end)
			}
			return
		}
		runID := state.selectedWordRunID
		a.runWordCommand("剪切文字", func(editor document.WordTextEditor) error { return editor.ReplaceTextRun(runID, "") })
	case "paste":
		return
	case "bold", "italic", "underline":
		a.toggleWordTextStyle(actionID)
	case "font-size-decrease":
		a.adjustWordFontSize(-wordFontSizeStepPt)
	case "font-size-increase":
		a.adjustWordFontSize(wordFontSizeStepPt)
	case "font-size":
		a.toggleWordOfficeRibbonMenu("font-size")
	case "font-family":
		a.showWordFontFamilyDialog()
	case "font-color":
		state.wordToolbarHighlightPicker = false
		state.wordToolbarColorPicker = !state.wordToolbarColorPicker
	case "text-highlight":
		state.wordToolbarColorPicker = false
		state.wordToolbarHighlightPicker = !state.wordToolbarHighlightPicker
	case "align-left", "align-center", "align-right", "align-justify":
		a.setWordAlignment(actionID)
	case "bullet-list", "numbered-list":
		a.setWordList(actionID)
	case "decrease-indent", "increase-indent":
		a.adjustWordParagraphIndent(actionID)
	case "layout-before", "layout-after":
		a.showWordParagraphSpacingDialog(actionID)
	case "layout-indent":
		a.showWordParagraphIndentDialog()
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
	case "page-break":
		a.queueWordPageBreak()
	case "blank-page":
		a.queueWordBlankPage()
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
	case "design-compact":
		if !a.runWordApplicationCommand("紧凑段落间距", "word.paragraph.spacing_all.set", wordParagraphSpacingAllSetArguments{BeforePoints: 0, AfterPoints: 4}) {
			a.runWordSpacingCommand("紧凑段落间距", func(editor document.WordParagraphSpacingEditor) error {
				return editor.SetAllParagraphsSpacing(0, 80)
			})
		}
	case "design-spacious":
		if !a.runWordApplicationCommand("宽松段落间距", "word.paragraph.spacing_all.set", wordParagraphSpacingAllSetArguments{BeforePoints: 0, AfterPoints: 12}) {
			a.runWordSpacingCommand("宽松段落间距", func(editor document.WordParagraphSpacingEditor) error {
				return editor.SetAllParagraphsSpacing(0, 240)
			})
		}
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
	case "ruler-indents":
		intent := state.wordRulerIndentIntent
		if intent == nil {
			return
		}
		paragraphID, leftEMU, rightEMU := intent.paragraphID, intent.leftEMU, intent.rightEMU
		a.runWordCommand("调整段落缩进", func(editor document.WordTextEditor) error {
			return editor.SetParagraphIndents(paragraphID, leftEMU, rightEMU)
		})
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

func (a *App) maybeAutoSaveWord(state *documentSessionState) {
	if state == nil || state != a.documentSessionState || state.word == nil || !state.wordAutoSave || state.saveAsRequired || sessionPath(state) == "" || a.isSaving() || state.formatRunning.Load() || state.wordFlushRequested.Load() || !sessionDirty(state) {
		return
	}
	a.startSaveForReason(wordSaveAuto)
}
