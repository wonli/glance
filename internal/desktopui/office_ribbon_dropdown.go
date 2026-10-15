package desktopui

import (
	"image"
	"strconv"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
)

const officeRibbonMenuButtonCount = 12

type officeRibbonMenuKind uint8

const (
	officeRibbonMenuClosed officeRibbonMenuKind = iota
	officeRibbonMenuWord
	officeRibbonMenuSpreadsheet
)

type officeRibbonDropdownState struct {
	kind       officeRibbonMenuKind
	wordAction string
	sheetSlot  int
	bounds     image.Rectangle
	anchor     image.Rectangle
	dismiss    widget.Clickable
	buttons    [officeRibbonMenuButtonCount]widget.Clickable
}

type officeRibbonMenuOption struct {
	label   string
	command string
	enabled bool
	checked bool
}

type officeRibbonMenuDefinition struct {
	title   string
	options []officeRibbonMenuOption
}

func (a *App) officeRibbonMenuDefinition() officeRibbonMenuDefinition {
	state := a.documentSessionState
	if state == nil {
		return officeRibbonMenuDefinition{}
	}
	switch state.officeRibbonMenu.kind {
	case officeRibbonMenuWord:
		return wordOfficeRibbonMenu(state.officeRibbonMenu.wordAction)
	case officeRibbonMenuSpreadsheet:
		return spreadsheetOfficeRibbonMenu(state.officeRibbonMenu.sheetSlot)
	default:
		return officeRibbonMenuDefinition{}
	}
}

