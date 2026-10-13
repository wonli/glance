package desktopui

import (
	"encoding/json"

	"glance/internal/document"
)

func (a *App) runSpreadsheetProtectionCommand(operation, scope string, arguments any, edit func(document.SpreadsheetProtectionEditor) error) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetProtectionEditor)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	a.launchManagedBackground(func() {
		commandRouted := a.applicationSessionRegistered(token.documentID)
		changed := true
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.protection.set", "spreadsheet_protection", scope, sheet, arguments)
			err = commandErr
			if err == nil {
				var outcome struct {
					Changed bool `json:"changed"`
				}
				err = json.Unmarshal(result.Data, &outcome)
				changed = outcome.Changed
			}
		} else {
			unlock := lockSessionAdapterWrite(state)
			err = edit(editor)
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation}:
		case <-a.shutdown:
		}
	})
}

type spreadsheetprotectionGoScopeSheetArguments struct {
	Scope    string `json:"scope"`
	Sheet    string `json:"sheet"`
	Enabled  bool   `json:"enabled"`
	Password string `json:"password"`
}

func (a *App) showSpreadsheetProtectSheetDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetProtectionEditor)
	if !ok {
		return
	}
	sheet := state.sheet
	isProtected, _ := editor.GetSheetProtection(sheet)

	if isProtected {
		a.modal.ShowInput("撤销保护工作表", "当前工作表已受保护。\n若设置了密码请输入密码（无密码直接确定）：", "", "密码（可选）", func(action string) {
			pwd := a.modal.Input.Text()
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			a.runSpreadsheetProtectionCommand("撤销保护工作表", "sheet", spreadsheetprotectionGoScopeSheetArguments{Scope: "sheet", Sheet: sheet, Enabled: false, Password: pwd}, func(ed document.SpreadsheetProtectionEditor) error {
				return ed.UnprotectSheet(sheet, pwd)
			})
			a.setStatus("已撤销工作表保护")
		},
			modalDialogActionSpec{ID: "confirm", Label: "撤销保护"},
			modalDialogActionSpec{ID: "cancel", Label: "取消"},
		)
	} else {
		a.modal.ShowInput("保护工作表", "保护工作表可防止其他用户更改工作表中的数据。\n可设置保护密码（可选，留空无密码）：", "", "密码（可选）", func(action string) {
			pwd := a.modal.Input.Text()
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			a.runSpreadsheetProtectionCommand("保护工作表", "sheet", spreadsheetprotectionGoScopeSheetArguments{Scope: "sheet", Sheet: sheet, Enabled: true, Password: pwd}, func(ed document.SpreadsheetProtectionEditor) error {
				return ed.ProtectSheet(sheet, pwd)
			})
			a.setStatus("已保护工作表")
		},
			modalDialogActionSpec{ID: "confirm", Label: "保护工作表"},
			modalDialogActionSpec{ID: "cancel", Label: "取消"},
		)
	}
}

func (a *App) runSpreadsheetWorkbookProtectionCommand(operation string, arguments any, edit func(document.SpreadsheetWorkbookProtectionEditor) error) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetWorkbookProtectionEditor)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	a.launchManagedBackground(func() {
		commandRouted := a.applicationSessionRegistered(token.documentID)
		changed := true
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.protection.set", "spreadsheet_protection", "workbook", sheet, arguments)
			err = commandErr
			if err == nil {
				var outcome struct {
					Changed bool `json:"changed"`
				}
				err = json.Unmarshal(result.Data, &outcome)
				changed = outcome.Changed
			}
		} else {
			unlock := lockSessionAdapterWrite(state)
			err = edit(editor)
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation}:
		case <-a.shutdown:
		}
	})
}

type spreadsheetprotectionGoScopeEnabledArguments struct {
	Scope    string `json:"scope"`
	Enabled  bool   `json:"enabled"`
	Password string `json:"password"`
}

func (a *App) showSpreadsheetProtectWorkbookDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetWorkbookProtectionEditor)
	if !ok {
		return
	}
	isProtected, _ := editor.GetWorkbookProtection()

	if isProtected {
		a.modal.ShowInput("撤销保护工作簿", "当前工作簿已受保护。\n若设置了密码请输入密码（无密码直接确定）：", "", "密码（可选）", func(action string) {
			pwd := a.modal.Input.Text()
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			a.runSpreadsheetWorkbookProtectionCommand("撤销保护工作簿", spreadsheetprotectionGoScopeEnabledArguments{Scope: "workbook", Enabled: false, Password: pwd}, func(ed document.SpreadsheetWorkbookProtectionEditor) error {
				return ed.UnprotectWorkbook(pwd)
			})
			a.setStatus("已撤销工作簿保护")
		},
			modalDialogActionSpec{ID: "confirm", Label: "撤销保护"},
			modalDialogActionSpec{ID: "cancel", Label: "取消"},
		)
	} else {
		a.modal.ShowInput("保护工作簿", "保护工作簿结构可防止其他用户查看隐藏的工作表、添加、移动、删除或重命名工作表。\n可设置保护密码（可选，留空无密码）：", "", "密码（可选）", func(action string) {
			pwd := a.modal.Input.Text()
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			a.runSpreadsheetWorkbookProtectionCommand("保护工作簿", spreadsheetprotectionGoScopeEnabledArguments{Scope: "workbook", Enabled: true, Password: pwd}, func(ed document.SpreadsheetWorkbookProtectionEditor) error {
				return ed.ProtectWorkbook(pwd)
			})
			a.setStatus("已保护工作簿")
		},
			modalDialogActionSpec{ID: "confirm", Label: "保护工作簿"},
			modalDialogActionSpec{ID: "cancel", Label: "取消"},
		)
	}
}
