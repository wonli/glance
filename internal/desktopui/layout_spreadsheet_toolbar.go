package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"strconv"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
)

type spreadsheetToolbarTab uint8

const (
	toolbarTabFile spreadsheetToolbarTab = iota
	toolbarTabHome
	toolbarTabInsert
	toolbarTabPageLayout
	toolbarTabFormula
	toolbarTabData
	toolbarTabReview
	toolbarTabView
	toolbarTabPicture
)

const (
	toolbarActionFormatPainter = 29 + iota
	toolbarActionFontFamily
	toolbarActionAlignLeft
	toolbarActionAlignRight
	toolbarActionWrapText
	toolbarActionPercentFormat
	toolbarActionConditionalFormat
	toolbarActionTableFormat
	toolbarActionSum
	toolbarActionFind
)

const (
	toolbarActionInsertTable = 39 + iota
	toolbarActionPivotTable
	toolbarActionInsertShapes
	toolbarActionInsertColumnChart
	toolbarActionInsertLineChart
	toolbarActionInsertPieChart
	toolbarActionInsertLink
	toolbarActionInsertTextBox
	toolbarActionLayoutMargins
	toolbarActionLayoutOrientation
	toolbarActionLayoutSize
	toolbarActionPrintArea
	toolbarActionLayoutGridlines
	toolbarActionHeaderRow
	toolbarActionInsertFunction
	toolbarActionRecentFunctions
	toolbarActionLogicalFunctions
	toolbarActionTextFunctions
	toolbarActionDateTimeFunctions
	toolbarActionNameManager
	toolbarActionRecalculate
	toolbarActionRefreshAll
	toolbarActionTextToColumns
	toolbarActionFlashFill
	toolbarActionRemoveDuplicates
	toolbarActionDataValidation
	toolbarActionSpellCheck
	toolbarActionTranslate
	toolbarActionNewComment
	toolbarActionDeleteComment
	toolbarActionProtectSheet
	toolbarActionProtectWorkbook
	toolbarActionNormalView
	toolbarActionPageBreakPreview
	toolbarActionRuler
	toolbarActionViewGridlines
	toolbarActionZoomIn
	toolbarActionZoom100
	toolbarActionFreezePanes
	toolbarActionOpenFile
	toolbarActionSaveAs
	toolbarActionPrint
	toolbarActionExport
	toolbarActionShare
	toolbarActionLast = toolbarActionShare
)

var baseToolbarCategoryLabels = []string{"文件", "开始", "插入", "页面布局", "公式", "数据", "审阅", "视图"}

func toolbarCategoryLabelsForState(state *documentSessionState) []string {
	labels := append([]string(nil), baseToolbarCategoryLabels...)
	if state != nil && state.selectedGridPictureID != "" {
		labels = append(labels, "图片")
	}
	return labels
}

// homeRibbonActions follows the Excel Pen reference's Start category.
var homeRibbonActions = []spreadsheetToolbarAction{
	{slot: 3, label: "剪切", group: "剪贴板", icon: "scissors"},
	{slot: 2, label: "复制", group: "剪贴板", icon: "copy"},
	{slot: 21, label: "粘贴", group: "剪贴板", icon: "clipboard-paste"},
	{slot: toolbarActionFormatPainter, label: "格式刷", group: "剪贴板", icon: "paintbrush"},
	{slot: toolbarActionFontFamily, label: "字体", group: "字体", icon: "type"},
	{slot: 4, label: "加粗", group: "字体", icon: "bold"},
	{slot: 5, label: "斜体", group: "字体", icon: "italic"},
	{slot: 6, label: "下划线", group: "字体", icon: "underline"},
	{slot: toolbarActionAlignLeft, label: "左对齐", group: "对齐", icon: "format_align_left"},
	{slot: 9, label: "居中", group: "对齐", icon: "format_align_center"},
	{slot: toolbarActionAlignRight, label: "右对齐", group: "对齐", icon: "format_align_right"},
	{slot: toolbarActionWrapText, label: "自动换行", group: "对齐", icon: "text-wrap"},
	{slot: toolbarActionPercentFormat, label: "百分比", group: "格式", icon: "percent"},
	{slot: 10, label: "数字格式", group: "格式", icon: "hash"},
	{slot: 12, label: "插入", group: "单元格", icon: "square-plus"},
	{slot: 13, label: "删除", group: "单元格", icon: "trash-2"},
	{slot: toolbarActionConditionalFormat, label: "条件格式", group: "单元格", icon: "sparkles"},
	{slot: toolbarActionTableFormat, label: "表格格式", group: "单元格", icon: "table"},
	{slot: toolbarActionSum, label: "求和", group: "编辑", icon: "sigma"},
	{slot: 18, label: "排序", group: "编辑", icon: "arrow-up-down"},
	{slot: 20, label: "筛选", group: "编辑", icon: "list-filter"},
	{slot: toolbarActionFind, label: "查找", group: "编辑", icon: "search"},
	{slot: 0, label: "撤销", group: "历史", icon: "undo-2"},
	{slot: 1, label: "重做", group: "历史", icon: "redo-2"},
}

