package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode"

	"gioui.org/io/clipboard"
	"gioui.org/io/semantic"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"glance/internal/document"
	"glance/internal/format/docx"
)

type wordToolbarTab uint8
type wordToolbarAction = ribbonAction

const (
	wordTabFile wordToolbarTab = iota
	wordTabHome
	wordTabInsert
	wordTabDesign
	wordTabLayout
	wordTabReview
	wordTabView
	wordTabPicture
)

const (
	wordDefaultFontSizePt      = 11.0
	wordMinFontSizePt          = 1.0
	wordMaxFontSizePt          = 1638.0
	wordFontSizeStepPt         = 1.0
	wordParagraphIndentStepEMU = int64(457200)
	wordMaxParagraphIndentEMU  = int64(1<<31-1) * 635
)

var (
	wordToolbarTabLabels = []string{"文件", "开始", "插入", "设计", "布局", "审阅", "视图", "图片"}
	wordFileActions      = wordActions(
		"new|新建|文档|file-plus", "open|打开|文档|folder-open", "recent|最近使用|文档|clock-3",
		"save|保存|保存|save", "save-as|另存为|保存|copy", "autosave|自动保存|保存|refresh-cw",
		"print|打印|输出|printer", "export-pdf|导出 PDF|输出|file-down", "share|共享|输出|share-2",
		"doc-info|文档信息|信息|info", "protect-document|保护文档|信息|shield-check", "version-history|版本历史|信息|history",
	)
	wordHomeActions = []ribbonAction{
		{slot: -1, id: "paste", label: "粘贴", group: "剪贴板", icon: "paste"},
		{slot: -1, id: "cut", label: "剪切", group: "剪贴板", icon: "cut"},
		{slot: -1, id: "copy", label: "复制", group: "剪贴板", icon: "copy"},
		{slot: -1, id: "format-painter", label: "格式刷", group: "剪贴板", icon: "paintbrush"},
		{slot: -1, id: "font-family", label: "字体", group: "字体", icon: "type"},
		{slot: -1, id: "font-size-decrease", label: "减小字号", group: "字体", icon: "minus"},
		{slot: -1, id: "font-size-increase", label: "增大字号", group: "字体", icon: "plus"},
		{slot: -1, id: "bold", label: "加粗", group: "字体", icon: "bold"},
		{slot: -1, id: "italic", label: "斜体", group: "字体", icon: "italic"},
		{slot: -1, id: "underline", label: "下划线", group: "字体", icon: "underline"},
		{slot: -1, id: "text-highlight", label: "高亮", group: "字体", icon: "paint-bucket"},
		{slot: -1, id: "font-color", label: "字体颜色", group: "字体", icon: "type"},
		{slot: -1, id: "bullet-list", label: "项目符号", group: "段落", icon: "list"},
		{slot: -1, id: "numbered-list", label: "编号", group: "段落", icon: "sort-asc"},
		{slot: -1, id: "align-left", label: "左对齐", group: "段落", icon: "align-left"},
		{slot: -1, id: "align-center", label: "居中", group: "段落", icon: "align-center"},
		{slot: -1, id: "align-right", label: "右对齐", group: "段落", icon: "align-right"},
		{slot: -1, id: "align-justify", label: "两端对齐", group: "段落", icon: "align-distributed"},
		{slot: -1, id: "decrease-indent", label: "减少缩进", group: "段落", icon: "minus"},
		{slot: -1, id: "increase-indent", label: "增加缩进", group: "段落", icon: "plus"},
		{slot: -1, id: "find", label: "查找", group: "编辑", icon: "search"},
		{slot: -1, id: "replace", label: "替换", group: "编辑", icon: "list-filter"},
		{slot: -1, id: "select", label: "选择", group: "编辑", icon: "grid"},
		{slot: -1, id: "editor", label: "编辑器", group: "编辑器", icon: "file-text"},
		{slot: -1, id: "dictate", label: "听写", group: "编辑器", icon: "file"},
		{slot: -1, id: "add-ins", label: "加载项", group: "编辑器", icon: "grid"},
		{slot: -1, id: "designer", label: "设计器", group: "编辑器", icon: "star"},
	}
	wordInsertActions = wordActions(
		"cover|封面|页面|file-text", "blank-page|空白页|页面|file-plus", "page-break|分页|页面|split",
		"insert-table|表格|表格|table", "insert-image|图片|插图|image", "insert-shapes|形状|插图|shapes",
		"insert-chart|图表|插图|chart-no-axes-column", "insert-link|链接|链接|link", "insert-bookmark|书签|链接|bookmark",
		"insert-header|页眉|页眉页脚|panel-top", "insert-footer|页脚|页眉页脚|panel-bottom", "insert-page-number|页码|页眉页脚|list-ordered",
		"insert-textbox|文本框|文本|text-cursor-input", "insert-wordart|艺术字|文本|type", "insert-equation|公式|符号|function-square", "insert-symbol|符号|符号|omega",
	)
	wordDesignActions = wordActions(
		"design-themes|主题|文档格式|layout-grid", "design-colors|颜色|文档格式|palette", "design-fonts|字体|文档格式|type",
		"design-effects|效果|文档格式|sparkles", "design-compact|紧凑|段落间距|rows-3", "design-spacious|宽松|段落间距|rows-4",
		"design-watermark|水印|页面背景|stamp", "design-page-color|页面颜色|页面背景|paint-bucket", "design-page-border|页面边框|页面背景|square",
	)
	wordLayoutActions = wordActions(
		"layout-margins|页边距|页面设置|panel-left", "layout-orientation|方向|页面设置|rectangle-horizontal", "layout-size|纸张大小|页面设置|file-text",
		"layout-columns|分栏|页面设置|columns-2", "layout-breaks|分隔符|分隔|panel-top", "layout-line-numbers|行号|分隔|list-ordered",
		"layout-indent|缩进|段落|indent-increase", "layout-before|段前|段落|arrow-up-to-line", "layout-after|段后|段落|arrow-down-to-line",
		"layout-position|位置|排列|move", "layout-wrap|环绕文字|排列|wrap-text", "layout-align|对齐|排列|align-horizontal-justify-center",
	)
	wordReviewActions = wordActions(
		"review-editor|编辑器|校对|spell-check", "review-word-count|字数统计|校对|whole-word", "review-translate|翻译|语言|languages",
		"review-language|语言|语言|languages", "review-new-comment|新建批注|批注|message-square-plus", "review-delete-comment|删除批注|批注|message-square-x",
		"review-track|修订|修订|highlighter", "review-accept|接受|修订|check", "review-reject|拒绝|修订|x",
		"review-compare|比较|保护|git-compare", "review-restrict|限制编辑|保护|shield-check",
	)
	wordViewActions = wordActions(
		"view-reading|阅读模式|视图|book-open", "view-print-layout|打印布局|视图|printer", "view-web-layout|Web版式|视图|panels-top-left",
		"view-ruler|标尺|显示|ruler", "view-gridlines|网格线|显示|grid-2x2", "view-navigation|导航窗格|显示|panel-left",
		"view-zoom|缩放|缩放|zoom-in", "view-single-page|单页|缩放|file", "view-multiple-pages|多页|缩放|files",
		"view-new-window|新建窗口|窗口|app-window", "view-side-by-side|并排查看|窗口|columns-2", "view-macros|宏|宏|code-xml",
	)
	wordHistoryActions = wordActions("undo|撤销|历史|undo", "redo|重做|历史|redo")
	wordPictureActions = []ribbonAction{
		{slot: -1, id: "replace-image", label: "替换图片", group: "图片", icon: "copy"},
		{slot: -1, id: "delete-image", label: "删除图片", group: "图片", icon: "trash-2"},
		{slot: -1, id: "resize-image", label: "调整大小", group: "图片", icon: "settings-2"},
		{slot: -1, id: "rotate-left", label: "向左旋转 90°", group: "图片", icon: "undo"},
		{slot: -1, id: "rotate-right", label: "向右旋转 90°", group: "图片", icon: "redo"},
	}
	wordFontColors = []struct {
		label string
		value string
		color color.NRGBA
	}{
		{label: "黑色", value: "000000", color: color.NRGBA{R: 0, G: 0, B: 0, A: 255}},
		{label: "红色", value: "F44336", color: color.NRGBA{R: 244, G: 67, B: 54, A: 255}},
		{label: "橙色", value: "FF9800", color: color.NRGBA{R: 255, G: 152, B: 0, A: 255}},
		{label: "黄色", value: "FFC107", color: color.NRGBA{R: 255, G: 193, B: 7, A: 255}},
		{label: "绿色", value: "4CAF50", color: color.NRGBA{R: 76, G: 175, B: 80, A: 255}},
		{label: "蓝色", value: "2196F3", color: color.NRGBA{R: 33, G: 150, B: 243, A: 255}},
		{label: "紫色", value: "9C27B0", color: color.NRGBA{R: 156, G: 39, B: 176, A: 255}},
		{label: "白色", value: "FFFFFF", color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}},
	}
	wordHighlightColors = []struct {
		label string
		value string
		color color.NRGBA
	}{
		{label: "黄色", value: "yellow", color: color.NRGBA{R: 255, G: 255, B: 0, A: 255}},
		{label: "绿色", value: "green", color: color.NRGBA{R: 0, G: 255, B: 0, A: 255}},
		{label: "青色", value: "cyan", color: color.NRGBA{R: 0, G: 255, B: 255, A: 255}},
		{label: "品红", value: "magenta", color: color.NRGBA{R: 255, G: 0, B: 255, A: 255}},
		{label: "蓝色", value: "blue", color: color.NRGBA{R: 0, G: 0, B: 255, A: 255}},
		{label: "红色", value: "red", color: color.NRGBA{R: 255, G: 0, B: 0, A: 255}},
		{label: "深灰", value: "darkGray", color: color.NRGBA{R: 128, G: 128, B: 128, A: 255}},
		{label: "无高亮", value: "", color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}},
	}
)

