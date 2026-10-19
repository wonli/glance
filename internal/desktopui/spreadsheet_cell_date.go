package desktopui

import (
	"encoding/json"
	"fmt"
	"time"

	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/i18n"
)

type spreadsheetCellDateArguments struct {
	Sheet string `json:"sheet"`
	Cell  string `json:"cell"`
	Date  string `json:"date"`
}

func (a *App) insertCurrentDateIntoSpreadsheetCell() error {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return fmt.Errorf("没有打开的工作簿")
	}
	if state.sourceFormat != "" && state.sourceFormat != application.FormatXLSX {
		return fmt.Errorf("当前文件格式不支持插入静态日期")
	}
	if state.formatRunning.Load() || a.isSaving() {
		return fmt.Errorf("表格正在处理其他操作")
	}
	editor, ok := state.book.(document.SpreadsheetCellDateEditor)
	if !ok {
		return fmt.Errorf("当前工作簿不支持插入静态日期")
	}
	active := spreadsheetActiveCell(state, a.selectedCell)
	if active.Row < 0 || active.Column < 0 || active.Row >= xlsxRows || active.Column >= xlsxColumns {
		return fmt.Errorf("当前单元格超出工作表范围")
	}
	cell := spreadsheetCellName(active)
	sheet := state.sheet
	date := time.Now()
	dateString := date.Format("2006-01-02")
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return fmt.Errorf("表格正在处理其他操作")
	}
	generation, window := a.documentGeneration, state.virtualGrid
	arguments := spreadsheetCellDateArguments{Sheet: sheet, Cell: cell, Date: dateString}
	if !a.launchManagedBackground(func() {
		supportedFormat := state.sourceFormat == "" || state.sourceFormat == application.FormatXLSX
		commandRouted := supportedFormat && a.applicationSessionRegistered(token.documentID)
		changed := false
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.cell.date.insert", "spreadsheet.cell", cell, sheet, arguments)
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
			changed, err = editor.SetCellDateChanged(sheet, cell, date)
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		select {
		case a.formatResults <- spreadsheetFormatResult{
			state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation,
			sheet: sheet, window: window, err: err, operation: "插入日期", applied: changed,
			noOp: err == nil && !changed, details: i18n.Message{ID: "spreadsheet.status.noop.current_date"},
		}:
		case <-a.shutdown:
		}
	}) {
		state.finishOperation(token)
		return fmt.Errorf("日期插入任务无法启动")
	}
	return nil
}
