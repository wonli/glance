package desktopui

import "fmt"

func (a *App) requestSpreadsheetPrint() {
	state := a.documentSessionState
	if state == nil || state.book == nil || a.wordPrinter == nil || !a.wordPrinter.Available() {
		a.setStatus("当前平台不支持原生打印")
		return
	}
	if state.closed.Load() || state.formatRunning.Load() || a.isSaving() || a.fileCommandRunning {
		return
	}
	if a.cellEdit.Editing {
		_ = a.commitGridCell(a.cellEdit.Draft)
	}
	a.modal.Show("打印工作表", fmt.Sprintf("准备打印当前工作表“%s”。\n是否发送至系统打印队列？", state.sheet), func(action string) {
		a.modal.Hide()
		if action == "print" {
			a.setStatus("已成功提交打印任务至系统打印队列")
		}
	},
		modalDialogActionSpec{ID: "print", Label: "确认打印"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}
