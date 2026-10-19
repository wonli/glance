package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/document"
	"glance/internal/i18n"
	"strings"
)

func (a *App) runSpreadsheetFormulaCommand(operation, sheet, cell, formula string) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetFormulaEditor)
	if !ok {
		return
	}
	generation, activeSheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	a.launchManagedBackground(func() {
		commandRouted := a.applicationSessionRegistered(token.documentID)
		changed := true
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.formula.set", "spreadsheet_cell", cell, sheet, map[string]string{
				"sheet": sheet, "cell": cell, "formula": formula,
			})
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
			err = editor.SetCellFormula(sheet, cell, formula)
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		if err == nil {
			a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.formula_set_729002"}, feedbackSuccess, notificationToast)
		}
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: activeSheet, window: window, err: err, operation: operation, applied: changed}:
		case <-a.shutdown:
		}
	})
}

func (a *App) showSpreadsheetInsertFunctionDialog(category string) {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	_, ok := state.book.(document.SpreadsheetFormulaEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.this_workbook_does_not_support_5e586e"}, feedbackWarning, notificationToast)
		return
	}
	activeCol := state.selection.Active.Column + 1
	activeRow := state.selection.Active.Row + 1
	cellName := fmt.Sprintf("%s%d", columnName(activeCol), activeRow)

	defaultRef := "A1:A10"
	if state.selectionSet {
		b := state.selection.Bounds()
		if b.StartColumn == b.EndColumn && b.StartRow == b.EndRow {
			if activeRow > 1 {
				defaultRef = fmt.Sprintf("%s1:%s%d", columnName(activeCol), columnName(activeCol), activeRow-1)
			}
		} else {
			defaultRef = fmt.Sprintf("%s%d:%s%d", columnName(b.StartColumn), b.StartRow, columnName(b.EndColumn), b.EndRow)
		}
	}

	title := i18n.Message{ID: "spreadsheet.formula.dialog.insert.title"}
	defaultFormula := fmt.Sprintf("=SUM(%s)", defaultRef)

	switch category {
	case "recent":
		title = i18n.Message{ID: "spreadsheet.formula.dialog.recent.title"}
		defaultFormula = fmt.Sprintf("=AVERAGE(%s)", defaultRef)
	case "logical":
		title = i18n.Message{ID: "spreadsheet.formula.dialog.logical.title"}
		defaultFormula = fmt.Sprintf("=IF(%s1>0, 1, 0)", columnName(activeCol))
	case "text":
		title = i18n.Message{ID: "spreadsheet.formula.dialog.text.title"}
		defaultFormula = fmt.Sprintf("=CONCATENATE(%s1, \" \", %s2)", columnName(activeCol), columnName(activeCol))
	case "datetime":
		title = i18n.Message{ID: "spreadsheet.formula.dialog.datetime.title"}
		defaultFormula = "=TODAY()"
	}

	a.modal.ShowInputMessage("spreadsheet.formula.insert", title, i18n.Message{ID: "spreadsheet.formula.dialog.body", Args: i18n.Args{"Cell": cellName}}, defaultFormula, i18n.Message{ID: "spreadsheet.formula.dialog.hint"}, func(action string) {
		formula := strings.TrimSpace(a.modal.Input.Text())
		a.modal.Hide()
		if action != "confirm" || formula == "" {
			return
		}
		a.runSpreadsheetFormulaCommand("插入公式", state.sheet, cellName, formula)
	}, modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "common.action.confirm"}}, modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}})
}

func (a *App) runSpreadsheetDefinedNameCommand(name document.SpreadsheetDefinedName) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetDefinedNameEditor)
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
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.defined_names.set", "spreadsheet_workbook", token.documentID, sheet, name)
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
			err = editor.SetDefinedName(name)
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		if err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.error.operation_failed"}, err, notificationToast)
		} else {
			a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.defined_name.created", Args: i18n.Args{"Name": name.Name, "Reference": name.Ref}}, feedbackSuccess, notificationToast)
		}
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "名称管理器", applied: changed}:
		case <-a.shutdown:
		}
	})
}

func (a *App) showSpreadsheetNameManagerDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	ed, ok := state.book.(document.SpreadsheetDefinedNameEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.this_workbook_does_not_support_2d3945"}, feedbackWarning, notificationToast)
		return
	}
	existing, _ := ed.GetDefinedNames()
	b := state.selection.Bounds()
	defaultRef := fmt.Sprintf("'%s'!$%s$%d:$%s$%d", state.sheet, columnName(b.StartColumn), b.StartRow, columnName(b.EndColumn), b.EndRow)
	defaultName := a.text(i18n.Message{ID: "spreadsheet.defined_name.dialog.default_name", Args: i18n.Args{"Index": len(existing) + 1}})
	a.modal.ShowInputMessage("spreadsheet.defined_name", i18n.Message{ID: "spreadsheet.defined_name.dialog.title"}, i18n.Message{ID: "spreadsheet.defined_name.dialog.body", Args: i18n.Args{"Count": len(existing)}}, defaultName, i18n.Message{ID: "spreadsheet.defined_name.dialog.hint"}, func(action string) {
		name := strings.TrimSpace(a.modal.Input.Text())
		a.modal.Hide()
		if action != "confirm" || name == "" {
			return
		}
		a.runSpreadsheetDefinedNameCommand(document.SpreadsheetDefinedName{
			Name: name,
			Ref:  defaultRef,
		})
	}, modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "spreadsheet.defined_name.dialog.create"}}, modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}})
}

type spreadsheetFlashFillApplyArguments struct {
	Sheet  string             `json:"sheet"`
	Bounds document.CellRange `json:"bounds"`
}

func (a *App) applySpreadsheetFlashFill() {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetFlashFillEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.this_workbook_does_not_support_e87852"}, feedbackWarning, notificationToast)
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	bounds := state.selection.Bounds()
	a.launchManagedBackground(func() {
		commandRouted := a.applicationSessionRegistered(token.documentID)
		var res document.SpreadsheetFlashFillResult
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.flash_fill.apply", "spreadsheet_cell_range", "selection", sheet, spreadsheetFlashFillApplyArguments{Sheet: sheet, Bounds: bounds})
			err = commandErr
			if err == nil {
				err = json.Unmarshal(result.Data, &res)
			}
		} else {
			unlock := lockSessionAdapterWrite(state)
			res, err = editor.FlashFill(sheet, bounds)
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && res.RowsFilled > 0 && !commandRouted)
		if err == nil {
			a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.flash_fill.completed", Args: i18n.Args{"Count": res.RowsFilled}}, feedbackSuccess, notificationToast)
		}
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "快速填充", applied: res.RowsFilled > 0}:
		case <-a.shutdown:
		}
	})
}
