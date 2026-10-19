package desktopui

import (
	"glance/internal/document"
	"glance/internal/i18n"
)

type wordDocumentInfoResult struct {
	state      *documentSessionState
	token      sessionOperationToken
	generation uint64
	properties document.WordDocumentProperties
	err        error
}

func (a *App) startWordDocumentInfoLoad() {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	reader, ok := any(state.word).(document.WordDocumentPropertiesEditor)
	if !ok {
		return
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		return
	}
	a.setStatusMessage(i18n.Message{ID: "word.status.inline.reading_document_information_053d93"}, feedbackInfo, notificationSuppressed)
	generation := a.documentGeneration
	a.launchManagedBackground(func() {
		unlock := lockSessionAdapterRead(state)
		properties, err := reader.GetWordDocumentProperties()
		unlock()
		state.finishOperation(token)
		if a.wordDocumentInfoResults == nil {
			return
		}
		select {
		case a.wordDocumentInfoResults <- wordDocumentInfoResult{state: state, token: token, generation: generation, properties: properties, err: err}:
			if a.window != nil {
				a.window.Invalidate()
			}
		case <-a.shutdown:
		}
	})
}

func (a *App) showWordDocumentInfoDialog(properties document.WordDocumentProperties) {
	labels := []i18n.Message{{ID: "word.document_info.field.title"}, {ID: "word.document_info.field.subject"}, {ID: "word.document_info.field.author"}, {ID: "word.document_info.field.keywords"}, {ID: "word.document_info.field.description"}}
	hints := []i18n.Message{{ID: "word.document_info.hint.title"}, {ID: "word.document_info.hint.subject"}, {ID: "word.document_info.hint.author"}, {ID: "word.document_info.hint.keywords"}, {ID: "word.document_info.hint.description"}}
	initial := []string{properties.Title, properties.Subject, properties.Creator, properties.Keywords, properties.Description}
	a.modal.ShowFormMessage(dialogIDWordDocumentInfo, i18n.Message{ID: "word.document_info.title"}, i18n.Message{ID: "word.document_info.body"}, labels, hints, initial, func(action string) {
		switch action {
		case "confirm":
			properties := document.WordDocumentProperties{
				Title:       a.modal.FormInputs[0].Text(),
				Subject:     a.modal.FormInputs[1].Text(),
				Creator:     a.modal.FormInputs[2].Text(),
				Keywords:    a.modal.FormInputs[3].Text(),
				Description: a.modal.FormInputs[4].Text(),
			}
			a.modal.Hide()
			a.runWordDocumentInfoCommand(properties)
		case "cancel":
			a.modal.Hide()
		}
	}, modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "common.action.apply"}}, modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}})
}

func (a *App) runWordDocumentInfoCommand(properties document.WordDocumentProperties) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := any(state.word).(document.WordDocumentPropertiesEditor)
	if !ok {
		return
	}
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return
	}
	generation := a.documentGeneration
	a.launchManagedBackground(func() {
		blocks, err := runWordMutation(state, func() error { return editor.SetWordDocumentProperties(properties) })
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: "更新文档信息", err: err, blocks: blocks})
	})
}