func wordOfficeRibbonMenu(actionID string) officeRibbonMenuDefinition {
	option := func(label, command string, enabled bool) officeRibbonMenuOption {
		return officeRibbonMenuOption{label: label, command: command, enabled: enabled}
	}
	disabled := func(label string) officeRibbonMenuOption { return option(label, "", false) }
	switch actionID {
	case "new":
		return officeRibbonMenuDefinition{"新建", []officeRibbonMenuOption{option("空白文档", "new", true), disabled("从模板新建"), disabled("最近使用的模板"), disabled("在线模板")}}
	case "save":
		return officeRibbonMenuDefinition{"保存", []officeRibbonMenuOption{option("保存", "save", true), option("另存为…", "save-as", true), option("导出 PDF…", "export-pdf", true), option("自动保存设置", "autosave", true)}}
	case "print":
		return officeRibbonMenuDefinition{"打印", []officeRibbonMenuOption{option("打印…", "print", true), option("快速打印", "print", true), option("打印预览", "view-print-layout", true), option("页面设置…", "layout-margins", true)}}
	case "doc-info":
		return officeRibbonMenuDefinition{"文档信息", []officeRibbonMenuOption{option("属性…", "doc-info", true), disabled("检查文档"), option("版本历史", "version-history", true), option("保护文档…", "review-restrict", true)}}
	case "paste":
		return officeRibbonMenuDefinition{"粘贴选项", []officeRibbonMenuOption{disabled("保留源格式"), disabled("合并格式"), option("仅保留文本", "paste-text", true), disabled("选择性粘贴…")}}
	case "find":
		return officeRibbonMenuDefinition{"查找", []officeRibbonMenuOption{option("打开导航窗格…", "find", true), disabled("按标题浏览"), disabled("按页面浏览"), disabled("最近搜索")}}
	case "select":
		return officeRibbonMenuDefinition{"选择", []officeRibbonMenuOption{option("全选", "select-all", true), disabled("选择对象"), disabled("选择相同格式"), disabled("选择窗格…")}}
	case "cover":
		return officeRibbonMenuDefinition{"封面", []officeRibbonMenuOption{option("选择封面…", "cover", true), disabled("更多封面模板")}}
	case "insert-image":
		return officeRibbonMenuDefinition{"插入图片", []officeRibbonMenuOption{option("此设备…", "insert-image", true), disabled("最近使用"), disabled("图片库"), disabled("在线图片")}}
	case "insert-link":
		return officeRibbonMenuDefinition{"链接", []officeRibbonMenuOption{option("插入链接…", "insert-link", true), disabled("最近使用的链接")}}
	case "insert-header":
		return officeRibbonMenuDefinition{"页眉", []officeRibbonMenuOption{option("编辑页眉…", "insert-header", true), disabled("空白"), disabled("简洁"), disabled("横线样式")}}
	case "insert-textbox":
		return officeRibbonMenuDefinition{"文本框", []officeRibbonMenuOption{option("插入文本框…", "insert-textbox", true), disabled("简洁文本框"), disabled("侧边栏文本框"), disabled("绘制文本框")}}
	case "insert-equation":
		return officeRibbonMenuDefinition{"公式", []officeRibbonMenuOption{option("插入公式…", "insert-equation", true), disabled("分数"), disabled("上下标"), disabled("根式"), disabled("积分、求和与矩阵")}}
	case "references-toc":
		return officeRibbonMenuDefinition{"目录", []officeRibbonMenuOption{disabled("自动目录 1"), disabled("自动目录 2"), disabled("手动目录"), disabled("自定义目录…")}}
	case "references-footnotes":
		return officeRibbonMenuDefinition{"脚注和尾注", []officeRibbonMenuOption{disabled("插入脚注"), disabled("插入尾注"), disabled("下一条脚注"), disabled("脚注选项…")}}
	case "references-citations":
		return officeRibbonMenuDefinition{"引文", []officeRibbonMenuOption{disabled("最近使用的源"), disabled("添加新源…"), disabled("管理源"), disabled("引用样式")}}
	case "references-caption":
		return officeRibbonMenuDefinition{"题注", []officeRibbonMenuOption{disabled("插入题注…"), disabled("新建标签…"), disabled("题注位置")}}
	case "design-themes":
		return officeRibbonMenuDefinition{"主题", []officeRibbonMenuOption{option("主题设置…", "design-themes", true), disabled("Office"), disabled("简约"), disabled("专业")}}
	case "design-colors":
		return officeRibbonMenuDefinition{"主题颜色", []officeRibbonMenuOption{option("颜色方案…", "design-colors", true), disabled("自定义颜色")}}
	case "design-fonts":
		return officeRibbonMenuDefinition{"主题字体", []officeRibbonMenuOption{option("字体方案…", "design-fonts", true), disabled("自定义字体")}}
	case "design-effects":
		return officeRibbonMenuDefinition{"主题效果", []officeRibbonMenuOption{option("效果设置…", "design-effects", true), disabled("自定义效果")}}
	case "design-compact", "design-spacious":
		return officeRibbonMenuDefinition{"段落间距", []officeRibbonMenuOption{option("紧凑", "design-compact", true), disabled("默认"), option("宽松", "design-spacious", true), disabled("自定义间距…")}}
	case "design-watermark":
		return officeRibbonMenuDefinition{"水印", []officeRibbonMenuOption{option("水印设置…", "design-watermark", true), disabled("无水印"), disabled("机密"), disabled("草稿")}}
	case "layout-breaks":
		return officeRibbonMenuDefinition{"分隔符", []officeRibbonMenuOption{option("分页符", "page-break", true), option("分节符…", "layout-breaks", true), disabled("分栏符")}}
	case "layout-indent":
		return officeRibbonMenuDefinition{"缩进", []officeRibbonMenuOption{option("缩进设置…", "layout-indent", true), option("减少缩进", "decrease-indent", true), option("增加缩进", "increase-indent", true), disabled("特殊格式")}}
	case "layout-position":
		return officeRibbonMenuDefinition{"位置", []officeRibbonMenuOption{option("布局位置设置…", "layout-position", true), disabled("左上"), disabled("水平居中"), disabled("右下")}}
	case "review-new-comment":
		return officeRibbonMenuDefinition{"批注", []officeRibbonMenuOption{option("新建批注", "review-new-comment", true), disabled("新建并提及"), disabled("打开批注窗格"), option("删除当前批注", "review-delete-comment", true)}}
	case "review-compare":
		return officeRibbonMenuDefinition{"比较文档", []officeRibbonMenuOption{option("选择另一份文档…", "review-compare", true)}}
	case "view-reading":
		return officeRibbonMenuDefinition{"阅读视图", []officeRibbonMenuOption{option("阅读模式", "view-reading", true), option("打印布局", "view-print-layout", true), option("Web 版式", "view-web-layout", true), option("单页", "view-single-page", true), option("多页", "view-multiple-pages", true)}}
	case "view-ruler":
		return officeRibbonMenuDefinition{"显示", []officeRibbonMenuOption{option("水平标尺", "view-ruler", true), disabled("垂直标尺"), option("网格线", "view-gridlines", true), option("导航窗格", "view-navigation", true)}}
	case "view-new-window":
		return officeRibbonMenuDefinition{"窗口", []officeRibbonMenuOption{option("新建窗口", "view-new-window", true), disabled("全部重排"), disabled("拆分窗口"), option("并排查看", "view-side-by-side", true), disabled("同步滚动")}}
	default:
		return officeRibbonMenuDefinition{}
	}
}

