package desktopui

import (
	"fmt"
	"glance/internal/document"
	"glance/internal/format/xlsx"
	"strings"
)

func (a *App) showSpreadsheetNewCommentDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetCommentEditor)
	if !ok {
		a.setStatus("当前工作簿不支持批注")
		return
	}
	activeCol := state.selection.Active.Column + 1
	activeRow := state.selection.Active.Row + 1
	cellName := xlsx.CellName(activeCol, activeRow)
	if cellName == "" {
		cellName = fmt.Sprintf("%s%d", columnName(activeCol), activeRow)
	}

	prompt := fmt.Sprintf("为单元格 %s 添加批注：", cellName)
	a.modal.ShowInput("新建批注", prompt, "", "输入批注内容", func(action string) {
		text := strings.TrimSpace(a.modal.Input.Text())
		a.modal.Hide()
		if action != "confirm" || text == "" {
			return
		}
		generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
		token, ok := state.beginOperation(sessionOperationMutation)
		if !ok {
			return
		}
		go func() {
			unlock := lockSessionAdapterWrite(state)
			err := editor.AddComment(sheet, document.SpreadsheetComment{
				Author: "用户",
				Cell:   cellName,
				Text:   text,
			})
			unlock()
			a.completeSessionOperation(state, token, err, true)
			if err == nil {
				a.setStatus(fmt.Sprintf("已为单元格 %s 添加批注", cellName))
			}
			select {
			case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "新建批注"}:
			case <-a.shutdown:
			}
		}()
	}, modalDialogActionSpec{ID: "confirm", Label: "确定"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}

func (a *App) deleteSpreadsheetComment() {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetCommentEditor)
	if !ok {
		a.setStatus("当前工作簿不支持批注")
		return
	}
	activeCol := state.selection.Active.Column + 1
	activeRow := state.selection.Active.Row + 1
	cellName := xlsx.CellName(activeCol, activeRow)
	if cellName == "" {
		cellName = fmt.Sprintf("%s%d", columnName(activeCol), activeRow)
	}

	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	go func() {
		unlock := lockSessionAdapterWrite(state)
		err := editor.DeleteComment(sheet, cellName)
		unlock()
		a.completeSessionOperation(state, token, err, true)
		if err == nil {
			a.setStatus(fmt.Sprintf("已删除单元格 %s 的批注", cellName))
		}
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "删除批注"}:
		case <-a.shutdown:
		}
	}()
}
