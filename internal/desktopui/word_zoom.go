package desktopui

import (
	"fmt"
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
	a.modal.ShowInput("缩放", "设置打印布局缩放比例（50%–150%）", initialPercent, "百分比", func(action string) {
		if action != "confirm" {
			a.modal.Hide()
			return
		}
		value := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(a.modal.Input.Text()), "%"))
		percent, err := strconv.Atoi(value)
		if err != nil || percent < 50 || percent > 150 {
			a.setStatus("缩放比例请输入 50–150 的整数百分比")
			return
		}
		setWordManualZoom(state, float64(percent)/100)
		a.setStatus(fmt.Sprintf("缩放：%d%%", percent))
		a.modal.Hide()
	}, modalDialogActionSpec{ID: "confirm", Label: "应用"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}
