package desktopui

import (
	"glance/internal/document"
	"glance/internal/i18n"
	"strings"
)

func (a *App) showWordCoverPageDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordCoverPageEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_current_document_does_not_4e91b9"}, feedbackWarning, notificationToast)
		return
	}
	a.modal.ShowInputMessage("word.cover_page", i18n.Message{ID: "word.cover_page.dialog.title"}, i18n.Message{ID: "word.cover_page.dialog.body"}, a.text(i18n.Message{ID: "word.cover_page.default_title"}), i18n.Message{ID: "word.cover_page.dialog.hint"}, func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		title := strings.TrimSpace(a.modal.Input.Text())
		if title == "" {
			title = a.text(i18n.Message{ID: "word.cover_page.default_title"})
		}
		err := editor.InsertCoverPage(title, a.text(i18n.Message{ID: "word.cover_page.default_subtitle"}), a.text(i18n.Message{ID: "word.cover_page.default_author"}))
		if err != nil {
			a.setWordCommandStatus(err)
			return
		}
		if state.word != nil {
			state.setWordBlocks(state.word.WordBlocks())
			a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
		}
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.inserted_a_standard_cover_page_ddfc3b"}, feedbackSuccess, notificationToast)
		if a.window != nil {
			a.window.Invalidate()
		}
	},
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "word.cover_page.action.insert"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

func (a *App) showWordLanguageDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordLanguageEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_current_document_does_not_a61cb2"}, feedbackWarning, notificationToast)
		return
	}
	runID := state.selectedWordRunID
	targetDesc := a.text(i18n.Message{ID: "word.language.target.selection"})
	if runID == "" {
		targetDesc = a.text(i18n.Message{ID: "word.language.target.document"})
	}
	a.modal.ShowInputMessage("word.language", i18n.Message{ID: "word.language.dialog.title"}, i18n.Message{ID: "word.language.dialog.body", Args: i18n.Args{"Target": targetDesc}}, "zh-CN", i18n.Message{ID: "word.language.dialog.hint"}, func(action string) {
		a.modal.Hide()
		var lang string
		switch action {
		case "confirm":
			lang = strings.TrimSpace(a.modal.Input.Text())
		case "zh-CN":
			lang = "zh-CN"
		case "en-US":
			lang = "en-US"
		case "ja-JP":
			lang = "ja-JP"
		default:
			return
		}
		if lang == "" {
			lang = "zh-CN"
		}
		err := editor.SetLanguage(runID, lang)
		if err != nil {
			a.setWordCommandStatus(err)
			return
		}
		messageID := "word.insert.language.status.applied.document"
		if runID != "" {
			messageID = "word.insert.language.status.applied.selection"
		}
		a.setStatusMessage(i18n.Message{ID: messageID, Args: i18n.Args{"Language": lang}}, feedbackSuccess, notificationToast)
		if a.window != nil {
			a.window.Invalidate()
		}
	},
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "word.language.action.custom"}},
		modalDialogActionSpec{ID: "zh-CN", LabelRef: i18n.Message{ID: "locale.name.zh_cn"}},
		modalDialogActionSpec{ID: "en-US", LabelRef: i18n.Message{ID: "locale.name.en_us"}},
		modalDialogActionSpec{ID: "ja-JP", LabelRef: i18n.Message{ID: "word.language.locale.ja_jp"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

func (a *App) showWordBookmarkDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordBookmarkEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_current_document_does_not_526b19"}, feedbackWarning, notificationToast)
		return
	}
	runID := state.selectedWordRunID
	a.modal.ShowInputMessage("word.insert.bookmark", i18n.Message{ID: "word.insert.bookmark.title"}, i18n.Message{ID: "word.insert.bookmark.body"}, "Bookmark1", i18n.Message{ID: "word.insert.bookmark.hint"}, func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		name := strings.TrimSpace(a.modal.Input.Text())
		if name == "" {
			name = "Bookmark1"
		}
		err := editor.InsertBookmark(runID, name)
		if err != nil {
			a.setWordCommandStatus(err)
			return
		}
		if state.word != nil {
			state.setWordBlocks(state.word.WordBlocks())
			a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
		}
		a.setStatusMessage(i18n.Message{ID: "word.insert.bookmark.status.inserted", Args: i18n.Args{"Name": name}}, feedbackSuccess, notificationToast)
		if a.window != nil {
			a.window.Invalidate()
		}
	},
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "word.insert.bookmark.action.insert"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

func (a *App) showWordHyperlinkDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordHyperlinkEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_current_document_does_not_c9db67"}, feedbackWarning, notificationToast)
		return
	}
	runID := state.selectedWordRunID
	var defaultText string
	selectionStart, selectionEnd, hasSelection := wordPageOrderedSelection(state.wordPageTextSelection)
	if hasSelection {
		var ok bool
		defaultText, ok = wordPageSelectedText(state)
		if !ok {
			a.setStatusMessage(i18n.Message{ID: "word.status.inline.could_not_identify_the_current_e11d47"}, feedbackWarning, notificationToast)
			return
		}
	} else if run, ok := selectedWordRun(wordBlocksForState(state), runID); ok {
		defaultText = run.Text
	}
	if defaultText == "" {
		defaultText = a.text(i18n.Message{ID: "word.hyperlink.default_text"})
	}

	a.modal.ShowDualInputMessage("word.hyperlink", i18n.Message{ID: "word.hyperlink.dialog.title"}, i18n.Message{ID: "word.hyperlink.dialog.body"},
		i18n.Message{ID: "word.hyperlink.dialog.url_hint"}, i18n.Message{ID: "word.hyperlink.dialog.text_hint"},
		"https://example.com", defaultText,
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			urlVal := strings.TrimSpace(a.modal.Input.Text())
			textVal := a.modal.SecondInput.Text()
			if urlVal == "" {
				a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_hyperlink_address_cannot_be_34921d"}, feedbackError, notificationToast)
				return
			}
			if strings.TrimSpace(textVal) == "" {
				textVal = urlVal
			}
			var err error
			if hasSelection {
				selectionEditor, supported := any(state.word).(document.WordTextSelectionHyperlinkEditor)
				if !supported {
					a.setStatusMessage(i18n.Message{ID: "word.status.inline.this_document_does_not_yet_229ca8"}, feedbackWarning, notificationToast)
					return
				}
				err = selectionEditor.InsertHyperlinkForTextSelection(selectionStart.runID, selectionStart.runeOffset, selectionEnd.runID, selectionEnd.runeOffset, textVal, urlVal)
			} else {
				err = editor.InsertHyperlink(runID, textVal, urlVal)
			}
			if err != nil {
				a.setWordCommandStatus(err)
				return
			}
			if state.word != nil {
				state.wordPageTextSelection = nil
				state.setWordBlocks(state.word.WordBlocks())
				a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
			}
			a.setStatusMessage(i18n.Message{ID: "word.status.inline.inserted_a_hyperlink_508d90"}, feedbackSuccess, notificationToast)
			if a.window != nil {
				a.window.Invalidate()
			}
		},
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "common.action.insert"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