func spreadsheetOfficeRibbonMenu(slot int) officeRibbonMenuDefinition {
	option := func(label, command string, enabled bool) officeRibbonMenuOption {
		return officeRibbonMenuOption{label: label, command: command, enabled: enabled}
	}
	disabled := func(label string) officeRibbonMenuOption { return option(label, "", false) }
	switch slot {
	case toolbarActionOpenFile:
		return officeRibbonMenuDefinition{"打开工作簿", []officeRibbonMenuOption{option("打开…", "excel-open", true), option("最近使用", "excel-recent", true), option("浏览…", "excel-open", true), disabled("恢复未保存的工作簿")}}
	case 21:
		return officeRibbonMenuDefinition{"粘贴选项", []officeRibbonMenuOption{disabled("保留源格式"), disabled("合并格式"), option("仅保留文本", "excel-paste-text", true), disabled("选择性粘贴…")}}
	case toolbarActionFontFamily:
		return officeRibbonMenuDefinition{"字体", []officeRibbonMenuOption{option("等线", "excel-font-dengxian", true), option("宋体", "excel-font-simsun", true), option("微软雅黑", "excel-font-yahei", true), option("Arial", "excel-font-arial", true), option("更多字体…", "excel-font-custom", true)}}
	case toolbarActionFontSize:
		return officeRibbonMenuDefinition{"字号", []officeRibbonMenuOption{option("10", "excel-size-10", true), option("11", "excel-size-11", true), option("12", "excel-size-12", true), option("14", "excel-size-14", true), option("16", "excel-size-16", true), option("自定义…", "excel-size-custom", true)}}
	case toolbarActionCellStyles:
		return officeRibbonMenuDefinition{"单元格样式", []officeRibbonMenuOption{option("常规", "excel-style-normal", true), option("强调", "excel-style-emphasis", true), option("标题", "excel-style-title", true), disabled("其他样式")}}
	case 13:
		return officeRibbonMenuDefinition{"删除", []officeRibbonMenuOption{disabled("删除单元格并上移"), disabled("删除单元格并左移"), option("删除整行", "excel-delete-row", true), option("删除整列", "excel-delete-column", true), option("删除工作表", "excel-delete-sheet", true)}}
	case toolbarActionFormat:
		return officeRibbonMenuDefinition{"单元格格式", []officeRibbonMenuOption{disabled("行高…"), disabled("自动调整行高"), disabled("列宽…"), disabled("自动调整列宽"), disabled("隐藏或取消隐藏"), disabled("设置单元格格式…")}}
	case toolbarActionSum:
		return officeRibbonMenuDefinition{"自动求和", []officeRibbonMenuOption{option("求和", "excel-sum", true), disabled("平均值"), disabled("计数"), disabled("最大值"), disabled("最小值"), disabled("其他函数…")}}
	case 18:
		return officeRibbonMenuDefinition{"排序和筛选", []officeRibbonMenuOption{option("升序", "excel-sort-asc", true), option("降序", "excel-sort-desc", true), disabled("自定义排序…"), option("筛选", "excel-filter", true), option("清除筛选", "excel-clear-filter", true), disabled("重新应用")}}
	case toolbarActionFind:
		return officeRibbonMenuDefinition{"查找和选择", []officeRibbonMenuOption{option("查找…", "excel-find", true), disabled("替换…"), disabled("定位…"), disabled("定位条件…"), disabled("选择对象")}}
	case toolbarActionInsertTable:
		return officeRibbonMenuDefinition{"插入表格", []officeRibbonMenuOption{option("设置表格范围…", "excel-insert-table", true), disabled("表格大小选择网格")}}
	case toolbarActionLayoutMargins:
		return officeRibbonMenuDefinition{"页边距", []officeRibbonMenuOption{option("普通", "excel-margin-normal", true), option("窄", "excel-margin-narrow", true), option("宽", "excel-margin-wide", true), disabled("上次自定义"), option("自定义…", "excel-margin-custom", true)}}
	case toolbarActionRefreshAll:
		return officeRibbonMenuDefinition{"刷新", []officeRibbonMenuOption{disabled("刷新全部"), disabled("刷新当前连接"), disabled("连接属性…"), disabled("查询状态")}}
	case toolbarActionSpellCheck:
		return officeRibbonMenuDefinition{"校对", []officeRibbonMenuOption{disabled("建议"), disabled("忽略一次"), disabled("更改语言…"), disabled("自动更正选项…")}}
	case toolbarActionNormalView:
		return officeRibbonMenuDefinition{"工作簿视图", []officeRibbonMenuOption{option("普通", "excel-view-normal", true), option("分页预览", "excel-view-page-break", true), disabled("页面布局"), disabled("自定义视图")}}
	default:
		return officeRibbonMenuDefinition{}
	}
}

