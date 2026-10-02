package desktopui

import (
	"glance/internal/document"
)

type wordDocumentInfoResult struct {
	state      *documentSessionState
	generation uint64
	properties document.WordDocumentProperties
	err        error
}

func (a *App) startWordDocumentInfoLoad() {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning || a.saving {
		return
	}
	reader, ok := any(state.word).(document.WordDocumentPropertiesEditor)
	if !ok {
		return
	}
	state.formatRunning = true
	a.status = "正在读取文档信息"
	generation := a.documentGeneration
	go func() {
		properties, err := reader.GetWordDocumentProperties()
		if a.wordDocumentInfoResults == nil {
			return
		}
		select {
		case a.wordDocumentInfoResults <- wordDocumentInfoResult{state: state, generation: generation, properties: properties, err: err}:
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
	if state == nil || state.word == nil || state.formatRunning || a.saving {
		return
	}
	editor, ok := any(state.word).(document.WordDocumentPropertiesEditor)
	if !ok {
		return
	}
	state.formatRunning = true
	generation := a.documentGeneration
	go func() {
		err := editor.SetWordDocumentProperties(properties)
		var blocks []document.WordBlock
		if err == nil {
			blocks = cloneWordBlockSnapshot(state.word.WordBlocks())
		}
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: "更新文档信息", err: err, blocks: blocks})
	}()
}
