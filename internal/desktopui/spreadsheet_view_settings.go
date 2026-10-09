package desktopui

import "glance/internal/document"

func (a *App) runSpreadsheetViewCommand(operation string, edit func(document.SpreadsheetViewsEditor) error) {
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
	a.runSpreadsheetViewCommand("切换"+label, func(editor document.SpreadsheetViewsEditor) error {
		return editor.SetSheetViewMode(sheet, mode)
	})
	a.setStatus("已切换为" + label)
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) toggleSpreadsheetRuler() {
	state := a.documentSessionState
	if state == nil {
		return
	}
	state.spreadsheetRulerVisible = !state.spreadsheetRulerVisible
	show := state.spreadsheetRulerVisible
	sheet := state.sheet
	a.runSpreadsheetViewCommand("切换标尺", func(editor document.SpreadsheetViewsEditor) error {
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
