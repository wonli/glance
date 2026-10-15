package desktopui

import "fmt"

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
)

const (
	toolbarActionFontSize = toolbarActionShare + 1 + iota
	toolbarActionCellStyles
	toolbarActionFormat
	toolbarActionLast = toolbarActionFormat
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
	{slot: toolbarActionFontSize, label: "字号", group: "字体", icon: "type"},
	{slot: 4, label: "加粗", group: "字体", icon: "bold"},
	{slot: 5, label: "斜体", group: "字体", icon: "italic"},
	{slot: 6, label: "下划线", group: "字体", icon: "underline"},
	{slot: toolbarActionAlignLeft, label: "左对齐", group: "对齐", icon: "format_align_left"},
	{slot: 9, label: "居中", group: "对齐", icon: "format_align_center"},
	{slot: toolbarActionAlignRight, label: "右对齐", group: "对齐", icon: "format_align_right"},
	{slot: toolbarActionWrapText, label: "自动换行", group: "对齐", icon: "text-wrap"},
	{slot: toolbarActionPercentFormat, label: "百分比", displayLabel: "%", group: "格式", icon: "percent"},
	{slot: 10, label: "数字格式", displayLabel: "常规", group: "格式", icon: "hash"},
	{slot: 12, label: "插入", group: "单元格", icon: "square-plus"},
	{slot: 13, label: "删除", group: "单元格", icon: "trash-2"},
	{slot: toolbarActionConditionalFormat, label: "条件格式", group: "单元格", icon: "sparkles"},
	{slot: toolbarActionCellStyles, label: "单元格样式", group: "单元格", icon: "grid"},
	{slot: toolbarActionFormat, label: "格式", group: "单元格", icon: "settings-2"},
	{slot: toolbarActionTableFormat, label: "表格格式", group: "单元格", icon: "table"},
	{slot: toolbarActionSum, label: "求和", group: "编辑", icon: "sigma"},
	{slot: 18, label: "排序和筛选", group: "编辑", icon: "list-filter"},
	{slot: 20, label: "筛选", group: "编辑", icon: "list-filter"},
	{slot: toolbarActionFind, label: "查找", group: "编辑", icon: "search"},
	{slot: 0, label: "撤销", group: "历史", icon: "undo-2"},
	{slot: 1, label: "重做", group: "历史", icon: "redo-2"},
}

var fileToolbarActions = []spreadsheetToolbarAction{
	{slot: toolbarActionOpenFile, label: "打开", group: "文件", icon: "folder-open"},
	{slot: 17, label: "保存", group: "文件", icon: "save"},
	{slot: toolbarActionSaveAs, label: "另存为", group: "文件", icon: "file-down"},
	{slot: toolbarActionExport, label: "导出", group: "文件", icon: "file-output"},
	{slot: toolbarActionPrint, label: "打印", group: "文件", icon: "printer"},
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
	{slot: toolbarActionHeaderRow, label: "打印标题", group: "页面设置", icon: "heading"},
	{slot: toolbarActionLayoutGridlines, label: "打印网格线", group: "页面设置", icon: "printer-check"},
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
	{slot: toolbarActionPageBreakPreview, label: "分页符预览", group: "工作簿视图", icon: "file-spreadsheet"},
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
	case toolbarActionOpenFile, toolbarActionSaveAs, toolbarActionPrint, toolbarActionExport, toolbarActionShare,
		toolbarActionFormatPainter, toolbarActionFontFamily, toolbarActionFontSize, toolbarActionCellStyles, toolbarActionFormat,
		toolbarActionAlignLeft, toolbarActionAlignRight, toolbarActionWrapText, toolbarActionPercentFormat,
		toolbarActionSum, toolbarActionFind,
		toolbarActionZoomIn, toolbarActionZoom100,
		toolbarActionLayoutMargins, toolbarActionLayoutOrientation, toolbarActionLayoutSize,
		toolbarActionPrintArea, toolbarActionViewGridlines, toolbarActionLayoutGridlines,
		toolbarActionInsertTable, toolbarActionTableFormat, toolbarActionPivotTable, toolbarActionConditionalFormat,
		toolbarActionInsertLink, toolbarActionInsertShapes, toolbarActionInsertTextBox,
		toolbarActionInsertColumnChart, toolbarActionInsertLineChart, toolbarActionInsertPieChart,
		toolbarActionHeaderRow,
		toolbarActionFreezePanes, toolbarActionRemoveDuplicates, toolbarActionTextToColumns,
		toolbarActionRecalculate, toolbarActionRefreshAll,
		toolbarActionInsertFunction, toolbarActionRecentFunctions, toolbarActionLogicalFunctions,
		toolbarActionTextFunctions, toolbarActionDateTimeFunctions, toolbarActionNameManager,
		toolbarActionFlashFill,
		toolbarActionDataValidation,
		toolbarActionNewComment, toolbarActionDeleteComment,
		toolbarActionSpellCheck, toolbarActionTranslate,
		toolbarActionNormalView, toolbarActionPageBreakPreview, toolbarActionRuler,
		toolbarActionProtectSheet, toolbarActionProtectWorkbook:
		return false
	default:
		return true
	}
}

func (a *App) showSpreadsheetFeatureMissing(name string) {
	a.setStatus(fmt.Sprintf("“%s”功能暂未实现", name))
	if a.window != nil {
		a.window.Invalidate()
	}
}