func wordActions(specs ...string) []ribbonAction {
	actions := make([]ribbonAction, 0, len(specs))
	for _, spec := range specs {
		parts := strings.SplitN(spec, "|", 4)
		if len(parts) != 4 {
			continue
		}
		actions = append(actions, ribbonAction{slot: -1, id: parts[0], label: parts[1], group: parts[2], icon: parts[3]})
	}
	return actions
}

func wordToolbarTabs(state *documentSessionState) []wordToolbarTab {
	tabs := []wordToolbarTab{wordTabFile, wordTabHome, wordTabInsert, wordTabDesign, wordTabLayout, wordTabReview, wordTabView}
	if state != nil && state.selectedWordImageID != "" {
		tabs = append(tabs, wordTabPicture)
	}
	return tabs
}

func wordToolbarActions(tab wordToolbarTab) []ribbonAction {
	switch tab {
	case wordTabFile:
		return wordFileActions
	case wordTabHome:
		return wordHomeActions
	case wordTabInsert:
		return wordInsertActions
	case wordTabDesign:
		return wordDesignActions
	case wordTabLayout:
		return wordLayoutActions
	case wordTabReview:
		return wordReviewActions
	case wordTabView:
		return wordViewActions
	case wordTabPicture:
		return wordPictureActions
	default:
		return nil
	}
}

func (a *App) wordToolbarActionEnabled(action ribbonAction) bool {
	state := a.documentSessionState
	if action.id == "view-new-window" {
		enabled, _ := wordNewWindowAvailability(state, a.desktopRuntime)
		return enabled
	}
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() || a.fileCommandRunning || a.wordImagePickerRunning {
		return false
	}
	switch action.id {
	case "new", "open", "recent", "save-as", "export-pdf", "share":
		return true
	case "version-history":
		return canonicalSessionPath(sessionPath(state)) != ""
	case "print":
		return a.wordPrinter != nil && a.wordPrinter.Available()
	case "autosave":
		return true
	case "doc-info":
		_, supported := any(state.word).(document.WordDocumentPropertiesEditor)
		return supported
	case "view-reading", "view-print-layout", "view-web-layout", "view-navigation", "view-zoom", "view-gridlines", "view-single-page", "view-multiple-pages":
		return true
	case "view-ruler":
		return true
	case "undo", "redo":
		history, ok := any(state.word).(document.WordHistory)
		if !ok {
			return false
		}
		if action.id == "undo" {
			return history.CanUndo()
		}
		return history.CanRedo()
	case "cut", "copy", "paste":
		run, ok := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
		return ok && (action.id == "copy" || run.Editable)
	case "bold", "italic", "underline", "font-color", "text-highlight":
		_, editable := any(state.word).(document.WordTextEditor)
		run, ok := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
		return editable && ok && run.Editable
	case "font-family":
		_, editable := any(state.word).(document.WordTextFontFamilyEditor)
		run, ok := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
		return editable && ok && run.Editable
	case "font-size-decrease", "font-size-increase":
		_, editable := any(state.word).(document.WordTextFontSizeEditor)
		run, ok := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
		if !editable || !ok || !run.Editable {
			return false
		}
		size := effectiveWordFontSize(run.Style.FontSize)
		if size < wordMinFontSizePt || size > wordMaxFontSizePt {
			return false
		}
		if action.id == "font-size-increase" {
			return size < wordMaxFontSizePt
		}
		return size > wordMinFontSizePt
	case "align-left", "align-center", "align-right", "align-justify":
		_, editable := any(state.word).(document.WordTextEditor)
		_, ok := selectedWordParagraph(wordBlocksForState(state), state.selectedWordParagraphID)
		return editable && ok
	case "bullet-list", "numbered-list":
		_, editable := any(state.word).(document.WordParagraphListEditor)
		paragraph, ok := selectedWordParagraph(wordBlocksForState(state), state.selectedWordParagraphID)
		return editable && ok && (paragraph.ListKind == document.WordListNone || paragraph.ListKind == document.WordListBullet || paragraph.ListKind == document.WordListNumbered)
	case "decrease-indent", "increase-indent":
		paragraph, ok := selectedWordParagraph(wordBlocksForState(state), state.selectedWordParagraphID)
		if !ok {
			return false
		}
		if paragraph.ListKind == document.WordListBullet || paragraph.ListKind == document.WordListNumbered {
			if action.id == "decrease-indent" {
				return paragraph.ListLevel >= 0
			}
			return paragraph.ListLevel < paragraph.ListMaxLevel
		}
		if paragraph.ListKind == document.WordListUnsupported {
			return false
		}
		_, editable := any(state.word).(document.WordTextEditor)
		if !editable {
			return false
		}
		if action.id == "decrease-indent" {
			return paragraph.LeftIndentEMU > 0
		}
		return paragraph.LeftIndentEMU <= wordMaxParagraphIndentEMU-wordParagraphIndentStepEMU
	case "layout-before", "layout-after":
		_, editable := any(state.word).(document.WordParagraphSpacingEditor)
		_, ok := selectedWordParagraph(wordBlocksForState(state), state.selectedWordParagraphID)
		return editable && ok
	case "layout-indent":
		_, editable := any(state.word).(document.WordTextEditor)
		_, ok := selectedWordParagraph(wordBlocksForState(state), state.selectedWordParagraphID)
		return editable && ok && !state.formatRunning.Load() && !a.isSaving()
	case "layout-orientation", "layout-margins", "layout-size", "layout-columns":
		_, editable := any(state.word).(document.WordPageSetupEditor)
		return editable && !state.formatRunning.Load() && !a.isSaving()
	case "find":
		_, content := any(state.word).(document.WordContentReader)
		return content
	case "replace":
		_, content := any(state.word).(document.WordContentReader)
		_, editor := any(state.word).(document.WordTextRangeEditor)
		return content && editor
	case "insert-symbol":
		_, editable := any(state.word).(document.WordTextRangeEditor)
		run, ok := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
		return editable && ok && run.Editable && wordBodyRunEditable(wordBlocksForState(state), run.ID)
	case "page-break", "layout-breaks", "blank-page":
		_, editor := any(state.word).(document.WordPageBreakEditor)
		run, ok := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
		return editor && ok && run.Editable && wordBodyRunEditable(wordBlocksForState(state), run.ID)
	case "format-painter":
		if state.wordFormatPainterActive {
			return true
		}
		_, painter := any(state.word).(document.WordFormatPainter)
		run, ok := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
		return painter && ok && run.Editable
	case "review-word-count":
		_, content := any(state.word).(document.WordContentReader)
		return content
	case "review-restrict":
		_, ok := any(state.word).(document.WordDocumentProtectionEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case "save":
		return (sessionDirty(state) || wordHasPendingTextDrafts(state) || state.wordParagraphSplitIntent != nil || state.wordParagraphJoinIntent != nil) && sessionPath(state) != ""
	case "insert-image":
		_, editable := any(state.word).(document.WordImageEditor)
		_, ok := selectedWordParagraph(wordBlocksForState(state), state.selectedWordParagraphID)
		return editable && ok
	case "replace-image", "delete-image", "resize-image":
		_, editable := any(state.word).(document.WordImageEditor)
		return editable && selectedWordImage(wordBlocksForState(state), state.selectedWordImageID)
	case "rotate-left", "rotate-right":
		_, rotatable := any(state.word).(document.WordImageRotator)
		return rotatable && selectedWordImage(wordBlocksForState(state), state.selectedWordImageID)
	case "design-compact", "design-spacious":
		_, ok := any(state.word).(document.WordParagraphSpacingEditor)
		return ok && len(state.wordBlocks) > 0 && !state.formatRunning.Load() && !a.isSaving()
	default:
		return false
	}
}

func wordToolbarButton(state *documentSessionState, id string) *widget.Clickable {
	if state.wordToolbarActionButtons == nil {
		state.wordToolbarActionButtons = make(map[string]*widget.Clickable)
	}
	button := state.wordToolbarActionButtons[id]
	if button == nil {
		button = new(widget.Clickable)
		state.wordToolbarActionButtons[id] = button
	}
	return button
}

func wordToolbarActionActive(state *documentSessionState, action ribbonAction) bool {
	if state == nil || state.word == nil {
		return false
	}
	if action.id == "format-painter" {
		return state.wordFormatPainterActive
	}
	if action.id == "autosave" {
		return state.wordAutoSave
	}
	if action.id == "view-ruler" {
		return !state.wordRulerHidden
	}
	if action.id == "view-gridlines" {
		return state.wordGridlinesVisible
	}
	if action.id == "view-reading" {
		return state.wordReadingMode
	}
	if action.id == "view-print-layout" {
		return state.wordViewMode == wordViewPrintLayout
	}
	if action.id == "view-web-layout" {
		return state.wordViewMode == wordViewWebLayout
	}
	if action.id == "view-navigation" {
		return state.wordNavigationVisible
	}
	if action.id == "view-single-page" {
		return !state.wordMultiplePagesMode && state.wordPageFitMode && state.wordViewMode == wordViewPrintLayout
	}
	if action.id == "view-multiple-pages" {
		return state.wordMultiplePagesMode && state.wordViewMode == wordViewPrintLayout
	}
	if action.id == "review-restrict" {
		return state.wordProtected
	}
	blocks := wordBlocksForState(state)
	if run, ok := selectedWordRun(blocks, state.selectedWordRunID); ok {
		switch action.id {
		case "bold":
			return run.Style.Bold
		case "italic":
			return run.Style.Italic
		case "underline":
			return run.Style.Underline
		case "font-color":
			return run.Style.FontColor != ""
		case "text-highlight":
			return run.Style.Highlight != ""
		}
	}
	if paragraph, ok := selectedWordParagraph(blocks, state.selectedWordParagraphID); ok {
		if action.id == "bullet-list" {
			return paragraph.ListKind == document.WordListBullet
		}
		if action.id == "numbered-list" {
			return paragraph.ListKind == document.WordListNumbered
		}
		alignment, isAlignment := map[string]document.WordAlignment{"align-left": document.WordAlignLeft, "align-center": document.WordAlignCenter, "align-right": document.WordAlignRight, "align-justify": document.WordAlignJustified}[action.id]
		return isAlignment && alignment == paragraph.Alignment
	}
	return false
}

func (a *App) wordToolbarActionDescription(action ribbonAction) string {
	if action.id == "version-history" && canonicalSessionPath(sessionPath(a.documentSessionState)) == "" {
		return action.label + "，不可用：请先保存文档"
	}
	return action.label
}

func (a *App) handleWordToolbarEvents(gtx layout.Context) {
	state := a.documentSessionState
	if state == nil || state.word == nil || a.modal.Visible || a.wordVersionHistoryVisible || a.wordSymbolPickerVisible {
		return
	}
	tabs := wordToolbarTabs(state)
	for index, tab := range tabs {
		if state.wordToolbarTabs[index].Clicked(gtx) {
			state.wordToolbarTab = tab
			state.wordToolbarActionList.List.Position = layout.Position{}
			state.wordToolbarColorPicker = false
			state.wordToolbarHighlightPicker = false
			break
		}
	}
	for _, action := range wordToolbarActions(state.wordToolbarTab) {
		button := state.wordToolbarActionButtons[action.id]
		if button == nil || !button.Clicked(gtx) || !a.wordToolbarActionEnabled(action) {
			continue
		}
		if action.id == "view-new-window" {
			a.requestWordNewWindow()
			continue
		}
		if wordHasPendingTextDrafts(state) && action.id == "paste" {
			state.wordClipboardRunID = state.selectedWordRunID
			state.wordForceDraftFlush.Store(true)
			state.wordFlushRequested.Store(true)
			state.wordFlushAllDrafts.Store(true)
			gtx.Execute(clipboard.ReadCmd{Tag: &state.wordClipboardTarget})
			continue
		}
		if wordHasPendingTextDrafts(state) && action.id == "cut" {
			if editor := state.wordRunEditors[state.selectedWordRunID]; editor != nil {
				value := editor.Text()
				if selected := editor.SelectedText(); selected != "" {
					value = selected
				}
				gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(value))})
			}
			state.wordForceDraftFlush.Store(true)
			state.wordFlushRequested.Store(true)
			state.wordFlushAllDrafts.Store(true)
			state.wordDeferredToolbarAction = action.id
			continue
		}
		if wordHasPendingTextDrafts(state) && action.id != "copy" {
			state.wordForceDraftFlush.Store(true)
			state.wordFlushRequested.Store(true)
			state.wordFlushAllDrafts.Store(true)
			if action.id == "save" {
				state.wordSaveAfterDrafts = true
				state.wordSaveAfterDraftsReason = wordSaveManual
			} else {
				state.wordDeferredToolbarAction = action.id
			}
			continue
		}
		if state.wordToolbarTab == wordTabFile && action.id != "save" {
			a.executeWordToolbarAction(action.id)
			continue
		}
		switch action.id {
		case "format-painter":
			a.toggleWordFormatPainter()
		case "undo":
			a.applyWordHistory(true)
		case "redo":
			a.applyWordHistory(false)
		case "cut", "copy":
			if action.id == "copy" {
				if selected, ok := wordPageSelectedText(state); ok {
					gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(selected))})
					continue
				}
			}
			if run, ok := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID); ok {
				value := run.Text
				if editor := state.wordRunEditors[run.ID]; editor != nil {
					value = editor.Text()
					if selected := editor.SelectedText(); selected != "" {
						value = selected
					}
				}
				gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(value))})
				if action.id == "cut" {
					runID := state.selectedWordRunID
					a.runWordCommand("剪切文字", func(editor document.WordTextEditor) error { return editor.ReplaceTextRun(runID, "") })
				}
			}
		case "paste":
			state.wordClipboardRunID = state.selectedWordRunID
			gtx.Execute(clipboard.ReadCmd{Tag: &state.wordClipboardTarget})
		case "bold", "italic", "underline":
			a.toggleWordTextStyle(action.id)
		case "font-size-decrease":
			a.adjustWordFontSize(-wordFontSizeStepPt)
		case "font-size-increase":
			a.adjustWordFontSize(wordFontSizeStepPt)
		case "font-family":
			a.showWordFontFamilyDialog()
		case "font-color":
			state.wordToolbarHighlightPicker = false
			state.wordToolbarColorPicker = !state.wordToolbarColorPicker
		case "text-highlight":
			state.wordToolbarColorPicker = false
			state.wordToolbarHighlightPicker = !state.wordToolbarHighlightPicker
		case "align-left", "align-center", "align-right", "align-justify":
			a.setWordAlignment(action.id)
		case "bullet-list", "numbered-list":
			a.setWordList(action.id)
		case "decrease-indent", "increase-indent":
			a.adjustWordParagraphIndent(action.id)
		case "layout-before", "layout-after":
			a.showWordParagraphSpacingDialog(action.id)
		case "layout-orientation":
			a.showWordOrientationDialog()
		case "layout-margins":
			a.showWordMarginsDialog()
		case "layout-size":
			a.showWordPageSizeDialog()
		case "layout-columns":
			a.showWordColumnsDialog()
		case "layout-breaks":
			a.showWordBreaksDialog()
		case "find":
			a.showWordFindDialog()
		case "replace":
			a.showWordReplaceDialog()
		case "insert-symbol":
			a.openWordSymbolPicker()
		case "review-word-count":
			a.showWordCountDetails()
		case "review-restrict":
			a.showWordDocumentProtectionDialog()
		case "view-print-layout", "view-web-layout", "view-navigation", "view-single-page", "view-multiple-pages", "view-reading", "view-zoom", "view-ruler", "view-gridlines":
			a.executeWordToolbarAction(action.id)
		case "save":
			a.startSave()
		case "insert-image":
			a.startWordImagePicker(wordImageInsert)
		case "replace-image":
			a.startWordImagePicker(wordImageReplace)
		case "delete-image":
			imageID := state.selectedWordImageID
			a.runWordImageCommand("删除图片", imageID, func(editor document.WordImageEditor) error { return editor.DeleteInlineImage(imageID) }, true)
		case "resize-image":
			a.showWordImageResizeDialog()
		case "rotate-left":
			a.rotateSelectedWordImage(-90)
		case "rotate-right":
			a.rotateSelectedWordImage(90)
		}
	}
	if state.wordToolbarColorPicker || state.wordToolbarHighlightPicker {
		for index := range state.wordToolbarColorButtons {
			if state.wordToolbarColorButtons[index].Clicked(gtx) {
				if state.wordToolbarHighlightPicker {
					a.chooseWordHighlightColor(index)
				} else {
					colorValue := wordFontColors[index].value
					runID := state.selectedWordRunID
					state.wordToolbarColorPicker = false
					a.setWordFontColor(runID, colorValue)
				}
				break
			}
		}
	}
}

