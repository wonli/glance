package desktopui

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
	sheetpdf "glance/internal/format/pdf"
	"glance/internal/wordlayout"
)

type pdfExportPreviewState struct {
	Active           bool
	Format           string // "word", "spreadsheet"
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
	go func() {
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
	}()
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
	go func() {
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
	}()
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

func (a *App) layoutPDFExportPreview(gtx layout.Context) layout.Dimensions {
	prev := &a.pdfExportPreview

	// Handle Orientation Toggle Click Events
	if prev.portraitBtn.Clicked(gtx) && prev.Orientation != "portrait" {
		if prev.Format == "spreadsheet" {
			pagination := sheetpdf.ComputeSpreadsheetPaginationForTrimmedWindow(prev.sheetWindow, sheetpdf.SpreadsheetPDFOptions{
				SheetName:      prev.sheetName,
				Orientation:    "portrait",
				FitToPageWidth: true,
			})
			prev.sheetPagination = pagination
			prev.TotalPages = pagination.TotalPages
			prev.Orientation = "portrait"
			prev.CurrentPageIndex = 0
		} else if prev.Format == "word" {
			if err := a.updateWordPreviewOrientation("portrait"); err != nil {
				a.setStatus("切换 PDF 方向失败：" + err.Error())
				return layout.Dimensions{}
			}
		}
		if a.window != nil {
			a.window.Invalidate()
		}
	}
	if prev.landscapeBtn.Clicked(gtx) && prev.Orientation != "landscape" {
		if prev.Format == "spreadsheet" {
			pagination := sheetpdf.ComputeSpreadsheetPaginationForTrimmedWindow(prev.sheetWindow, sheetpdf.SpreadsheetPDFOptions{
				SheetName:      prev.sheetName,
				Orientation:    "landscape",
				FitToPageWidth: true,
			})
			prev.sheetPagination = pagination
			prev.TotalPages = pagination.TotalPages
			prev.Orientation = "landscape"
			prev.CurrentPageIndex = 0
		} else if prev.Format == "word" {
			if err := a.updateWordPreviewOrientation("landscape"); err != nil {
				a.setStatus("切换 PDF 方向失败：" + err.Error())
				return layout.Dimensions{}
			}
		}
		if a.window != nil {
			a.window.Invalidate()
		}
	}

	// Handle Pagination Controls
	if prev.prevPageBtn.Clicked(gtx) && prev.CurrentPageIndex > 0 {
		prev.CurrentPageIndex--
		if a.window != nil {
			a.window.Invalidate()
		}
	}
	if prev.nextPageBtn.Clicked(gtx) && prev.CurrentPageIndex < prev.TotalPages-1 {
		prev.CurrentPageIndex++
		if a.window != nil {
			a.window.Invalidate()
		}
	}

	height := gtx.Dp(unit.Dp(320))
	if height > gtx.Constraints.Max.Y {
		height = gtx.Constraints.Max.Y
	}
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
		layout.Flexed(0.56, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(a.layoutPDFExportSummaryRow),
				layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
				layout.Flexed(1, a.layoutPDFExportPaperCanvas),
			)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(20)}.Layout),
		layout.Flexed(0.44, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(a.layoutPDFExportOrientationSelector),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} }),
				layout.Rigid(a.layoutConfirmDialogActions),
			)
		}),
	)
}

func (a *App) layoutPDFExportOrientationSelector(gtx layout.Context) layout.Dimensions {
	prev := &a.pdfExportPreview
	card := func(gtx layout.Context, title, subtitle string, landscape, active bool, btn *widget.Clickable) layout.Dimensions {
		height := gtx.Dp(unit.Dp(64))
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		return a.layoutOrientationCard(gtx, title, subtitle, landscape, active, btn)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return card(gtx, "纵向 (Portrait)", "适合高表格与标准文本", false, prev.Orientation == "portrait", &prev.portraitBtn)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return card(gtx, "横向 (Landscape)", "适合宽表格", true, prev.Orientation == "landscape", &prev.landscapeBtn)
		}),
	)
}

