package desktopui

import (
	"encoding/json"

	"glance/internal/document"
)

type spreadsheetGridlinesSetArguments struct {
	Sheet string `json:"sheet"`
	Show  bool   `json:"show"`
}

func (a *App) runSpreadsheetGridlinesCommand(operation string, show bool) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetGridlinesEditor)
	if !ok {
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
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.gridlines.set", "spreadsheet_sheet", sheet, sheet, spreadsheetGridlinesSetArguments{Sheet: sheet, Show: show})
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
			var current bool
			current, err = editor.GetSheetGridlines(sheet)
			changed = current != show
			if err == nil && changed {
				err = editor.SetSheetGridlines(sheet, show)
			}
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation, applied: changed}:
		case <-a.shutdown:
		}
	})
}

func (a *App) runSpreadsheetViewCommand(operation, commandID string, arguments any, edit func(document.SpreadsheetViewsEditor) error) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetViewsEditor)
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
			result, commandErr := a.executeRibbonApplicationCommand(token, commandID, "spreadsheet_sheet", sheet, sheet, arguments)
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
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation, applied: changed}:
		case <-a.shutdown:
		}
	})
}

type spreadsheetViewModeSetArguments struct {
	Sheet string `json:"sheet"`
	Mode  string `json:"mode"`
}

func (a *App) setSpreadsheetViewMode(mode string) {
	state := a.documentSessionState
	if state == nil {
		return
	}
	label := "普通视图"
	if mode == "pageBreakPreview" {
		label = "分页预览"
	}
	state.spreadsheetViewMode = mode
	sheet := state.sheet
	a.runSpreadsheetViewCommand("切换"+label, "spreadsheet.view.mode.set", spreadsheetViewModeSetArguments{Sheet: sheet, Mode: mode}, func(editor document.SpreadsheetViewsEditor) error {
		return editor.SetSheetViewMode(sheet, mode)
	})
	a.setStatus("已切换为" + label)
	if a.window != nil {
		a.window.Invalidate()
	}
}

type spreadsheetViewRulerSetArguments struct {
	Sheet     string `json:"sheet"`
	ShowRuler bool   `json:"show_ruler"`
}

func (a *App) toggleSpreadsheetRuler() {
	state := a.documentSessionState
	if state == nil {
		return
	}
	state.spreadsheetRulerVisible = !state.spreadsheetRulerVisible
	show := state.spreadsheetRulerVisible
	sheet := state.sheet
	a.runSpreadsheetViewCommand("切换标尺", "spreadsheet.view.ruler.set", spreadsheetViewRulerSetArguments{Sheet: sheet, ShowRuler: show}, func(editor document.SpreadsheetViewsEditor) error {
		return editor.SetSheetRuler(sheet, show)
	})
	if show {
		a.setStatus("已显示标尺")
	} else {
		a.setStatus("已隐藏标尺")
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}
