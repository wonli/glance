package desktopui

import (
	"context"
	"fmt"

	"glance/internal/document"
)

type spreadsheetPrintResult struct {
	state      *documentSessionState
	generation uint64
	revision   uint64
	err        error
}

func (a *App) requestSpreadsheetPrint() {
	state := a.documentSessionState
	if state == nil || state.book == nil || a.nativePrinter == nil || !a.nativePrinter.Available() {
		a.setStatus("当前平台不支持原生打印")
		return
	}
	if state.closed.Load() || state.formatRunning.Load() || a.isSaving() || a.fileCommandRunning || state.spreadsheetPrintRunning {
		return
	}
	if a.cellEdit.Editing {
		if err := a.commitGridCell(a.cellEdit.Draft); err != nil {
			a.setStatus("提交单元格编辑失败，未打印：" + err.Error())
			return
		}
	}
	dimensions, ok := state.book.(interface {
		Dimension(string) (int, int, error)
	})
	if !ok {
		a.setStatus("当前工作簿不支持读取打印范围")
		return
	}
	sheet := state.sheet
	unlock := lockSessionAdapterRead(state)
	maxColumn, maxRow, err := dimensions.Dimension(sheet)
	unlock()
	if err != nil {
		a.setStatus("读取工作表打印范围失败：" + err.Error())
		return
	}
	if maxColumn <= 0 || maxRow <= 0 {
		maxColumn, maxRow = 1, 1
	}
	if maxColumn > 100 || maxRow > 5000 {
		a.setStatus("当前工作表已用范围超过打印上限 5000 行 × 100 列")
		return
	}
	bounds := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: maxRow, EndColumn: maxColumn}
	revision := state.wordRevisionAtomic.Load()
	generation := a.documentGeneration
	rangeLabel := fmt.Sprintf("%s%d:%s%d", columnName(bounds.StartColumn), bounds.StartRow, columnName(bounds.EndColumn), bounds.EndRow)
	a.modal.Show("打印工作表", fmt.Sprintf("工作表：%s\n打印范围：%s\n方向：纵向\n\n确认后将根据当前工作簿修订版本生成 PDF 并发送到系统打印面板。", sheet, rangeLabel), func(action string) {
		a.modal.Hide()
		if action != "print" {
			return
		}
		if state != a.documentSessionState || state.closed.Load() || generation != a.documentGeneration || state.wordRevisionAtomic.Load() != revision {
			a.setStatus("工作簿在确认期间已变化，未打印；请重新发起打印")
			return
		}
		a.beginSpreadsheetPrint(state, sheet, bounds)
	},
		modalDialogActionSpec{ID: "print", Label: "确认打印"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

type spreadsheetDocumentPrintArguments struct {
	Sheet       string             `json:"sheet"`
	Bounds      document.CellRange `json:"bounds"`
	Orientation string             `json:"orientation"`
}

func (a *App) beginSpreadsheetPrint(state *documentSessionState, sheet string, bounds document.CellRange) {
	if state == nil || state.book == nil || state.closed.Load() || a.documentSessionState != state {
		return
	}
	if !a.applicationSessionRegistered(state.documentID) {
		a.setStatus("MCP 应用运行时不可用，无法通过共享打印能力执行")
		return
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		a.setStatus("工作簿正在处理其他操作，未打印")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	state.spreadsheetPrintCancel = cancel
	state.spreadsheetPrintRunning = true
	results, shutdown, window := a.spreadsheetPrintResults, a.shutdown, a.window
	if results == nil {
		state.finishOperation(token)
		cancel()
		state.spreadsheetPrintCancel = nil
		state.spreadsheetPrintRunning = false
		a.setStatus("打印服务不可用")
		return
	}
	generation := a.documentGeneration
	a.launchManagedBackground(func() {
		result := spreadsheetPrintResult{state: state, generation: generation, revision: token.revision}
		command, err := a.executeRibbonApplicationCommandWithContext(ctx, token, "spreadsheet.document.print", "spreadsheet_range", sheet, sheet, spreadsheetDocumentPrintArguments{Sheet: sheet, Bounds: bounds, Orientation: "portrait"})
		result.err = err
		if err == nil {
			result.revision = command.Revision
		}
		state.finishOperation(token)
		cancel()
		select {
		case results <- result:
			if window != nil {
				window.Invalidate()
			}
		case <-shutdown:
		}
	})
	a.setStatus("正在生成并发送工作表打印页面…")
}

func (a *App) applySpreadsheetPrintResult(result spreadsheetPrintResult) {
	state := result.state
	if state == nil {
		return
	}
	state.spreadsheetPrintRunning = false
	if state.spreadsheetPrintCancel != nil {
		state.spreadsheetPrintCancel()
		state.spreadsheetPrintCancel = nil
	}
	if state.closed.Load() || state != a.documentSessionState || result.generation != a.documentGeneration {
		return
	}
	if result.err != nil {
		a.setStatus("工作表打印失败：" + result.err.Error())
	} else {
		a.setStatus(fmt.Sprintf("已将工作表修订版本 r%d 发送到系统打印服务", result.revision))
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}