func (a *App) executeWordToolbarAction(actionID string) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	switch actionID {
	case "new":
		word, err := docx.NewBlank()
		if err != nil {
			a.setStatus("新建 Word 文档失败：" + err.Error())
			return
		}
		if err := a.installDocument(loadedDocument{word: word}); err != nil {
			a.setStatus("新建 Word 文档失败：" + err.Error())
		}
	case "open":
		a.startOpenFilePicker()
	case "recent":
		a.activateRecent()
	case "save-as":
		a.startSaveAsPicker("docx")
	case "export-pdf":
		a.requestWordPDFExport()
	case "share":
		a.requestWordShareCopy()
	case "print":
		a.requestWordPrint()
	case "doc-info":
		a.startWordDocumentInfoLoad()
	case "insert-symbol":
		a.openWordSymbolPicker()
	case "version-history":
		a.openWordVersionHistory()
	case "view-new-window":
		a.requestWordNewWindow()
	case "autosave":
		state.wordAutoSave = !state.wordAutoSave
		if state.wordAutoSave {
			if state.path == "" {
				a.setStatus("自动保存将在文档另存为后启用")
			} else {
				a.setStatus("已开启自动保存")
			}
		} else {
			a.setStatus("已关闭自动保存")
		}
		a.persistActiveSession()
		a.maybeAutoSaveWord(state)
	case "format-painter":
		a.toggleWordFormatPainter()
	case "view-print-layout":
		a.toggleWordPrintLayout()
	case "view-web-layout":
		a.toggleWordWebLayout()
	case "view-navigation":
		a.toggleWordNavigationPane()
	case "view-single-page":
		a.selectWordSinglePageView()
	case "view-multiple-pages":
		a.selectWordMultiplePagesView()
	case "view-reading":
		a.toggleWordReadingMode()
	case "view-zoom":
		a.showWordZoomDialog()
	case "view-ruler":
		state.wordRulerHidden = !state.wordRulerHidden
		if state.wordRulerHidden {
			a.setStatus("已隐藏标尺")
		} else {
			a.setStatus("已显示标尺")
		}
	case "view-gridlines":
		state.wordGridlinesVisible = !state.wordGridlinesVisible
		if state.wordGridlinesVisible {
			a.setStatus("已显示网格线")
		} else {
			a.setStatus("已隐藏网格线")
		}
	case "apply-format-painter":
		a.startWordFormatPainter()
	case "undo":
		a.applyWordHistory(true)
	case "redo":
		a.applyWordHistory(false)
	case "cut":
		runID := state.selectedWordRunID
		a.runWordCommand("剪切文字", func(editor document.WordTextEditor) error { return editor.ReplaceTextRun(runID, "") })
	case "paste":
		return
	case "bold", "italic", "underline":
		a.toggleWordTextStyle(actionID)
	case "font-size-decrease":
		a.adjustWordFontSize(-wordFontSizeStepPt)
	case "font-size-increase":
		a.adjustWordFontSize(wordFontSizeStepPt)
	case "font-family":
		a.showWordFontFamilyDialog()
	case "font-color":
		state.wordToolbarHighlightPicker = false
		state.wordToolbarColorPicker = !state.wordToolbarColorPicker
	case "text-highlight":
		state.wordToolbarColorPicker = false
		state.wordToolbarHighlightPicker = !state.wordToolbarHighlightPicker
	case "align-left", "align-center", "align-right", "align-justify":
		a.setWordAlignment(actionID)
	case "bullet-list", "numbered-list":
		a.setWordList(actionID)
	case "decrease-indent", "increase-indent":
		a.adjustWordParagraphIndent(actionID)
	case "layout-before", "layout-after":
		a.showWordParagraphSpacingDialog(actionID)
	case "layout-indent":
		a.showWordParagraphIndentDialog()
	case "layout-orientation":
		a.showWordOrientationDialog()
	case "layout-margins":
		a.showWordMarginsDialog()
	case "layout-size":
		a.showWordPageSizeDialog()
	case "layout-columns":
		a.showWordColumnsDialog()
	case "layout-breaks":
		a.showWordBreaksDialog()
	case "find":
		a.showWordFindDialog()
	case "replace":
		a.showWordReplaceDialog()
	case "page-break":
		a.queueWordPageBreak()
	case "blank-page":
		a.queueWordBlankPage()
	case "design-compact":
		a.runWordSpacingCommand("紧凑段落间距", func(editor document.WordParagraphSpacingEditor) error {
			return editor.SetAllParagraphsSpacing(0, 80)
		})
	case "design-spacious":
		a.runWordSpacingCommand("宽松段落间距", func(editor document.WordParagraphSpacingEditor) error {
			return editor.SetAllParagraphsSpacing(0, 240)
		})
	case "review-word-count":
		a.showWordCountDetails()
	case "review-restrict":
		a.showWordDocumentProtectionDialog()
	case "ruler-indents":
		intent := state.wordRulerIndentIntent
		if intent == nil {
			return
		}
		paragraphID, leftEMU, rightEMU := intent.paragraphID, intent.leftEMU, intent.rightEMU
		a.runWordCommand("调整段落缩进", func(editor document.WordTextEditor) error {
			return editor.SetParagraphIndents(paragraphID, leftEMU, rightEMU)
		})
	case "save":
		a.startSave()
	case "insert-image":
		a.startWordImagePicker(wordImageInsert)
	case "replace-image":
		a.startWordImagePicker(wordImageReplace)
	case "delete-image":
		imageID := state.selectedWordImageID
		a.runWordImageCommand("删除图片", imageID, func(editor document.WordImageEditor) error { return editor.DeleteInlineImage(imageID) }, true)
	case "resize-image":
		a.showWordImageResizeDialog()
	case "rotate-left":
		a.rotateSelectedWordImage(-90)
	case "rotate-right":
		a.rotateSelectedWordImage(90)
	}
}

