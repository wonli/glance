package desktopui

import "glance/internal/document"

func (a *App) runWordProtectionApplicationCommand(operation string, enabled bool) bool {
	state := a.documentSessionState
	if state == nil || state.word == nil || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	if !a.runWordApplicationCommand(operation, "word.document.protection.set", map[string]any{"enabled": enabled}) {
		a.setStatus("Word 文档正在处理其他操作")
	}
	return true
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
				if !a.runWordProtectionApplicationCommand("解除限制编辑", false) {
					a.setStatus("当前文档的应用命令运行时不可用，未解除编辑限制")
				}
			}
		},
			modalDialogActionSpec{ID: "unprotect", Label: "解除限制"},
			modalDialogActionSpec{ID: "cancel", Label: "取消"},
		)
	} else {
		a.modal.Show("限制编辑", "开启编辑限制后，文档将被标记为受保护（只读模式），防止误修改正文内容。\n是否开启限制编辑？", func(action string) {
			a.modal.Hide()
			if action == "protect" {
				if !a.runWordProtectionApplicationCommand("开启限制编辑", true) {
					a.setStatus("当前文档的应用命令运行时不可用，未开启编辑限制")
				}
			}
		},
			modalDialogActionSpec{ID: "protect", Label: "开启限制"},
			modalDialogActionSpec{ID: "cancel", Label: "取消"},
		)
	}
}