func (a *App) showWordTextBoxDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordTextBoxEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_current_document_does_not_d8e3be"}, feedbackWarning, notificationToast)
		return
	}
	a.modal.ShowInputMessage("word.text_box", i18n.Message{ID: "word.text_box.dialog.title"}, i18n.Message{ID: "word.text_box.dialog.body"}, a.text(i18n.Message{ID: "word.text_box.default_text"}), i18n.Message{ID: "word.text_box.dialog.hint"}, func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		text := strings.TrimSpace(a.modal.Input.Text())
		if text == "" {
			text = a.text(i18n.Message{ID: "word.text_box.default_text"})
		}
		err := editor.InsertTextBox("", text)
		if err != nil {
			a.setWordCommandStatus(err)
			return
		}
		if state.word != nil {
			state.setWordBlocks(state.word.WordBlocks())
			a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
		}
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.inserted_a_text_box_e627aa"}, feedbackSuccess, notificationToast)
		if a.window != nil {
			a.window.Invalidate()
		}
	},
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "word.text_box.action.insert"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

func (a *App) showWordHeaderDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordHeaderFooterEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_current_document_does_not_62f6b1"}, feedbackWarning, notificationToast)
		return
	}
	a.modal.ShowInputMessage("word.header", i18n.Message{ID: "word.header.dialog.title"}, i18n.Message{ID: "word.header.dialog.body"}, a.text(i18n.Message{ID: "word.header.default_text"}), i18n.Message{ID: "word.header.dialog.hint"}, func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		text := strings.TrimSpace(a.modal.Input.Text())
		if text == "" {
			text = a.text(i18n.Message{ID: "word.header.default_text"})
		}
		err := editor.SetHeader(text)
		if err != nil {
			a.setWordCommandStatus(err)
			return
		}
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.header_set_e5de44"}, feedbackSuccess, notificationToast)
		if a.window != nil {
			a.window.Invalidate()
		}
	},
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "word.header.action.set"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

func (a *App) showWordFooterDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordHeaderFooterEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_current_document_does_not_96cf67"}, feedbackWarning, notificationToast)
		return
	}
	a.modal.ShowInputMessage("word.footer", i18n.Message{ID: "word.footer.dialog.title"}, i18n.Message{ID: "word.footer.dialog.body"}, a.text(i18n.Message{ID: "word.footer.default_text"}), i18n.Message{ID: "word.footer.dialog.hint"}, func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		text := strings.TrimSpace(a.modal.Input.Text())
		if text == "" {
			text = a.text(i18n.Message{ID: "word.footer.default_text"})
		}
		err := editor.SetFooter(text)
		if err != nil {
			a.setWordCommandStatus(err)
			return
		}
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.footer_set_57199c"}, feedbackSuccess, notificationToast)
		if a.window != nil {
			a.window.Invalidate()
		}
	},
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "word.footer.action.set"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

func (a *App) showWordPageNumberDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordHeaderFooterEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_current_document_does_not_e114ba"}, feedbackWarning, notificationToast)
		return
	}
	a.modal.ShowInputMessage("word.page_number", i18n.Message{ID: "word.page_number.dialog.title"}, i18n.Message{ID: "word.page_number.dialog.body"}, "center", i18n.Message{ID: "word.page_number.dialog.hint"}, func(action string) {
		a.modal.Hide()
		var align string
		switch action {
		case "confirm":
			align = strings.TrimSpace(a.modal.Input.Text())
		case "center":
			align = "center"
		case "right":
			align = "right"
		case "left":
			align = "left"
		default:
			return
		}
		if align == "" {
			align = "center"
		}
		err := editor.SetPageNumber(align)
		if err != nil {
			a.setWordCommandStatus(err)
			return
		}
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.inserted_page_numbers_in_the_7fbc35"}, feedbackSuccess, notificationToast)
		if a.window != nil {
			a.window.Invalidate()
		}
	},
		modalDialogActionSpec{ID: "center", LabelRef: i18n.Message{ID: "word.page_number.align.center"}},
		modalDialogActionSpec{ID: "right", LabelRef: i18n.Message{ID: "word.page_number.align.right"}},
		modalDialogActionSpec{ID: "left", LabelRef: i18n.Message{ID: "word.page_number.align.left"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}