func (a *App) layoutOrientationCard(gtx layout.Context, title, subtitle string, isLandscape, active bool, btn *widget.Clickable) layout.Dimensions {
	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		borderColor := color.NRGBA{R: 226, G: 232, B: 240, A: 255}
		bgColor := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		titleColor := color.NRGBA{R: 51, G: 65, B: 85, A: 255}
		borderWidth := float32(gtx.Dp(unit.Dp(1)))

		if active {
			borderColor = color.NRGBA{R: 52, G: 120, B: 209, A: 255}
			bgColor = color.NRGBA{R: 240, G: 246, B: 255, A: 255}
			titleColor = color.NRGBA{R: 52, G: 120, B: 209, A: 255}
			borderWidth = float32(gtx.Dp(unit.Dp(2)))
		}

		return layout.Stack{Alignment: layout.Center}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				bounds := image.Rectangle{Max: gtx.Constraints.Max}
				radius := gtx.Dp(unit.Dp(8))
				shape := clip.RRect{Rect: bounds, NW: radius, NE: radius, SW: radius, SE: radius}
				paint.FillShape(gtx.Ops, bgColor, shape.Op(gtx.Ops))
				paint.FillShape(gtx.Ops, borderColor, clip.Stroke{Path: shape.Path(gtx.Ops), Width: borderWidth}.Op())
				return layout.Dimensions{Size: bounds.Max}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						// Icon thumbnail
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							w, h := gtx.Dp(unit.Dp(14)), gtx.Dp(unit.Dp(20))
							if isLandscape {
								w, h = gtx.Dp(unit.Dp(20)), gtx.Dp(unit.Dp(14))
							}
							bounds := image.Rect(0, 0, w, h)
							rect := clip.RRect{Rect: bounds, NW: gtx.Dp(unit.Dp(2)), NE: gtx.Dp(unit.Dp(2)), SW: gtx.Dp(unit.Dp(2)), SE: gtx.Dp(unit.Dp(2))}
							paint.FillShape(gtx.Ops, borderColor, clip.Stroke{Path: rect.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1.5)))}.Op())
							return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(22)), gtx.Dp(unit.Dp(22)))}
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									lbl := material.Body2(a.theme, title)
									lbl.Font.Weight = font.SemiBold
									lbl.TextSize = unit.Sp(13)
									lbl.Color = titleColor
									return lbl.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									sub := material.Caption(a.theme, subtitle)
									sub.TextSize = unit.Sp(11)
									sub.Color = color.NRGBA{R: 100, G: 116, B: 139, A: 255}
									return sub.Layout(gtx)
								}),
							)
						}),
					)
				})
			}),
		)
	})
}

func (a *App) layoutPDFExportSummaryRow(gtx layout.Context) layout.Dimensions {
	prev := &a.pdfExportPreview

	paperLabel := "A4 纵向 (210 × 297 mm)"
	if prev.Orientation == "landscape" {
		paperLabel = "A4 横向 (297 × 210 mm)"
	}

	hintText := "正文根据所选方向流式重排"
	hintColor := color.NRGBA{R: 71, G: 85, B: 105, A: 255}
	if prev.Format == "word" && prev.wordPreviewPending {
		hintText = "正在按所选方向重新排版…"
	}
	if prev.Format == "spreadsheet" {
		if prev.Orientation == "landscape" && prev.TotalPages == 1 {
			hintText = "✓ 已裁剪多余空白，横向单页完整容纳全部列"
			hintColor = color.NRGBA{R: 22, G: 101, B: 52, A: 255}
		} else if prev.Orientation == "portrait" && prev.TotalPages > 1 {
			hintText = "⚠ 纵向宽度超出，将被水平切分成多页；推荐使用横向"
			hintColor = color.NRGBA{R: 194, G: 65, B: 12, A: 255}
		} else {
			hintText = "✓ 已裁剪末尾空白行列，仅导出有效数据区域"
		}
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return a.layoutBadge(gtx, paperLabel, color.NRGBA{R: 241, G: 245, B: 249, A: 255}, color.NRGBA{R: 51, G: 65, B: 85, A: 255})
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							pageBadgeColor := color.NRGBA{R: 234, G: 242, B: 252, A: 255}
							pageTextColor := color.NRGBA{R: 52, G: 120, B: 209, A: 255}
							return a.layoutBadge(gtx, fmt.Sprintf("共 %d 页", prev.TotalPages), pageBadgeColor, pageTextColor)
						}),
					)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Caption(a.theme, hintText)
					lbl.TextSize = unit.Sp(11)
					lbl.Color = hintColor
					return lbl.Layout(gtx)
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if prev.TotalPages <= 1 {
				return layout.Dimensions{}
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := material.Button(a.theme, &prev.prevPageBtn, "‹")
					btn.Background = color.NRGBA{R: 241, G: 245, B: 249, A: 255}
					btn.Color = color.NRGBA{R: 51, G: 65, B: 85, A: 255}
					btn.TextSize = unit.Sp(14)
					btn.Inset = layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(8), Right: unit.Dp(8)}
					return btn.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Caption(a.theme, fmt.Sprintf("%d / %d", prev.CurrentPageIndex+1, prev.TotalPages))
					lbl.TextSize = unit.Sp(12)
					lbl.Color = color.NRGBA{R: 51, G: 65, B: 85, A: 255}
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := material.Button(a.theme, &prev.nextPageBtn, "›")
					btn.Background = color.NRGBA{R: 241, G: 245, B: 249, A: 255}
					btn.Color = color.NRGBA{R: 51, G: 65, B: 85, A: 255}
					btn.TextSize = unit.Sp(14)
					btn.Inset = layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(8), Right: unit.Dp(8)}
					return btn.Layout(gtx)
				}),
			)
		}),
	)
}

