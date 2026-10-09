package desktopui

import (
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
	go func() {
		unlock := lockSessionAdapterWrite(state)
		var applied bool
		var err error
		if undo {
			applied, err = history.Undo()
		} else {
			applied, err = history.Redo()
		}
		unlock()
		a.completeSessionOperation(state, token, err, applied)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, history: true, applied: applied, undo: undo}:
		case <-a.shutdown:
		}
	}()
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
	go func() {
		unlock := lockSessionAdapterWrite(state)
		err := formatter.SetCellFormats(sheet, selection, patch)
		unlock()
		a.completeSessionOperation(state, token, err, true)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err}:
		case <-a.shutdown:
		}
	}()
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
	go func() {
		unlock := lockSessionAdapterWrite(state)
		err := editor.SetCells(sheet, edits)
		unlock()
		a.completeSessionOperation(state, token, err, true)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation}:
		case <-a.shutdown:
		}
	}()
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
	go func() {
		unlock := lockSessionAdapterWrite(state)
		err := editor.EditStructure(edit)
		unlock()
		a.completeSessionOperation(state, token, err, true)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "结构操作", structure: &edit}:
		case <-a.shutdown:
		}
	}()
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
	go func() {
		unlock := lockSessionAdapterWrite(state)
		err := sorter.SortRange(request)
		unlock()
		a.completeSessionOperation(state, token, err, true)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: "排序", sort: &request}:
		case <-a.shutdown:
		}
	}()
}
