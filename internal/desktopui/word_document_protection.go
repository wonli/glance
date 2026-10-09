package desktopui

import "glance/internal/document"

func (a *App) runWordProtectionCommand(operation string, edit func(document.WordDocumentProtectionEditor) error) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := any(state.word).(document.WordDocumentProtectionEditor)
	if !ok {
		return
	}
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return
	}
	generation := a.documentGeneration
	go func() {
		blocks, err := runWordMutation(state, func() error { return edit(editor) })
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: operation, err: err, blocks: blocks})
	}()
}

func (a *App) showWordDocumentProtectionDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordDocumentProtectionEditor)
	if !ok {
		return
	}
	protected := state.wordProtected
	if isProtected, err := editor.GetWordDocumentProtection(); err == nil {
		protected = isProtected
		state.wordProtected = isProtected
	}

	if protected {
		a.modal.Show("限制编辑", "当前文档已开启编辑限制（只读保护）。\n是否解除编辑限制以允许修改？", func(action string) {
			a.modal.Hide()
			if action == "unprotect" {
				a.runWordProtectionCommand("解除限制编辑", func(ed document.WordDocumentProtectionEditor) error {
					return ed.SetWordDocumentProtection(false)
				})
			}
		},
			modalDialogActionSpec{ID: "unprotect", Label: "解除限制"},
			modalDialogActionSpec{ID: "cancel", Label: "取消"},
		)
	} else {
		a.modal.Show("限制编辑", "开启编辑限制后，文档将被标记为受保护（只读模式），防止误修改正文内容。\n是否开启限制编辑？", func(action string) {
			a.modal.Hide()
			if action == "protect" {
				a.runWordProtectionCommand("开启限制编辑", func(ed document.WordDocumentProtectionEditor) error {
					return ed.SetWordDocumentProtection(true)
				})
			}
		},
			modalDialogActionSpec{ID: "protect", Label: "开启限制"},
			modalDialogActionSpec{ID: "cancel", Label: "取消"},
		)
	}
}
