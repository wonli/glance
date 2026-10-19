package desktopui

import (
	"encoding/json"
	"fmt"

	"glance/internal/document"
	"glance/internal/i18n"
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
	noOp        bool
	undo        bool
	operation   string
	structure   *document.StructureEdit
	sort        *document.SortRequest
	filter      *ValueFilter
	clearFilter bool
	details     i18n.Message
	detail      string
	rangeEntry  *string
	replaceAll  *spreadsheetReplaceAllPlan
	dimensions  []spreadsheetDimensionChange
	panes       *document.SpreadsheetPaneFreeze
	merge       *document.SpreadsheetMergeResult
	mergeBounds *document.CellRange
}

type spreadsheetDimensionChange struct {
	index   int
	value   float64
	row     bool
	autoFit bool
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
	if state == nil || !state.selectionSet {
		return
	}
	a.applyCellFormatTo(state, state.sheet, state.selection.Bounds(), patch)
}

func (a *App) applyCellFormatTo(state *documentSessionState, sheet string, selection document.CellRange, patch document.CellFormatPatch) {
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	formatter, ok := state.book.(document.SpreadsheetFormatter)
	if !ok {
		return
	}
	if state.valueFilter != nil {
		a.clearValueFilter(state)
	}
	generation, window := a.documentGeneration, state.virtualGrid
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
	return a.applyBatchEditsWithDetails(edits, operation, i18n.Message{})
}

func (a *App) applyBatchEditsWithDetails(edits []document.CellEdit, operation string, details i18n.Message) error {
	return a.applyBatchEditsWithDetailsForSheet(edits, operation, details, "")
}

func (a *App) applyBatchEditsForSheet(edits []document.CellEdit, operation, sheet string) error {
	return a.applyBatchEditsWithDetailsForSheet(edits, operation, i18n.Message{}, sheet)
}

func (a *App) applyBatchEditsWithDetailsForSheet(edits []document.CellEdit, operation string, details i18n.Message, sheet string) error {
	return a.applyBatchEditsWithDetailsForSheetAndRangeEntry(edits, operation, details, sheet, nil)
}

func (a *App) applyBatchEditsForRangeEntry(edits []document.CellEdit, sheet, value string) error {
	return a.applyBatchEditsWithDetailsForSheetAndRangeEntry(edits, "填充选区", i18n.Message{}, sheet, &value)
}

func (a *App) applyBatchEditsWithDetailsForSheetAndRangeEntry(edits []document.CellEdit, operation string, details i18n.Message, sheet string, rangeEntry *string) error {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return fmt.Errorf("表格正在处理其他操作")
	}
	if sheet == "" {
		sheet = state.sheet
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
	generation, window := a.documentGeneration, state.virtualGrid
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
		noOp, resultDetails := rangeEntry != nil && err == nil && !changed, details
		if noOp {
			resultDetails = i18n.Message{ID: "spreadsheet.status.no_changes"}
		}
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation, applied: changed, noOp: noOp, details: resultDetails, rangeEntry: rangeEntry}:
		case <-a.shutdown:
		}
	})
	return nil
}

type spreadsheetCellTransferArguments struct {
	Sheet        string             `json:"sheet"`
	Source       document.CellRange `json:"source"`
	TargetRow    int                `json:"target_row"`
	TargetColumn int                `json:"target_column"`
	Cells        [][]document.Cell  `json:"cells"`
}

func (a *App) applyCellRangeTransferForSheet(source document.CellRange, targetRow, targetColumn int, cells [][]document.Cell, sheet string) error {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return fmt.Errorf("表格正在处理其他操作")
	}
	if sheet == "" {
		sheet = state.sheet
	}
	editor, ok := state.book.(document.SpreadsheetCellTransferEditor)
	if !ok {
		return fmt.Errorf("当前表格不支持保留公式和格式的粘贴")
	}
	if state.valueFilter != nil {
		a.clearValueFilter(state)
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return fmt.Errorf("表格正在处理其他操作")
	}
	generation, window := a.documentGeneration, state.virtualGrid
	arguments := spreadsheetCellTransferArguments{
		Sheet: sheet, Source: source, TargetRow: targetRow, TargetColumn: targetColumn,
		Cells: cloneSpreadsheetClipboardCells(cells),
	}
	a.launchManagedBackground(func() {
		commandRouted := a.applicationSessionRegistered(token.documentID)
		changed := true
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.cells.transfer", "spreadsheet_cells", "batch", sheet, arguments)
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
			changed, err = editor.TransferCellRangeChanged(sheet, source, targetRow, targetColumn, arguments.Cells)
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "粘贴", applied: changed}:
		case <-a.shutdown:
		}
	})
	return nil
}

type spreadsheetStructureEditArguments struct {
	Operation string `json:"operation"`
	Sheet     string `json:"sheet"`
	NewName   string `json:"new_name"`
	Index     int    `json:"index"`
	Count     int    `json:"count"`
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
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.structure.edit", "spreadsheet_workbook", token.documentID, edit.Sheet, spreadsheetStructureEditArguments{Operation: operation, Sheet: edit.Sheet, NewName: edit.NewName, Index: edit.Index, Count: edit.Count})
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
