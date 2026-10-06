package desktopui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"glance/internal/document"
	sheetpdf "glance/internal/format/pdf"
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

type spreadsheetPDFImageSource struct {
	reader document.SpreadsheetImageReader
}

func (s spreadsheetPDFImageSource) SpreadsheetImages(sheet string) ([]document.SpreadsheetImage, error) {
	if s.reader == nil {
		return nil, nil
	}
	return s.reader.SpreadsheetImages(sheet)
}

func (s spreadsheetPDFImageSource) SpreadsheetImageBytes(imageID string) ([]byte, error) {
	if s.reader == nil {
		return nil, nil
	}
	return s.reader.SpreadsheetImageBytes(imageID)
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
	if a.wordPDFFontSource == nil {
		shaper, err := newWordTextShaper()
		if err != nil {
			a.setStatus("初始化 PDF 字体服务失败：" + err.Error())
			return
		}
		a.wordPDFFontSource = shaper
	}

	reader, ok := state.book.(document.GridReader)
	if !ok {
		a.setStatus("当前电子表格不支持网格视图，无法导出 PDF")
		return
	}

	token, ok := state.beginOperation(sessionOperationSnapshot)
	if !ok {
		a.setStatus("电子表格正在处理其他操作，请稍后重试")
		return
	}

	var imageSrc sheetpdf.SpreadsheetImageSource
	if imgReader, ok := state.book.(document.SpreadsheetImageReader); ok {
		imageSrc = spreadsheetPDFImageSource{reader: imgReader}
	}

	fontSource := a.wordPDFFontSource
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
	go func() {
		defer stateCopy.finishOperation(token)
		result := spreadsheetPDFExportResult{state: stateCopy, sheet: sheet, path: path}
		unlock := lockSessionAdapterRead(stateCopy)
		window, _, snapshotErr := readSpreadsheetPDFPreviewSnapshot(stateCopy, reader, sheet)
		unlock()
		opts := sheetpdf.SpreadsheetPDFOptions{
			SheetName:      sheet,
			Orientation:    orientation,
			FitToPageWidth: true,
		}
		if snapshotErr != nil {
			result.err = errors.New("读取工作表数据失败：" + snapshotErr.Error())
		} else {
			result.err = sheetpdf.ExportSpreadsheetPDFFileWithCommitGuard(ctx, path, window, opts, fontSource, imageSrc, func() (func(), error) {
				stateCopy.spreadsheetExportCommitMu.Lock()
				release := stateCopy.spreadsheetExportCommitMu.Unlock
				if ctx.Err() != nil {
					release()
					return nil, ctx.Err()
				}
				if stateCopy.closed.Load() {
					release()
					return nil, errors.New("工作簿已关闭，未覆盖目标文件")
				}
				return release, nil
			})
		}
		select {
		case results <- result:
			if appWindow != nil {
				appWindow.Invalidate()
			}
		case <-shutdown:
		}
	}()
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
