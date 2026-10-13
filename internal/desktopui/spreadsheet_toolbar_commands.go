package desktopui

import (
	"encoding/json"

	"glance/internal/document"
)

func (a *App) recalculateSpreadsheet() {
	a.runSpreadsheetRecalculate("重新计算公式")
}

type spreadsheetWorkbookRecalculateArguments struct {
}

func (a *App) runSpreadsheetRecalculate(operation string) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	recalculator, ok := state.book.(document.SpreadsheetRecalculateEditor)
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
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.workbook.recalculate", "spreadsheet_workbook", token.documentID, sheet, spreadsheetWorkbookRecalculateArguments{})
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
			err = recalculator.RecalculateFormulas()
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation, applied: changed}:
		case <-a.shutdown:
		}
	})
}

func (a *App) refreshAllSpreadsheet() {
	a.runSpreadsheetRecalculate("刷新全部工作表")
}
