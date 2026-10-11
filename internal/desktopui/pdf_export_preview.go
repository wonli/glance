package desktopui

import (
	"context"
	"errors"
	"fmt"
	"image"
	"strings"

	"gioui.org/widget"

	"glance/internal/document"
	sheetpdf "glance/internal/format/pdf"
	"glance/internal/wordlayout"
)

type pdfExportPreviewState struct {
	Active           bool
	Format           string // "word", "spreadsheet", "markdown", "presentation"
	markdownResult   markdownFileResult
	resourceWarnings widget.List
	Orientation      string // "portrait", "landscape"
	CurrentPageIndex int    // 0-indexed
	TotalPages       int

	// Spreadsheet data
	sheetWindow     document.SheetWindow
	sheetPagination sheetpdf.SpreadsheetPagination
	sheetName       string
	sheetState      *documentSessionState

	// Word data
	wordState          *documentSessionState
	wordKey            wordLayoutKey
	wordResult         *wordlayout.Result
	wordBlocks         []document.WordBlock
	wordLayout         document.WordLayoutData
	wordPreviewPending bool

	// Presentation data is an immutable snapshot shared by the preview and export.
	presentationState          *documentSessionState
	presentationSnapshot       document.PresentationSnapshot
	presentationPreviewImage   image.Image
	presentationPreviewPending bool
	presentationPreviewError   string
	presentationPreviewIndex   int
	presentationPreviewGen     uint64
	presentationPrintMode      bool

	// Interactive widgets (persistent in state)
	portraitBtn  widget.Clickable
	landscapeBtn widget.Clickable
	prevPageBtn  widget.Clickable
	nextPageBtn  widget.Clickable
}

type wordPDFPreviewResult struct {
	state       *documentSessionState
	key         wordLayoutKey
	orientation string
	result      wordlayout.Result
	err         error
}

type spreadsheetPDFPreviewResult struct {
	state  *documentSessionState
	sheet  string
	window document.SheetWindow
	err    error
}

