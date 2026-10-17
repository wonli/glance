package desktopui

import (
	"encoding/json"
	"fmt"
	"time"

	"glance/internal/application"
	"glance/internal/document"
)

type spreadsheetCellTimeArguments struct {
	Sheet string `json:"sheet"`
	Cell  string `json:"cell"`
	Time  string `json:"time"`
}

func (a *App) insertCurrentTimeIntoSpreadsheetCell() error {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return fmt.Errorf("没有打开的工作簿")
	}
	if state.sourceFormat != "" && state.sourceFormat != application.FormatXLSX {
		return fmt.Errorf("当前文件格式不支持插入静态时间")
	}
	if state.formatRunning.Load() || a.isSaving() {
		return fmt.Errorf("表格正在处理其他操作")
	}
	editor, ok := state.book.(document.SpreadsheetCellTimeEditor)
	if !ok {
		return fmt.Errorf("当前工作簿不支持插入静态时间")
	}
	active := spreadsheetActiveCell(state, a.selectedCell)
	if active.Row < 0 || active.Column < 0 || active.Row >= xlsxRows || active.Column >= xlsxColumns {
		return fmt.Errorf("当前单元格超出工作表范围")
	}
	cell := spreadsheetCellName(active)
	sheet := state.sheet
	value := time.Now()
	timeString := value.Format("15:04")
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return fmt.Errorf("表格正在处理其他操作")
	}
	generation, window := a.documentGeneration, state.virtualGrid
	arguments := spreadsheetCellTimeArguments{Sheet: sheet, Cell: cell, Time: timeString}
	if !a.launchManagedBackground(func() {
		supportedFormat := state.sourceFormat == "" || state.sourceFormat == application.FormatXLSX
		commandRouted := supportedFormat && a.applicationSessionRegistered(token.documentID)
		changed := false
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, "spreadsheet.cell.time.insert", "spreadsheet.cell", cell, sheet, arguments)
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
			changed, err = editor.SetCellTimeChanged(sheet, cell, value)
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		select {
		case a.formatResults <- spreadsheetFormatResult{
			state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation,
			sheet: sheet, window: window, err: err, operation: "插入时间", applied: changed,
			noOp: err == nil && !changed, details: "单元格已是当前时间",
		}:
		case <-a.shutdown:
		}
	}) {
		state.finishOperation(token)
		return fmt.Errorf("时间插入任务无法启动")
	}
	return nil
}