func (a *App) maybeAutoSaveWord(state *documentSessionState) {
	if state == nil || state != a.documentSessionState || state.word == nil || !state.wordAutoSave || sessionPath(state) == "" || a.isSaving() || state.formatRunning.Load() || state.wordFlushRequested.Load() || !sessionDirty(state) {
		return
	}
	a.startSaveForReason(wordSaveAuto)
}

func (a *App) handleWordClipboardEvents(gtx layout.Context) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	for {
		ev, ok := gtx.Event(transfer.TargetFilter{Target: &state.wordClipboardTarget, Type: "application/text"})
		if !ok {
			break
		}
		data, ok := ev.(transfer.DataEvent)
		if !ok {
			continue
		}
		reader := data.Open()
		value, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			a.setStatus("读取剪贴板失败：" + err.Error())
			continue
		}
		runID := state.wordClipboardRunID
		state.wordClipboardRunID = ""
		if state.formatRunning.Load() || wordHasPendingTextDrafts(state) {
			state.wordPendingPasteRunID = runID
			state.wordPendingPasteText = string(value)
			if wordHasPendingTextDrafts(state) {
				state.wordForceDraftFlush.Store(true)
				state.wordFlushRequested.Store(true)
				state.wordFlushAllDrafts.Store(true)
			}
			continue
		}
		a.runWordCommand("粘贴文字", func(editor document.WordTextEditor) error { return editor.ReplaceTextRun(runID, string(value)) })
	}
}

func (a *App) toggleWordTextStyle(property string) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	run, ok := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
	if !ok {
		return
	}
	runID := run.ID
	patch := document.WordTextFormatPatch{}
	switch property {
	case "bold":
		value := !run.Style.Bold
		patch.Bold = &value
	case "italic":
		value := !run.Style.Italic
		patch.Italic = &value
	case "underline":
		value := !run.Style.Underline
		patch.Underline = &value
	default:
		return
	}
	a.runWordCommand("文字格式", func(editor document.WordTextEditor) error { return editor.SetTextRunStyle(runID, patch) })
}

func (a *App) setWordFontColor(runID, value string) {
	a.runWordCommand("文字颜色", func(editor document.WordTextEditor) error {
		return editor.SetTextRunStyle(runID, document.WordTextFormatPatch{FontColor: &value})
	})
}

func (a *App) chooseWordHighlightColor(index int) {
	state := a.documentSessionState
	if state == nil || state.word == nil || index < 0 || index >= len(wordHighlightColors) {
		return
	}
	runID := state.selectedWordRunID
	run, ok := selectedWordRun(wordBlocksForState(state), runID)
	if !ok || !run.Editable {
		state.wordToolbarHighlightPicker = false
		return
	}
	value := wordHighlightColors[index].value
	state.wordToolbarHighlightPicker = false
	a.setWordTextHighlight(runID, value)
}

func (a *App) setWordTextHighlight(runID, value string) {
	a.runWordCommand("文字高亮", func(editor document.WordTextEditor) error {
		return editor.SetTextRunStyle(runID, document.WordTextFormatPatch{Highlight: &value})
	})
}

func effectiveWordFontSize(points float64) float64 {
	if points <= 0 || math.IsNaN(points) || math.IsInf(points, 0) {
		return wordDefaultFontSizePt
	}
	return points
}

func (a *App) adjustWordFontSize(delta float64) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	run, ok := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
	if !ok || !run.Editable {
		return
	}
	if _, ok := any(state.word).(document.WordTextFontSizeEditor); !ok {
		return
	}
	next := math.Max(wordMinFontSizePt, math.Min(wordMaxFontSizePt, effectiveWordFontSize(run.Style.FontSize)+delta))
	runID := run.ID
	a.runWordCommand("字号", func(editor document.WordTextEditor) error {
		fontSizeEditor, ok := any(editor).(document.WordTextFontSizeEditor)
		if !ok {
			return fmt.Errorf("DOCX adapter does not support font-size editing")
		}
		return fontSizeEditor.SetTextRunFontSize(runID, next)
	})
}

func (a *App) showWordFontFamilyDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordTextFontFamilyEditor); !ok {
		return
	}
	run, ok := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
	if !ok || !run.Editable {
		return
	}
	runID := run.ID
	a.modal.ShowInput("设置字体", "输入字体名称", run.Style.FontFamily, "字体名称", func(action string) {
		value := strings.TrimSpace(a.modal.Input.Text())
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		if value == "" {
			a.setStatus("字体名称不能为空")
			return
		}
		a.runWordCommand("字体", func(editor document.WordTextEditor) error {
			fontFamilyEditor, ok := any(editor).(document.WordTextFontFamilyEditor)
			if !ok {
				return fmt.Errorf("DOCX adapter does not support font-family editing")
			}
			return fontFamilyEditor.SetTextRunFontFamily(runID, value)
		})
	}, modalDialogActionSpec{ID: "confirm", Label: "应用"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}

func (a *App) toggleWordFormatPainter() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if state.wordFormatPainterActive {
		state.wordFormatPainterActive = false
		state.wordFormatPainterSourceRunID = ""
		state.wordFormatPainterTargetRunID = ""
		state.wordFormatPainterTargetParagraphID = ""
		a.setStatus("格式刷已取消")
		return
	}
	run, ok := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
	if !ok || !run.Editable {
		return
	}
	if _, ok := any(state.word).(document.WordFormatPainter); !ok {
		return
	}
	state.wordFormatPainterSourceRunID = run.ID
	state.wordFormatPainterActive = true
	a.setStatus("已启动格式刷；单击目标文字应用，再点格式刷取消")
}

func (a *App) queueWordFormatPainterTarget(paragraphID, runID string) {
	state := a.documentSessionState
	if state == nil || state.word == nil || !state.wordFormatPainterActive || runID == state.wordFormatPainterSourceRunID {
		return
	}
	blocks := wordBlocksForState(state)
	run, ok := selectedWordRun(blocks, runID)
	paragraph, paragraphOK := selectedWordParagraph(blocks, paragraphID)
	if !ok || !run.Editable || !paragraphOK || !wordParagraphContainsRun(paragraph, runID) {
		return
	}
	if _, ok := any(state.word).(document.WordFormatPainter); !ok {
		return
	}
	state.wordFormatPainterActive = false
	state.selectedWordParagraphID = paragraphID
	state.selectedWordRunID = runID
	state.wordFormatPainterTargetParagraphID = paragraphID
	state.wordFormatPainterTargetRunID = runID
	if wordHasPendingTextDrafts(state) {
		state.wordForceDraftFlush.Store(true)
		state.wordFlushRequested.Store(true)
		state.wordFlushAllDrafts.Store(true)
		state.wordDeferredToolbarAction = "apply-format-painter"
		return
	}
	a.startWordFormatPainter()
}

