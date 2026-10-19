package desktopui

import (
	"glance/internal/document"
	"glance/internal/i18n"
	"strings"
)

func (a *App) runWordWatermarkCommand(operation string, edit func(document.WordWatermarkEditor) error) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := any(state.word).(document.WordWatermarkEditor)
	if !ok {
		return
	}
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return
	}
	generation := a.documentGeneration
	a.launchManagedBackground(func() {
		blocks, err := runWordMutation(state, func() error { return edit(editor) })
		if err == nil {
			state.invalidateWordLayoutAfterBlocksChange()
		}
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: operation, err: err, blocks: blocks})
	})
}

type wordReviewTrackSetArguments struct {
	Enabled bool `json:"enabled"`
}

func (a *App) toggleWordTrackRevisions() {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := any(state.word).(document.WordTrackRevisionsEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_current_document_does_not_cc2e8c"}, feedbackWarning, notificationToast)
		return
	}
	current, _ := editor.GetTrackRevisions()
	next := !current
	label := "开启修订跟踪"
	if !next {
		label = "关闭修订跟踪"
	}
	if !a.runWordApplicationCommand(label, "word.review.track.set", wordReviewTrackSetArguments{Enabled: next}) {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_application_command_runtime_is_27bdc3"}, feedbackWarning, notificationToast)
	}
}

type wordreviewActionArguments struct {
	Action string `json:"action"`
}

func (a *App) acceptWordRevisions() {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	_, ok := any(state.word).(document.WordTrackRevisionsEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_current_document_does_not_3a085b"}, feedbackWarning, notificationToast)
		return
	}
	a.modal.ShowMessage("word.review.accept_all", i18n.Message{ID: "word.review.accept_all.dialog.title"}, i18n.Message{ID: "word.review.accept_all.dialog.body"}, func(action string) {
		a.modal.Hide()
		if action == "confirm" {
			if !a.runWordApplicationCommand("接受全部修订", "word.review.revisions.apply", wordreviewActionArguments{Action: "accept_all"}) {
				a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_application_command_runtime_is_43e738"}, feedbackWarning, notificationToast)
			}
		}
	},
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "word.review.accept_all.action"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

func (a *App) rejectWordRevisions() {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	_, ok := any(state.word).(document.WordTrackRevisionsEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_current_document_does_not_3a085b"}, feedbackWarning, notificationToast)
		return
	}
	a.modal.ShowMessage("word.review.reject_all", i18n.Message{ID: "word.review.reject_all.dialog.title"}, i18n.Message{ID: "word.review.reject_all.dialog.body"}, func(action string) {
		a.modal.Hide()
		if action == "confirm" {
			if !a.runWordApplicationCommand("拒绝全部修订", "word.review.revisions.apply", wordreviewActionArguments{Action: "reject_all"}) {
				a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_application_command_runtime_is_8a0e98"}, feedbackWarning, notificationToast)
			}
		}
	},
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "word.review.reject_all.action"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

func (a *App) showWordWatermarkDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordWatermarkEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_current_document_does_not_273f9e"}, feedbackWarning, notificationToast)
		return
	}
	currentWatermark, _ := editor.GetWatermark()
	defaultVal := currentWatermark
	if defaultVal == "" {
		defaultVal = a.text(i18n.Message{ID: "word.watermark.default_text"})
	}
	a.modal.ShowInputMessage("word.watermark", i18n.Message{ID: "word.watermark.dialog.title"}, i18n.Message{ID: "word.watermark.dialog.body"},
		defaultVal,
		i18n.Message{ID: "word.watermark.dialog.hint"},
		func(action string) {
			a.modal.Hide()
			switch action {
			case "set":
				text := strings.TrimSpace(a.modal.Input.Text())
				if text == "" {
					text = a.text(i18n.Message{ID: "word.watermark.default_text"})
				}
				a.runWordWatermarkCommand("设置文档水印", func(ed document.WordWatermarkEditor) error {
					return ed.SetWatermark(text)
				})
			case "clear":
				a.runWordWatermarkCommand("清除文档水印", func(ed document.WordWatermarkEditor) error {
					return ed.ClearWatermark()
				})
			}
		},
		modalDialogActionSpec{ID: "set", LabelRef: i18n.Message{ID: "word.watermark.action.apply"}},
		modalDialogActionSpec{ID: "clear", LabelRef: i18n.Message{ID: "word.watermark.action.clear"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}