func (a *App) officeRibbonMenuOptionEnabled(option officeRibbonMenuOption) bool {
	if !option.enabled || option.command == "" {
		return false
	}
	state := a.documentSessionState
	if state == nil {
		return false
	}
	switch state.officeRibbonMenu.kind {
	case officeRibbonMenuWord:
		if option.command == "paste-text" {
			return a.wordToolbarActionEnabled(ribbonAction{id: "paste"})
		}
		if option.command == "select-all" {
			return state.word != nil && len(wordBlocksForState(state)) > 0
		}
		return a.wordToolbarActionEnabled(ribbonAction{id: option.command})
	case officeRibbonMenuSpreadsheet:
		switch option.command {
		case "excel-open":
			return a.toolbarActionEnabled(toolbarActionOpenFile)
		case "excel-recent":
			return state.book != nil && !state.formatRunning.Load()
		case "excel-paste-text":
			return a.toolbarActionEnabled(21)
		case "excel-font-dengxian", "excel-font-simsun", "excel-font-yahei", "excel-font-arial", "excel-font-custom":
			return a.toolbarActionEnabled(toolbarActionFontFamily)
		case "excel-size-10", "excel-size-11", "excel-size-12", "excel-size-14", "excel-size-16", "excel-size-custom":
			return a.toolbarActionEnabled(toolbarActionFontSize)
		case "excel-style-normal", "excel-style-emphasis", "excel-style-title":
			return a.toolbarActionEnabled(toolbarActionCellStyles)
		case "excel-delete-row", "excel-delete-column":
			return a.toolbarActionEnabled(13)
		case "excel-delete-sheet":
			_, editable := state.book.(document.SpreadsheetStructureEditor)
			return editable && len(state.sheets) > 1 && !state.formatRunning.Load() && !a.isSaving()
		case "excel-sum":
			return a.toolbarActionEnabled(toolbarActionSum)
		case "excel-sort-asc":
			return a.toolbarActionEnabled(18)
		case "excel-sort-desc":
			return a.toolbarActionEnabled(19)
		case "excel-filter", "excel-clear-filter":
			return a.toolbarActionEnabled(20)
		case "excel-find":
			return a.toolbarActionEnabled(toolbarActionFind)
		case "excel-insert-table":
			return a.toolbarActionEnabled(toolbarActionInsertTable)
		case "excel-margin-normal", "excel-margin-narrow", "excel-margin-wide", "excel-margin-custom":
			return a.toolbarActionEnabled(toolbarActionLayoutMargins)
		case "excel-view-normal":
			return a.toolbarActionEnabled(toolbarActionNormalView)
		case "excel-view-page-break":
			return a.toolbarActionEnabled(toolbarActionPageBreakPreview)
		default:
			return false
		}
	default:
		return false
	}
}