var fileToolbarActions = []spreadsheetToolbarAction{
	{slot: toolbarActionOpenFile, label: "打开", group: "文件", icon: "folder-open"},
	{slot: 17, label: "保存", group: "文件", icon: "save"},
	{slot: toolbarActionSaveAs, label: "另存为", group: "文件", icon: "file-down"},
	{slot: toolbarActionPrint, label: "打印", group: "文件", icon: "printer"},
	{slot: toolbarActionExport, label: "导出", group: "文件", icon: "file-output"},
	{slot: toolbarActionShare, label: "共享", group: "文件", icon: "share-2"},
}

var insertToolbarActions = []spreadsheetToolbarAction{
	{slot: toolbarActionInsertTable, label: "表格", group: "插入", icon: "table"},
	{slot: toolbarActionPivotTable, label: "数据透视表", group: "插入", icon: "layout-grid"},
	{slot: 25, label: "图片", group: "插入", icon: "image"},
	{slot: toolbarActionInsertShapes, label: "形状", group: "插入", icon: "shapes"},
	{slot: toolbarActionInsertColumnChart, label: "柱状图", group: "图表", icon: "chart-column"},
	{slot: toolbarActionInsertLineChart, label: "折线图", group: "图表", icon: "route"},
	{slot: toolbarActionInsertPieChart, label: "饼图", group: "图表", icon: "chart-pie"},
	{slot: toolbarActionInsertLink, label: "链接", group: "链接", icon: "link"},
	{slot: toolbarActionInsertTextBox, label: "文本框", group: "文本", icon: "text-cursor-input"},
}

var pageLayoutToolbarActions = []spreadsheetToolbarAction{
	{slot: toolbarActionLayoutMargins, label: "页边距", group: "页面设置", icon: "panel-left"},
	{slot: toolbarActionLayoutOrientation, label: "纸张方向", group: "页面设置", icon: "rotate-cw"},
	{slot: toolbarActionLayoutSize, label: "纸张大小", group: "页面设置", icon: "file"},
	{slot: toolbarActionPrintArea, label: "打印区域", group: "页面设置", icon: "scan"},
	{slot: toolbarActionLayoutGridlines, label: "网格线", group: "页面设置", icon: "printer-check"},
	{slot: toolbarActionHeaderRow, label: "标题行", group: "页面设置", icon: "heading"},
}

var formulaToolbarActions = []spreadsheetToolbarAction{
	{slot: toolbarActionInsertFunction, label: "插入函数", group: "函数库", icon: "sigma"},
	{slot: toolbarActionSum, label: "自动求和", group: "函数库", icon: "sigma"},
	{slot: toolbarActionRecentFunctions, label: "最近使用", group: "函数库", icon: "clock-3"},
	{slot: toolbarActionLogicalFunctions, label: "逻辑函数", group: "函数库", icon: "git-compare-arrows"},
	{slot: toolbarActionTextFunctions, label: "文本函数", group: "函数库", icon: "type"},
	{slot: toolbarActionDateTimeFunctions, label: "日期时间", group: "函数库", icon: "clock-3"},
	{slot: toolbarActionNameManager, label: "名称管理器", group: "定义名称", icon: "tag"},
	{slot: toolbarActionRecalculate, label: "重新计算", group: "计算", icon: "rotate-cw"},
}

var dataToolbarActions = []spreadsheetToolbarAction{
	{slot: toolbarActionRefreshAll, label: "刷新全部", group: "数据工具", icon: "rotate-cw"},
	{slot: 18, label: "排序", group: "排序和筛选", icon: "arrow-up-down"},
	{slot: 20, label: "筛选", group: "排序和筛选", icon: "list-filter"},
	{slot: toolbarActionTextToColumns, label: "分列", group: "数据工具", icon: "columns-3"},
	{slot: toolbarActionFlashFill, label: "快速填充", group: "数据工具", icon: "wand-sparkles"},
	{slot: toolbarActionRemoveDuplicates, label: "删除重复项", group: "数据工具", icon: "trash-2"},
	{slot: toolbarActionDataValidation, label: "数据验证", group: "数据工具", icon: "circle-check"},
}

