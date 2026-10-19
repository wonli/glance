package desktopui

import (
	"glance/internal/document"
	"glance/internal/i18n"
)

type wordDocumentProtectionSetArguments struct {
	Enabled bool `json:"enabled"`
}

func (a *App) runWordProtectionApplicationCommand(operation string, enabled bool) bool {
	state := a.documentSessionState
	if state == nil || state.word == nil || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	if !a.runWordApplicationCommand(operation, "word.document.protection.set", wordDocumentProtectionSetArguments{Enabled: enabled}) {
		a.setStatusMessage(i18n.Message{ID: "word.status.operation_busy"}, feedbackWarning, notificationToast)
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
		a.modal.ShowMessage(dialogIDWordProtection, i18n.Message{ID: "word.protection.title"}, i18n.Message{ID: "word.protection.body.unprotect"}, func(action string) {
			a.modal.Hide()
			if action == "unprotect" {
				if !a.runWordProtectionApplicationCommand("解除限制编辑", false) {
					a.setStatusMessage(i18n.Message{ID: "word.protection.error.unprotect_unavailable"}, feedbackWarning, notificationToast)
				}
			}
		},
			modalDialogActionSpec{ID: "unprotect", LabelRef: i18n.Message{ID: "word.protection.action.unprotect"}},
			modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
		)
	} else {
		a.modal.ShowMessage(dialogIDWordProtection, i18n.Message{ID: "word.protection.title"}, i18n.Message{ID: "word.protection.body.protect"}, func(action string) {
			a.modal.Hide()
			if action == "protect" {
				if !a.runWordProtectionApplicationCommand("开启限制编辑", true) {
					a.setStatusMessage(i18n.Message{ID: "word.protection.error.protect_unavailable"}, feedbackWarning, notificationToast)
				}
			}
		},
			modalDialogActionSpec{ID: "protect", LabelRef: i18n.Message{ID: "word.protection.action.protect"}},
			modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
		)
	}
}