func (a *App) layoutBadge(gtx layout.Context, text string, bg, fg color.NRGBA) layout.Dimensions {
	height := gtx.Dp(unit.Dp(22))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			bounds := image.Rectangle{Max: gtx.Constraints.Max}
			shape := clip.RRect{Rect: bounds, NW: gtx.Dp(unit.Dp(4)), NE: gtx.Dp(unit.Dp(4)), SW: gtx.Dp(unit.Dp(4)), SE: gtx.Dp(unit.Dp(4))}
			paint.FillShape(gtx.Ops, bg, shape.Op(gtx.Ops))
			return layout.Dimensions{Size: bounds.Max}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Caption(a.theme, text)
				lbl.TextSize = unit.Sp(11)
				lbl.Font.Weight = font.Medium
				lbl.Color = fg
				return lbl.Layout(gtx)
			})
		}),
	)
}

func (a *App) layoutPDFExportPaperCanvas(gtx layout.Context) layout.Dimensions {
	prev := &a.pdfExportPreview

	boxHeight := gtx.Constraints.Max.Y
	if boxHeight > gtx.Dp(unit.Dp(250)) {
		boxHeight = gtx.Dp(unit.Dp(250))
	}
	gtx.Constraints.Min.Y = boxHeight
	gtx.Constraints.Max.Y = boxHeight

	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		// Gray background board
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			bounds := image.Rectangle{Max: gtx.Constraints.Max}
			radius := gtx.Dp(unit.Dp(8))
			shape := clip.RRect{Rect: bounds, NW: radius, NE: radius, SW: radius, SE: radius}
			paint.FillShape(gtx.Ops, color.NRGBA{R: 248, G: 250, B: 252, A: 255}, shape.Op(gtx.Ops))
			paint.FillShape(gtx.Ops, color.NRGBA{R: 226, G: 232, B: 240, A: 255}, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
			return layout.Dimensions{Size: bounds.Max}
		}),
		// Centered Miniature Paper Sheet
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			paperW := gtx.Dp(unit.Dp(150))
			paperH := gtx.Dp(unit.Dp(212))
			if prev.Orientation == "landscape" {
				paperW = gtx.Dp(unit.Dp(212))
				paperH = gtx.Dp(unit.Dp(150))
			}
			maxW := gtx.Constraints.Max.X - gtx.Dp(unit.Dp(16))
			maxH := gtx.Constraints.Max.Y - gtx.Dp(unit.Dp(16))
			if maxW > 0 && paperW > maxW {
				paperH = paperH * maxW / paperW
				paperW = maxW
			}
			if maxH > 0 && paperH > maxH {
				paperW = paperW * maxH / paperH
				paperH = maxH
			}

			gtx.Constraints.Min = image.Pt(paperW, paperH)
			gtx.Constraints.Max = image.Pt(paperW, paperH)

			return layout.Stack{Alignment: layout.Center}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					bounds := image.Rectangle{Max: image.Pt(paperW, paperH)}
					radius := gtx.Dp(unit.Dp(3))
					// Subtle shadow
					shadowBounds := bounds.Add(image.Pt(0, gtx.Dp(unit.Dp(2))))
					paint.FillShape(gtx.Ops, color.NRGBA{R: 30, G: 41, B: 59, A: 20}, clip.RRect{Rect: shadowBounds, NW: radius, NE: radius, SW: radius, SE: radius}.Op(gtx.Ops))
					// White Paper
					paperShape := clip.RRect{Rect: bounds, NW: radius, NE: radius, SW: radius, SE: radius}
					paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, paperShape.Op(gtx.Ops))
					paint.FillShape(gtx.Ops, color.NRGBA{R: 203, G: 213, B: 225, A: 255}, clip.Stroke{Path: paperShape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
					return layout.Dimensions{Size: bounds.Max}
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						if prev.Format == "spreadsheet" {
							return a.layoutMiniatureSpreadsheetPaper(gtx)
						}
						return a.layoutMiniatureWordPaper(gtx)
					})
				}),
			)
		}),
	)
}