var reviewToolbarActions = []spreadsheetToolbarAction{
	{slot: toolbarActionSpellCheck, label: "拼写检查", group: "校对", icon: "spell-check-2"},
	{slot: toolbarActionTranslate, label: "翻译", group: "校对", icon: "languages"},
	{slot: toolbarActionNewComment, label: "新建批注", group: "批注", icon: "message-square-plus"},
	{slot: toolbarActionDeleteComment, label: "删除批注", group: "批注", icon: "message-square-x"},
	{slot: toolbarActionProtectSheet, label: "保护工作表", group: "保护", icon: "lock-keyhole"},
	{slot: toolbarActionProtectWorkbook, label: "保护工作簿", group: "保护", icon: "shield-check"},
}

var viewToolbarActions = []spreadsheetToolbarAction{
	{slot: toolbarActionNormalView, label: "普通视图", group: "工作簿视图", icon: "layout-grid"},
	{slot: toolbarActionPageBreakPreview, label: "分页预览", group: "工作簿视图", icon: "file-spreadsheet"},
	{slot: toolbarActionRuler, label: "标尺", group: "显示", icon: "ruler"},
	{slot: toolbarActionViewGridlines, label: "网格线", group: "显示", icon: "grid-3x3"},
	{slot: toolbarActionZoomIn, label: "放大", group: "缩放", icon: "zoom-in"},
	{slot: toolbarActionZoom100, label: "100%显示", group: "缩放", icon: "maximize"},
	{slot: toolbarActionFreezePanes, label: "冻结窗格", group: "窗口", icon: "panels-top-left"},
}

// Keep contextual picture actions available after a picture is selected.
var pictureToolbarActions = []spreadsheetToolbarAction{
	{slot: 26, label: "替换图片", group: "图片", icon: "file-output"},
	{slot: 27, label: "删除图片", group: "图片", icon: "trash-2"},
	{slot: 28, label: "设置宽度", group: "尺寸", icon: "settings-2"},
}

func (a *App) spreadsheetToolbarVisible() bool {
	if a == nil || a.book == nil || !a.book.Editable() {
		return false
	}
	_, ok := a.book.(document.GridReader)
	return ok
}

