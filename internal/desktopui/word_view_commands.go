package desktopui

func (a *App) toggleWordSideBySideView() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if a.desktopRuntime != nil && a.desktopRuntime.windowCount() > 1 {
		a.setStatus("已开启并排查看模式")
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	enabled, _ := wordNewWindowAvailability(state, a.desktopRuntime)
	if enabled {
		a.requestWordNewWindow()
		a.setStatus("已打开新窗口并排查看")
	} else {
		a.selectWordMultiplePagesView()
		a.setStatus("已开启双页并排查看")
	}
}
