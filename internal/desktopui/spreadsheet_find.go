package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/document"
	"glance/internal/format/xlsx"
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
		a.setStatus("当前表格不支持查找")
		return
	}
	_, hasGrid := state.book.(document.GridReader)
	_, hasBatch := state.book.(document.SpreadsheetBatchEditor)
	_, hasValueKind := state.book.(document.SpreadsheetCellValueKindReader)
	canReplace := hasGrid && hasBatch && hasValueKind
	if replaceTab && !canReplace {
		a.setStatus("当前表格不支持安全替换")
		return
	}
	query := spreadsheetFindInitialQuery(state)
	actions := []modalDialogActionSpec{{ID: "find-next", Label: "查找下一个", Style: modalDialogActionPrimary}}
	if canReplace {
		actions = []modalDialogActionSpec{
			{ID: "find-next", Label: "查找下一个", Style: modalDialogActionSecondary, WidthDp: 96, TextSizeSp: 11},
			{ID: "replace", Label: "替换", Style: modalDialogActionPrimary, WidthDp: 76, TextSizeSp: 12},
			{ID: "replace-all", Label: "全部替换", Style: modalDialogActionSecondary, WidthDp: 92, TextSizeSp: 11},
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
			a.setStatus("请输入要查找的内容")
			a.modal.SearchReplaceSummary = "请输入要查找的内容"
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
		a.modal.ShowDualInput("查找和替换", "", query, "查找内容", state.excelReplaceText, "替换为", onAction, actions...)
	} else {
		a.modal.ShowInput("查找和替换", "", query, "查找内容", onAction, actions...)
	}
	a.modal.Input.SetCaret(0, len([]rune(query)))
	a.modal.SearchReplace = true
	a.modal.SearchReplaceHasReplace = canReplace
	a.modal.SearchReplaceReplaceTab = replaceTab
	a.modal.SearchReplaceScope = "当前工作表"
	a.modal.SearchReplaceSummary = "输入查找内容后，点击“查找下一个”"
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
			a.setStatus("表格正在处理其他操作")
			a.modal.SearchReplaceSummary = "表格正在处理其他操作"
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
		a.setStatus("查找失败：" + err.Error())
		a.modal.SearchReplaceSummary = "查找失败"
		return
	}
	if len(matches) == 0 {
		a.setStatus("未找到：" + query)
		a.modal.SearchReplaceSummary = "未找到匹配项"
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
	countSuffix := fmt.Sprintf("共 %d 处匹配", len(matches))
	if truncated {
		countSuffix = fmt.Sprintf("至少 %d 处匹配，结果已截断", len(matches))
	}
	a.setStatus(fmt.Sprintf("已定位至 %s（%s）", xlsx.CellName(target.Column, target.Row), countSuffix))
	a.modal.SearchReplaceSummary = fmt.Sprintf("找到 %d 个匹配项 · 当前单元格 %s", len(matches), xlsx.CellName(target.Column, target.Row))
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
		a.setStatus("读取当前单元格失败：" + err.Error())
		return
	}
	if len(window.Cells) == 0 || len(window.Cells[0]) == 0 {
		a.setStatus("无法读取当前单元格")
		return
	}
	cell := window.Cells[0][0]
	if cell.Formula != "" {
		a.setStatus("公式单元格暂不支持替换")
		a.modal.SearchReplaceSummary = "公式单元格暂不支持替换"
		return
	}
	typeReader, ok := state.book.(document.SpreadsheetCellValueKindReader)
	if !ok {
		a.setStatus("当前表格不支持安全替换")
		return
	}
	valueKind, err := typeReader.StoredCellValueKind(state.sheet, xlsx.CellName(column, row))
	if err != nil {
		a.setStatus("读取单元格类型失败：" + err.Error())
		return
	}
	if valueKind != document.CellValueText {
		a.setStatus("目前仅支持替换文本单元格")
		a.modal.SearchReplaceSummary = "目前仅支持替换文本单元格"
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
		a.setStatus("替换内容与当前值相同")
		a.modal.SearchReplaceSummary = "替换内容与当前值相同"
		return
	}
	if strings.HasPrefix(strings.TrimSpace(updated), "=") {
		a.setStatus("替换结果不能以 = 开头")
		a.modal.SearchReplaceSummary = "替换结果不能以 = 开头"
		return
	}
	typedValue := document.CellValue{Kind: document.CellValueText, Text: updated}
	if err := a.applyBatchEdits([]document.CellEdit{{Row: row, Column: column, Value: updated, TypedValue: &typedValue}}, "替换单元格"); err != nil {
		a.setStatus("替换失败：" + err.Error())
		a.modal.SearchReplaceSummary = "替换失败"
		return
	}
	a.modal.SearchReplaceSummary = "正在替换当前单元格…"
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
		a.setStatus("当前工作簿不支持安全替换")
		a.modal.SearchReplaceSummary = "当前工作簿不支持安全替换"
		return
	}
	if a.formatResults == nil {
		a.setStatus("替换服务不可用")
		a.modal.SearchReplaceSummary = "替换服务不可用"
		return
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		a.setStatus("表格正在处理其他操作")
		a.modal.SearchReplaceSummary = "表格正在处理其他操作"
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	results, shutdown, appWindow := a.formatResults, a.shutdown, a.window
	a.modal.SearchReplaceSummary = "正在扫描当前工作表…"
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
		a.setStatus("替换任务无法启动")
		a.modal.SearchReplaceSummary = "替换任务无法启动"
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
		a.setStatus("替换全部失败：" + result.err.Error())
		a.modal.SearchReplaceSummary = "替换失败"
		return
	}
	if plan.truncated {
		const message = "匹配超过 10,000 个单元格，未修改；请缩小查找内容后重试"
		a.setStatus(message)
		a.modal.SearchReplaceSummary = message
		return
	}
	if len(plan.edits) == 0 {
		message := "未找到匹配项"
		if plan.foundCells > 0 && plan.skippedMatches > 0 {
			message = "找到匹配项，但没有可替换的文本单元格"
		} else if plan.foundCells > 0 {
			message = "匹配内容已是替换结果"
		}
		a.setStatus(message)
		a.modal.SearchReplaceSummary = message
		a.modal.FocusInputRequested = true
		return
	}
	details := fmt.Sprintf("已替换 %d 个单元格", plan.changedCells)
	if plan.skippedMatches > 0 {
		details += fmt.Sprintf("；跳过 %d 个公式或非文本单元格", plan.skippedMatches)
	}
	progress := fmt.Sprintf("正在替换 %d 个单元格…", plan.changedCells)
	a.modal.SearchReplaceSummary = progress
	a.setStatus(progress)
	if err := a.applyBatchEditsWithDetails(plan.edits, "替换全部", details); err != nil {
		a.setStatus("替换全部失败：" + err.Error())
		a.modal.SearchReplaceSummary = "替换失败"
	}
}