func (a *App) handleSpreadsheetToolbarEvents(gtx layout.Context) {
	state := a.documentSessionState
	if state == nil || a.modal.Visible {
		return
	}
	labels := toolbarCategoryLabelsForState(state)
	for index := 0; index < len(labels) && index < len(state.toolbarTabs); index++ {
		if state.toolbarTabs[index].Clicked(gtx) {
			state.toolbarTab = spreadsheetToolbarTab(index)
			state.toolbarPicker = pickerNone
			state.toolbarActionList.List.Position = layout.Position{}
			break
		}
	}
	actions := toolbarActionsForTab(state.toolbarTab)
	for _, action := range actions {
		if action.slot < 0 || !state.toolbarActionButtons[action.slot].Clicked(gtx) {
			continue
		}
		if unimplementedSpreadsheetToolbarAction(action.slot) {
			a.showSpreadsheetFeatureMissing(action.label)
			continue
		}
		if !a.toolbarActionEnabled(action.slot) {
			continue
		}
		if picker := pickerForToolbarAction(action.slot); picker != pickerNone {
			if state.toolbarPicker == picker {
				state.toolbarPicker = pickerNone
			} else {
				state.toolbarPicker = picker
				state.toolbarPickerSlot = action.slot
			}
			continue
		}
		state.toolbarPicker = pickerNone
		if action.slot == 0 || action.slot == 1 {
			a.applyHistory(action.slot == 0)
			continue
		}
		if action.slot == 2 || action.slot == 3 {
			text, err := a.copySelection()
			if err == nil {
				gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(text))})
				if action.slot == 3 {
					a.clearSelectionAsync()
				}
			} else {
				a.status = "剪贴板操作失败：" + err.Error()
			}
			continue
		}
		if action.slot == 21 {
			gtx.Execute(clipboard.ReadCmd{Tag: &state.clipboardTarget})
			continue
		}
		if action.slot == 18 || action.slot == 19 {
			bounds := state.selection.Bounds()
			a.applySort(document.SortRequest{Sheet: state.sheet, Bounds: bounds, KeyColumn: state.selection.Active.Column + 1, Descending: action.slot == 19})
			continue
		}
		if action.slot == 20 {
			a.toggleValueFilter()
			continue
		}
		if action.slot == 17 {
			a.startSave()
			continue
		}
		if action.slot == toolbarActionOpenFile {
			a.startOpenFilePicker()
			continue
		}
		if action.slot == toolbarActionSaveAs {
			a.startSaveAsPicker("xlsx")
			continue
		}
		if action.slot == toolbarActionExport {
			a.requestSpreadsheetPDFExport()
			continue
		}
		if action.slot >= 12 && action.slot <= 15 {
			pos := state.selection.Active
			kind := []document.StructureEditKind{document.StructureInsertRows, document.StructureDeleteRows, document.StructureInsertColumns, document.StructureDeleteColumns}[action.slot-12]
			index := pos.Row + 1
			if action.slot >= 14 {
				index = pos.Column + 1
			}
			a.applyStructure(document.StructureEdit{Kind: kind, Sheet: state.sheet, Index: index, Count: 1})
			continue
		}
		if action.slot == 22 {
			a.applyStructure(document.StructureEdit{Kind: document.StructureCreateSheet})
			continue
		}
		if action.slot == 23 {
			a.modal.ShowInput("重命名工作表", "输入新的工作表名称", state.sheet, "工作表名称", func(action string) {
				a.modal.Hide()
				if action != "confirm" {
					return
				}
				name := strings.TrimSpace(a.modal.Input.Text())
				if name == "" {
					a.status = "工作表名称不能为空"
					a.window.Invalidate()
					return
				}
				a.applyStructure(document.StructureEdit{Kind: document.StructureRenameSheet, Sheet: state.sheet, NewName: name})
			}, modalDialogActionSpec{ID: "confirm", Label: "重命名"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
			continue
		}
		if action.slot == 24 {
			a.applyStructure(document.StructureEdit{Kind: document.StructureDeleteSheet, Sheet: state.sheet})
			continue
		}
		switch action.slot {
		case 25:
			a.startSpreadsheetImagePicker(spreadsheetImageInsert)
			continue
		case 26:
			a.startSpreadsheetImagePicker(spreadsheetImageReplace)
			continue
		case 27:
			if picture, ok := gridPictureByID(activeGridPictures(state, state.sheet), state.selectedGridPictureID); ok {
				a.startGridPictureCommand(state, gridPictureCommandRequest{operation: "删除图片", kind: gridPictureCommandDelete, imageID: picture.ID})
			}
			continue
		case 28:
			if picture, ok := gridPictureByID(activeGridPictures(state, state.sheet), state.selectedGridPictureID); ok {
				a.modal.ShowInput("设置图片宽度", "输入图片宽度（像素）", fmt.Sprint(picture.WidthPx), "1–10000", func(action string) {
					draft := strings.TrimSpace(a.modal.Input.Text())
					a.modal.Hide()
					if action != "confirm" {
						return
					}
					width, err := strconv.Atoi(draft)
					if err != nil || width < 1 || width > 10000 {
						a.status = "图片宽度必须是 1 到 10000 之间的整数"
						if a.window != nil {
							a.window.Invalidate()
						}
						return
					}
					a.startGridPictureCommand(state, gridPictureCommandRequest{operation: "调整图片尺寸", kind: gridPictureCommandResize, imageID: picture.ID, widthPx: width})
				}, modalDialogActionSpec{ID: "confirm", Label: "确定"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
			}
			continue
		}
		if action.slot == toolbarActionSum {
			a.applySpreadsheetAutoSum()
			continue
		}
		if action.slot == toolbarActionFind {
			a.showSpreadsheetFindDialog()
			continue
		}
		style := a.selectedCellStyle()
		if patch, ok := formatPatchForToolbarAction(action.slot, style); ok {
			a.applyCellFormat(patch)
			continue
		}
	}
	if state.toolbarPicker != pickerNone {
		for index := range state.toolbarPickerButtons {
			if state.toolbarPickerButtons[index].Clicked(gtx) {
				if patch, ok := toolbarPickerPatch(state.toolbarPicker, index); ok {
					a.applyCellFormat(patch)
				}
				state.toolbarPicker = pickerNone
				break
			}
		}
	}
}

func toolbarActionsForTab(tab spreadsheetToolbarTab) []spreadsheetToolbarAction {
	switch tab {
	case toolbarTabFile:
		return fileToolbarActions
	case toolbarTabHome:
		return homeRibbonActions
	case toolbarTabInsert:
		return insertToolbarActions
	case toolbarTabPageLayout:
		return pageLayoutToolbarActions
	case toolbarTabFormula:
		return formulaToolbarActions
	case toolbarTabPicture:
		return pictureToolbarActions
	case toolbarTabData:
		return dataToolbarActions
	case toolbarTabReview:
		return reviewToolbarActions
	case toolbarTabView:
		return viewToolbarActions
	default:
		return nil
	}
}

func unimplementedSpreadsheetToolbarTab(tab spreadsheetToolbarTab) (string, bool) {
	return "", false
}

func unimplementedSpreadsheetToolbarAction(slot int) bool {
	if slot < toolbarActionFormatPainter || slot > toolbarActionLast {
		return false
	}
	switch slot {
	case toolbarActionOpenFile, toolbarActionSaveAs, toolbarActionExport,
		toolbarActionAlignLeft, toolbarActionAlignRight, toolbarActionWrapText, toolbarActionPercentFormat,
		toolbarActionSum, toolbarActionFind:
		return false
	default:
		return true
	}
}

func (a *App) showSpreadsheetFeatureMissing(name string) {
	a.status = fmt.Sprintf("“%s”功能暂未实现", name)
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) toolbarActionEnabled(slot int) bool {
	state := a.documentSessionState
	if state == nil || slot < 0 || slot >= len(state.toolbarActionButtons) {
		return false
	}
	if unimplementedSpreadsheetToolbarAction(slot) {
		return state.book != nil && !state.formatRunning.Load() && !a.isSaving()
	}
	switch slot {
	case 0, 1:
		_, ok := state.book.(document.SpreadsheetHistory)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case 4, 5, 6, 7, 8, 9, 10, 11, 16,
		toolbarActionAlignLeft, toolbarActionAlignRight, toolbarActionWrapText, toolbarActionPercentFormat:
		_, ok := state.book.(document.SpreadsheetFormatter)
		return ok && state.selectionSet && selectionCellCount(state.selection.Bounds()) <= maxClipboardCellsUI && !state.cellEdit.Editing && !state.formatRunning.Load() && !a.isSaving()
	case toolbarActionSum:
		_, reader := state.book.(document.SpreadsheetRangeReader)
		_, editor := state.book.(document.SpreadsheetBatchEditor)
		return reader && editor && state.selectionSet && !state.formatRunning.Load() && !a.isSaving()
	case toolbarActionFind:
		_, ok := state.book.(document.SpreadsheetSearcher)
		return ok && !state.formatRunning.Load()
	case 2, 3:
		_, reader := state.book.(document.SpreadsheetRangeReader)
		_, editor := state.book.(document.SpreadsheetBatchEditor)
		return reader && editor && state.selectionSet && selectionCellCount(state.selection.Bounds()) <= maxClipboardCellsUI && !state.formatRunning.Load()
	case 21:
		_, ok := state.book.(document.SpreadsheetBatchEditor)
		return ok && state.selectionSet && !state.formatRunning.Load()
	case 18, 19:
		_, ok := state.book.(document.SpreadsheetSorter)
		bounds := state.selection.Bounds()
		return ok && state.selectionSet && bounds.EndRow > bounds.StartRow && selectionCellCount(bounds) <= maxClipboardCellsUI && !state.formatRunning.Load() && !a.isSaving()
	case 20:
		_, ok := state.book.(document.SpreadsheetRangeReader)
		return state.valueFilter != nil || (ok && state.selectionSet && state.selection.Bounds().EndRow > state.selection.Bounds().StartRow && selectionCellCount(state.selection.Bounds()) <= maxClipboardCellsUI && !state.formatRunning.Load())
	case 12, 13, 14, 15, 22, 23, 24:
		_, ok := state.book.(document.SpreadsheetStructureEditor)
		if slot == 24 && len(state.sheets) <= 1 {
			return false
		}
		return ok && !state.formatRunning.Load() && !a.isSaving() && ((slot < 22) == state.selectionSet || slot >= 22)
	case 25:
		_, ok := state.book.(document.SpreadsheetImageEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving() && !a.xlsxImagePickerRunning
	case 26, 27, 28:
		editor, editorOK := state.book.(document.SpreadsheetImageEditor)
		picture, selected := gridPictureByID(activeGridPictures(state, state.sheet), state.selectedGridPictureID)
		return editorOK && editor != nil && selected && picture.Editable && !state.formatRunning.Load() && !a.isSaving() && !a.xlsxImagePickerRunning
	case 17:
		return state.book != nil && sessionDirty(state) && !a.isSaving() && sessionPath(state) != ""
	case toolbarActionOpenFile:
		return !state.formatRunning.Load()
	case toolbarActionSaveAs:
		return state.book != nil && !a.isSaving()
	case toolbarActionExport:
		return state.book != nil && !state.formatRunning.Load() && !a.isSaving() && !state.spreadsheetPDFExportRunning
	default:
		return false
	}
}

func selectionCellCount(bounds document.CellRange) int {
	return (bounds.EndRow - bounds.StartRow + 1) * (bounds.EndColumn - bounds.StartColumn + 1)
}

func (a *App) selectedCellStyle() document.CellStyle {
	state := a.documentSessionState
	if state == nil || state.virtualGrid == nil || !state.selectionSet {
		return document.CellStyle{}
	}
	pos := state.selection.Active
	return windowCell(state.virtualGrid.Window, pos.Row+1, pos.Column+1).Style
}

func (a *App) layoutSpreadsheetToolbar(gtx layout.Context) layout.Dimensions {
	if !a.spreadsheetToolbarVisible() {
		return layout.Dimensions{}
	}
	height := gtx.Dp(unit.Dp(102))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	state := a.documentSessionState
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, contentSurface, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutRibbonCategoryTabs(gtx, a.theme, toolbarCategoryLabelsForState(state), state.toolbarTabs[:], int(state.toolbarTab))
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					stripHeight := gtx.Dp(unit.Dp(68))
					gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = stripHeight, stripHeight
					actions := toolbarActionsForTab(state.toolbarTab)
					iconSize := toolbarIconSize
					return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return state.toolbarActionList.Layout(gtx, len(actions), func(gtx layout.Context, index int) layout.Dimensions {
							action := actions[index]
							style := a.selectedCellStyle()
							mixed := action.slot >= 0 && formatStyleSlot(action.slot) && a.selectionFormatMixed(action.slot)
							active := action.slot >= 0 && !mixed && formatStyleActive(action.slot, style)
							enabled := action.slot >= 0 && a.toolbarActionEnabled(action.slot)
							var button *widget.Clickable
							if action.slot >= 0 {
								button = &state.toolbarActionButtons[action.slot]
							}
							iconColor, background := toolbarIconColor(true), contentSurface
							if active {
								background = toolbarActiveBackground
							}
							if mixed {
								background = mixedToolbarBackground
							}
							if !enabled {
								background = contentSurface
							}
							icon, _ := tintedToolbarIcon(action.icon, iconColor)
							return layoutToolbarActionItem(gtx, action, index, actions, func(gtx layout.Context) layout.Dimensions {
								btnGtx := gtx
								btnGtx.Constraints.Min = image.Pt(gtx.Dp(excelToolbarButtonWidth), gtx.Dp(excelToolbarButtonHeight))
								btnGtx.Constraints.Max = btnGtx.Constraints.Min
								return toolbarIconButton(btnGtx, a.theme, button, icon, iconSize, action.label, action.label, enabled, mixed, background)
							})
						})
					})
				}),
			)
		},
	)
}

