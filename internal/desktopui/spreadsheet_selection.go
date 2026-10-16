package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/document"
)

func (a *App) clearSelectionAsync() { a.clearSpreadsheetSelectionContents("剪切") }

// Clearing large selections visits the adapter's stored cells, not every blank
// coordinate in a full row, column, or worksheet.
func (a *App) clearSpreadsheetSelectionContents(operation string) {
	state := a.documentSessionState
	if state == nil || state.book == nil || !state.selectionSet || state.formatRunning.Load() || a.isSaving() {
		return
	}
	bounds := state.selection.Bounds()
	editor, sparse := state.book.(document.SpreadsheetRangeContentEditor)
	if !sparse {
		count := selectionCellCount(bounds)
		if count > maxClipboardCellsUI {
			a.setStatus(fmt.Sprintf("%s选区超过 %d 个单元格", operation, maxClipboardCellsUI))
			return
		}
		edits := make([]document.CellEdit, 0, count)
		for row := bounds.StartRow; row <= bounds.EndRow; row++ {
			for column := bounds.StartColumn; column <= bounds.EndColumn; column++ {
				edits = append(edits, document.CellEdit{Row: row, Column: column})
			}
		}
		if err := a.applyBatchEdits(edits, operation); err != nil {
			a.setStatus(operation + "失败：" + err.Error())
		}
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
	if !a.launchManagedBackground(func() {
		routed := a.applicationSessionRegistered(token.documentID)
		var changed bool
		var err error
		if routed {
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.range.clear", "spreadsheet_range", "selection", sheet, spreadsheetRangeCopyArguments{Sheet: sheet, Bounds: bounds})
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
			changed, err = editor.ClearCellRange(sheet, bounds)
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !routed)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation, applied: changed}:
		case <-a.shutdown:
		}
	}) {
		state.finishOperation(token)
	}
}