func (a *App) openSpreadsheetPDFExportPreview(state *documentSessionState, sheet string) {
	if state == nil || state.book == nil || state.closed.Load() {
		return
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

	if a.spreadsheetPDFPreviewResults == nil {
		a.spreadsheetPDFPreviewResults = make(chan spreadsheetPDFPreviewResult, 1)
	}
	results, shutdown, appWindow := a.spreadsheetPDFPreviewResults, a.shutdown, a.window
	a.launchManagedBackground(func() {
		result := spreadsheetPDFPreviewResult{state: state, sheet: sheet}
		unlock := lockSessionAdapterRead(state)
		var images []document.SpreadsheetImage
		result.window, images, result.err = readSpreadsheetPDFPreviewSnapshot(state, reader, sheet)
		unlock()
		if result.err == nil {
			result.window, _, _ = sheetpdf.TrimSheetWindow(result.window, images)
		}
		state.finishOperation(token)
		select {
		case results <- result:
			if appWindow != nil {
				appWindow.Invalidate()
			}
		case <-shutdown:
		}
	})
	a.setStatus("正在准备 PDF 预览…")
}

func readSpreadsheetPDFPreviewSnapshot(state *documentSessionState, reader document.GridReader, sheet string) (document.SheetWindow, []document.SpreadsheetImage, error) {
	d, ok := state.book.(interface {
		Dimension(string) (int, int, error)
	})
	if !ok {
		return document.SheetWindow{}, nil, errors.New("工作簿不支持读取完整打印区域")
	}
	maxCol, maxRow, err := d.Dimension(sheet)
	if err != nil {
		return document.SheetWindow{}, nil, err
	}
	if maxCol <= 0 || maxRow <= 0 {
		maxCol, maxRow = 1, 1
	}
	if maxCol > 100 || maxRow > 5000 {
		return document.SheetWindow{}, nil, fmt.Errorf("工作表范围为 %d 行 × %d 列，超过当前 PDF 导出上限 5000 行 × 100 列", maxRow, maxCol)
	}
	window, err := reader.Window(sheet, 1, 1, maxRow, maxCol)
	if err != nil {
		return document.SheetWindow{}, nil, err
	}
	var images []document.SpreadsheetImage
	if imgReader, ok := state.book.(document.SpreadsheetImageReader); ok {
		images, err = imgReader.SpreadsheetImages(sheet)
	}
	return window, images, err
}

func (a *App) applySpreadsheetPDFPreviewResult(result spreadsheetPDFPreviewResult) {
	state := result.state
	if state == nil || state.closed.Load() || state != a.documentSessionState || state.sheet != result.sheet {
		return
	}
	if result.err != nil {
		a.setStatus("准备 PDF 预览失败：" + result.err.Error())
		return
	}
	trimmedWindow := result.window

	// Determine default orientation:
	// A4 portrait printable width is ~523 pt. If trimmed grid width exceeds 480 pt, default to landscape.
	totalWidth := 0.0
	for _, w := range trimmedWindow.ColumnWidthsChars {
		cw := 54.0
		if w > 0 {
			cw = w * 7.0
			if cw < 24.0 {
				cw = 24.0
			}
		}
		totalWidth += cw
	}

	defaultOrientation := "portrait"
	if totalWidth > 480.0 {
		defaultOrientation = "landscape"
	}

	pagination := sheetpdf.ComputeSpreadsheetPaginationForTrimmedWindow(trimmedWindow, sheetpdf.SpreadsheetPDFOptions{
		SheetName:      result.sheet,
		Orientation:    defaultOrientation,
		FitToPageWidth: true,
	})

	a.pdfExportPreview = pdfExportPreviewState{
		Active:           true,
		Format:           "spreadsheet",
		Orientation:      defaultOrientation,
		CurrentPageIndex: 0,
		TotalPages:       pagination.TotalPages,
		sheetWindow:      trimmedWindow,
		sheetPagination:  pagination,
		sheetName:        result.sheet,
		sheetState:       state,
	}

	a.modal.ShowCustom("导出 PDF 预览", "设置页面方向并预览导出效果", 680, a.layoutPDFExportPreview, func(action string) {
		if action == "confirm" && a.pdfExportPreview.wordPreviewPending {
			a.setStatus("正在按所选方向重新排版，请稍后再导出")
			return
		}
		a.modal.Hide()
		if action != "confirm" {
			a.pdfExportPreview.Active = false
			a.setStatus("已取消导出 PDF")
			return
		}
		chosenOrientation := a.pdfExportPreview.Orientation
		a.pdfExportPreview.Active = false
		a.startSpreadsheetPDFSavePicker(state, result.sheet, chosenOrientation)
	}, modalDialogActionSpec{ID: "cancel", Label: "取消", Style: modalDialogActionSecondary},
		modalDialogActionSpec{ID: "confirm", Label: "导出 PDF", Style: modalDialogActionPrimary})
	a.modal.CustomActionsInLayout = true
}

func (a *App) openWordPDFExportPreview(state *documentSessionState, key wordLayoutKey) {
	if state == nil || state.word == nil || state.closed.Load() || state.wordLayoutResult == nil {
		return
	}

	reader, ok := any(state.word).(document.WordLayoutReader)
	if !ok {
		a.setStatus("DOCX 适配器未提供分页信息，无法导出 PDF")
		return
	}

	unlock := lockSessionAdapterRead(state)
	layoutData := reader.WordLayoutData()
	blocks := cloneWordBlockSnapshot(wordBlocksForState(state))
	unlock()

	defaultOrientation := "portrait"
	if len(state.wordLayoutResult.Pages) > 0 {
		p0 := state.wordLayoutResult.Pages[0]
		if p0.Clip.WidthPT > p0.Clip.HeightPT {
			defaultOrientation = "landscape"
		}
	}

	a.pdfExportPreview = pdfExportPreviewState{
		Active:           true,
		Format:           "word",
		Orientation:      defaultOrientation,
		CurrentPageIndex: 0,
		TotalPages:       len(state.wordLayoutResult.Pages),
		wordState:        state,
		wordKey:          key,
		wordResult:       state.wordLayoutResult,
		wordBlocks:       blocks,
		wordLayout:       layoutData,
	}

	a.modal.ShowCustom("导出 PDF 预览", "设置页面方向并预览导出效果", 680, a.layoutPDFExportPreview, func(action string) {
		if action == "confirm" && a.pdfExportPreview.wordPreviewPending {
			a.setStatus("正在按所选方向重新排版，请稍后再导出")
			return
		}
		a.modal.Hide()
		if action != "confirm" {
			a.pdfExportPreview.Active = false
			a.setStatus("已取消导出 PDF")
			return
		}
		chosenOrientation := a.pdfExportPreview.Orientation
		a.pdfExportPreview.Active = false
		a.startWordPDFSavePicker(state, key, chosenOrientation)
	}, modalDialogActionSpec{ID: "cancel", Label: "取消", Style: modalDialogActionSecondary},
		modalDialogActionSpec{ID: "confirm", Label: "导出 PDF", Style: modalDialogActionPrimary})
	a.modal.CustomActionsInLayout = true
}

func (a *App) updateWordPreviewOrientation(orientation string) error {
	prev := &a.pdfExportPreview
	if prev.wordState == nil || prev.Format != "word" {
		return errors.New("Word 预览已失效")
	}
	if prev.wordPreviewPending {
		return nil
	}
	isLandscape := strings.ToLower(orientation) == "landscape"
	w, h := 595.28, 841.89
	orient := "portrait"
	if isLandscape {
		w, h = 841.89, 595.28
		orient = "landscape"
	}

	setup := document.WordPageSetup{
		WidthPT: w, HeightPT: h, Orientation: orient,
		MarginTopPT: 72, MarginRightPT: 72, MarginBottomPT: 72, MarginLeftPT: 72,
		Set: document.WordPageWidthSet | document.WordPageHeightSet | document.WordPageOrientationSet,
	}

	layoutData := prev.wordLayout
	if len(layoutData.Sections) > 0 {
		sections := make([]document.WordSection, len(layoutData.Sections))
		copy(sections, layoutData.Sections)
		sections[0].PageSetup.Orientation = orient
		sections[0].PageSetup.Set |= document.WordPageOrientationSet
		if isLandscape && sections[0].PageSetup.WidthPT < sections[0].PageSetup.HeightPT {
			sections[0].PageSetup.WidthPT, sections[0].PageSetup.HeightPT = sections[0].PageSetup.HeightPT, sections[0].PageSetup.WidthPT
		} else if !isLandscape && sections[0].PageSetup.WidthPT > sections[0].PageSetup.HeightPT {
			sections[0].PageSetup.WidthPT, sections[0].PageSetup.HeightPT = sections[0].PageSetup.HeightPT, sections[0].PageSetup.WidthPT
		}
		layoutData.Sections = sections
	}

	if a.wordPDFPreviewResults == nil {
		a.wordPDFPreviewResults = make(chan wordPDFPreviewResult, 1)
	}
	prev.wordPreviewPending = true
	result := wordPDFPreviewResult{state: prev.wordState, key: prev.wordKey, orientation: orientation}
	blocks := prev.wordBlocks
	fonts := a.wordPDFFontSource
	results, shutdown, appWindow := a.wordPDFPreviewResults, a.shutdown, a.window
	a.launchManagedBackground(func() {
		var shaper wordlayout.TextShaper
		if existing, ok := fonts.(wordlayout.TextShaper); ok {
			shaper = existing
		} else {
			shaper, result.err = newWordTextShaper()
		}
		if result.err == nil {
			engine := wordlayout.NewEngine(shaper)
			result.result, result.err = engine.Layout(context.Background(), wordlayout.Input{
				DocumentID:       result.key.DocumentID,
				Revision:         result.key.Revision,
				Blocks:           blocks,
				Layout:           layoutData,
				DefaultPageSetup: setup,
			})
			if result.err == nil && !result.result.CanPreview {
				result.err = errors.New(blockingWordLayoutMessage(result.result))
			}
		}
		select {
		case results <- result:
			if appWindow != nil {
				appWindow.Invalidate()
			}
		case <-shutdown:
		}
	})
	return nil
}

func (a *App) applyWordPDFPreviewResult(result wordPDFPreviewResult) {
	prev := &a.pdfExportPreview
	if !prev.Active || prev.Format != "word" || prev.wordState != result.state || prev.wordKey != result.key || result.state.closed.Load() {
		return
	}
	prev.wordPreviewPending = false
	if result.err != nil {
		a.setStatus("切换 PDF 方向失败：" + result.err.Error())
		return
	}
	prev.Orientation = result.orientation
	prev.wordResult = &result.result
	prev.TotalPages = len(result.result.Pages)
	prev.CurrentPageIndex = 0
}
