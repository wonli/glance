package desktopui

import (
	"fmt"
	"glance/internal/document"
	"glance/internal/format/xlsx"
	"strings"
)

func (a *App) showSpreadsheetFindDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	searcher, ok := state.book.(document.SpreadsheetSearcher)
	if !ok {
		a.setStatus("当前表格不支持查找")
		return
	}
	a.modal.ShowInput("查找", "在当前工作表中查找内容", state.excelFindQuery, "查找内容", func(action string) {
		query := strings.TrimSpace(a.modal.Input.Text())
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		if query == "" {
			a.setStatus("请输入要查找的内容")
			return
		}
		state.excelFindQuery = query
		matches, err := searcher.SearchSheet(state.sheet, query)
		if err != nil {
			a.setStatus("查找失败：" + err.Error())
			return
		}
		if len(matches) == 0 {
			a.setStatus("未找到：" + query)
			return
		}
		currentPos := state.selection.Active
		curRow, curCol := currentPos.Row+1, currentPos.Column+1
		target := matches[0]
		for _, m := range matches {
			if m.Row > curRow || (m.Row == curRow && m.Column > curCol) {
				target = m
				break
			}
		}
		a.navigateToCell(target.Row, target.Column)
		a.setStatus(fmt.Sprintf("已定位至 %s（共 %d 处匹配）", xlsx.CellName(target.Column, target.Row), len(matches)))
	}, modalDialogActionSpec{ID: "confirm", Label: "查找下一个"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}