func (a *App) processWordFormatPainterTargetIntent() {
	state := a.documentSessionState
	if state == nil || state.wordFormatPainterPendingRunID == "" {
		return
	}
	paragraphID, runID := state.wordFormatPainterPendingParagraphID, state.wordFormatPainterPendingRunID
	state.wordFormatPainterPendingParagraphID = ""
	state.wordFormatPainterPendingRunID = ""
	a.queueWordFormatPainterTarget(paragraphID, runID)
}

func wordParagraphContainsRun(paragraph document.WordParagraph, runID string) bool {
	for _, inline := range paragraph.Inlines {
		if inline.Text != nil && inline.Text.ID == runID {
			return true
		}
	}
	return false
}

func (a *App) startWordFormatPainter() {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() || state.wordFormatPainterSourceRunID == "" || state.wordFormatPainterTargetRunID == "" {
		return
	}
	painter, ok := any(state.word).(document.WordFormatPainter)
	if !ok {
		state.wordFormatPainterSourceRunID = ""
		state.wordFormatPainterTargetRunID = ""
		state.wordFormatPainterTargetParagraphID = ""
		return
	}
	sourceRunID, targetRunID := state.wordFormatPainterSourceRunID, state.wordFormatPainterTargetRunID
	state.wordFormatPainterSourceRunID = ""
	state.wordFormatPainterTargetRunID = ""
	state.wordFormatPainterTargetParagraphID = ""
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return
	}
	a.setStatus("正在应用格式刷")
	generation := a.documentGeneration
	go func() {
		blocks, err := runWordMutation(state, func() error { return painter.ApplyWordFormatting(sourceRunID, targetRunID) })
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: "格式刷", err: err, blocks: blocks})
	}()
}

func (a *App) setWordAlignment(actionID string) {
	state := a.documentSessionState
	if state == nil {
		return
	}
	paragraphID := state.selectedWordParagraphID
	alignment := map[string]document.WordAlignment{"align-left": document.WordAlignLeft, "align-center": document.WordAlignCenter, "align-right": document.WordAlignRight, "align-justify": document.WordAlignJustified}[actionID]
	a.runWordCommand("段落对齐", func(editor document.WordTextEditor) error {
		return editor.SetParagraphAlignment(paragraphID, alignment)
	})
}

func (a *App) setWordList(actionID string) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	paragraph, ok := selectedWordParagraph(wordBlocksForState(state), state.selectedWordParagraphID)
	if !ok {
		return
	}
	kind := document.WordListBullet
	if actionID == "numbered-list" {
		kind = document.WordListNumbered
	}
	if paragraph.ListKind == kind {
		kind = document.WordListNone
	}
	paragraphID := paragraph.ID
	a.runWordListCommand("段落列表", func(editor document.WordParagraphListEditor) error {
		return editor.SetParagraphList(paragraphID, kind)
	})
}

type wordTextMatch struct {
	ParagraphID string
	RunID       string
	Start       int
	End         int
}

func findWordTextMatch(blocks []document.WordBlock, query, currentRunID string, currentEnd int, drafts map[string]string) (wordTextMatch, bool) {
	needle := []rune(query)
	if len(needle) == 0 {
		return wordTextMatch{}, false
	}
	needle = lowerWordRunes(needle)
	type candidate struct {
		paragraphID string
		run         document.WordTextRun
	}
	var candidates []candidate
	currentIndex := -1
	for _, paragraph := range allWordParagraphs(blocks) {
		for _, inline := range paragraph.Inlines {
			if inline.Text == nil || !inline.Text.Editable {
				continue
			}
			if inline.Text.ID == currentRunID {
				currentIndex = len(candidates)
			}
			candidates = append(candidates, candidate{paragraphID: paragraph.ID, run: *inline.Text})
		}
	}
	if len(candidates) == 0 {
		return wordTextMatch{}, false
	}
	search := func(index, from int) (wordTextMatch, bool) {
		item := candidates[index]
		value := item.run.Text
		if draft, ok := drafts[item.run.ID]; ok {
			value = draft
		}
		start := findWordRunes(lowerWordRunes([]rune(value)), needle, from)
		if start < 0 {
			return wordTextMatch{}, false
		}
		return wordTextMatch{ParagraphID: item.paragraphID, RunID: item.run.ID, Start: start, End: start + len(needle)}, true
	}
	if currentIndex < 0 {
		for index := range candidates {
			if match, ok := search(index, 0); ok {
				return match, true
			}
		}
		return wordTextMatch{}, false
	}
	if currentEnd < 0 {
		currentEnd = 0
	}
	if match, ok := search(currentIndex, currentEnd); ok {
		return match, true
	}
	for offset := 1; offset < len(candidates); offset++ {
		index := (currentIndex + offset) % len(candidates)
		if match, ok := search(index, 0); ok {
			return match, true
		}
	}
	return search(currentIndex, 0)
}

func lowerWordRunes(value []rune) []rune {
	lowered := make([]rune, len(value))
	for index, r := range value {
		lowered[index] = unicode.ToLower(r)
	}
	return lowered
}

func findWordRunes(value, needle []rune, from int) int {
	if len(needle) == 0 || from < 0 {
		return -1
	}
	for start := from; start+len(needle) <= len(value); start++ {
		matched := true
		for offset, want := range needle {
			if value[start+offset] != want {
				matched = false
				break
			}
		}
		if matched {
			return start
		}
	}
	return -1
}

func (a *App) showWordFindDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordContentReader); !ok {
		return
	}
	a.modal.ShowInput("查找", "在当前文档中查找文字", state.wordFindQuery, "查找内容", func(action string) {
		query := a.modal.Input.Text()
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		if query == "" {
			a.setStatus("请输入要查找的文字")
			return
		}
		state.wordFindQuery = query
		blocks := wordBlocksForState(state)
		drafts := state.wordDraftSnapshot()
		for runID, editor := range state.wordRunEditors {
			drafts[runID] = editor.Text()
		}
		currentEnd := 0
		if editor := state.wordRunEditors[state.selectedWordRunID]; editor != nil {
			_, currentEnd = editor.Selection()
		}
		match, found := findWordTextMatch(blocks, query, state.selectedWordRunID, currentEnd, drafts)
		if !found {
			a.setStatus("未找到：" + query)
			return
		}
		a.focusWordTextMatch(state, blocks, match)
		a.setStatus("已定位：" + query)
	})
}

func (a *App) showWordReplaceDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordContentReader); !ok {
		return
	}
	if _, ok := any(state.word).(document.WordTextRangeEditor); !ok {
		return
	}
	a.modal.ShowDualInput("替换", "在当前文档中查找和替换文字", state.wordFindQuery, "查找内容", "", "替换为", func(action string) {
		if action == "close" {
			a.modal.Hide()
			return
		}
		query := a.modal.Input.Text()
		replacement := a.modal.SecondInput.Text()
		if query == "" {
			a.setStatus("请输入要查找的文字")
			return
		}
		state.wordFindQuery = query
		if action == "confirm" {
			a.findNextWordText(query)
			return
		}
		if action != "replace" {
			a.findNextWordText(query)
			return
		}
		match, found := selectedWordTextQueryMatch(state, query)
		if !found {
			a.findNextWordText(query)
			return
		}
		if !a.runWordRangeReplacement(match, query, replacement) {
			a.setStatus("当前 DOCX 暂时无法替换文字")
		}
	},
		modalDialogActionSpec{ID: "find-next", Label: "查找下一个"},
		modalDialogActionSpec{ID: "replace", Label: "替换"},
		modalDialogActionSpec{ID: "close", Label: "关闭"},
	)
}

func (a *App) findNextWordText(query string) {
	state := a.documentSessionState
	if state == nil || state.word == nil || query == "" {
		return
	}
	blocks := wordBlocksForState(state)
	drafts := state.wordDraftSnapshot()
	for runID, editor := range state.wordRunEditors {
		drafts[runID] = editor.Text()
	}
	currentEnd := 0
	if editor := state.wordRunEditors[state.selectedWordRunID]; editor != nil {
		_, currentEnd = editor.Selection()
	}
	match, found := findWordTextMatch(blocks, query, state.selectedWordRunID, currentEnd, drafts)
	if !found {
		a.setStatus("未找到：" + query)
		return
	}
	a.focusWordTextMatch(state, blocks, match)
	a.setStatus("已定位：" + query)
}

func selectedWordTextQueryMatch(state *documentSessionState, query string) (wordTextMatch, bool) {
	if state == nil || query == "" {
		return wordTextMatch{}, false
	}
	blocks := wordBlocksForState(state)
	run, ok := selectedWordRun(blocks, state.selectedWordRunID)
	if !ok || !run.Editable {
		return wordTextMatch{}, false
	}
	editor := state.wordRunEditors[run.ID]
	if editor == nil {
		return wordTextMatch{}, false
	}
	start, end := editor.Selection()
	value := []rune(editor.Text())
	needle := lowerWordRunes([]rune(query))
	if start < 0 || end <= start || end > len(value) || end-start != len(needle) {
		return wordTextMatch{}, false
	}
	if !equalWordRunes(lowerWordRunes(value[start:end]), needle) {
		return wordTextMatch{}, false
	}
	paragraph, ok := selectedWordParagraph(blocks, state.selectedWordParagraphID)
	if !ok {
		return wordTextMatch{}, false
	}
	return wordTextMatch{ParagraphID: paragraph.ID, RunID: run.ID, Start: start, End: end}, true
}

