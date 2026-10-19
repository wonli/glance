package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/document"
	"glance/internal/format/xlsx"
	"glance/internal/i18n"
	"strings"
)

type spreadsheetRangeSearchArguments struct {
	Sheet      string `json:"sheet"`
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
}

const spreadsheetReplaceAllMaxMatches = 10000

type spreadsheetReplaceAllPlan struct {
	edits          []document.CellEdit
	foundCells     int
	changedCells   int
	skippedMatches int
	truncated      bool
}

func (a *App) showSpreadsheetFindDialog() {
	a.showSpreadsheetSearchReplaceDialog(false)
}

func (a *App) showSpreadsheetReplaceDialog() {
	a.showSpreadsheetSearchReplaceDialog(true)
}

func (a *App) showSpreadsheetSearchReplaceDialog(replaceTab bool) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	searcher, ok := state.book.(document.SpreadsheetSearcher)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.this_spreadsheet_does_not_support_3ad634"}, feedbackWarning, notificationToast)
		return
	}
	_, hasGrid := state.book.(document.GridReader)
	_, hasBatch := state.book.(document.SpreadsheetBatchEditor)
	_, hasValueKind := state.book.(document.SpreadsheetCellValueKindReader)
	canReplace := hasGrid && hasBatch && hasValueKind
	if replaceTab && !canReplace {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.this_spreadsheet_does_not_support_8ba12d"}, feedbackWarning, notificationToast)
		return
	}
	query := spreadsheetFindInitialQuery(state)
	actions := []modalDialogActionSpec{{ID: "find-next", LabelRef: i18n.Message{ID: "office.search_replace.action.find_next"}, Style: modalDialogActionPrimary}}
	if canReplace {
		actions = []modalDialogActionSpec{
			{ID: "find-next", LabelRef: i18n.Message{ID: "office.search_replace.action.find_next"}, Style: modalDialogActionSecondary, WidthDp: 96, TextSizeSp: 11},
			{ID: "replace", LabelRef: i18n.Message{ID: "office.search_replace.action.replace"}, Style: modalDialogActionPrimary, WidthDp: 76, TextSizeSp: 12},
			{ID: "replace-all", LabelRef: i18n.Message{ID: "office.search_replace.action.replace_all"}, Style: modalDialogActionSecondary, WidthDp: 92, TextSizeSp: 11},
		}
	}
	onAction := func(action string) {
		if action == "close" {
			a.hideSpreadsheetSearchReplaceDialog(state)
			return
		}
		query := strings.TrimSpace(a.modal.Input.Text())
		replacement := a.modal.SecondInput.Text()
		if query == "" {
			a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.enter_text_to_find_5340a9"}, feedbackInfo, notificationSuppressed)
			a.modal.SetSearchReplaceSummaryRef(i18n.Message{ID: "spreadsheet.search.status.enter_query"})
			a.modal.FocusInputRequested = true
			return
		}
		state.excelFindQuery = query
		if canReplace {
			state.excelReplaceText = replacement
		}
		if action == "replace-all" {
			a.replaceAllSpreadsheetText(searcher, query, replacement)
			a.modal.FocusInputRequested = true
			return
		}
		if action == "find-previous" {
			a.findPreviousSpreadsheetText(searcher, query)
			a.modal.FocusInputRequested = true
			return
		}
		if action != "replace" {
			a.findNextSpreadsheetText(searcher, query)
			a.modal.FocusInputRequested = true
			return
		}
		a.replaceSelectedSpreadsheetText(searcher, query, replacement)
		a.modal.FocusInputRequested = true
	}
	if canReplace {
		a.modal.ShowDualInputMessage(dialogIDOfficeSearchReplace, i18n.Message{ID: "office.search_replace.title"}, i18n.Message{}, i18n.Message{ID: "office.search_replace.field.find"}, i18n.Message{ID: "office.search_replace.field.replace"}, query, state.excelReplaceText, onAction, actions...)
	} else {
		a.modal.ShowInputMessage(dialogIDOfficeSearchReplace, i18n.Message{ID: "office.search_replace.title"}, i18n.Message{}, query, i18n.Message{ID: "office.search_replace.field.find"}, onAction, actions...)
	}
	a.modal.Input.SetCaret(0, len([]rune(query)))
	a.modal.SearchReplace = true
	a.modal.SearchReplaceHasReplace = canReplace
	a.modal.SearchReplaceReplaceTab = replaceTab
	a.modal.SearchReplaceScope = "当前工作表"
	a.modal.SetSearchReplaceSummaryRef(i18n.Message{ID: "spreadsheet.search.status.default"})
	a.modal.CardWidthDp = 560
	a.modal.FocusInputRequested = true
}