func (a *App) toggleWordOfficeRibbonMenu(actionID string) bool {
	definition := wordOfficeRibbonMenu(actionID)
	state := a.documentSessionState
	if state == nil || definition.title == "" {
		return false
	}
	menu := &state.officeRibbonMenu
	if menu.kind == officeRibbonMenuWord && menu.wordAction == actionID {
		menu.kind = officeRibbonMenuClosed
		menu.wordAction = ""
		return true
	}
	menu.kind = officeRibbonMenuWord
	menu.wordAction = actionID
	menu.sheetSlot = 0
	return true
}

func (a *App) toggleSpreadsheetOfficeRibbonMenu(slot int) bool {
	definition := spreadsheetOfficeRibbonMenu(slot)
	state := a.documentSessionState
	if state == nil || definition.title == "" {
		return false
	}
	menu := &state.officeRibbonMenu
	if menu.kind == officeRibbonMenuSpreadsheet && menu.sheetSlot == slot {
		menu.kind = officeRibbonMenuClosed
		menu.wordAction = ""
		return true
	}
	menu.kind = officeRibbonMenuSpreadsheet
	menu.sheetSlot = slot
	menu.wordAction = ""
	return true
}

func (a *App) closeOfficeRibbonMenu() {
	if state := a.documentSessionState; state != nil {
		state.officeRibbonMenu.kind = officeRibbonMenuClosed
		state.officeRibbonMenu.wordAction = ""
		state.officeRibbonMenu.bounds = image.Rectangle{}
		state.officeRibbonMenu.anchor = image.Rectangle{}
	}
}

func (a *App) handleOfficeRibbonMenuEvents(gtx layout.Context) bool {
	state := a.documentSessionState
	if state == nil || state.officeRibbonMenu.kind == officeRibbonMenuClosed {
		return false
	}
	if state.officeRibbonMenu.dismiss.Clicked(gtx) {
		history := state.officeRibbonMenu.dismiss.History()
		if len(history) != 0 {
			point := history[len(history)-1].Position
			if !point.In(state.officeRibbonMenu.bounds) && !point.In(state.officeRibbonMenu.anchor) {
				a.closeOfficeRibbonMenu()
			}
		}
	}
	definition := a.officeRibbonMenuDefinition()
	for index, option := range definition.options {
		if index >= len(state.officeRibbonMenu.buttons) {
			break
		}
		if a.officeRibbonMenuOptionEnabled(option) && state.officeRibbonMenu.buttons[index].Clicked(gtx) {
			kind := state.officeRibbonMenu.kind
			command := option.command
			a.closeOfficeRibbonMenu()
			a.executeOfficeRibbonMenuCommand(gtx, kind, command)
			return true
		}
	}
	return false
}

