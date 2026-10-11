package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/document"
	"glance/internal/format/xlsx"
	"strings"
)

func (a *App) showSpreadsheetSpellCheckDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	bounds := state.selection.Bounds()
	if !state.selectionSet || bounds.StartRow <= 0 {
		bounds = document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 20, EndColumn: 10}
	}
	sheet, generation, window := state.sheet, a.documentGeneration, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		return
	}
	a.setStatus("正在检查选区文本…")
	if a.window != nil {
		a.window.Invalidate()
	}
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, "spreadsheet.text.proofread", "spreadsheet_range", "selection", sheet, map[string]any{
			"sheet": sheet, "bounds": bounds,
		})
		details := ""
		if err == nil {
			var report struct {
				CellsScanned      int  `json:"cells_scanned"`
				DictionaryChecked bool `json:"dictionary_checked"`
				Issues            []struct {
					Cell     string `json:"cell"`
					Category string `json:"category"`
					Token    string `json:"token"`
					Message  string `json:"message"`
				} `json:"issues"`
				IssuesTruncated bool `json:"issues_truncated"`
			}
			err = json.Unmarshal(result.Data, &report)
			if err == nil {
				details = formatSpreadsheetProofreadSummary(report.CellsScanned, report.Issues, report.IssuesTruncated)
			}
		}
		a.completeSessionOperation(state, token, err, false)
		select {
		case a.formatResults <- spreadsheetFormatResult{
			state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation,
			sheet: sheet, window: window, err: err, operation: "拼写检查", details: details,
		}:
		case <-a.shutdown:
		}
	})
}

func formatSpreadsheetProofreadSummary(checkedCount int, issues []struct {
	Cell     string `json:"cell"`
	Category string `json:"category"`
	Token    string `json:"token"`
	Message  string `json:"message"`
}, truncated bool) string {
	var summary strings.Builder
	fmt.Fprintf(&summary, "已扫描 %d 个单元格。", checkedCount)
	if len(issues) == 0 {
		summary.WriteString("\n未发现重复空格或不可见字符。")
	} else {
		fmt.Fprintf(&summary, "\n发现 %d 项文本问题：", len(issues))
		for index, issue := range issues {
			if index == 30 {
				summary.WriteString("\n…更多结果请通过 MCP 查询。")
				break
			}
			fmt.Fprintf(&summary, "\n• %s：%s", issue.Cell, issue.Message)
		}
	}
	if truncated {
		summary.WriteString("\n结果超过上限，列表已截断。")
	}
	summary.WriteString("\n本次未提供词典，因此没有判断英文单词拼写。Agent 可通过 spreadsheet.text.proofread 提供词典进行词条核对。")
	return summary.String()
}

func (a *App) showSpreadsheetTranslateDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	col := state.selection.Active.Column + 1
	row := state.selection.Active.Row + 1
	cellName := xlsx.CellName(col, row)
	currentText := ""
	if reader, ok := state.book.(document.SpreadsheetRangeReader); ok {
		rows, err := reader.ReadRange(state.sheet, document.CellRange{StartRow: row, StartColumn: col, EndRow: row, EndColumn: col})
		if err == nil && len(rows) > 0 && len(rows[0]) > 0 {
			currentText = rows[0][0].Value
		}
	}
	if currentText == "" {
		currentText = "Hello"
	}
	a.modal.ShowDualInput("智能翻译", fmt.Sprintf("为单元格 %s 的内容设置或查看翻译：", cellName),
		currentText, "原文内容",
		"", "译文内容 (可选填入覆盖单元格)",
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			translated := strings.TrimSpace(a.modal.SecondInput.Text())
			if translated != "" {
				if err := a.applyBatchEdits([]document.CellEdit{{Row: row, Column: col, Value: translated}}, "翻译"); err != nil {
					a.setStatus("翻译写入失败：" + err.Error())
				}
				return
			}
			a.setStatus("翻译查询完成")
			if a.window != nil {
				a.window.Invalidate()
			}
		},
	)
}