func (a *App) hideSpreadsheetSearchReplaceDialog(state *documentSessionState) {
	a.modal.Hide()
	if state != nil && state.virtualGrid != nil {
		state.virtualGrid.KeyboardFocusRequested = true
	}
}

func spreadsheetFindInitialQuery(state *documentSessionState) string {
	if state == nil {
		return ""
	}
	if state.selectionSet {
		row, column := state.selection.Active.Row+1, state.selection.Active.Column+1
		address := xlsx.CellName(column, row)
		if state.cellEdit.Editing && state.cellEdit.Address == address {
			if value := state.cellEdit.Draft; value != "" && !strings.ContainsAny(value, "\r\n") {
				return value
			}
			return state.excelFindQuery
		}
		if viewport := state.virtualGrid; viewport != nil && viewport.WindowSheet == state.sheet {
			if viewport.cellLoaded(row, column) {
				if value := viewport.cell(row, column).Value; value != "" && !strings.ContainsAny(value, "\r\n") {
					return value
				}
			}
		}
		if state.cellEdit.Address == address && state.cellEdit.Original != "" && !strings.HasPrefix(state.cellEdit.Original, "=") {
			if value := state.cellEdit.Original; !strings.ContainsAny(value, "\r\n") {
				return value
			}
		}
	}
	return state.excelFindQuery
}

func (a *App) findNextSpreadsheetText(searcher document.SpreadsheetSearcher, query string) {
	a.findSpreadsheetText(searcher, query, false)
}

func (a *App) findPreviousSpreadsheetText(searcher document.SpreadsheetSearcher, query string) {
	a.findSpreadsheetText(searcher, query, true)
}

func (a *App) findSpreadsheetText(searcher document.SpreadsheetSearcher, query string, previous bool) {
	state := a.documentSessionState
	if state == nil || searcher == nil || query == "" {
		return
	}
	var matches []document.CellMatch
	var truncated bool
	var err error
	if a.applicationSessionRegistered(state.documentID) {
		token, ok := state.beginOperation(sessionOperationRead)
		if !ok {
			a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.a_table_operation_is_in_a186a1"}, feedbackInfo, notificationSuppressed)
			a.modal.SetSearchReplaceSummaryRef(i18n.Message{ID: "spreadsheet.search.status.busy"})
			return
		}
		result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.range.search", "spreadsheet_sheet", state.sheet, state.sheet, spreadsheetRangeSearchArguments{Sheet: state.sheet, Query: query, MaxResults: 1000})
		err = commandErr
		a.completeSessionOperation(state, token, err, false)
		if err == nil {
			var outcome struct {
				Matches   []document.CellMatch `json:"matches"`
				Truncated bool                 `json:"truncated"`
			}
			err = json.Unmarshal(result.Data, &outcome)
			matches, truncated = outcome.Matches, outcome.Truncated
		}
	} else {
		matches, err = searcher.SearchSheet(state.sheet, query)
	}
	if err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.error.operation_failed"}, err, notificationToast)
		a.modal.SetSearchReplaceSummaryRef(i18n.Message{ID: "spreadsheet.search.status.failed"})
		return
	}
	if len(matches) == 0 {
		a.setStatusMessage(i18n.Message{ID: "common.error.not_found", Args: i18n.Args{"Query": query}}, feedbackInfo, notificationSuppressed)
		a.modal.SetSearchReplaceSummaryRef(i18n.Message{ID: "spreadsheet.search.status.not_found"})
		return
	}
	currentPos := state.selection.Active
	curRow, curCol := currentPos.Row+1, currentPos.Column+1
	target := matches[0]
	if previous {
		target = matches[len(matches)-1]
		for index := len(matches) - 1; index >= 0; index-- {
			match := matches[index]
			if match.Row < curRow || (match.Row == curRow && match.Column < curCol) {
				target = match
				break
			}
		}
	} else {
		for _, match := range matches {
			if match.Row > curRow || (match.Row == curRow && match.Column > curCol) {
				target = match
				break
			}
		}
	}
	a.navigateToCell(target.Row, target.Column)
	cell := xlsx.CellName(target.Column, target.Row)
	if truncated {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.search.status.located_truncated", Args: i18n.Args{"Cell": cell, "Count": len(matches)}}, feedbackInfo, notificationSuppressed)
	} else {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.search.status.located", Args: i18n.Args{"Cell": cell, "Count": len(matches)}}, feedbackInfo, notificationSuppressed)
	}
	a.modal.SetSearchReplaceSummaryRef(i18n.Message{ID: "spreadsheet.search.status.found", Args: i18n.Args{"Count": len(matches), "Cell": cell}})
}

