package desktopui

import (
	"fmt"
	"glance/internal/i18n"
)

func wordNewWindowAvailability(state *documentSessionState, runtime *desktopRuntime) (bool, i18n.Message) {
	if state == nil || state.word == nil {
		return false, i18n.Message{ID: "word.new_window.reason.open_document"}
	}
	if state.sharedDocumentSession == nil {
		return false, i18n.Message{ID: "word.new_window.reason.shared_session"}
	}
	if state.closed.Load() || state.finalized.Load() {
		return false, i18n.Message{ID: "word.new_window.reason.closed"}
	}
	if !supportsMultipleDesktopWindows() {
		return false, i18n.Message{ID: "word.new_window.reason.platform"}
	}
	if runtime == nil {
		return false, i18n.Message{ID: "word.new_window.reason.runtime"}
	}
	documentID := state.documentID
	if documentID == "" {
		documentID = state.sharedDocumentSession.documentID
	}
	shared, ok := runtime.session(documentID)
	if !ok || shared != state.sharedDocumentSession {
		return false, i18n.Message{ID: "word.new_window.reason.desktop_session"}
	}
	if !runtime.hasWindow(state.windowID) {
		return false, i18n.Message{ID: "word.new_window.reason.window"}
	}
	return true, i18n.Message{}
}

func (a *App) requestWordNewWindow() {
	if err := a.openWordNewWindow(); err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "word.new_window.error.open"}, err, notificationToast)
	}
}

func (a *App) openWordNewWindow() error {
	state := a.documentSessionState
	enabled, reason := wordNewWindowAvailability(state, a.desktopRuntime)
	if !enabled {
		return fmt.Errorf("%s", a.text(reason))
	}
	documentID := state.documentID
	if documentID == "" {
		documentID = state.sharedDocumentSession.documentID
	}
	if documentID == "" {
		return fmt.Errorf("创建新窗口失败：Word 文档会话没有 ID")
	}
	if err := a.desktopRuntime.openDocumentWindow(documentID, captureWindowView(a), a); err != nil {
		return fmt.Errorf("创建新窗口失败：%w", err)
	}
	return nil
}