func (a *App) layoutMiniatureSpreadsheetPaper(gtx layout.Context) layout.Dimensions {
	prev := &a.pdfExportPreview
	size := gtx.Constraints.Max

	p := prev.sheetPagination
	colBandIdx := 0
	rowBandIdx := 0
	if len(p.ColBands) > 0 && len(p.RowBands) > 0 {
		numRowBands := len(p.RowBands)
		bandIdx := prev.CurrentPageIndex
		colBandIdx = bandIdx / numRowBands
		rowBandIdx = bandIdx % numRowBands
		if colBandIdx >= len(p.ColBands) {
			colBandIdx = len(p.ColBands) - 1
		}
		if rowBandIdx >= len(p.RowBands) {
			rowBandIdx = len(p.RowBands) - 1
		}
	}

	startCol, endCol := 0, len(p.ColWidths)
	if colBandIdx < len(p.ColBands) {
		startCol = p.ColBands[colBandIdx].Start
		endCol = p.ColBands[colBandIdx].End
	}
	visibleColCount := endCol - startCol
	if visibleColCount <= 0 {
		visibleColCount = 1
	}

	startRow, endRow := 0, len(p.RowHeights)
	if rowBandIdx < len(p.RowBands) {
		startRow = p.RowBands[rowBandIdx].Start
		endRow = p.RowBands[rowBandIdx].End
	}
	visibleRowCount := endRow - startRow
	if visibleRowCount <= 0 {
		visibleRowCount = 1
	}
	if visibleRowCount > 12 {
		visibleRowCount = 12
	}

	colW := float64(size.X) / float64(visibleColCount)
	rowH := float64(size.Y) / float64(visibleRowCount)
	if rowH > float64(gtx.Dp(unit.Dp(14))) {
		rowH = float64(gtx.Dp(unit.Dp(14)))
	}

	// 1. Draw header background
	headerRect := image.Rect(0, 0, size.X, int(rowH))
	paint.FillShape(gtx.Ops, color.NRGBA{R: 219, G: 234, B: 254, A: 255}, clip.Rect{Max: headerRect.Max}.Op())

	// 2. Draw horizontal grid lines
	for r := 0; r <= visibleRowCount; r++ {
		y := int(float64(r) * rowH)
		if y > size.Y {
			y = size.Y
		}
		lineRect := image.Rect(0, y, size.X, y+1)
		paint.FillShape(gtx.Ops, color.NRGBA{R: 226, G: 232, B: 240, A: 255}, clip.Rect{Min: lineRect.Min, Max: lineRect.Max}.Op())
	}

	// 3. Draw vertical grid lines
	for c := 0; c <= visibleColCount; c++ {
		x := int(float64(c) * colW)
		if x > size.X {
			x = size.X
		}
		totalH := int(float64(visibleRowCount) * rowH)
		lineRect := image.Rect(x, 0, x+1, totalH)
		paint.FillShape(gtx.Ops, color.NRGBA{R: 226, G: 232, B: 240, A: 255}, clip.Rect{Min: lineRect.Min, Max: lineRect.Max}.Op())
	}

	// 4. Subtle data bars inside cells
	for r := 0; r < visibleRowCount; r++ {
		for c := 0; c < visibleColCount; c++ {
			x := int(float64(c)*colW) + gtx.Dp(unit.Dp(2))
			y := int(float64(r)*rowH) + int(rowH/2) - 1
			w := int(colW*0.6) - gtx.Dp(unit.Dp(4))
			if w < 4 {
				w = 4
			}
			barColor := color.NRGBA{R: 203, G: 213, B: 225, A: 255}
			if r == 0 {
				barColor = color.NRGBA{R: 147, G: 197, B: 253, A: 255}
			}
			paint.FillShape(gtx.Ops, barColor, clip.Rect{Min: image.Pt(x, y), Max: image.Pt(x+w, y+2)}.Op())
		}
	}

	return layout.Dimensions{Size: size}
}