func (a *App) executeOfficeRibbonMenuCommand(gtx layout.Context, kind officeRibbonMenuKind, command string) {
	state := a.documentSessionState
	if state == nil {
		return
	}
	if kind == officeRibbonMenuWord {
		switch command {
		case "paste-text":
			state.wordClipboardRunID = state.selectedWordRunID
			if wordHasPendingTextDrafts(state) {
				state.wordForceDraftFlush.Store(true)
				state.wordFlushRequested.Store(true)
				state.wordFlushAllDrafts.Store(true)
			}
			gtx.Execute(clipboard.ReadCmd{Tag: &state.wordClipboardTarget})
		case "select-all":
		default:
		}
		if command != "paste-text" && wordHasPendingTextDrafts(state) {
			state.wordForceDraftFlush.Store(true)
			state.wordFlushRequested.Store(true)
			state.wordFlushAllDrafts.Store(true)
			if command == "save" {
				state.wordSaveAfterDrafts = true
				state.wordSaveAfterDraftsReason = wordSaveManual
			} else {
				state.wordDeferredToolbarAction = command
			}
			return
		}
		if command == "select-all" {
			a.selectAllWordText()
		} else if command != "paste-text" {
			a.executeWordToolbarAction(command)
		}
		return
	}
	if kind != officeRibbonMenuSpreadsheet {
		return
	}
	switch command {
	case "excel-open":
		a.startOpenFilePicker()
	case "excel-recent":
		a.activateRecent()
	case "excel-paste-text":
		gtx.Execute(clipboard.ReadCmd{Tag: &state.clipboardTarget})
	case "excel-font-dengxian", "excel-font-simsun", "excel-font-yahei", "excel-font-arial":
		family := map[string]string{"excel-font-dengxian": "等线", "excel-font-simsun": "宋体", "excel-font-yahei": "Microsoft YaHei", "excel-font-arial": "Arial"}[command]
		a.applyCellFormat(document.CellFormatPatch{FontFamily: &family})
	case "excel-font-custom":
		a.showSpreadsheetFontFamilyDialog()
	case "excel-size-10", "excel-size-11", "excel-size-12", "excel-size-14", "excel-size-16":
		size, _ := strconv.ParseFloat(strings.TrimPrefix(command, "excel-size-"), 64)
		a.applyCellFormat(document.CellFormatPatch{FontSize: &size})
	case "excel-size-custom":
		a.modal.ShowInput("设置字号", "输入字号（磅）", "11", "1–409 之间", func(action string) {
			value := strings.TrimSpace(a.modal.Input.Text())
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			size, err := strconv.ParseFloat(value, 64)
			if err != nil || size < 1 || size > 409 {
				a.setStatus("字号必须是 1 到 409 之间的数值")
				if a.window != nil {
					a.window.Invalidate()
				}
				return
			}
			a.applyCellFormat(document.CellFormatPatch{FontSize: &size})
		}, modalDialogActionSpec{ID: "confirm", Label: "应用"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
	case "excel-style-normal":
		a.applyCellFormat(document.CellFormatPatch{ClearFormat: true})
	case "excel-style-emphasis":
		bold := true
		a.applyCellFormat(document.CellFormatPatch{Bold: &bold})
	case "excel-style-title":
		bold, size := true, 16.0
		a.applyCellFormat(document.CellFormatPatch{Bold: &bold, FontSize: &size})
	case "excel-delete-row":
		a.applyStructure(document.StructureEdit{Kind: document.StructureDeleteRows, Sheet: state.sheet, Index: state.selection.Active.Row + 1, Count: 1})
	case "excel-delete-column":
		a.applyStructure(document.StructureEdit{Kind: document.StructureDeleteColumns, Sheet: state.sheet, Index: state.selection.Active.Column + 1, Count: 1})
	case "excel-delete-sheet":
		a.applyStructure(document.StructureEdit{Kind: document.StructureDeleteSheet, Sheet: state.sheet})
	case "excel-sum":
		a.applySpreadsheetAutoSum()
	case "excel-sort-asc", "excel-sort-desc":
		bounds := state.selection.Bounds()
		a.applySort(document.SortRequest{Sheet: state.sheet, Bounds: bounds, KeyColumn: state.selection.Active.Column + 1, Descending: command == "excel-sort-desc"})
	case "excel-filter":
		a.toggleValueFilter()
	case "excel-clear-filter":
		if state.valueFilter != nil {
			a.toggleValueFilter()
		}
	case "excel-find":
		a.showSpreadsheetFindDialog()
	case "excel-insert-table":
		a.showSpreadsheetInsertTableDialog("")
	case "excel-margin-normal", "excel-margin-narrow", "excel-margin-wide":
		top, side := 0.75, 0.7
		label := "普通"
		if command == "excel-margin-narrow" {
			side, label = 0.25, "窄"
		} else if command == "excel-margin-wide" {
			top, side, label = 1, 1, "宽"
		}
		sheet := state.sheet
		a.runSpreadsheetPageLayoutCommand("设置"+label+"页边距", showSpreadsheetMarginsDialogArguments{Sheet: sheet, Operation: "margins", MarginTop: top, MarginRight: side, MarginBottom: top, MarginLeft: side}, func(editor document.SpreadsheetPageLayoutEditor) error {
			return editor.SetSheetPageMargins(sheet, top, side, top, side)
		})
	case "excel-margin-custom":
		a.showSpreadsheetMarginsDialog()
	case "excel-view-normal":
		a.setSpreadsheetViewMode("normal")
	case "excel-view-page-break":
		a.setSpreadsheetViewMode("pageBreakPreview")
	}
}

func (a *App) layoutOfficeRibbonDropdown(gtx layout.Context) layout.Dimensions {
	state := a.documentSessionState
	if state == nil || state.officeRibbonMenu.kind == officeRibbonMenuClosed {
		return layout.Dimensions{}
	}
	definition := a.officeRibbonMenuDefinition()
	if definition.title == "" {
		return layout.Dimensions{}
	}
	panelWidth := min(gtx.Dp(unit.Dp(272)), gtx.Constraints.Max.X)
	x := 12
	if state.officeRibbonMenu.kind == officeRibbonMenuSpreadsheet {
		x = toolbarPickerPanelX(state.officeRibbonMenu.sheetSlot, toolbarActionsForTab(state.toolbarTab), state.toolbarActionList.List.Position, gtx.Dp(unit.Dp(51)), gtx.Dp(unit.Dp(55)), gtx.Constraints.Max.X, panelWidth) + gtx.Dp(unit.Dp(8))
	} else {
		actions := wordToolbarActions(state.wordToolbarTab)
		positions := wordPenActionPositions[state.wordToolbarTab]
		first := state.wordToolbarActionList.List.Position.First
		for index, action := range actions {
			if action.id != state.officeRibbonMenu.wordAction {
				continue
			}
			if index < len(positions) {
				if first < 0 || first >= len(positions) {
					first = 0
				}
				x = gtx.Dp(unit.Dp(24+positions[index]-positions[first])) - state.wordToolbarActionList.List.Position.Offset
			}
			break
		}
	}
	maxX := max(0, gtx.Constraints.Max.X-panelWidth)
	anchorX := x
	if state.officeRibbonMenu.kind == officeRibbonMenuSpreadsheet {
		anchorX -= gtx.Dp(unit.Dp(8))
	}
	x = min(max(0, x), maxX)
	y := gtx.Dp(unit.Dp(wordRibbonHeightDP + 2))
	rowHeight := gtx.Dp(unit.Dp(30))
	desiredHeight := gtx.Dp(unit.Dp(36)) + len(definition.options)*rowHeight
	bottomLimit := max(0, gtx.Constraints.Max.Y-gtx.Dp(unit.Dp(12)))
	if y+desiredHeight > bottomLimit {
		y = max(0, bottomLimit-desiredHeight)
	}
	panelHeight := min(desiredHeight, max(0, bottomLimit-y))
	state.officeRibbonMenu.bounds = image.Rect(x, y, x+panelWidth, y+panelHeight)
	anchorLeft := max(0, anchorX-gtx.Dp(unit.Dp(4)))
	anchorTop := gtx.Dp(unit.Dp(wordRibbonCategoryHeightDP))
	state.officeRibbonMenu.anchor = image.Rect(anchorLeft, anchorTop, min(gtx.Constraints.Max.X, anchorLeft+gtx.Dp(unit.Dp(56))), gtx.Dp(unit.Dp(wordRibbonHeightDP)))
	return layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = gtx.Constraints.Max
			pass := pointer.PassOp{}.Push(gtx.Ops)
			return state.officeRibbonMenu.dismiss.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				defer pass.Pop()
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			transform := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
			defer transform.Pop()
			gtx.Constraints = layout.Constraints{Min: image.Pt(panelWidth, panelHeight), Max: image.Pt(panelWidth, panelHeight)}
			return a.layoutOfficeRibbonDropdownPanel(gtx, definition)
		}),
	)
}