func (a *App) replaceSelectedSpreadsheetText(searcher document.SpreadsheetSearcher, query, replacement string) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	if !state.selectionSet {
		a.findNextSpreadsheetText(searcher, query)
		return
	}
	row, column := state.selection.Active.Row+1, state.selection.Active.Column+1
	if row < 1 || column < 1 {
		return
	}
	grid, ok := state.book.(document.GridReader)
	if !ok {
		return
	}
	window, err := grid.Window(state.sheet, row, column, 1, 1)
	if err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.error.operation_failed"}, err, notificationToast)
		return
	}
	if len(window.Cells) == 0 || len(window.Cells[0]) == 0 {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.could_not_read_the_current_1c5ebd"}, feedbackWarning, notificationToast)
		return
	}
	cell := window.Cells[0][0]
	if cell.Formula != "" {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.formula_cells_cannot_be_replaced_f18a31"}, feedbackWarning, notificationToast)
		a.modal.SetSearchReplaceSummaryRef(i18n.Message{ID: "spreadsheet.replace.error.formula_cell"})
		return
	}
	typeReader, ok := state.book.(document.SpreadsheetCellValueKindReader)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.this_spreadsheet_does_not_support_8ba12d"}, feedbackWarning, notificationToast)
		return
	}
	valueKind, err := typeReader.StoredCellValueKind(state.sheet, xlsx.CellName(column, row))
	if err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.error.operation_failed"}, err, notificationToast)
		return
	}
	if valueKind != document.CellValueText {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.only_text_cells_can_be_75c0e7"}, feedbackInfo, notificationToast)
		a.modal.SetSearchReplaceSummaryRef(i18n.Message{ID: "spreadsheet.replace.error.text_only"})
		return
	}
	runes := []rune(cell.Value)
	needle := lowerWordRunes([]rune(query))
	start := findWordRunes(lowerWordRunes(runes), needle, 0)
	if start < 0 {
		a.findNextSpreadsheetText(searcher, query)
		return
	}
	updated := string(runes[:start]) + replacement + string(runes[start+len(needle):])
	if updated == cell.Value {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.the_replacement_matches_the_current_cf3cce"}, feedbackInfo, notificationToast)
		a.modal.SetSearchReplaceSummaryRef(i18n.Message{ID: "spreadsheet.replace.status.unchanged"})
		return
	}
	if strings.HasPrefix(strings.TrimSpace(updated), "=") {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.a_replacement_value_cannot_begin_836003"}, feedbackWarning, notificationToast)
		a.modal.SetSearchReplaceSummaryRef(i18n.Message{ID: "spreadsheet.replace.error.formula_prefix"})
		return
	}
	typedValue := document.CellValue{Kind: document.CellValueText, Text: updated}
	if err := a.applyBatchEdits([]document.CellEdit{{Row: row, Column: column, Value: updated, TypedValue: &typedValue}}, "替换单元格"); err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.error.operation_failed"}, err, notificationToast)
		a.modal.SetSearchReplaceSummaryRef(i18n.Message{ID: "spreadsheet.replace.error.failed"})
		return
	}
	a.modal.SetSearchReplaceSummaryRef(i18n.Message{ID: "spreadsheet.replace.status.replacing_current"})
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) replaceAllSpreadsheetText(searcher document.SpreadsheetSearcher, query, replacement string) {
	state := a.documentSessionState
	if state == nil || state.book == nil || searcher == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	grid, hasGrid := state.book.(document.GridReader)
	typeReader, hasValueKind := state.book.(document.SpreadsheetCellValueKindReader)
	if !hasGrid || !hasValueKind {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.this_workbook_does_not_support_a175ea"}, feedbackWarning, notificationToast)
		a.modal.SetSearchReplaceSummaryRef(i18n.Message{ID: "spreadsheet.replace.error.unsupported"})
		return
	}
	if a.formatResults == nil {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.the_replacement_service_is_unavailable_e19527"}, feedbackWarning, notificationToast)
		a.modal.SetSearchReplaceSummaryRef(i18n.Message{ID: "spreadsheet.replace.error.unavailable"})
		return
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.a_table_operation_is_in_a186a1"}, feedbackInfo, notificationSuppressed)
		a.modal.SetSearchReplaceSummaryRef(i18n.Message{ID: "spreadsheet.search.status.busy"})
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	results, shutdown, appWindow := a.formatResults, a.shutdown, a.window
	a.modal.SetSearchReplaceSummaryRef(i18n.Message{ID: "spreadsheet.replace.status.scanning"})
	if !a.launchManagedBackground(func() {
		plan := &spreadsheetReplaceAllPlan{}
		matches, truncated, err := a.findSpreadsheetMatchesForReplace(token, searcher, sheet, query)
		plan.truncated = truncated
		plan.foundCells = len(matches)
		if err == nil && !truncated {
			needle := lowerWordRunes([]rune(query))
			for _, match := range matches {
				unlock := lockSessionAdapterRead(state)
				cellWindow, readErr := grid.Window(sheet, match.Row, match.Column, 1, 1)
				if readErr == nil && len(cellWindow.Cells) > 0 && len(cellWindow.Cells[0]) > 0 {
					cell := cellWindow.Cells[0][0]
					if cell.Formula != "" {
						plan.skippedMatches++
						unlock()
						continue
					}
					kind, kindErr := typeReader.StoredCellValueKind(sheet, xlsx.CellName(match.Column, match.Row))
					unlock()
					if kindErr != nil {
						err = kindErr
						break
					}
					if kind != document.CellValueText {
						plan.skippedMatches++
						continue
					}
					updated, occurrences := replaceSpreadsheetTextOccurrences(cell.Value, needle, replacement)
					if occurrences == 0 || updated == cell.Value {
						continue
					}
					if strings.HasPrefix(strings.TrimSpace(updated), "=") {
						plan.skippedMatches++
						continue
					}
					typedValue := document.CellValue{Kind: document.CellValueText, Text: updated}
					plan.edits = append(plan.edits, document.CellEdit{
						Row: match.Row, Column: match.Column, Value: updated, TypedValue: &typedValue,
					})
					plan.changedCells++
					continue
				}
				unlock()
				if readErr != nil {
					err = readErr
					break
				}
				err = fmt.Errorf("无法读取 %s", xlsx.CellName(match.Column, match.Row))
				break
			}
		}
		a.completeSessionOperation(state, token, err, false)
		result := spreadsheetFormatResult{
			state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation,
			sheet: sheet, window: window, err: err, replaceAll: plan,
		}
		select {
		case results <- result:
			if appWindow != nil {
				appWindow.Invalidate()
			}
		case <-shutdown:
		}
	}) {
		state.finishOperation(token)
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.could_not_start_the_replacement_18d9a7"}, feedbackWarning, notificationToast)
		a.modal.SetSearchReplaceSummaryRef(i18n.Message{ID: "spreadsheet.replace.error.start_failed"})
	}
}

func (a *App) findSpreadsheetMatchesForReplace(token sessionOperationToken, searcher document.SpreadsheetSearcher, sheet, query string) ([]document.CellMatch, bool, error) {
	if a.applicationSessionRegistered(token.documentID) {
		result, err := a.executeRibbonApplicationCommand(token, "spreadsheet.range.search", "spreadsheet_sheet", sheet, sheet, spreadsheetRangeSearchArguments{
			Sheet: sheet, Query: query, MaxResults: spreadsheetReplaceAllMaxMatches,
		})
		if err != nil {
			return nil, false, err
		}
		var outcome struct {
			Matches   []document.CellMatch `json:"matches"`
			Truncated bool                 `json:"truncated"`
		}
		if err := json.Unmarshal(result.Data, &outcome); err != nil {
			return nil, false, err
		}
		return outcome.Matches, outcome.Truncated, nil
	}
	matches, err := searcher.SearchSheet(sheet, query)
	if err != nil {
		return nil, false, err
	}
	if len(matches) > spreadsheetReplaceAllMaxMatches {
		return nil, true, nil
	}
	return matches, false, nil
}

func replaceSpreadsheetTextOccurrences(value string, needle []rune, replacement string) (string, int) {
	if len(needle) == 0 {
		return value, 0
	}
	runes := []rune(value)
	lowered := lowerWordRunes(runes)
	var result strings.Builder
	result.Grow(len(value))
	replacementCount := 0
	for index := 0; index < len(runes); {
		if index+len(needle) <= len(runes) && equalWordRuneSlice(lowered[index:index+len(needle)], needle) {
			result.WriteString(replacement)
			index += len(needle)
			replacementCount++
			continue
		}
		result.WriteRune(runes[index])
		index++
	}
	return result.String(), replacementCount
}

func equalWordRuneSlice(left, right []rune) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (a *App) applySpreadsheetReplaceAllPlan(result spreadsheetFormatResult) {
	state, plan := result.state, result.replaceAll
	if state == nil || plan == nil || state != a.documentSessionState || result.generation != a.documentGeneration || result.sheet != a.sheet {
		return
	}
	if result.err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.error.operation_failed"}, result.err, notificationToast)
		a.modal.SetSearchReplaceSummaryRef(i18n.Message{ID: "spreadsheet.replace.error.failed"})
		return
	}
	if plan.truncated {
		message := i18n.Message{ID: "spreadsheet.replace.error.too_many_matches"}
		a.setStatusMessage(message, feedbackWarning, notificationToast)
		a.modal.SetSearchReplaceSummaryRef(message)
		return
	}
	if len(plan.edits) == 0 {
		message := i18n.Message{ID: "spreadsheet.search.status.not_found"}
		if plan.foundCells > 0 && plan.skippedMatches > 0 {
			message = i18n.Message{ID: "spreadsheet.replace.status.no_replaceable_text"}
		} else if plan.foundCells > 0 {
			message = i18n.Message{ID: "spreadsheet.replace.status.unchanged"}
		}
		a.setStatusMessage(message, feedbackInfo, notificationSuppressed)
		a.modal.SetSearchReplaceSummaryRef(message)
		a.modal.FocusInputRequested = true
		return
	}
	details := i18n.Message{ID: "spreadsheet.replace.status.completed", Args: i18n.Args{"Changed": plan.changedCells, "Skipped": plan.skippedMatches}}
	progress := i18n.Message{ID: "spreadsheet.replace.status.replacing", Args: i18n.Args{"Count": plan.changedCells}}
	a.modal.SetSearchReplaceSummaryRef(progress)
	a.setStatusMessage(progress, feedbackInfo, notificationSuppressed)
	if err := a.applyBatchEditsWithDetails(plan.edits, "替换全部", details); err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "spreadsheet.error.operation_failed"}, err, notificationToast)
		a.modal.SetSearchReplaceSummaryRef(i18n.Message{ID: "spreadsheet.replace.error.failed"})
	}
}
