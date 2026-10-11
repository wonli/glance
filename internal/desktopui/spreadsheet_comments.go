package desktopui

import (
	"encoding/json"
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
	_, ok := state.book.(document.SpreadsheetCommentEditor)
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
		sheet := state.sheet
		comment := document.SpreadsheetComment{
			Author: "用户",
			Cell:   cellName,
			Text:   text,
		}
		a.runSpreadsheetCommentCommand("新建批注", "spreadsheet.comment.add", map[string]any{"sheet": sheet, "cell": cellName, "author": comment.Author, "text": comment.Text}, func(ed document.SpreadsheetCommentEditor) error {
			return ed.AddComment(sheet, comment)
		})
	}, modalDialogActionSpec{ID: "confirm", Label: "确定"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}

func (a *App) deleteSpreadsheetComment() {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	_, ok := state.book.(document.SpreadsheetCommentEditor)
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

	sheet := state.sheet
	a.runSpreadsheetCommentCommand("删除批注", "spreadsheet.comment.delete", map[string]any{"sheet": sheet, "cell": cellName}, func(ed document.SpreadsheetCommentEditor) error {
		return ed.DeleteComment(sheet, cellName)
	})
}

func (a *App) runSpreadsheetCommentCommand(operation, commandID string, arguments any, edit func(document.SpreadsheetCommentEditor) error) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	if _, ok := state.book.(document.SpreadsheetCommentEditor); !ok {
		return
	}
	sheet, generation, window := state.sheet, a.documentGeneration, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	a.launchManagedBackground(func() {
		commandRouted := a.applicationSessionRegistered(token.documentID)
		changed := true
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, commandID, "spreadsheet_cell", "selection", sheet, arguments)
			err = commandErr
			if err == nil {
				var outcome struct {
					Changed bool `json:"changed"`
				}
				err = json.Unmarshal(result.Data, &outcome)
				changed = outcome.Changed
			}
		} else {
			editor := state.book.(document.SpreadsheetCommentEditor)
			unlock := lockSessionAdapterWrite(state)
			err = edit(editor)
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		if err == nil {
			a.setStatus("已完成" + operation)
		}
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation, applied: changed}:
		case <-a.shutdown:
		}
	})
}