func equalWordRunes(left, right []rune) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (a *App) runWordRangeReplacement(match wordTextMatch, query, replacement string) bool {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return false
	}
	editor, ok := any(state.word).(document.WordTextRangeEditor)
	if !ok {
		return false
	}
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return false
	}
	generation := a.documentGeneration
	go func() {
		blocks, err := runWordMutation(state, func() error { return editor.ReplaceTextRange(match.RunID, match.Start, match.End, replacement) })
		a.publishWordCommand(wordCommandResult{
			state: state, generation: generation, operation: "替换文字", err: err, blocks: blocks,
			wordRangeReplacement: true, replaceRunID: match.RunID, replaceParagraphID: match.ParagraphID, replaceStart: match.Start,
			replaceText: replacement, replaceQuery: query,
		})
	}()
	return true
}

func (a *App) completeWordRangeReplacement(result wordCommandResult) {
	state := result.state
	if state == nil {
		return
	}
	if result.err != nil {
		a.setStatus(wordCommandError(result.operation, result.err))
		return
	}
	if result.blocks != nil {
		state.setWordBlocks(result.blocks)
		a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
	}
	blocks := wordBlocksForState(state)
	run, ok := selectedWordRun(blocks, result.replaceRunID)
	if !ok || !run.Editable {
		a.setStatus("文字已替换，但无法定位编辑位置")
		return
	}
	state.setCommittedWordTextAndClearDraft(run.ID, run.Text)
	editor := wordRunEditor(state, run)
	editor.SetText(run.Text)
	state.selectedWordRunID = run.ID
	state.selectedWordParagraphID = result.replaceParagraphID
	if state.selectedWordParagraphID == "" {
		for _, paragraph := range allWordParagraphs(blocks) {
			for _, inline := range paragraph.Inlines {
				if inline.Text != nil && inline.Text.ID == run.ID {
					state.selectedWordParagraphID = paragraph.ID
					break
				}
			}
		}
	}
	state.selectedWordImageID = ""
	state.selectedBlock = -1
	state.selectedBlockEditable = true
	startAfter := result.replaceStart + len([]rune(result.replaceText))
	match, found := findWordTextMatch(blocks, result.replaceQuery, run.ID, startAfter, nil)
	if found {
		a.focusWordTextMatch(state, blocks, match)
		a.setStatus("已替换，并已定位下一处")
	} else {
		startAfter = max(0, min(startAfter, len([]rune(run.Text))))
		editor.SetCaret(startAfter, startAfter)
		state.wordPendingFocusRunID = run.ID
		a.setStatus("已替换；没有更多匹配项")
	}
	a.persistActiveSession()
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) focusWordTextMatch(state *documentSessionState, blocks []document.WordBlock, match wordTextMatch) {
	run, ok := selectedWordRun(blocks, match.RunID)
	paragraph, paragraphOK := selectedWordParagraph(blocks, match.ParagraphID)
	if !ok || !paragraphOK || !run.Editable {
		return
	}
	editor := wordRunEditor(state, run)
	editor.SetCaret(match.Start, match.End)
	state.selectedWordRunID = run.ID
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordImageID = ""
	state.wordImageFocusRequestID = ""
	state.selectedBlock = -1
	state.selectedBlockEditable = true
	if state.wordToolbarTab == wordTabPicture {
		state.wordToolbarTab = wordTabHome
	}
	state.wordPendingFocusRunID = run.ID
	state.wordPendingCaretRunID = ""
	state.wordPendingCaretByX = false
	for index, block := range blocks {
		if wordBlockContainsParagraph(block, paragraph.ID) {
			state.wordListState.Position.First = index
			state.wordListState.Position.Offset = 0
			state.wordListState.Position.OffsetLast = 0
			state.wordListState.Position.BeforeEnd = false
			break
		}
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func wordBlockContainsParagraph(block document.WordBlock, paragraphID string) bool {
	if block.Paragraph != nil {
		return block.Paragraph.ID == paragraphID
	}
	if block.Table == nil {
		return false
	}
	for _, row := range block.Table.Rows {
		for _, cell := range row.Cells {
			for _, paragraph := range cell.Paragraphs {
				if paragraph.ID == paragraphID {
					return true
				}
			}
		}
	}
	return false
}

func (a *App) adjustWordParagraphIndent(actionID string) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	paragraph, ok := selectedWordParagraph(wordBlocksForState(state), state.selectedWordParagraphID)
	if !ok {
		return
	}
	if paragraph.ListKind == document.WordListBullet || paragraph.ListKind == document.WordListNumbered {
		_, ok := any(state.word).(document.WordParagraphListEditor)
		if !ok {
			return
		}
		paragraphID, level := paragraph.ID, paragraph.ListLevel
		if actionID == "increase-indent" {
			a.runWordListCommand("调整列表层级", func(editor document.WordParagraphListEditor) error {
				return editor.SetParagraphListLevel(paragraphID, level+1)
			})
			return
		}
		if level > 0 {
			a.runWordListCommand("调整列表层级", func(editor document.WordParagraphListEditor) error {
				return editor.SetParagraphListLevel(paragraphID, level-1)
			})
			return
		}
		a.runWordListCommand("取消列表", func(editor document.WordParagraphListEditor) error {
			return editor.SetParagraphList(paragraphID, document.WordListNone)
		})
		return
	}
	if paragraph.ListKind == document.WordListUnsupported {
		return
	}
	leftIndent := paragraph.LeftIndentEMU
	if actionID == "increase-indent" {
		if leftIndent > wordMaxParagraphIndentEMU-wordParagraphIndentStepEMU {
			return
		}
		leftIndent += wordParagraphIndentStepEMU
	} else {
		leftIndent = max(0, leftIndent-wordParagraphIndentStepEMU)
	}
	paragraphID, rightIndent := paragraph.ID, paragraph.RightIndentEMU
	a.runWordCommand("调整段落缩进", func(editor document.WordTextEditor) error {
		return editor.SetParagraphIndents(paragraphID, leftIndent, rightIndent)
	})
}

func (a *App) showWordParagraphSpacingDialog(actionID string) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	paragraph, ok := selectedWordParagraph(wordBlocksForState(state), state.selectedWordParagraphID)
	if !ok {
		return
	}
	paragraphID := paragraph.ID
	title, message, currentPT, opName := "段前间距", "设置段前间距（磅，0–1584）", paragraph.Layout.BeforePT, "设置段前间距"
	if actionID == "layout-after" {
		title, message, currentPT, opName = "段后间距", "设置段后间距（磅，0–1584）", paragraph.Layout.AfterPT, "设置段后间距"
	}
	initialVal := strconv.FormatFloat(currentPT, 'f', -1, 64)
	a.modal.ShowInput(title, message, initialVal, "间距（磅）", func(action string) {
		draft := strings.TrimSpace(a.modal.Input.Text())
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		pt, err := strconv.ParseFloat(draft, 64)
		if err != nil || math.IsNaN(pt) || math.IsInf(pt, 0) || pt < 0 || pt > 1584 {
			a.setStatus(title + "必须在 0 到 1584 磅之间")
			if a.window != nil {
				a.window.Invalidate()
			}
			return
		}
		twips := int64(math.Round(pt * 20.0))
		a.runWordSpacingCommand(opName, func(editor document.WordParagraphSpacingEditor) error {
			if actionID == "layout-before" {
				return editor.SetParagraphSpacing(paragraphID, &twips, nil)
			}
			return editor.SetParagraphSpacing(paragraphID, nil, &twips)
		})
	}, modalDialogActionSpec{ID: "confirm", Label: "确定"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}

func (a *App) runWordSpacingCommand(operation string, edit func(document.WordParagraphSpacingEditor) error) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := any(state.word).(document.WordParagraphSpacingEditor)
	if !ok {
		return
	}
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return
	}
	generation := a.documentGeneration
	go func() {
		blocks, err := runWordMutation(state, func() error { return edit(editor) })
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: operation, err: err, blocks: blocks})
	}()
}

func (a *App) runWordProtectionCommand(operation string, edit func(document.WordDocumentProtectionEditor) error) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := any(state.word).(document.WordDocumentProtectionEditor)
	if !ok {
		return
	}
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return
	}
	generation := a.documentGeneration
	go func() {
		blocks, err := runWordMutation(state, func() error { return edit(editor) })
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: operation, err: err, blocks: blocks})
	}()
}

func (a *App) showWordDocumentProtectionDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordDocumentProtectionEditor)
	if !ok {
		return
	}
	protected := state.wordProtected
	if isProtected, err := editor.GetWordDocumentProtection(); err == nil {
		protected = isProtected
		state.wordProtected = isProtected
	}

	if protected {
		a.modal.Show("限制编辑", "当前文档已开启编辑限制（只读保护）。\n是否解除编辑限制以允许修改？", func(action string) {
			a.modal.Hide()
			if action == "unprotect" {
				a.runWordProtectionCommand("解除限制编辑", func(ed document.WordDocumentProtectionEditor) error {
					return ed.SetWordDocumentProtection(false)
				})
			}
		},
			modalDialogActionSpec{ID: "unprotect", Label: "解除限制"},
			modalDialogActionSpec{ID: "cancel", Label: "取消"},
		)
	} else {
		a.modal.Show("限制编辑", "开启编辑限制后，文档将被标记为受保护（只读模式），防止误修改正文内容。\n是否开启限制编辑？", func(action string) {
			a.modal.Hide()
			if action == "protect" {
				a.runWordProtectionCommand("开启限制编辑", func(ed document.WordDocumentProtectionEditor) error {
					return ed.SetWordDocumentProtection(true)
				})
			}
		},
			modalDialogActionSpec{ID: "protect", Label: "开启限制"},
			modalDialogActionSpec{ID: "cancel", Label: "取消"},
		)
	}
}