func (a *App) layoutSpreadsheetToolbarPickerOverlay(gtx layout.Context) layout.Dimensions {
	state := a.documentSessionState
	if state == nil || state.toolbarPicker == pickerNone {
		return layout.Dimensions{}
	}
	options := toolbarPickerOptions(state.toolbarPicker)
	if len(options) == 0 {
		return layout.Dimensions{}
	}
	actions := toolbarActionsForTab(state.toolbarTab)
	panelWidth := gtx.Dp(unit.Dp(276))
	if panelWidth > gtx.Constraints.Max.X {
		panelWidth = gtx.Constraints.Max.X
	}
	normalWidth := gtx.Dp(unit.Dp(51))
	groupWidth := gtx.Dp(unit.Dp(55))
	panelX := toolbarPickerPanelX(state.toolbarPickerSlot, actions, state.toolbarActionList.List.Position, normalWidth, groupWidth, gtx.Constraints.Max.X, panelWidth)
	panelX += gtx.Dp(unit.Dp(8))
	maxX := gtx.Constraints.Max.X - panelWidth
	if maxX < 0 {
		maxX = 0
	}
	if panelX > maxX {
		panelX = maxX
	}
	left := unit.Dp(0)
	if gtx.Metric.PxPerDp > 0 {
		left = unit.Dp(float32(panelX) / gtx.Metric.PxPerDp)
	}
	return layout.Inset{Left: left, Top: unit.Dp(106)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		width := panelWidth
		if width > gtx.Constraints.Max.X {
			width = gtx.Constraints.Max.X
		}
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
		return a.layoutToolbarPickerPanel(gtx, options)
	})
}

