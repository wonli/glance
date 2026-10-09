package desktopui

import (
	"fmt"
	"glance/internal/document"
	"glance/internal/format/xlsx"
	"strings"
)

func (a *App) runSpreadsheetHyperlinkCommand(operation string, edit func(document.SpreadsheetHyperlinkEditor) error) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetHyperlinkEditor)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	go func() {
		unlock := lockSessionAdapterWrite(state)
		err := edit(editor)
		unlock()
		a.completeSessionOperation(state, token, err, true)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation}:
		case <-a.shutdown:
		}
	}()
}

func (a *App) showSpreadsheetLinkDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetHyperlinkEditor)
	if !ok {
		return
	}
	col := state.selection.Active.Column + 1
	row := state.selection.Active.Row + 1
	cellName := xlsx.CellName(col, row)
	sheet := state.sheet

	existingLink, hasLink, _ := editor.GetCellHyperlink(sheet, cellName)
	initialTarget := existingLink.Target
	initialTooltip := existingLink.Tooltip

	actions := []modalDialogActionSpec{
		{ID: "confirm", Label: "确定"},
	}
	if hasLink {
		actions = append(actions, modalDialogActionSpec{ID: "remove", Label: "删除链接"})
	}
	actions = append(actions, modalDialogActionSpec{ID: "cancel", Label: "取消"})

	a.modal.ShowDualInput("插入链接", fmt.Sprintf("为单元格 %s 设置超链接：", cellName),
		initialTarget, "链接地址 (例如 https://example.com 或 Sheet2!A1)",
		initialTooltip, "提示文本 (可选)",
		func(action string) {
			switch action {
			case "confirm":
				target := strings.TrimSpace(a.modal.Input.Text())
				tooltip := strings.TrimSpace(a.modal.SecondInput.Text())
				a.modal.Hide()
				if target == "" {
					a.setStatus("链接地址不能为空")
					return
				}
				a.runSpreadsheetHyperlinkCommand("插入链接", func(ed document.SpreadsheetHyperlinkEditor) error {
					return ed.SetCellHyperlink(sheet, cellName, target, tooltip)
				})
				a.setStatus(fmt.Sprintf("已为 %s 设置链接：%s", cellName, target))
			case "remove":
				a.modal.Hide()
				a.runSpreadsheetHyperlinkCommand("删除链接", func(ed document.SpreadsheetHyperlinkEditor) error {
					return ed.RemoveCellHyperlink(sheet, cellName)
				})
				a.setStatus(fmt.Sprintf("已删除 %s 的超链接", cellName))
			case "cancel":
				a.modal.Hide()
			}
		},
		actions...,
	)
}