func (a *App) showWordParagraphIndentDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	paragraph, ok := selectedWordParagraph(wordBlocksForState(state), state.selectedWordParagraphID)
	if !ok {
		return
	}
	paragraphID := paragraph.ID
	leftPT := float64(paragraph.LeftIndentEMU) / 12700.0
	rightPT := float64(paragraph.RightIndentEMU) / 12700.0
	labels := []string{"左侧缩进（磅）", "右侧缩进（磅）"}
	hints := []string{"0", "0"}
	initial := []string{
		strconv.FormatFloat(leftPT, 'f', -1, 64),
		strconv.FormatFloat(rightPT, 'f', -1, 64),
	}
	a.modal.ShowForm("段落缩进", "设置所选段落的左侧和右侧缩进（0–1000 磅）：",
		labels, hints, initial,
		func(action string) {
			switch action {
			case "none":
				a.modal.Hide()
				a.runWordCommand("清除段落缩进", func(editor document.WordTextEditor) error {
					return editor.SetParagraphIndents(paragraphID, 0, 0)
				})
			case "confirm":
				left, err1 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[0].Text()), 64)
				right, err2 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[1].Text()), 64)
				if err1 != nil || err2 != nil || left < 0 || right < 0 || left > 1000 || right > 1000 {
					a.setStatus("缩进必须是 0 到 1000 磅之间的数值")
					if a.window != nil {
						a.window.Invalidate()
					}
					return
				}
				a.modal.Hide()
				leftEMU := int64(math.Round(left * 12700.0))
				rightEMU := int64(math.Round(right * 12700.0))
				a.runWordCommand("设置段落缩进", func(editor document.WordTextEditor) error {
					return editor.SetParagraphIndents(paragraphID, leftEMU, rightEMU)
				})
			case "cancel":
				a.modal.Hide()
			}
		},
		modalDialogActionSpec{ID: "confirm", Label: "应用"},
		modalDialogActionSpec{ID: "none", Label: "无缩进 (0pt)"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordOrientationDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordPageSetupEditor); !ok {
		return
	}
	currentOrient := "portrait"
	if data, ok := wordLayoutDataForState(state); ok && len(data.Sections) > 0 {
		if o := strings.ToLower(data.Sections[0].PageSetup.Orientation); o != "" {
			currentOrient = o
		}
	}
	label := "纵向"
	if currentOrient == "landscape" {
		label = "横向"
	}
	message := fmt.Sprintf("当前页面方向：%s\n请选择目标方向：", label)
	a.modal.Show("页面方向", message, func(action string) {
		a.modal.Hide()
		if action == "portrait" || action == "landscape" {
			a.runWordPageSetupCommand("修改页面方向", func(editor document.WordPageSetupEditor) error {
				return editor.SetPageOrientation(action)
			})
		}
	},
		modalDialogActionSpec{ID: "portrait", Label: "纵向"},
		modalDialogActionSpec{ID: "landscape", Label: "横向"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordMarginsDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordPageSetupEditor); !ok {
		return
	}
	topPT, rightPT, bottomPT, leftPT := 72.0, 72.0, 72.0, 72.0
	if data, ok := wordLayoutDataForState(state); ok && len(data.Sections) > 0 {
		s := data.Sections[0].PageSetup
		if s.Set&document.WordPageMarginTopSet != 0 || s.MarginTopPT > 0 {
			topPT = s.MarginTopPT
		}
		if s.Set&document.WordPageMarginRightSet != 0 || s.MarginRightPT > 0 {
			rightPT = s.MarginRightPT
		}
		if s.Set&document.WordPageMarginBottomSet != 0 || s.MarginBottomPT > 0 {
			bottomPT = s.MarginBottomPT
		}
		if s.Set&document.WordPageMarginLeftSet != 0 || s.MarginLeftPT > 0 {
			leftPT = s.MarginLeftPT
		}
	}
	labels := []string{"上边距（磅）", "右边距（磅）", "下边距（磅）", "左边距（磅）"}
	hints := []string{"72", "72", "72", "72"}
	initial := []string{
		strconv.FormatFloat(topPT, 'f', -1, 64),
		strconv.FormatFloat(rightPT, 'f', -1, 64),
		strconv.FormatFloat(bottomPT, 'f', -1, 64),
		strconv.FormatFloat(leftPT, 'f', -1, 64),
	}
	a.modal.ShowForm("页边距设置", "可选择快速预设或输入自定义页边距（磅，1 厘米 ≈ 28.35 磅）：",
		labels, hints, initial,
		func(action string) {
			switch action {
			case "normal":
				a.modal.Hide()
				a.runWordPageSetupCommand("设置普通页边距", func(editor document.WordPageSetupEditor) error {
					return editor.SetPageMargins(72, 72, 72, 72)
				})
			case "narrow":
				a.modal.Hide()
				a.runWordPageSetupCommand("设置窄页边距", func(editor document.WordPageSetupEditor) error {
					return editor.SetPageMargins(36, 36, 36, 36)
				})
			case "confirm":
				top, err1 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[0].Text()), 64)
				right, err2 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[1].Text()), 64)
				bottom, err3 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[2].Text()), 64)
				left, err4 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[3].Text()), 64)
				if err1 != nil || err2 != nil || err3 != nil || err4 != nil ||
					top < 0 || right < 0 || bottom < 0 || left < 0 ||
					top > 1000 || right > 1000 || bottom > 1000 || left > 1000 {
					a.setStatus("页边距必须是 0 到 1000 磅之间的数值")
					if a.window != nil {
						a.window.Invalidate()
					}
					return
				}
				a.modal.Hide()
				a.runWordPageSetupCommand("设置页边距", func(editor document.WordPageSetupEditor) error {
					return editor.SetPageMargins(top, right, bottom, left)
				})
			case "cancel":
				a.modal.Hide()
			}
		},
		modalDialogActionSpec{ID: "confirm", Label: "应用"},
		modalDialogActionSpec{ID: "normal", Label: "普通 (72pt)"},
		modalDialogActionSpec{ID: "narrow", Label: "窄边距 (36pt)"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordPageSizeDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordPageSetupEditor); !ok {
		return
	}
	wPT, hPT := 595.3, 841.9
	if data, ok := wordLayoutDataForState(state); ok && len(data.Sections) > 0 {
		s := data.Sections[0].PageSetup
		if s.WidthPT > 0 {
			wPT = s.WidthPT
		}
		if s.HeightPT > 0 {
			hPT = s.HeightPT
		}
	}
	labels := []string{"宽度（磅）", "高度（磅）"}
	hints := []string{"A4 约为 595.3", "A4 约为 841.9"}
	initial := []string{
		strconv.FormatFloat(wPT, 'f', 1, 64),
		strconv.FormatFloat(hPT, 'f', 1, 64),
	}
	a.modal.ShowForm("纸张大小设置", "可选择标准纸张规格或输入自定义尺寸（磅）：",
		labels, hints, initial,
		func(action string) {
			switch action {
			case "a4":
				a.modal.Hide()
				a.runWordPageSetupCommand("设置 A4 纸张大小", func(editor document.WordPageSetupEditor) error {
					return editor.SetPageSize(595.3, 841.9)
				})
			case "letter":
				a.modal.Hide()
				a.runWordPageSetupCommand("设置 Letter 纸张大小", func(editor document.WordPageSetupEditor) error {
					return editor.SetPageSize(612, 792)
				})
			case "confirm":
				w, err1 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[0].Text()), 64)
				h, err2 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[1].Text()), 64)
				if err1 != nil || err2 != nil || w <= 0 || h <= 0 || w > 3000 || h > 3000 {
					a.setStatus("纸张宽度和高度必须在 1 到 3000 磅之间")
					if a.window != nil {
						a.window.Invalidate()
					}
					return
				}
				a.modal.Hide()
				a.runWordPageSetupCommand("设置纸张大小", func(editor document.WordPageSetupEditor) error {
					return editor.SetPageSize(w, h)
				})
			case "cancel":
				a.modal.Hide()
			}
		},
		modalDialogActionSpec{ID: "confirm", Label: "应用"},
		modalDialogActionSpec{ID: "a4", Label: "A4 (210×297mm)"},
		modalDialogActionSpec{ID: "letter", Label: "Letter (8.5×11in)"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordColumnsDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordPageSetupEditor); !ok {
		return
	}
	message := "选择页面分栏排版布局："
	a.modal.Show("分栏设置", message, func(action string) {
		a.modal.Hide()
		var cols int
		switch action {
		case "col1":
			cols = 1
		case "col2":
			cols = 2
		case "col3":
			cols = 3
		default:
			return
		}
		a.runWordPageSetupCommand(fmt.Sprintf("设置页面分栏为 %d 栏", cols), func(editor document.WordPageSetupEditor) error {
			return editor.SetPageColumns(cols)
		})
	},
		modalDialogActionSpec{ID: "col1", Label: "一栏 (单栏常规)"},
		modalDialogActionSpec{ID: "col2", Label: "两栏 (双栏排版)"},
		modalDialogActionSpec{ID: "col3", Label: "三栏 (三栏报刊)"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordBreaksDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordPageBreakEditor); !ok {
		return
	}
	message := "选择在当前光标位置插入的分隔符类型："
	a.modal.Show("插入分隔符", message, func(action string) {
		a.modal.Hide()
		switch action {
		case "page":
			a.queueWordBreak(document.WordBreakPage)
		case "column":
			a.queueWordBreak(document.WordBreakColumn)
		}
	},
		modalDialogActionSpec{ID: "page", Label: "分页符 (进入下一页)"},
		modalDialogActionSpec{ID: "column", Label: "分栏符 (进入下一栏)"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) runWordPageSetupCommand(operation string, edit func(document.WordPageSetupEditor) error) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := any(state.word).(document.WordPageSetupEditor)
	if !ok {
		return
	}
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return
	}
	generation := a.documentGeneration
	go func() {
		blocks, err := runWordMutation(state, func() error { return edit(editor) })
		if err == nil {
			state.invalidateWordLayoutAfterBlocksChange()
		}
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: operation, err: err, blocks: blocks})
	}()
}