func (a *App) layoutToolbarPickerPanel(gtx layout.Context, options []string) layout.Dimensions {
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			bounds := image.Rectangle{Max: gtx.Constraints.Min}
			shadowBounds := bounds.Add(image.Pt(1, 2))
			paint.FillShape(gtx.Ops, color.NRGBA{R: 28, G: 38, B: 54, A: 28}, clip.UniformRRect(shadowBounds, gtx.Dp(unit.Dp(9))).Op(gtx.Ops))
			paint.FillShape(gtx.Ops, gridLine, clip.UniformRRect(bounds, gtx.Dp(unit.Dp(9))).Op(gtx.Ops))
			inner := bounds.Inset(1)
			paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.UniformRRect(inner, gtx.Dp(unit.Dp(8))).Op(gtx.Ops))
			return layout.Dimensions{Size: bounds.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Top: unit.Dp(6), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				rows := make([]layout.FlexChild, 0, (len(options)+1)/2+2)
				title := material.Body2(a.theme, toolbarPickerTitle(a.documentSessionState.toolbarPicker))
				title.Color = mutedText
				rows = append(rows, layout.Rigid(title.Layout))
				rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(0, gtx.Dp(unit.Dp(4)))}
				}))
				for row := 0; row < (len(options)+1)/2; row++ {
					rowIndex := row
					rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						children := make([]layout.FlexChild, 0, 2)
						for column := 0; column < 2; column++ {
							index := rowIndex*2 + column
							if index >= len(options) {
								children = append(children, layout.Flexed(1, func(layout.Context) layout.Dimensions { return layout.Dimensions{} }))
								continue
							}
							children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								button := material.Button(a.theme, &a.documentSessionState.toolbarPickerButtons[index], options[index])
								button.TextSize = a.theme.TextSize * 12 / 16
								button.Background = contentSurface
								button.Color = mutedText
								button.CornerRadius = 6
								button.Inset = layout.Inset{Left: unit.Dp(5), Right: unit.Dp(5), Top: unit.Dp(4), Bottom: unit.Dp(4)}
								return layout.Inset{Left: unit.Dp(2), Right: unit.Dp(2), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, button.Layout)
							}))
						}
						return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
					}))
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
			})
		},
	)
}

