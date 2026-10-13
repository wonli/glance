package desktopui

import (
	"context"
	"path/filepath"
	"strings"

	"glance/internal/document"
)

type spreadsheetPDFPickerRequest struct {
	state       *documentSessionState
	sheet       string
	orientation string
}

type spreadsheetPDFExportResult struct {
	state *documentSessionState
	sheet string
	path  string
	err   error
}

func (a *App) requestSpreadsheetPDFExport() {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.closed.Load() || state.formatRunning.Load() || a.isSaving() || a.fileCommandRunning || state.spreadsheetPDFExportRunning {
		return
	}
	if a.cellEdit.Editing {
		_ = a.commitGridCell(a.cellEdit.Draft)
	}
	a.openSpreadsheetPDFExportPreview(state, state.sheet)
}

func (a *App) startSpreadsheetPDFSavePicker(state *documentSessionState, sheet string, orientation string) {
	if state == nil || state.book == nil || a.fileCommandRunning {
		return
	}
	a.spreadsheetPDFPickerRequest = &spreadsheetPDFPickerRequest{state: state, sheet: sheet, orientation: orientation}
	a.startFileCommand(commandExportPDF, "pdf")
}

type spreadsheetDocumentExportPdfArguments struct {
	Sheet       string             `json:"sheet"`
	Bounds      document.CellRange `json:"bounds"`
	TargetPath  string             `json:"target_path"`
	Orientation string             `json:"orientation"`
}

func (a *App) startSpreadsheetPDFFileExport(state *documentSessionState, sheet string, path string, orientation string) {
	if state == nil || state.book == nil || state.closed.Load() {
		a.setStatus("PDF 导出状态已过期，请重新导出")
		return
	}
	if pathsReferToSameFile(path, state.path) {
		a.setStatus("PDF 保存路径不能覆盖源工作簿")
		return
	}
	if strings.ToLower(filepath.Ext(path)) != ".pdf" {
		path += ".pdf"
	}
	if a.documentSessionState != state {
		a.setStatus("工作簿已切换，PDF 导出已取消")
		return
	}
	dimensions, ok := state.book.(interface {
		Dimension(string) (int, int, error)
	})
	if !ok {
		a.setStatus("当前电子表格不支持读取打印范围，无法导出 PDF")
		return
	}
	unlock := lockSessionAdapterRead(state)
	maxColumn, maxRow, err := dimensions.Dimension(sheet)
	unlock()
	if err != nil {
		a.setStatus("读取工作表范围失败：" + err.Error())
		return
	}
	if maxColumn <= 0 || maxRow <= 0 {
		maxColumn, maxRow = 1, 1
	}
	if maxColumn > 100 || maxRow > 5000 {
		a.setStatus("工作表范围超过 PDF 导出上限 5000 行 × 100 列")
		return
	}
	bounds := document.CellRange{StartRow: 1, StartColumn: 1, EndRow: maxRow, EndColumn: maxColumn}
	if !a.applicationSessionRegistered(state.documentID) {
		a.setStatus("MCP 应用运行时不可用，无法通过共享导出能力执行")
		return
	}
	token, ok := state.beginOperation(sessionOperationSnapshot)
	if !ok {
		a.setStatus("电子表格正在处理其他操作，请稍后重试")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	state.spreadsheetPDFExportCancel = cancel
	state.spreadsheetPDFExportRunning = true
	results, shutdown, appWindow := a.spreadsheetPDFExportResults, a.shutdown, a.window
	if results == nil {
		state.finishOperation(token)
		cancel()
		state.spreadsheetPDFExportCancel = nil
		state.spreadsheetPDFExportRunning = false
		a.setStatus("PDF 导出不可用")
		return
	}

	stateCopy := state
	a.launchManagedBackground(func() {
		defer cancel()
		defer stateCopy.finishOperation(token)
		result := spreadsheetPDFExportResult{state: stateCopy, sheet: sheet, path: path}
		_, result.err = a.executeRibbonApplicationCommandWithContext(ctx, token,
			"spreadsheet.document.export_pdf", "spreadsheet_sheet", sheet, sheet, spreadsheetDocumentExportPdfArguments{Sheet: sheet, Bounds: bounds, TargetPath: path, Orientation: orientation},
		)
		select {
		case results <- result:
			if appWindow != nil {
				appWindow.Invalidate()
			}
		case <-shutdown:
		}
	})
	a.setStatus("正在生成电子表格 PDF…")
}

func (a *App) applySpreadsheetPDFExportResult(result spreadsheetPDFExportResult) {
	state := result.state
	if state == nil {
		return
	}
	state.spreadsheetPDFExportRunning = false
	state.spreadsheetPDFExportCancel = nil
	if state.closed.Load() {
		return
	}
	if result.err != nil {
		if state == a.documentSessionState {
			a.setStatus("导出 PDF 失败：" + result.err.Error())
		}
	} else if state == a.documentSessionState {
		a.setStatus("PDF 已导出：" + filepath.Base(result.path))
	}
}
