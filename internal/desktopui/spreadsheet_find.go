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
	actions := []modalDialogActionSpec{{ID: "find-next", Label: "查找下一个", Style: modalDialogActionPrimary}}
	if canReplace {
		actions = []modalDialogActionSpec{
			{ID: "find-next", Label: "查找下一个", Style: modalDialogActionSecondary, WidthDp: 96, TextSizeSp: 11},
			{ID: "replace", Label: "替换", Style: modalDialogActionPrimary, WidthDp: 76, TextSizeSp: 12},
		}
	}
	onAction := func(action string) {
		if action == "close" {
			a.modal.Hide()
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
		if action != "replace" {
			a.findNextSpreadsheetText(searcher, query)
			a.modal.FocusInputRequested = true
			return
		}
		a.replaceSelectedSpreadsheetText(searcher, query, replacement)
		a.modal.FocusInputRequested = true
	}
	if canReplace {
		a.modal.ShowDualInput("查找和替换", "", state.excelFindQuery, "查找内容", state.excelReplaceText, "替换为", onAction, actions...)
	} else {
		a.modal.ShowInput("查找和替换", "", state.excelFindQuery, "查找内容", onAction, actions...)
	}
	a.modal.SearchReplace = true
	a.modal.SearchReplaceHasReplace = canReplace
	a.modal.SearchReplaceReplaceTab = replaceTab
	a.modal.SearchReplaceScope = "当前工作表"
	a.modal.SearchReplaceSummary = "输入查找内容后，点击“查找下一个”"
	a.modal.CardWidthDp = 560
	a.modal.FocusInputRequested = true
}

func (a *App) findNextSpreadsheetText(searcher document.SpreadsheetSearcher, query string) {
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
	for _, match := range matches {
		if match.Row > curRow || (match.Row == curRow && match.Column > curCol) {
			target = match
			break
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
