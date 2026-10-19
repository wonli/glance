package desktopui

import "glance/internal/i18n"

func (a *App) toggleWordSideBySideView() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if a.desktopRuntime != nil && a.desktopRuntime.windowCount() > 1 {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.side_by_side_view_is_782e6d"}, feedbackSuccess, notificationSuppressed)
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	enabled, _ := wordNewWindowAvailability(state, a.desktopRuntime)
	if enabled {
		a.requestWordNewWindow()
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.opened_a_new_window_for_074edf"}, feedbackSuccess, notificationSuppressed)
	} else {
		a.selectWordMultiplePagesView()
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.two_page_side_by_side_71eeb7"}, feedbackSuccess, notificationSuppressed)
	}
}