func (a *App) layoutMiniatureWordPaper(gtx layout.Context) layout.Dimensions {
	prev := &a.pdfExportPreview
	size := gtx.Constraints.Max

	if prev.wordResult == nil || len(prev.wordResult.Pages) == 0 {
		// Draw default mock text lines
		return a.layoutMockWordLines(gtx, size)
	}

	pageIdx := prev.CurrentPageIndex
	if pageIdx >= len(prev.wordResult.Pages) {
		pageIdx = len(prev.wordResult.Pages) - 1
	}
	pageScene := prev.wordResult.Pages[pageIdx]

	clipW := pageScene.Clip.WidthPT
	clipH := pageScene.Clip.HeightPT
	if clipW <= 0 {
		clipW = 595.28
	}
	if clipH <= 0 {
		clipH = 841.89
	}

	scaleX := float64(size.X) / clipW
	scaleY := float64(size.Y) / clipH

	// Draw text lines
	lineColor := color.NRGBA{R: 148, G: 163, B: 184, A: 255}
	for _, f := range pageScene.Text {
		x := int(f.XPT * scaleX)
		y := int(f.BaselineYPT*scaleY) - 1
		w := int(float64(len([]rune(f.Text))) * 6.0 * scaleX)
		if w < 6 {
			w = 6
		}
		if x+w > size.X {
			w = size.X - x
		}
		if y >= 0 && y+2 <= size.Y && w > 0 {
			paint.FillShape(gtx.Ops, lineColor, clip.Rect{Min: image.Pt(x, y), Max: image.Pt(x+w, y+2)}.Op())
		}
	}

	// Draw image placements
	imgColor := color.NRGBA{R: 191, G: 219, B: 254, A: 255}
	for _, img := range pageScene.Images {
		x := int(img.Bounds.XPT * scaleX)
		y := int(img.Bounds.YPT * scaleY)
		w := int(img.Bounds.WidthPT * scaleX)
		h := int(img.Bounds.HeightPT * scaleY)
		if x >= 0 && y >= 0 && w > 0 && h > 0 {
			paint.FillShape(gtx.Ops, imgColor, clip.Rect{Min: image.Pt(x, y), Max: image.Pt(x+w, y+h)}.Op())
		}
	}

	return layout.Dimensions{Size: size}
}

func (a *App) layoutMockWordLines(gtx layout.Context, size image.Point) layout.Dimensions {
	lineColor := color.NRGBA{R: 148, G: 163, B: 184, A: 255}
	// Title
	paint.FillShape(gtx.Ops, color.NRGBA{R: 71, G: 85, B: 105, A: 255}, clip.Rect{Min: image.Pt(size.X/4, gtx.Dp(unit.Dp(6))), Max: image.Pt(size.X*3/4, gtx.Dp(unit.Dp(10)))}.Op())
	// Body paragraphs
	y := gtx.Dp(unit.Dp(16))
	step := gtx.Dp(unit.Dp(6))
	for y < size.Y-gtx.Dp(unit.Dp(10)) {
		paint.FillShape(gtx.Ops, lineColor, clip.Rect{Min: image.Pt(gtx.Dp(unit.Dp(4)), y), Max: image.Pt(size.X-gtx.Dp(unit.Dp(4)), y+2)}.Op())
		y += step
	}
	return layout.Dimensions{Size: size}
}
