package desktopui

import (
	"glance/internal/i18n"
	"math"
	"strconv"
	"strings"
)

func (a *App) showWordZoomDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	initialPercent := strconv.Itoa(int(math.Round(wordStatusZoomClamp(state.wordLayoutZoom) * 100)))
	a.modal.ShowInputMessage("word.zoom", i18n.Message{ID: "word.zoom.dialog.title"}, i18n.Message{ID: "word.zoom.dialog.body"}, initialPercent, i18n.Message{ID: "word.zoom.dialog.hint"}, func(action string) {
		if action != "confirm" {
			a.modal.Hide()
			return
		}
		value := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(a.modal.Input.Text()), "%"))
		percent, err := strconv.Atoi(value)
		if err != nil || percent < 50 || percent > 150 {
			a.setStatusMessage(i18n.Message{ID: "word.status.inline.enter_a_whole_number_zoom_20406a"}, feedbackInfo, notificationSuppressed)
			return
		}
		setWordManualZoom(state, float64(percent)/100)
		a.setStatusMessage(i18n.Message{ID: "word.zoom.status", Args: i18n.Args{"Percent": percent}}, feedbackInfo, notificationSuppressed)
		a.modal.Hide()
	}, modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "common.action.apply"}}, modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}})
}
