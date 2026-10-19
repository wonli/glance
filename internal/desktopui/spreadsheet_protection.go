package desktopui

import (
	"encoding/json"
	"glance/internal/i18n"

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
		a.modal.ShowInputMessage("spreadsheet.protection.sheet", i18n.Message{ID: "spreadsheet.protection.dialog.unprotect_sheet.title"}, i18n.Message{ID: "spreadsheet.protection.dialog.unprotect_sheet.body"}, "", i18n.Message{ID: "spreadsheet.protection.dialog.password_hint"}, func(action string) {
			pwd := a.modal.Input.Text()
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			a.runSpreadsheetProtectionCommand("撤销保护工作表", "sheet", spreadsheetprotectionGoScopeSheetArguments{Scope: "sheet", Sheet: sheet, Enabled: false, Password: pwd}, func(ed document.SpreadsheetProtectionEditor) error {
				return ed.UnprotectSheet(sheet, pwd)
			})
			a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.worksheet_protection_removed_a314d5"}, feedbackSuccess, notificationToast)
		},
			modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "spreadsheet.protection.action.unprotect"}},
			modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
		)
	} else {
		a.modal.ShowInputMessage("spreadsheet.protection.sheet", i18n.Message{ID: "spreadsheet.protection.dialog.protect_sheet.title"}, i18n.Message{ID: "spreadsheet.protection.dialog.protect_sheet.body"}, "", i18n.Message{ID: "spreadsheet.protection.dialog.password_hint"}, func(action string) {
			pwd := a.modal.Input.Text()
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			a.runSpreadsheetProtectionCommand("保护工作表", "sheet", spreadsheetprotectionGoScopeSheetArguments{Scope: "sheet", Sheet: sheet, Enabled: true, Password: pwd}, func(ed document.SpreadsheetProtectionEditor) error {
				return ed.ProtectSheet(sheet, pwd)
			})
			a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.worksheet_protection_enabled_a6ecf1"}, feedbackSuccess, notificationToast)
		},
			modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "spreadsheet.protection.action.protect_sheet"}},
			modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
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
		a.modal.ShowInputMessage("spreadsheet.protection.workbook", i18n.Message{ID: "spreadsheet.protection.dialog.unprotect_workbook.title"}, i18n.Message{ID: "spreadsheet.protection.dialog.unprotect_workbook.body"}, "", i18n.Message{ID: "spreadsheet.protection.dialog.password_hint"}, func(action string) {
			pwd := a.modal.Input.Text()
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			a.runSpreadsheetWorkbookProtectionCommand("撤销保护工作簿", spreadsheetprotectionGoScopeEnabledArguments{Scope: "workbook", Enabled: false, Password: pwd}, func(ed document.SpreadsheetWorkbookProtectionEditor) error {
				return ed.UnprotectWorkbook(pwd)
			})
			a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.workbook_protection_removed_aabf48"}, feedbackSuccess, notificationToast)
		},
			modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "spreadsheet.protection.action.unprotect"}},
			modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
		)
	} else {
		a.modal.ShowInputMessage("spreadsheet.protection.workbook", i18n.Message{ID: "spreadsheet.protection.dialog.protect_workbook.title"}, i18n.Message{ID: "spreadsheet.protection.dialog.protect_workbook.body"}, "", i18n.Message{ID: "spreadsheet.protection.dialog.password_hint"}, func(action string) {
			pwd := a.modal.Input.Text()
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			a.runSpreadsheetWorkbookProtectionCommand("保护工作簿", spreadsheetprotectionGoScopeEnabledArguments{Scope: "workbook", Enabled: true, Password: pwd}, func(ed document.SpreadsheetWorkbookProtectionEditor) error {
				return ed.ProtectWorkbook(pwd)
			})
			a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.workbook_protection_enabled_49da7c"}, feedbackSuccess, notificationToast)
		},
			modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "spreadsheet.protection.action.protect_workbook"}},
			modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
		)
	}
}