func (a *App) layoutOfficeRibbonDropdownPanel(gtx layout.Context, definition officeRibbonMenuDefinition) layout.Dimensions {
	colors := a.tokens().Colors
	rowHeight := gtx.Dp(unit.Dp(30))
	height := gtx.Constraints.Max.Y
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			bounds := image.Rectangle{Max: gtx.Constraints.Max}
			shadow := bounds.Add(image.Pt(1, 2))
			paint.FillShape(gtx.Ops, colors.Menu.Shadow, clip.UniformRRect(shadow, gtx.Dp(unit.Dp(9))).Op(gtx.Ops))
			paint.FillShape(gtx.Ops, colors.Menu.Border, clip.UniformRRect(bounds, gtx.Dp(unit.Dp(9))).Op(gtx.Ops))
			paint.FillShape(gtx.Ops, colors.Menu.Surface, clip.UniformRRect(bounds.Inset(1), gtx.Dp(unit.Dp(8))).Op(gtx.Ops))
			return layout.Dimensions{Size: bounds.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Top: unit.Dp(7), Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				children := make([]layout.FlexChild, 0, len(definition.options)+2)
				title := material.Body2(a.theme, definition.title)
				title.Color = colors.Text.Muted
				children = append(children, layout.Rigid(title.Layout))
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(0, gtx.Dp(unit.Dp(4)))}
				}))
				for index, option := range definition.options {
					index, option := index, option
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if index >= len(a.documentSessionState.officeRibbonMenu.buttons) {
							return layout.Dimensions{}
						}
						button := &a.documentSessionState.officeRibbonMenu.buttons[index]
						enabled := a.officeRibbonMenuOptionEnabled(option)
						gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, rowHeight))
						label := option.label
						if option.checked {
							label = "✓  " + label
						}
						draw := func(gtx layout.Context) layout.Dimensions {
							background := colors.Menu.Surface
							textColor := colors.Toolbar.Glyph
							if !enabled {
								textColor = colors.Toolbar.DisabledText
							} else if option.checked {
								background = colors.Menu.Selected
							} else if button.Hovered() {
								background = colors.Menu.Hover
							}
							return layout.Background{}.Layout(gtx,
								func(gtx layout.Context) layout.Dimensions {
									paint.FillShape(gtx.Ops, background, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(unit.Dp(4))).Op(gtx.Ops))
									return layout.Dimensions{Size: gtx.Constraints.Max}
								},
								func(gtx layout.Context) layout.Dimensions {
									style := material.Body2(a.theme, label)
									style.TextSize = unit.Sp(13)
									style.Color = textColor
									return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, layout.Rigid(style.Layout))
									})
								},
							)
						}
						if enabled {
							return button.Layout(gtx, draw)
						}
						return draw(gtx)
					}))
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			})
		},
	)
}
