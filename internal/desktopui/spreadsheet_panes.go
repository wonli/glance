package desktopui

import (
	"encoding/json"

	"gioui.org/layout"
	"glance/internal/document"
)

func (a *App) runSpreadsheetPanesCommand(operation string, arguments any, target document.SpreadsheetPaneFreeze, edit func(document.SpreadsheetPanesEditor) error) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetPanesEditor)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	if !a.launchManagedBackground(func() {
		commandRouted := a.applicationSessionRegistered(token.documentID)
		changed := true
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.panes.set", "spreadsheet_sheet", sheet, sheet, arguments)
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
			var current document.SpreadsheetPaneFreeze
			current, err = editor.GetSheetPanes(sheet)
			changed = current != target
			if err == nil && changed {
				err = edit(editor)
			}
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation, applied: changed, panes: &target}:
		case <-a.shutdown:
		}
	}) {
		state.finishOperation(token)
		a.setStatus(operation + "未能启动")
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	a.setStatus("正在" + operation + "…")
	if a.window != nil {
		a.window.Invalidate()
	}
}

type spreadsheetPanesOperationArguments struct {
	Sheet     string `json:"sheet"`
	Operation string `json:"operation"`
}

type spreadsheetpanesGoSheetOperationArguments struct {
	Sheet     string `json:"sheet"`
	Operation string `json:"operation"`
	Rows      int    `json:"rows"`
	Columns   int    `json:"columns"`
}

type spreadsheetpanesGoSheetOperationColumnsArguments struct {
	Sheet     string `json:"sheet"`
	Operation string `json:"operation"`
	Columns   int    `json:"columns"`
}

type spreadsheetpanesGoSheetOperationRowsArguments struct {
	Sheet     string `json:"sheet"`
	Operation string `json:"operation"`
	Rows      int    `json:"rows"`
}

func (a *App) applySpreadsheetPanesMenuCommand(gtx layout.Context, command string) {
	state := a.documentSessionState
	if state == nil || state.book == nil || !a.commitSpreadsheetEditorBeforeCommand(gtx, state) {
		return
	}
	sheet := state.sheet
	var target document.SpreadsheetPaneFreeze
	var arguments any
	operation := ""
	switch command {
	case "excel-freeze-selection":
		if !state.selectionSet {
			a.setStatus("请先选择要冻结的行列交界单元格")
			return
		}
		active := state.selection.Active
		target = document.SpreadsheetPaneFreeze{Frozen: true, XSplit: active.Column, YSplit: active.Row}
		if target.XSplit == 0 && target.YSplit == 0 {
			a.setStatus("请先选择首行或首列之后的单元格")
			return
		}
		operation = "冻结窗格"
		arguments = spreadsheetpanesGoSheetOperationArguments{Sheet: sheet, Operation: "freeze_at", Rows: active.Row, Columns: active.Column}
	case "excel-freeze-first-row":
		target = document.SpreadsheetPaneFreeze{Frozen: true, YSplit: 1}
		operation = "冻结首行"
		arguments = spreadsheetpanesGoSheetOperationRowsArguments{Sheet: sheet, Operation: "freeze_rows", Rows: 1}
	case "excel-freeze-first-column":
		target = document.SpreadsheetPaneFreeze{Frozen: true, XSplit: 1}
		operation = "冻结首列"
		arguments = spreadsheetpanesGoSheetOperationColumnsArguments{Sheet: sheet, Operation: "freeze_columns", Columns: 1}
	case "excel-unfreeze-panes":
		operation = "取消冻结窗格"
		arguments = spreadsheetPanesOperationArguments{Sheet: sheet, Operation: "unfreeze"}
	default:
		return
	}
	a.runSpreadsheetPanesCommand(operation, arguments, target, func(editor document.SpreadsheetPanesEditor) error {
		return editor.SetSheetPanes(sheet, target)
	})
}
