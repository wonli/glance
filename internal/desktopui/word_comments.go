package desktopui

import (
	"glance/internal/document"
	"strings"
)

func (a *App) showWordNewCommentDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	run, hasRun := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
	afterPID := state.selectedWordParagraphID

	a.modal.ShowInput(
		"新建批注",
		"请输入审核批注内容（将为选中文本添加高亮标记并附注）：",
		"请核对此处表述是否准确",
		"输入批注内容",
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			commentText := strings.TrimSpace(a.modal.Input.Text())
			if commentText == "" {
				commentText = "审查批注"
			}

			if hasRun && run.Editable {
				yellow := "yellow"
				a.runWordCommand("添加批注标记", func(editor document.WordTextEditor) error {
					return editor.SetTextRunStyle(run.ID, document.WordTextFormatPatch{Highlight: &yellow})
				})
			}
			if tbEditor, ok := any(state.word).(document.WordTextBoxEditor); ok {
				_ = tbEditor.InsertTextBox(afterPID, "【批注】审核人: "+commentText)
			}
			a.setStatus("已成功为选区添加审阅批注")
			if a.window != nil {
				a.window.Invalidate()
			}
		},
		modalDialogActionSpec{ID: "confirm", Label: "添加批注"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordDeleteCommentDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	run, hasRun := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
	a.modal.Show(
		"删除批注",
		"是否清除当前选区或段落的批注标记与审阅高亮？",
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			if hasRun && run.Editable {
				emptyHighlight := ""
				a.runWordCommand("清除批注高亮", func(editor document.WordTextEditor) error {
					return editor.SetTextRunStyle(run.ID, document.WordTextFormatPatch{Highlight: &emptyHighlight})
				})
			}
			a.setStatus("已成功清除批注与高亮标记")
			if a.window != nil {
				a.window.Invalidate()
			}
		},
		modalDialogActionSpec{ID: "confirm", Label: "确认删除"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}
