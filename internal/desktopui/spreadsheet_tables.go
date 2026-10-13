package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/document"
)

func (a *App) runSpreadsheetTableCommand(operation string, arguments any, edit func(document.SpreadsheetTableEditor) error) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetTableEditor)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	a.launchManagedBackground(func() {
		commandRouted := a.applicationSessionRegistered(token.documentID)
		changed := true
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.table.create", "spreadsheet_cell_range", "selection", sheet, arguments)
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
			err = edit(editor)
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		if err == nil {
			a.setStatus("已创建表格")
		}
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation, applied: changed}:
		case <-a.shutdown:
		}
	})
}

type showSpreadsheetInsertTableDialogArguments struct {
	Sheet     string             `json:"sheet"`
	Bounds    document.CellRange `json:"bounds"`
	StyleName string             `json:"style_name"`
}

func (a *App) showSpreadsheetInsertTableDialog(styleName string) {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	if !state.selectionSet {
		a.setStatus("请先选择创建表格的单元格区域")
		return
	}
	bounds := state.selection.Bounds()
	if bounds.EndRow <= bounds.StartRow {
		a.setStatus("表格选区至少需要 2 行（包含标题行与数据行）")
		return
	}
	sheet := state.sheet
	rangeRef := fmt.Sprintf("%s%d:%s%d", columnName(bounds.StartColumn), bounds.StartRow, columnName(bounds.EndColumn), bounds.EndRow)
	rowCount := bounds.EndRow - bounds.StartRow + 1
	colCount := bounds.EndColumn - bounds.StartColumn + 1
	msg := fmt.Sprintf("将在选区 %s（共 %d 行 %d 列）创建格式化表格。\n表格将自动启用筛选与行列交替颜色。", rangeRef, rowCount, colCount)

	createTable := func(style string) {
		a.runSpreadsheetTableCommand("创建表格", showSpreadsheetInsertTableDialogArguments{Sheet: sheet, Bounds: bounds, StyleName: style}, func(ed document.SpreadsheetTableEditor) error {
			return ed.AddTable(document.SpreadsheetTableSpec{
				Sheet:     sheet,
				Range:     bounds,
				StyleName: style,
			})
		})
	}

	if styleName != "" {
		createTable(styleName)
		return
	}

	a.modal.Show("创建表格", msg, func(action string) {
		a.modal.Hide()
		switch action {
		case "medium":
			createTable("TableStyleMedium2")
		case "light":
			createTable("TableStyleLight1")
		case "dark":
			createTable("TableStyleDark1")
		}
	},
		modalDialogActionSpec{ID: "medium", Label: "中等风格 (推荐)"},
		modalDialogActionSpec{ID: "light", Label: "浅色风格"},
		modalDialogActionSpec{ID: "dark", Label: "深色风格"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}
