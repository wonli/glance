package desktopui

import (
	"fmt"
	"glance/internal/document"
	"strings"
)

func (a *App) runWordRevisionCommand(operation string, edit func(document.WordTrackRevisionsEditor) error) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := any(state.word).(document.WordTrackRevisionsEditor)
	if !ok {
		return
	}
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return
	}
	generation := a.documentGeneration
	go func() {
		blocks, err := runWordMutation(state, func() error { return edit(editor) })
		if err == nil {
			state.invalidateWordLayoutAfterBlocksChange()
		}
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: operation, err: err, blocks: blocks})
	}()
}

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
	go func() {
		blocks, err := runWordMutation(state, func() error { return edit(editor) })
		if err == nil {
			state.invalidateWordLayoutAfterBlocksChange()
		}
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: operation, err: err, blocks: blocks})
	}()
}

func (a *App) toggleWordTrackRevisions() {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := any(state.word).(document.WordTrackRevisionsEditor)
	if !ok {
		a.setStatus("当前文档不支持修订跟踪")
		return
	}
	current, _ := editor.GetTrackRevisions()
	next := !current
	label := "开启修订跟踪"
	if !next {
		label = "关闭修订跟踪"
	}
	a.runWordRevisionCommand(label, func(ed document.WordTrackRevisionsEditor) error {
		if err := ed.SetTrackRevisions(next); err != nil {
			return err
		}
		state.wordTrackRevisions = next
		if next {
			a.setStatus("已开启修订跟踪")
		} else {
			a.setStatus("已关闭修订跟踪")
		}
		return nil
	})
}

func (a *App) acceptWordRevisions() {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	_, ok := any(state.word).(document.WordTrackRevisionsEditor)
	if !ok {
		a.setStatus("当前文档不支持修订操作")
		return
	}
	a.modal.Show("接受全部修订", "将接受文档中的所有插入并移除所有删除标记。是否继续？", func(action string) {
		a.modal.Hide()
		if action == "confirm" {
			a.runWordRevisionCommand("接受全部修订", func(ed document.WordTrackRevisionsEditor) error {
				count, err := ed.AcceptAllRevisions()
				if err != nil {
					return err
				}
				a.setStatus(fmt.Sprintf("已接受全部修订（处理 %d 处）", count))
				return nil
			})
		}
	},
		modalDialogActionSpec{ID: "confirm", Label: "接受修订"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) rejectWordRevisions() {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	_, ok := any(state.word).(document.WordTrackRevisionsEditor)
	if !ok {
		a.setStatus("当前文档不支持修订操作")
		return
	}
	a.modal.Show("拒绝全部修订", "将放弃文档中的所有插入并恢复所有已删除内容。是否继续？", func(action string) {
		a.modal.Hide()
		if action == "confirm" {
			a.runWordRevisionCommand("拒绝全部修订", func(ed document.WordTrackRevisionsEditor) error {
				count, err := ed.RejectAllRevisions()
				if err != nil {
					return err
				}
				a.setStatus(fmt.Sprintf("已拒绝全部修订（处理 %d 处）", count))
				return nil
			})
		}
	},
		modalDialogActionSpec{ID: "confirm", Label: "拒绝修订"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordWatermarkDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordWatermarkEditor)
	if !ok {
		a.setStatus("当前文档不支持设置水印")
		return
	}
	currentWatermark, _ := editor.GetWatermark()
	defaultVal := currentWatermark
	if defaultVal == "" {
		defaultVal = "机密"
	}
	a.modal.ShowInput(
		"文档水印",
		"请输入文字水印内容（或点击清除水印）：",
		defaultVal,
		"机密、草稿、内部传阅等",
		func(action string) {
			a.modal.Hide()
			switch action {
			case "set":
				text := strings.TrimSpace(a.modal.Input.Text())
				if text == "" {
					text = "机密"
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
		modalDialogActionSpec{ID: "set", Label: "应用水印"},
		modalDialogActionSpec{ID: "clear", Label: "清除水印"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}