func (a *App) showWordCountDetails() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordContentReader); !ok {
		return
	}
	statistics := wordTextStatisticsForState(state)
	message := fmt.Sprintf(
		"字符数（不含空白）：%d\n字符数（含空白）：%d\n段落数：%d\n表格数：%d",
		statistics.charactersWithoutWhitespace,
		statistics.charactersWithWhitespace,
		statistics.paragraphs,
		statistics.tables,
	)
	a.modal.Show("字数统计", message, func(action string) {
		if action == "close" {
			a.modal.Hide()
		}
	}, modalDialogActionSpec{ID: "close", Label: "关闭"})
}

func (a *App) runWordCommand(operation string, edit func(document.WordTextEditor) error) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := any(state.word).(document.WordTextEditor)
	if !ok {
		return
	}
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return
	}
	generation := a.documentGeneration
	go func() {
		blocks, err := runWordMutation(state, func() error { return edit(editor) })
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: operation, err: err, blocks: blocks})
	}()
}

func (a *App) runWordListCommand(operation string, edit func(document.WordParagraphListEditor) error) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := any(state.word).(document.WordParagraphListEditor)
	if !ok {
		return
	}
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return
	}
	generation := a.documentGeneration
	go func() {
		blocks, err := runWordMutation(state, func() error { return edit(editor) })
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: operation, err: err, blocks: blocks})
	}()
}

func (a *App) runWordImageCommand(operation, imageID string, edit func(document.WordImageEditor) error, clearSelection bool) bool {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return false
	}
	editor, ok := any(state.word).(document.WordImageEditor)
	if !ok {
		return false
	}
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return false
	}
	generation := a.documentGeneration
	go func() {
		blocks, err := runWordMutation(state, func() error { return edit(editor) })
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: operation, err: err, clearImageSelection: clearSelection && err == nil, imageID: imageID, blocks: blocks})
	}()
	return true
}

func (a *App) rotateSelectedWordImage(delta float64) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	image := findWordImage(wordBlocksForState(state), state.selectedWordImageID)
	if image == nil {
		return
	}
	imageID := image.ID
	a.runWordImageRotationCommand(imageID, image.RotationDegrees+delta)
}

type wordCommandResult struct {
	state                 *documentSessionState
	operationToken        sessionOperationToken
	revision              uint64
	blocks                []document.WordBlock
	selectInsertedImage   bool
	generation            uint64
	operation             string
	err                   error
	clearImageSelection   bool
	imageID               string
	undo                  bool
	history               bool
	applied               bool
	textDraft             bool
	paragraphSplit        bool
	paragraphJoin         bool
	runID                 string
	runText               string
	splitSourceRunID      string
	splitPrefix           string
	splitParagraphID      string
	splitRunID            string
	splitRunText          string
	joinParagraphID       string
	joinRunID             string
	joinCaret             int
	insertedParagraphID   string
	bodyParagraphInsert   bool
	wordRangeReplacement  bool
	wordSymbolInsertion   bool
	pageBreak             bool
	replaceRunID          string
	replaceParagraphID    string
	replaceStart          int
	replaceText           string
	replaceQuery          string
	insertedSymbol        string
	pageBreakSourcePrefix string
	pageBreakParagraphID  string
	pageBreakCaretRunID   string
}

func (a *App) publishWordCommand(result wordCommandResult) {
	if result.state != nil && result.operationToken.generation == 0 {
		result.operationToken, _ = result.state.currentOperationToken()
	}
	if result.state != nil && result.operationToken.generation != 0 {
		if result.err == nil && (!result.history || result.applied) {
			if result.blocks != nil {
				if result.history {
					result.state.setWordBlocksAndTextState(result.blocks, false)
				} else {
					result.state.setWordBlocks(result.blocks)
				}
			}
			result.revision = result.operationToken.revision + 1
			if !a.ensureDesktopRuntime().publishSessionRevision(result.state.documentID, max(result.revision, result.state.wordRevisionAtomic.Load())) {
				result.state.advanceRevision(result.revision)
			}
		}
		if result.revision == 0 {
			result.revision = result.state.wordRevisionAtomic.Load()
		}
		result.state.finishOperation(result.operationToken)
	}
	if a.wordCommandResults == nil {
		return
	}
	select {
	case a.wordCommandResults <- result:
		if a.window != nil {
			a.window.Invalidate()
		}
	case <-a.shutdown:
	}
}

func (a *App) applyWordHistory(undo bool) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	history, ok := any(state.word).(document.WordHistory)
	if !ok {
		return
	}
	if _, ok := state.beginOperation(sessionOperationMutation); !ok {
		return
	}
	generation := a.documentGeneration
	go func() {
		var applied bool
		blocks, err := runWordMutation(state, func() error {
			var operationErr error
			if undo {
				applied, operationErr = history.Undo()
			} else {
				applied, operationErr = history.Redo()
			}
			return operationErr
		})
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: "编辑", err: err, history: true, applied: applied, undo: undo, blocks: blocks})
	}()
}

func (a *App) showWordImageResizeDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	imageID := state.selectedWordImageID
	imageData := findWordImage(wordBlocksForState(state), imageID)
	if imageData == nil {
		return
	}
	defaultWidth := strconv.FormatFloat(float64(imageData.WidthEMU)/914400, 'f', 2, 64)
	a.modal.ShowInput("调整图片大小", "输入宽度（英寸，保持宽高比）", defaultWidth, "宽度", func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		widthInches, err := strconv.ParseFloat(strings.TrimSpace(a.modal.Input.Text()), 64)
		if err != nil || math.IsNaN(widthInches) || math.IsInf(widthInches, 0) || widthInches <= 0 || widthInches > 100 {
			a.setStatus("图片宽度必须大于 0 且不超过 100 英寸")
			return
		}
		widthEMU := int64(math.Round(widthInches * 914400))
		a.runWordImageCommand("调整图片", imageID, func(editor document.WordImageEditor) error { return editor.ResizeInlineImage(imageID, widthEMU) }, false)
	}, modalDialogActionSpec{ID: "confirm", Label: "应用"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}

func (a *App) layoutWordToolbarColorPicker(gtx layout.Context) layout.Dimensions {
	state := a.documentSessionState
	if state == nil || !state.wordToolbarColorPicker && !state.wordToolbarHighlightPicker {
		return layout.Dimensions{}
	}
	colors := wordFontColors
	if state.wordToolbarHighlightPicker {
		colors = wordHighlightColors
	}
	return layout.Inset{Top: unit.Dp(wordRibbonHeightDP + 8), Left: unit.Dp(440)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		width := gtx.Dp(unit.Dp(len(colors)*28 + (len(colors)-1)*6 + 16))
		height := gtx.Dp(unit.Dp(44))
		gtx.Constraints.Min = image.Pt(width, height)
		gtx.Constraints.Max = gtx.Constraints.Min
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				size := gtx.Constraints.Max
				shadow := image.Rectangle{Max: size}.Add(image.Pt(1, 2))
				paint.FillShape(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 22}, clip.UniformRRect(shadow, gtx.Dp(unit.Dp(5))).Op(gtx.Ops))
				paint.FillShape(gtx.Ops, contentSurface, clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(unit.Dp(5))).Op(gtx.Ops))
				return layout.Dimensions{Size: size}
			},
			func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					children := make([]layout.FlexChild, 0, len(colors))
					for index, item := range colors {
						index, item := index, item
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							button := &state.wordToolbarColorButtons[index]
							return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								size := gtx.Dp(unit.Dp(28))
								bounds := image.Rect(0, 0, size, size)
								paint.FillShape(gtx.Ops, item.color, clip.UniformRRect(bounds, gtx.Dp(unit.Dp(5))).Op(gtx.Ops))
								if state.wordToolbarHighlightPicker && item.value == "" {
									border := clip.UniformRRect(bounds, gtx.Dp(unit.Dp(5)))
									paint.FillShape(gtx.Ops, color.NRGBA{R: 170, G: 184, B: 199, A: 255}, clip.Stroke{Path: border.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
								}
								semantic.Button.Add(gtx.Ops)
								semantic.DescriptionOp(item.label).Add(gtx.Ops)
								return layout.Dimensions{Size: image.Pt(size, size)}
							})
						}))
					}
					return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx, children...)
				})
			},
		)
	})
}

func selectedWordRun(blocks []document.WordBlock, id string) (document.WordTextRun, bool) {
	for _, paragraph := range allWordParagraphs(blocks) {
		for _, inline := range paragraph.Inlines {
			if inline.Text != nil && inline.Text.ID == id {
				return *inline.Text, true
			}
		}
	}
	return document.WordTextRun{}, false
}

func selectedWordParagraph(blocks []document.WordBlock, id string) (document.WordParagraph, bool) {
	for _, paragraph := range allWordParagraphs(blocks) {
		if paragraph.ID == id {
			return paragraph, true
		}
	}
	return document.WordParagraph{}, false
}

func selectedWordImage(blocks []document.WordBlock, id string) bool {
	return findWordImage(blocks, id) != nil
}

func findWordImage(blocks []document.WordBlock, id string) *document.WordImage {
	for _, paragraph := range allWordParagraphs(blocks) {
		for _, inline := range paragraph.Inlines {
			if inline.Image != nil && inline.Image.ID == id {
				copy := *inline.Image
				return &copy
			}
		}
	}
	return nil
}

func allWordParagraphs(blocks []document.WordBlock) []document.WordParagraph {
	var paragraphs []document.WordParagraph
	for _, block := range blocks {
		if block.Paragraph != nil {
			paragraphs = append(paragraphs, *block.Paragraph)
		}
		if block.Table != nil {
			for _, row := range block.Table.Rows {
				for _, cell := range row.Cells {
					paragraphs = append(paragraphs, cell.Paragraphs...)
				}
			}
		}
	}
	return paragraphs
}

func wordCommandError(operation string, err error) string {
	if err == nil {
		return "已完成" + operation
	}
	return fmt.Sprintf("%s失败：%v", operation, err)
}
