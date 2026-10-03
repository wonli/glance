package desktopui

import (
	"glance/internal/document"
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
	a.status = "正在读取文档信息"
	generation := a.documentGeneration
	go func() {
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
	}()
}

func (a *App) showWordDocumentInfoDialog(properties document.WordDocumentProperties) {
	labels := []string{"标题", "主题", "作者", "关键词", "说明"}
	hints := []string{"输入文档标题", "输入文档主题", "输入作者", "输入关键词", "输入文档说明"}
	initial := []string{properties.Title, properties.Subject, properties.Creator, properties.Keywords, properties.Description}
	a.modal.ShowForm("文档信息", "编辑 DOCX 核心属性", labels, hints, initial, func(action string) {
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
	}, modalDialogActionSpec{ID: "confirm", Label: "应用"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
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
	go func() {
		blocks, err := runWordMutation(state, func() error { return editor.SetWordDocumentProperties(properties) })
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: "更新文档信息", err: err, blocks: blocks})
	}()
}
