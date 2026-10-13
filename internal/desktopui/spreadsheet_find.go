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
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	searcher, ok := state.book.(document.SpreadsheetSearcher)
	if !ok {
		a.setStatus("当前表格不支持查找")
		return
	}
	a.modal.ShowInput("查找", "在当前工作表中查找内容", state.excelFindQuery, "查找内容", func(action string) {
		query := strings.TrimSpace(a.modal.Input.Text())
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		if query == "" {
			a.setStatus("请输入要查找的内容")
			return
		}
		state.excelFindQuery = query
		var matches []document.CellMatch
		var truncated bool
		var err error
		if a.applicationSessionRegistered(state.documentID) {
			token, ok := state.beginOperation(sessionOperationRead)
			if !ok {
				a.setStatus("表格正在处理其他操作")
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
			return
		}
		if len(matches) == 0 {
			a.setStatus("未找到：" + query)
			return
		}
		currentPos := state.selection.Active
		curRow, curCol := currentPos.Row+1, currentPos.Column+1
		target := matches[0]
		for _, m := range matches {
			if m.Row > curRow || (m.Row == curRow && m.Column > curCol) {
				target = m
				break
			}
		}
		a.navigateToCell(target.Row, target.Column)
		countSuffix := fmt.Sprintf("共 %d 处匹配", len(matches))
		if truncated {
			countSuffix = fmt.Sprintf("至少 %d 处匹配，结果已截断", len(matches))
		}
		a.setStatus(fmt.Sprintf("已定位至 %s（%s）", xlsx.CellName(target.Column, target.Row), countSuffix))
	}, modalDialogActionSpec{ID: "confirm", Label: "查找下一个"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}
