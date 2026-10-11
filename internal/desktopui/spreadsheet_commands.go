package desktopui

import (
	"encoding/json"
	"fmt"

	"glance/internal/document"
)

type spreadsheetFormatResult struct {
	state       *documentSessionState
	token       sessionOperationToken
	revision    uint64
	generation  uint64
	sheet       string
	window      *gridViewport
	err         error
	history     bool
	applied     bool
	undo        bool
	operation   string
	structure   *document.StructureEdit
	sort        *document.SortRequest
	filter      *ValueFilter
	clearFilter bool
	details     string
}

func (a *App) applyHistory(undo bool) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	history, ok := state.book.(document.SpreadsheetHistory)
	if !ok {
		return
	}
	if state.valueFilter != nil {
		a.clearValueFilter(state)
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	a.launchManagedBackground(func() {
		commandRouted := a.applicationSessionRegistered(token.documentID)
		var applied bool
		var err error
		if commandRouted {
			action := "redo"
			if undo {
				action = "undo"
			}
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.history.apply", "spreadsheet_workbook", token.documentID, sheet, map[string]string{"action": action})
			err = commandErr
			if err == nil {
				var outcome struct {
					Changed bool `json:"changed"`
				}
				err = json.Unmarshal(result.Data, &outcome)
				applied = outcome.Changed
			}
		} else {
			unlock := lockSessionAdapterWrite(state)
			if undo {
				applied, err = history.Undo()
			} else {
				applied, err = history.Redo()
			}
			unlock()
		}
		a.completeSessionOperation(state, token, err, applied && !commandRouted)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, history: true, applied: applied, undo: undo}:
		case <-a.shutdown:
		}
	})
}

func (a *App) applyCellFormat(patch document.CellFormatPatch) {
	state := a.documentSessionState
	if state == nil || state.book == nil || !state.selectionSet || state.formatRunning.Load() {
		return
	}
	formatter, ok := state.book.(document.SpreadsheetFormatter)
	if !ok {
		return
	}
	if state.valueFilter != nil {
		a.clearValueFilter(state)
	}
	selection := state.selection.Bounds()
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
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.range.format.set", "spreadsheet_range", "selection", sheet, struct {
				Sheet  string                   `json:"sheet"`
				Bounds document.CellRange       `json:"bounds"`
				Patch  document.CellFormatPatch `json:"patch"`
			}{sheet, selection, patch})
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
			if changeEditor, ok := any(formatter).(document.SpreadsheetFormatChangeEditor); ok {
				changed, err = changeEditor.SetCellFormatsChanged(sheet, selection, patch)
			} else {
				err = formatter.SetCellFormats(sheet, selection, patch)
			}
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "设置格式", applied: changed}:
		case <-a.shutdown:
		}
	})
}

func (a *App) applyBatchEdits(edits []document.CellEdit, operation string) error {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return fmt.Errorf("表格正在处理其他操作")
	}
	editor, ok := state.book.(document.SpreadsheetBatchEditor)
	if !ok {
		return fmt.Errorf("当前表格不支持批量编辑")
	}
	if state.valueFilter != nil {
		a.clearValueFilter(state)
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return fmt.Errorf("表格正在处理其他操作")
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	a.launchManagedBackground(func() {
		commandRouted := a.applicationSessionRegistered(token.documentID)
		changed := len(edits) > 0
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.cells.set", "spreadsheet_cells", "batch", sheet, struct {
				Sheet string              `json:"sheet"`
				Edits []document.CellEdit `json:"edits"`
			}{sheet, edits})
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
			if changeEditor, ok := any(editor).(document.SpreadsheetBatchChangeEditor); ok {
				changed, err = changeEditor.SetCellsChanged(sheet, edits)
			} else {
				err = editor.SetCells(sheet, edits)
			}
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation, applied: changed}:
		case <-a.shutdown:
		}
	})
	return nil
}

func (a *App) applyStructure(edit document.StructureEdit) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetStructureEditor)
	if !ok {
		return
	}
	if state.valueFilter != nil {
		a.clearValueFilter(state)
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	a.launchManagedBackground(func() {
		commandRouted := a.applicationSessionRegistered(token.documentID)
		changed := true
		var err error
		if commandRouted {
			operation := spreadsheetStructureOperation(edit.Kind)
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.structure.edit", "spreadsheet_workbook", token.documentID, edit.Sheet, map[string]any{
				"operation": operation, "sheet": edit.Sheet, "new_name": edit.NewName, "index": edit.Index, "count": edit.Count,
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
			if edit.Kind == document.StructureRenameSheet && edit.NewName == edit.Sheet {
				changed = false
			} else {
				err = editor.EditStructure(edit)
			}
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "结构操作", structure: &edit, applied: changed}:
		case <-a.shutdown:
		}
	})
}

func spreadsheetStructureOperation(kind document.StructureEditKind) string {
	return map[document.StructureEditKind]string{
		document.StructureInsertRows: "insert_rows", document.StructureDeleteRows: "delete_rows",
		document.StructureInsertColumns: "insert_columns", document.StructureDeleteColumns: "delete_columns",
		document.StructureCreateSheet: "create_sheet", document.StructureRenameSheet: "rename_sheet",
		document.StructureDeleteSheet: "delete_sheet",
	}[kind]
}

func (a *App) applySort(request document.SortRequest) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	sorter, ok := state.book.(document.SpreadsheetSorter)
	if !ok {
		return
	}
	if state.valueFilter != nil {
		a.clearValueFilter(state)
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	a.launchManagedBackground(func() {
		commandRouted := a.applicationSessionRegistered(token.documentID)
		changed := true
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.range.sort", "spreadsheet_range", "selection", request.Sheet, request)
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
			if changeEditor, ok := any(sorter).(document.SpreadsheetSortChangeEditor); ok {
				changed, err = changeEditor.SortRangeChanged(request)
			} else {
				err = sorter.SortRange(request)
			}
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "排序", sort: &request, applied: changed}:
		case <-a.shutdown:
		}
	})
}
