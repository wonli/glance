package desktopui

import "fmt"

func wordNewWindowAvailability(state *documentSessionState, runtime *desktopRuntime) (bool, string) {
	if state == nil || state.word == nil {
		return false, "请先打开 Word 文档"
	}
	if state.sharedDocumentSession == nil {
		return false, "当前 Word 文档尚未连接到共享会话"
	}
	if state.closed.Load() || state.finalized.Load() {
		return false, "当前 Word 文档会话已关闭"
	}
	if !supportsMultipleDesktopWindows() {
		return false, "当前平台不支持多个桌面窗口"
	}
	if runtime == nil {
		return false, "新建窗口需要已连接的桌面运行时"
	}
	documentID := state.documentID
	if documentID == "" {
		documentID = state.sharedDocumentSession.documentID
	}
	shared, ok := runtime.session(documentID)
	if !ok || shared != state.sharedDocumentSession {
		return false, "当前 Word 文档尚未连接到桌面会话"
	}
	if !runtime.hasWindow(state.windowID) {
		return false, "当前 Word 视图未连接到已打开的桌面窗口"
	}
	return true, ""
}

func (a *App) requestWordNewWindow() {
	if err := a.openWordNewWindow(); err != nil {
		a.setStatus(err.Error())
	}
}

func (a *App) openWordNewWindow() error {
	state := a.documentSessionState
	enabled, reason := wordNewWindowAvailability(state, a.desktopRuntime)
	if !enabled {
		return fmt.Errorf("%s", reason)
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