func toolbarPickerTitle(picker spreadsheetToolbarPicker) string {
	switch picker {
	case pickerFontColor:
		return "字体颜色"
	case pickerFillColor:
		return "填充颜色"
	case pickerAlignment:
		return "对齐方式"
	case pickerNumberFormat:
		return "数字格式"
	case pickerBorder:
		return "单元格边框"
	default:
		return ""
	}
}

func formatStyleSlot(slot int) bool {
	return (slot >= 4 && slot <= 11) ||
		slot == toolbarActionAlignLeft ||
		slot == toolbarActionAlignRight ||
		slot == toolbarActionWrapText ||
		slot == toolbarActionPercentFormat
}

func formatStyleActive(slot int, style document.CellStyle) bool {
	switch slot {
	case 4:
		return style.Bold
	case 5:
		return style.Italic
	case 6:
		return style.Underline
	case 7:
		return style.FontColor != ""
	case 8:
		return style.FillColor != ""
	case 9:
		return style.HorizontalAlignment == "center"
	case toolbarActionAlignLeft:
		return style.HorizontalAlignment == "left"
	case toolbarActionAlignRight:
		return style.HorizontalAlignment == "right"
	case toolbarActionWrapText:
		return style.WrapText
	case toolbarActionPercentFormat:
		return style.NumberFormat == "0%" || style.NumberFormat == "0.00%"
	case 10:
		return style.NumberFormat != "" && style.NumberFormat != "General"
	case 11:
		for _, b := range style.Borders {
			if b.Style != 0 {
				return true
			}
		}
	}
	return false
}

func (a *App) selectionFormatMixed(slot int) bool {
	state := a.documentSessionState
	if state == nil || !state.selectionSet {
		return false
	}
	bounds := state.selection.Bounds()
	v := state.virtualGrid
	if v == nil || v.WindowSheet != state.sheet || !containsRange(v.WindowRange, bounds) {
		return true
	}
	base := windowCell(v.Window, state.selection.Active.Row+1, state.selection.Active.Column+1).Style
	for row := bounds.StartRow; row <= bounds.EndRow; row++ {
		for column := bounds.StartColumn; column <= bounds.EndColumn; column++ {
			current := windowCell(v.Window, row, column).Style
			var equal bool
			switch slot {
			case 4:
				equal = current.Bold == base.Bold
			case 5:
				equal = current.Italic == base.Italic
			case 6:
				equal = current.Underline == base.Underline
			case 7:
				equal = current.FontColor == base.FontColor
			case 8:
				equal = current.FillColor == base.FillColor
			case 9, toolbarActionAlignLeft, toolbarActionAlignRight:
				equal = current.HorizontalAlignment == base.HorizontalAlignment
			case toolbarActionWrapText:
				equal = current.WrapText == base.WrapText
			case 10, toolbarActionPercentFormat:
				equal = current.NumberFormat == base.NumberFormat
			case 11:
				equal = current.Borders == base.Borders
			}
			if !equal {
				return true
			}
		}
	}
	return false
}

type spreadsheetToolbarPicker uint8

const (
	pickerNone spreadsheetToolbarPicker = iota
	pickerFontColor
	pickerFillColor
	pickerAlignment
	pickerNumberFormat
	pickerBorder
)

func pickerForToolbarAction(slot int) spreadsheetToolbarPicker {
	switch slot {
	case 7:
		return pickerFontColor
	case 8:
		return pickerFillColor
	case 9:
		return pickerAlignment
	case 10:
		return pickerNumberFormat
	case 11:
		return pickerBorder
	}
	return pickerNone
}

func formatPatchForToolbarAction(slot int, style document.CellStyle) (document.CellFormatPatch, bool) {
	switch slot {
	case 4:
		v := !style.Bold
		return document.CellFormatPatch{Bold: &v}, true
	case 5:
		v := !style.Italic
		return document.CellFormatPatch{Italic: &v}, true
	case 6:
		v := !style.Underline
		return document.CellFormatPatch{Underline: &v}, true
	case 16:
		return document.CellFormatPatch{ClearFormat: true}, true
	case toolbarActionAlignLeft:
		align := "left"
		if strings.EqualFold(style.HorizontalAlignment, "left") {
			align = "general"
		}
		return document.CellFormatPatch{HorizontalAlignment: &align}, true
	case toolbarActionAlignRight:
		align := "right"
		if strings.EqualFold(style.HorizontalAlignment, "right") {
			align = "general"
		}
		return document.CellFormatPatch{HorizontalAlignment: &align}, true
	case toolbarActionWrapText:
		v := !style.WrapText
		return document.CellFormatPatch{WrapText: &v}, true
	case toolbarActionPercentFormat:
		format := "0.00%"
		if style.NumberFormat == "0.00%" || style.NumberFormat == "0%" {
			format = "General"
		}
		return document.CellFormatPatch{NumberFormat: &format}, true
	default:
		return document.CellFormatPatch{}, false
	}
}

func toolbarPickerOptions(kind spreadsheetToolbarPicker) []string {
	switch kind {
	case pickerFontColor:
		return []string{"黑色", "红色", "橙色", "黄色", "绿色", "蓝色", "紫色", "白色"}
	case pickerFillColor:
		return []string{"无填充", "浅红", "浅橙", "浅黄", "浅绿", "浅蓝", "浅紫", "灰色"}
	case pickerAlignment:
		return []string{"常规", "左对齐", "居中", "右对齐"}
	case pickerNumberFormat:
		return []string{"常规", "整数", "两位小数", "日期"}
	case pickerBorder:
		return []string{"所有边框", "外侧边框", "下框线", "无边框"}
	default:
		return nil
	}
}

func toolbarPickerPatch(kind spreadsheetToolbarPicker, index int) (document.CellFormatPatch, bool) {
	values := map[spreadsheetToolbarPicker][]string{pickerFontColor: {"000000", "F44336", "FF9800", "FFC107", "4CAF50", "2196F3", "9C27B0", "FFFFFF"}, pickerFillColor: {"", "FCE4D6", "FCE4D6", "FFF2CC", "E2F0D9", "DDEBF7", "E4DFEC", "D9E1F2"}, pickerAlignment: {"general", "left", "center", "right"}, pickerNumberFormat: {"General", "0", "0.00", "yyyy-mm-dd"}, pickerBorder: {"all", "outside", "bottom", "none"}}
	items, ok := values[kind]
	if !ok || index < 0 || index >= len(items) {
		return document.CellFormatPatch{}, false
	}
	v := items[index]
	switch kind {
	case pickerFontColor:
		return document.CellFormatPatch{FontColor: &v}, true
	case pickerFillColor:
		return document.CellFormatPatch{FillColor: &v}, true
	case pickerAlignment:
		return document.CellFormatPatch{HorizontalAlignment: &v}, true
	case pickerNumberFormat:
		return document.CellFormatPatch{NumberFormat: &v}, true
	case pickerBorder:
		return document.CellFormatPatch{BorderPreset: &v}, true
	}
	return document.CellFormatPatch{}, false
}
