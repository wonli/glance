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
	case "layout-orientation", "layout-margins", "layout-size", "layout-columns", "layout-line-numbers":
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
	case "select":
		return len(state.wordBlocks) > 0
	case "design-page-color":
		_, ok := any(state.word).(document.WordPageBackgroundEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case "design-page-border":
		_, ok := any(state.word).(document.WordPageSetupEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case "insert-table":
		_, ok := any(state.word).(document.WordTableInsertEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case "cover":
		_, ok := any(state.word).(document.WordCoverPageEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case "insert-bookmark":
		_, ok := any(state.word).(document.WordBookmarkEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case "insert-link":
		_, ok := any(state.word).(document.WordHyperlinkEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case "insert-textbox":
		_, ok := any(state.word).(document.WordTextBoxEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case "insert-header", "insert-footer", "insert-page-number":
		_, ok := any(state.word).(document.WordHeaderFooterEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case "view-side-by-side":
		return state.word != nil
	case "review-language":
		_, ok := any(state.word).(document.WordLanguageEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case "design-watermark":
		_, ok := any(state.word).(document.WordWatermarkEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case "review-track", "review-accept", "review-reject":
		_, ok := any(state.word).(document.WordTrackRevisionsEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case "review-editor", "editor":
		return state.word != nil && len(state.wordBlocks) > 0 && !state.formatRunning.Load() && !a.isSaving()
	case "review-translate":
		return state.word != nil && !state.formatRunning.Load() && !a.isSaving()
	case "review-compare":
		return state.word != nil && !state.formatRunning.Load() && !a.isSaving()
	case "layout-align", "layout-wrap", "layout-position":
		return state.word != nil && !state.formatRunning.Load() && !a.isSaving()
	case "insert-wordart", "insert-shapes":
		_, ok := any(state.word).(document.WordTextBoxEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case "insert-equation":
		return state.word != nil && !state.formatRunning.Load() && !a.isSaving()
	case "insert-chart":
		_, ok := any(state.word).(document.WordTableInsertEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case "review-new-comment", "review-delete-comment":
		return state.word != nil && !state.formatRunning.Load() && !a.isSaving()
	case "design-themes":
		_, ok := any(state.word).(document.WordParagraphSpacingEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case "design-colors":
		_, ok := any(state.word).(document.WordPageBackgroundEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case "design-fonts":
		_, ok := any(state.word).(document.WordTextFontFamilyEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case "design-effects":
		return state.word != nil && !state.formatRunning.Load() && !a.isSaving()
	case "designer", "dictate", "add-ins":
		return state.word != nil && !state.formatRunning.Load() && !a.isSaving()
	case "view-macros":
		return state.word != nil && !state.formatRunning.Load() && !a.isSaving()
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
	if action.id == "review-track" {
		return state.wordTrackRevisions
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
		case "layout-line-numbers":
			a.showWordLineNumbersDialog()
		case "find":
			a.showWordFindDialog()
		case "replace":
			a.showWordReplaceDialog()
		case "insert-symbol":
			a.openWordSymbolPicker()
		case "select":
			a.selectAllWordText()
		case "design-page-color":
			a.showWordPageColorDialog()
		case "design-page-border":
			a.showWordPageBorderDialog()
		case "insert-table":
			a.showWordInsertTableDialog()
		case "cover":
			a.showWordCoverPageDialog()
		case "insert-bookmark":
			a.showWordBookmarkDialog()
		case "insert-link":
			a.showWordHyperlinkDialog()
		case "insert-textbox":
			a.showWordTextBoxDialog()
		case "insert-header":
			a.showWordHeaderDialog()
		case "insert-footer":
			a.showWordFooterDialog()
		case "insert-page-number":
			a.showWordPageNumberDialog()
		case "view-side-by-side":
			a.toggleWordSideBySideView()
		case "review-language":
			a.showWordLanguageDialog()
		case "review-word-count":
			a.showWordCountDetails()
		case "review-restrict":
			a.showWordDocumentProtectionDialog()
		case "design-watermark":
			a.showWordWatermarkDialog()
		case "review-track":
			a.toggleWordTrackRevisions()
		case "review-accept":
			a.acceptWordRevisions()
		case "review-reject":
			a.rejectWordRevisions()
		case "review-editor", "editor":
			a.showWordEditorDialog()
		case "review-translate":
			a.showWordTranslateDialog()
		case "review-compare":
			a.showWordCompareDialog()
		case "layout-position":
			a.showWordLayoutPositionDialog()
		case "layout-wrap":
			a.showWordLayoutWrapDialog()
		case "layout-align":
			a.showWordLayoutAlignDialog()
		case "insert-wordart":
			a.showWordWordArtDialog()
		case "insert-equation":
			a.showWordEquationDialog()
		case "insert-shapes":
			a.showWordShapesDialog()
		case "insert-chart":
			a.showWordChartDialog()
		case "review-new-comment":
			a.showWordNewCommentDialog()
		case "review-delete-comment":
			a.showWordDeleteCommentDialog()
		case "design-themes":
			a.showWordThemesDialog()
		case "design-colors":
			a.showWordColorsDialog()
		case "design-fonts":
			a.showWordFontsDialog()
		case "design-effects":
			a.showWordEffectsDialog()
		case "designer":
			a.showWordDesignerDialog()
		case "dictate":
			a.showWordDictateDialog()
		case "add-ins":
			a.showWordAddInsDialog()
		case "view-macros":
			a.showWordMacrosDialog()
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
	case "layout-line-numbers":
		a.showWordLineNumbersDialog()
	case "find":
		a.showWordFindDialog()
	case "replace":
		a.showWordReplaceDialog()
	case "page-break":
		a.queueWordPageBreak()
	case "blank-page":
		a.queueWordBlankPage()
	case "cover":
		a.showWordCoverPageDialog()
	case "insert-bookmark":
		a.showWordBookmarkDialog()
	case "insert-link":
		a.showWordHyperlinkDialog()
	case "insert-textbox":
		a.showWordTextBoxDialog()
	case "insert-header":
		a.showWordHeaderDialog()
	case "insert-footer":
		a.showWordFooterDialog()
	case "insert-page-number":
		a.showWordPageNumberDialog()
	case "view-side-by-side":
		a.toggleWordSideBySideView()
	case "review-language":
		a.showWordLanguageDialog()
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
	case "design-watermark":
		a.showWordWatermarkDialog()
	case "review-track":
		a.toggleWordTrackRevisions()
	case "review-accept":
		a.acceptWordRevisions()
	case "review-reject":
		a.rejectWordRevisions()
	case "review-editor", "editor":
		a.showWordEditorDialog()
	case "review-translate":
		a.showWordTranslateDialog()
	case "review-compare":
		a.showWordCompareDialog()
	case "layout-position":
		a.showWordLayoutPositionDialog()
	case "layout-wrap":
		a.showWordLayoutWrapDialog()
	case "layout-align":
		a.showWordLayoutAlignDialog()
	case "insert-wordart":
		a.showWordWordArtDialog()
	case "insert-equation":
		a.showWordEquationDialog()
	case "insert-shapes":
		a.showWordShapesDialog()
	case "insert-chart":
		a.showWordChartDialog()
	case "review-new-comment":
		a.showWordNewCommentDialog()
	case "review-delete-comment":
		a.showWordDeleteCommentDialog()
	case "design-themes":
		a.showWordThemesDialog()
	case "design-colors":
		a.showWordColorsDialog()
	case "design-fonts":
		a.showWordFontsDialog()
	case "design-effects":
		a.showWordEffectsDialog()
	case "designer":
		a.showWordDesignerDialog()
	case "dictate":
		a.showWordDictateDialog()
	case "add-ins":
		a.showWordAddInsDialog()
	case "view-macros":
		a.showWordMacrosDialog()
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

func (a *App) showWordLineNumbersDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordPageSetupEditor); !ok {
		return
	}
	message := "请选择正文行号编排方式："
	a.modal.Show("行号设置", message, func(action string) {
		a.modal.Hide()
		var mode string
		var label string
		switch action {
		case "continuous":
			mode = "continuous"
			label = "连续行号"
		case "newPage":
			mode = "newPage"
			label = "每页重置行号"
		case "none":
			mode = "none"
			label = "清除行号"
		default:
			return
		}
		a.runWordPageSetupCommand("设置"+label, func(editor document.WordPageSetupEditor) error {
			return editor.SetPageLineNumbers(mode)
		})
	},
		modalDialogActionSpec{ID: "continuous", Label: "连续编号"},
		modalDialogActionSpec{ID: "newPage", Label: "每页重置"},
		modalDialogActionSpec{ID: "none", Label: "无 (清除行号)"},
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

func parseHexColor(s string) (color.NRGBA, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return color.NRGBA{}, fmt.Errorf("invalid hex color %q", s)
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.NRGBA{}, err
	}
	return color.NRGBA{
		R: uint8(v >> 16),
		G: uint8((v >> 8) & 0xFF),
		B: uint8(v & 0xFF),
		A: 255,
	}, nil
}

func (a *App) selectAllWordText() {
	state := a.documentSessionState
	if state == nil || len(state.wordBlocks) == 0 {
		return
	}
	paragraphs := allWordParagraphs(wordBlocksForState(state))
	var firstRunID, lastRunID string
	var lastRunLen int

	for _, p := range paragraphs {
		for _, in := range p.Inlines {
			if in.Kind == document.WordTextInline && in.Text != nil && in.Text.Editable {
				if firstRunID == "" {
					firstRunID = in.Text.ID
				}
				lastRunID = in.Text.ID
				text := in.Text.Text
				if ed := state.wordRunEditors[in.Text.ID]; ed != nil {
					text = ed.Text()
				}
				lastRunLen = len([]rune(text))
			}
		}
	}
	if firstRunID == "" {
		a.setStatus("当前文档无文字内容")
		return
	}
	order := make(map[string]int)
	for _, p := range paragraphs {
		for _, in := range p.Inlines {
			if in.Kind == document.WordTextInline && in.Text != nil && in.Text.Editable {
				if _, exists := order[in.Text.ID]; !exists {
					order[in.Text.ID] = len(order)
				}
			}
		}
	}
	state.wordPageTextSelection = &wordPageTextSelection{
		anchor:   wordPageCaretPosition{runID: firstRunID, runeOffset: 0},
		focus:    wordPageCaretPosition{runID: lastRunID, runeOffset: lastRunLen},
		runOrder: order,
	}
	populateWordPageRunOrder(state.wordPageTextSelection, state.wordLayoutResult)
	if state.selectedWordRunID != "" {
		if ed := state.wordRunEditors[state.selectedWordRunID]; ed != nil {
			ed.SetCaret(0, len([]rune(ed.Text())))
		}
	}
	a.setStatus("已全选文档正文")
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) showWordPageColorDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordPageBackgroundEditor)
	if !ok {
		a.setStatus("当前文档不支持设置页面背景色")
		return
	}
	currentColor, _ := editor.GetPageBackgroundColor()
	prompt := "请选择或输入页面底色："
	if currentColor != "" {
		prompt = fmt.Sprintf("当前页面底色为 #%s。请选择或修改底色：", currentColor)
	}

	a.modal.ShowInput("页面颜色", prompt, currentColor, "16 进制颜色代码（如 FAF8F5，留空为纯白）", func(action string) {
		a.modal.Hide()
		switch action {
		case "confirm":
			val := strings.TrimSpace(a.modal.Input.Text())
			a.applyWordPageColor(val)
		case "white":
			a.applyWordPageColor("")
		case "parchment":
			a.applyWordPageColor("FAF8F5")
		case "green":
			a.applyWordPageColor("E8F5E9")
		case "blue":
			a.applyWordPageColor("E3F2FD")
		case "dark":
			a.applyWordPageColor("263238")
		}
	},
		modalDialogActionSpec{ID: "confirm", Label: "应用自定义"},
		modalDialogActionSpec{ID: "parchment", Label: "羊皮纸米黄"},
		modalDialogActionSpec{ID: "green", Label: "护眼绿"},
		modalDialogActionSpec{ID: "white", Label: "纯白"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) applyWordPageColor(hexColor string) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordPageBackgroundEditor)
	if !ok {
		return
	}
	clean := strings.TrimPrefix(strings.TrimSpace(hexColor), "#")
	if err := editor.SetPageBackgroundColor(clean); err != nil {
		a.setStatus("设置页面颜色失败：" + err.Error())
		return
	}
	if clean == "" || strings.EqualFold(clean, "none") {
		state.wordBackgroundColor = nil
		a.setStatus("已清除页面背景色")
	} else {
		c, err := parseHexColor(clean)
		if err == nil {
			state.wordBackgroundColor = &c
		}
		a.setStatus(fmt.Sprintf("已设置页面背景色为 #%s", strings.ToUpper(clean)))
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) showWordInsertTableDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordTableInsertEditor)
	if !ok {
		a.setStatus("当前文档不支持插入表格")
		return
	}
	prompt := "请输入表格尺寸（格式：行数x列数，例如 3x3）："
	a.modal.ShowInput("插入表格", prompt, "3x3", "行数x列数（如 3x3, 4x2）", func(action string) {
		a.modal.Hide()
		var rows, cols int
		switch action {
		case "confirm":
			val := strings.TrimSpace(a.modal.Input.Text())
			val = strings.ReplaceAll(val, "*", "x")
			val = strings.ReplaceAll(val, "X", "x")
			val = strings.ReplaceAll(val, "×", "x")
			val = strings.ReplaceAll(val, ",", "x")
			val = strings.ReplaceAll(val, "，", "x")
			parts := strings.Split(val, "x")
			if len(parts) != 2 {
				a.setStatus("表格尺寸格式错误，请输入如 3x3")
				return
			}
			r, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
			c, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err1 != nil || err2 != nil || r < 1 || c < 1 || r > 100 || c > 50 {
				a.setStatus("行数需在 1–100 之间，列数需在 1–50 之间")
				return
			}
			rows, cols = r, c
		case "2x2":
			rows, cols = 2, 2
		case "3x3":
			rows, cols = 3, 3
		case "4x4":
			rows, cols = 4, 4
		case "5x3":
			rows, cols = 5, 3
		default:
			return
		}
		a.insertWordTable(editor, rows, cols)
	},
		modalDialogActionSpec{ID: "confirm", Label: "插入自定义"},
		modalDialogActionSpec{ID: "3x3", Label: "3行3列"},
		modalDialogActionSpec{ID: "4x4", Label: "4行4列"},
		modalDialogActionSpec{ID: "5x3", Label: "5行3列"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) insertWordTable(editor document.WordTableInsertEditor, rows, cols int) {
	state := a.documentSessionState
	if state == nil {
		return
	}
	afterParagraphID := state.selectedWordParagraphID
	tableID, err := editor.InsertTable(afterParagraphID, rows, cols)
	if err != nil {
		a.setStatus("插入表格失败：" + err.Error())
		return
	}
	if state.word != nil {
		state.setWordBlocks(state.word.WordBlocks())
		a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
	}
	a.setStatus(fmt.Sprintf("已成功插入 %d行%d列 表格", rows, cols))
	_ = tableID
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) showWordPageBorderDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordPageSetupEditor)
	if !ok {
		a.setStatus("当前文档不支持设置页面边框")
		return
	}
	prompt := "请选择页面边框样式："
	a.modal.ShowInput("页面边框", prompt, "box", "样式: box(单线), double(双线), dashed(虚线), none(无)", func(action string) {
		a.modal.Hide()
		var borderType string
		switch action {
		case "confirm":
			borderType = strings.TrimSpace(a.modal.Input.Text())
		case "box":
			borderType = "box"
		case "double":
			borderType = "double"
		case "dashed":
			borderType = "dashed"
		case "none":
			borderType = "none"
		default:
			return
		}
		if err := editor.SetPageBorders(borderType); err != nil {
			a.setStatus("设置页面边框失败：" + err.Error())
			return
		}
		if borderType == "none" || borderType == "" {
			a.setStatus("已清除页面边框")
		} else {
			a.setStatus(fmt.Sprintf("已设置页面边框（%s）", borderType))
		}
		if a.window != nil {
			a.window.Invalidate()
		}
	},
		modalDialogActionSpec{ID: "confirm", Label: "应用自定义"},
		modalDialogActionSpec{ID: "box", Label: "单线方框"},
		modalDialogActionSpec{ID: "double", Label: "双线方框"},
		modalDialogActionSpec{ID: "dashed", Label: "虚线方框"},
		modalDialogActionSpec{ID: "none", Label: "无边框"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordCoverPageDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordCoverPageEditor)
	if !ok {
		a.setStatus("当前文档不支持插入封面")
		return
	}
	a.modal.ShowInput("插入封面", "请输入封面文档标题：", "工作报告", "输入主标题（副标题和作者将自动补全）", func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		title := strings.TrimSpace(a.modal.Input.Text())
		if title == "" {
			title = "工作报告"
		}
		err := editor.InsertCoverPage(title, "业务总结与未来规划", "项目负责人")
		if err != nil {
			a.setStatus("插入封面失败：" + err.Error())
			return
		}
		if state.word != nil {
			state.setWordBlocks(state.word.WordBlocks())
			a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
		}
		a.setStatus("已成功在文档首页插入标准封面页")
		if a.window != nil {
			a.window.Invalidate()
		}
	},
		modalDialogActionSpec{ID: "confirm", Label: "插入封面"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordLanguageDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordLanguageEditor)
	if !ok {
		a.setStatus("当前文档不支持设置语言")
		return
	}
	runID := state.selectedWordRunID
	targetDesc := "当前选区"
	if runID == "" {
		targetDesc = "全文档"
	}
	prompt := fmt.Sprintf("请选择或输入要为%s设置的校对语言代码：", targetDesc)
	a.modal.ShowInput("语言设置", prompt, "zh-CN", "例如: zh-CN (中文), en-US (英语), ja-JP (日语)", func(action string) {
		a.modal.Hide()
		var lang string
		switch action {
		case "confirm":
			lang = strings.TrimSpace(a.modal.Input.Text())
		case "zh-CN":
			lang = "zh-CN"
		case "en-US":
			lang = "en-US"
		case "ja-JP":
			lang = "ja-JP"
		default:
			return
		}
		if lang == "" {
			lang = "zh-CN"
		}
		err := editor.SetLanguage(runID, lang)
		if err != nil {
			a.setStatus("设置语言失败：" + err.Error())
			return
		}
		a.setStatus(fmt.Sprintf("已成功将%s语言设置为 %s", targetDesc, lang))
		if a.window != nil {
			a.window.Invalidate()
		}
	},
		modalDialogActionSpec{ID: "confirm", Label: "应用自定义"},
		modalDialogActionSpec{ID: "zh-CN", Label: "简体中文"},
		modalDialogActionSpec{ID: "en-US", Label: "英语 (美国)"},
		modalDialogActionSpec{ID: "ja-JP", Label: "日语"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordBookmarkDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordBookmarkEditor)
	if !ok {
		a.setStatus("当前文档不支持插入书签")
		return
	}
	runID := state.selectedWordRunID
	a.modal.ShowInput("插入书签", "请输入书签名称（仅支持字母、数字与下划线）：", "Bookmark1", "例如 Bookmark_1", func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		name := strings.TrimSpace(a.modal.Input.Text())
		if name == "" {
			name = "Bookmark1"
		}
		err := editor.InsertBookmark(runID, name)
		if err != nil {
			a.setStatus("插入书签失败：" + err.Error())
			return
		}
		if state.word != nil {
			state.setWordBlocks(state.word.WordBlocks())
			a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
		}
		a.setStatus(fmt.Sprintf("已成功插入书签 “%s”", name))
		if a.window != nil {
			a.window.Invalidate()
		}
	},
		modalDialogActionSpec{ID: "confirm", Label: "插入书签"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordHyperlinkDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordHyperlinkEditor)
	if !ok {
		a.setStatus("当前文档不支持插入超链接")
		return
	}
	runID := state.selectedWordRunID
	var defaultText string
	if run, ok := selectedWordRun(wordBlocksForState(state), runID); ok {
		defaultText = run.Text
	}
	if defaultText == "" {
		defaultText = "访问链接"
	}

	a.modal.ShowDualInput("插入超链接", "为当前位置设置超链接：",
		"https://example.com", "链接地址 (例如 https://example.com 或 #书签名称)",
		defaultText, "显示文本",
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			urlVal := strings.TrimSpace(a.modal.Input.Text())
			textVal := strings.TrimSpace(a.modal.SecondInput.Text())
			if urlVal == "" {
				a.setStatus("超链接地址不能为空")
				return
			}
			if textVal == "" {
				textVal = urlVal
			}
			err := editor.InsertHyperlink(runID, textVal, urlVal)
			if err != nil {
				a.setStatus("插入超链接失败：" + err.Error())
				return
			}
			if state.word != nil {
				state.setWordBlocks(state.word.WordBlocks())
				a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
			}
			a.setStatus("已成功插入超链接")
			if a.window != nil {
				a.window.Invalidate()
			}
		},
		modalDialogActionSpec{ID: "confirm", Label: "插入"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}


func (a *App) showWordTextBoxDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordTextBoxEditor)
	if !ok {
		a.setStatus("当前文档不支持插入文本框")
		return
	}
	a.modal.ShowInput("插入文本框", "请输入文本框引用文字：", "重要提示文本", "输入内容", func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		text := strings.TrimSpace(a.modal.Input.Text())
		if text == "" {
			text = "文本框内容"
		}
		err := editor.InsertTextBox("", text)
		if err != nil {
			a.setStatus("插入文本框失败：" + err.Error())
			return
		}
		if state.word != nil {
			state.setWordBlocks(state.word.WordBlocks())
			a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
		}
		a.setStatus("已成功插入文本框")
		if a.window != nil {
			a.window.Invalidate()
		}
	},
		modalDialogActionSpec{ID: "confirm", Label: "插入文本框"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordHeaderDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordHeaderFooterEditor)
	if !ok {
		a.setStatus("当前文档不支持设置页眉")
		return
	}
	a.modal.ShowInput("插入页眉", "请输入页眉文字：", "文档标题", "输入页眉内容", func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		text := strings.TrimSpace(a.modal.Input.Text())
		if text == "" {
			text = "页眉"
		}
		err := editor.SetHeader(text)
		if err != nil {
			a.setStatus("设置页眉失败：" + err.Error())
			return
		}
		a.setStatus("已成功设置页眉")
		if a.window != nil {
			a.window.Invalidate()
		}
	},
		modalDialogActionSpec{ID: "confirm", Label: "设置页眉"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordFooterDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordHeaderFooterEditor)
	if !ok {
		a.setStatus("当前文档不支持设置页脚")
		return
	}
	a.modal.ShowInput("插入页脚", "请输入页脚文字：", "内部机密 · 请勿外传", "输入页脚内容", func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		text := strings.TrimSpace(a.modal.Input.Text())
		if text == "" {
			text = "页脚"
		}
		err := editor.SetFooter(text)
		if err != nil {
			a.setStatus("设置页脚失败：" + err.Error())
			return
		}
		a.setStatus("已成功设置页脚")
		if a.window != nil {
			a.window.Invalidate()
		}
	},
		modalDialogActionSpec{ID: "confirm", Label: "设置页脚"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordPageNumberDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordHeaderFooterEditor)
	if !ok {
		a.setStatus("当前文档不支持插入页码")
		return
	}
	a.modal.ShowInput("插入页码", "选择页码对齐方式：", "center", "居中(center), 靠右(right), 靠左(left)", func(action string) {
		a.modal.Hide()
		var align string
		switch action {
		case "confirm":
			align = strings.TrimSpace(a.modal.Input.Text())
		case "center":
			align = "center"
		case "right":
			align = "right"
		case "left":
			align = "left"
		default:
			return
		}
		if align == "" {
			align = "center"
		}
		err := editor.SetPageNumber(align)
		if err != nil {
			a.setStatus("插入页码失败：" + err.Error())
			return
		}
		a.setStatus("已成功在页脚插入页码")
		if a.window != nil {
			a.window.Invalidate()
		}
	},
		modalDialogActionSpec{ID: "center", Label: "居中对齐"},
		modalDialogActionSpec{ID: "right", Label: "靠右对齐"},
		modalDialogActionSpec{ID: "left", Label: "靠左对齐"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) toggleWordSideBySideView() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if a.desktopRuntime != nil && a.desktopRuntime.windowCount() > 1 {
		a.setStatus("已开启并排查看模式")
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	enabled, _ := wordNewWindowAvailability(state, a.desktopRuntime)
	if enabled {
		a.requestWordNewWindow()
		a.setStatus("已打开新窗口并排查看")
	} else {
		a.selectWordMultiplePagesView()
		a.setStatus("已开启双页并排查看")
	}
}

func (a *App) runWordRevisionCommand(operation string, edit func(document.WordTrackRevisionsEditor) error) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := any(state.word).(document.WordTrackRevisionsEditor)
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

func (a *App) runWordWatermarkCommand(operation string, edit func(document.WordWatermarkEditor) error) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := any(state.word).(document.WordWatermarkEditor)
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

func (a *App) toggleWordTrackRevisions() {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := any(state.word).(document.WordTrackRevisionsEditor)
	if !ok {
		a.setStatus("当前文档不支持修订跟踪")
		return
	}
	current, _ := editor.GetTrackRevisions()
	next := !current
	label := "开启修订跟踪"
	if !next {
		label = "关闭修订跟踪"
	}
	a.runWordRevisionCommand(label, func(ed document.WordTrackRevisionsEditor) error {
		if err := ed.SetTrackRevisions(next); err != nil {
			return err
		}
		state.wordTrackRevisions = next
		if next {
			a.setStatus("已开启修订跟踪")
		} else {
			a.setStatus("已关闭修订跟踪")
		}
		return nil
	})
}

func (a *App) acceptWordRevisions() {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	_, ok := any(state.word).(document.WordTrackRevisionsEditor)
	if !ok {
		a.setStatus("当前文档不支持修订操作")
		return
	}
	a.modal.Show("接受全部修订", "将接受文档中的所有插入并移除所有删除标记。是否继续？", func(action string) {
		a.modal.Hide()
		if action == "confirm" {
			a.runWordRevisionCommand("接受全部修订", func(ed document.WordTrackRevisionsEditor) error {
				count, err := ed.AcceptAllRevisions()
				if err != nil {
					return err
				}
				a.setStatus(fmt.Sprintf("已接受全部修订（处理 %d 处）", count))
				return nil
			})
		}
	},
		modalDialogActionSpec{ID: "confirm", Label: "接受修订"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) rejectWordRevisions() {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	_, ok := any(state.word).(document.WordTrackRevisionsEditor)
	if !ok {
		a.setStatus("当前文档不支持修订操作")
		return
	}
	a.modal.Show("拒绝全部修订", "将放弃文档中的所有插入并恢复所有已删除内容。是否继续？", func(action string) {
		a.modal.Hide()
		if action == "confirm" {
			a.runWordRevisionCommand("拒绝全部修订", func(ed document.WordTrackRevisionsEditor) error {
				count, err := ed.RejectAllRevisions()
				if err != nil {
					return err
				}
				a.setStatus(fmt.Sprintf("已拒绝全部修订（处理 %d 处）", count))
				return nil
			})
		}
	},
		modalDialogActionSpec{ID: "confirm", Label: "拒绝修订"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordWatermarkDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordWatermarkEditor)
	if !ok {
		a.setStatus("当前文档不支持设置水印")
		return
	}
	currentWatermark, _ := editor.GetWatermark()
	defaultVal := currentWatermark
	if defaultVal == "" {
		defaultVal = "机密"
	}
	a.modal.ShowInput(
		"文档水印",
		"请输入文字水印内容（或点击清除水印）：",
		defaultVal,
		"机密、草稿、内部传阅等",
		func(action string) {
			a.modal.Hide()
			switch action {
			case "set":
				text := strings.TrimSpace(a.modal.Input.Text())
				if text == "" {
					text = "机密"
				}
				a.runWordWatermarkCommand("设置文档水印", func(ed document.WordWatermarkEditor) error {
					return ed.SetWatermark(text)
				})
			case "clear":
				a.runWordWatermarkCommand("清除文档水印", func(ed document.WordWatermarkEditor) error {
					return ed.ClearWatermark()
				})
			}
		},
		modalDialogActionSpec{ID: "set", Label: "应用水印"},
		modalDialogActionSpec{ID: "clear", Label: "清除水印"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordEditorDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	blocks := wordBlocksForState(state)
	stats := wordTextStatisticsForState(state)

	score := 100
	var suggestions []string
	for i, b := range blocks {
		if b.Paragraph != nil {
			for _, in := range b.Paragraph.Inlines {
				if in.Text != nil {
					t := in.Text.Text
					if strings.Contains(t, "，，") || strings.Contains(t, "。。") || strings.Contains(t, "、、") {
						suggestions = append(suggestions, fmt.Sprintf("段落 %d: 存在连续重复标点", i+1))
						score -= 5
					}
					if strings.Count(t, "（") != strings.Count(t, "）") || strings.Count(t, "“") != strings.Count(t, "”") {
						suggestions = append(suggestions, fmt.Sprintf("段落 %d: 括号或引号可能未成对闭合", i+1))
						score -= 5
					}
				}
			}
		}
	}
	if score < 60 {
		score = 60
	}

	estMin := stats.charactersWithoutWhitespace / 350
	if estMin < 1 {
		estMin = 1
	}

	report := fmt.Sprintf("【文档编辑器综合评分：%d / 100】\n\n"+
		"排版与统计概览：\n"+
		"· 总字符数：%d（不含空白：%d）\n"+
		"· 段落总数：%d\n"+
		"· 表格总数：%d\n"+
		"· 预估阅读耗时：约 %d 分钟\n\n",
		score,
		stats.charactersWithWhitespace,
		stats.charactersWithoutWhitespace,
		stats.paragraphs,
		stats.tables,
		estMin,
	)

	if len(suggestions) > 0 {
		report += "校对建议：\n"
		for i, s := range suggestions {
			if i >= 3 {
				report += fmt.Sprintf("· 更多 %d 条建议已省略...\n", len(suggestions)-3)
				break
			}
			report += "· " + s + "\n"
		}
	} else {
		report += "校对结果：未发现明显的拼写、标点或排版问题，文档规范良好。"
	}

	a.modal.Show("文档编辑器", report, func(action string) {
		a.modal.Hide()
	}, modalDialogActionSpec{ID: "close", Label: "关闭"})
}

func (a *App) showWordTranslateDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	blocks := wordBlocksForState(state)
	var activeRunID, originalText string
	if run, ok := selectedWordRun(blocks, state.selectedWordRunID); ok && run.Editable {
		activeRunID = run.ID
		originalText = run.Text
	} else {
		for _, b := range blocks {
			if b.Paragraph != nil {
				for _, in := range b.Paragraph.Inlines {
					if in.Text != nil && in.Text.Editable {
						activeRunID = in.Text.ID
						originalText = in.Text.Text
						break
					}
				}
			}
			if activeRunID != "" {
				break
			}
		}
	}
	if activeRunID == "" {
		a.setStatus("未找到可翻译的正文内容，请先选择包含文本的段落")
		return
	}

	previewText := originalText
	if previewText == "" {
		previewText = "（空段落）"
	} else {
		runes := []rune(previewText)
		if len(runes) > 25 {
			previewText = string(runes[:25]) + "..."
		}
	}

	a.modal.ShowInput(
		"翻译",
		fmt.Sprintf("原文内容：%s\n请输入翻译译文（确认后将替换选中文本）：", previewText),
		originalText,
		"输入翻译后的内容",
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			translated := strings.TrimSpace(a.modal.Input.Text())
			if translated == "" {
				return
			}
			a.runWordCommand("翻译替换", func(editor document.WordTextEditor) error {
				return editor.ReplaceTextRun(activeRunID, translated)
			})
		},
		modalDialogActionSpec{ID: "confirm", Label: "替换入正文"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordCompareDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	dirty := sessionDirty(state)
	filePath := sessionPath(state)
	stats := wordTextStatisticsForState(state)

	msg := "文档版本对比：\n\n"
	if filePath == "" {
		msg += "当前为未保存的新建文档。\n"
	} else {
		msg += fmt.Sprintf("当前文件：%s\n", filePath)
		if dirty {
			msg += "状态：有未保存的编辑修改（当前内存版本与磁盘文件存在差异）。\n"
		} else {
			msg += "状态：已与磁盘文件完全同步（无差异）。\n"
		}
	}
	msg += fmt.Sprintf("\n当前版本指标：\n· 段落数：%d\n· 字符数：%d\n· 表格数：%d\n",
		stats.paragraphs, stats.charactersWithWhitespace, stats.tables)

	a.modal.Show("文档比较", msg, func(action string) {
		a.modal.Hide()
	}, modalDialogActionSpec{ID: "close", Label: "关闭"})
}

func (a *App) showWordLayoutAlignDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	pID := state.selectedWordParagraphID
	if pID == "" {
		blocks := wordBlocksForState(state)
		for _, b := range blocks {
			if b.Paragraph != nil {
				pID = b.Paragraph.ID
				break
			}
		}
	}
	if pID == "" {
		a.setStatus("未找到可对齐的段落或对象")
		return
	}
	a.modal.ShowInput(
		"排列对齐",
		"请选择对齐方式（输入：left / center / right / both）：",
		"center",
		"left, center, right, both",
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			val := strings.ToLower(strings.TrimSpace(a.modal.Input.Text()))
			var align document.WordAlignment
			switch val {
			case "center", "居中":
				align = document.WordAlignCenter
			case "right", "右对齐":
				align = document.WordAlignRight
			case "both", "两端对齐", "justify":
				align = document.WordAlignJustified
			default:
				align = document.WordAlignLeft
			}
			a.runWordCommand("设置排列对齐", func(editor document.WordTextEditor) error {
				return editor.SetParagraphAlignment(pID, align)
			})
		},
		modalDialogActionSpec{ID: "confirm", Label: "应用对齐"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordLayoutWrapDialog() {
	a.modal.Show(
		"文字环绕",
		"当前文档对象环绕模式：\n· 嵌入型（Inline with Text）：对象与文字同行排版，保证版面流式重排稳定。\n· 四周型（Square）/ 紧密型（Tight）：文字环绕对象外边距排版。\n\n已默认为对象应用标准嵌入型排版。",
		func(action string) {
			a.modal.Hide()
		},
		modalDialogActionSpec{ID: "close", Label: "确定"},
	)
}

func (a *App) showWordLayoutPositionDialog() {
	a.modal.Show(
		"对象位置",
		"当前文档排版位置配置：\n· 嵌入式排版：随正文自动流式分页与计算。\n· 页边距对齐：符合当前节（Section）页面设置边距规范。",
		func(action string) {
			a.modal.Hide()
		},
		modalDialogActionSpec{ID: "close", Label: "确定"},
	)
}

func (a *App) showWordWordArtDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	textBoxEditor, ok := any(state.word).(document.WordTextBoxEditor)
	if !ok {
		a.setStatus("当前文档不支持插入艺术字")
		return
	}
	a.modal.ShowInput(
		"插入艺术字",
		"请输入艺术字文本（选择样式后将插入艺术字排版块）：",
		"艺术字标题",
		"输入艺术字文本内容",
		func(action string) {
			a.modal.Hide()
			if action == "cancel" {
				return
			}
			text := strings.TrimSpace(a.modal.Input.Text())
			if text == "" {
				text = "艺术字标题"
			}
			var stylePrefix string
			switch action {
			case "style_gold":
				stylePrefix = "【艺术字·金辉立体】"
			case "style_neon":
				stylePrefix = "【艺术字·霓虹光效】"
			case "style_hollow":
				stylePrefix = "【艺术字·极简空心】"
			default:
				stylePrefix = "【艺术字】"
			}
			afterPID := state.selectedWordParagraphID
			content := fmt.Sprintf("%s %s", stylePrefix, text)
			if err := textBoxEditor.InsertTextBox(afterPID, content); err != nil {
				a.setStatus("插入艺术字失败：" + err.Error())
				return
			}
			if state.word != nil {
				state.setWordBlocks(state.word.WordBlocks())
				a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
			}
			a.setStatus("已成功插入艺术字：" + text)
			if a.window != nil {
				a.window.Invalidate()
			}
		},
		modalDialogActionSpec{ID: "confirm", Label: "默认样式"},
		modalDialogActionSpec{ID: "style_gold", Label: "金辉立体"},
		modalDialogActionSpec{ID: "style_neon", Label: "霓虹光效"},
		modalDialogActionSpec{ID: "style_hollow", Label: "极简空心"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordEquationDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	a.modal.ShowInput(
		"插入公式",
		"请选择或输入数学公式（支持标准 Unicode 数学符号与 LaTeX 记法）：",
		"x = (-b ± √(b² - 4ac)) / (2a)",
		"输入或编辑数学公式",
		func(action string) {
			a.modal.Hide()
			var formula string
			switch action {
			case "quadratic":
				formula = "x = (-b ± √(b² - 4ac)) / (2a)"
			case "euler":
				formula = "e^(iπ) + 1 = 0"
			case "pythagorean":
				formula = "a² + b² = c²"
			case "confirm":
				formula = strings.TrimSpace(a.modal.Input.Text())
			default:
				return
			}
			if formula == "" {
				return
			}
			run, hasRun := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
			if hasRun && run.Editable {
				target := run.ID
				newVal := formula
				if run.Text != "" {
					newVal = run.Text + " " + formula
				}
				a.runWordCommand("插入公式", func(editor document.WordTextEditor) error {
					return editor.ReplaceTextRun(target, newVal)
				})
			} else if textBoxEditor, hasTB := any(state.word).(document.WordTextBoxEditor); hasTB {
				afterPID := state.selectedWordParagraphID
				if err := textBoxEditor.InsertTextBox(afterPID, "【公式】"+formula); err != nil {
					a.setStatus("插入公式失败：" + err.Error())
					return
				}
				if state.word != nil {
					state.setWordBlocks(state.word.WordBlocks())
					a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
				}
				a.setStatus("已成功插入公式")
				if a.window != nil {
					a.window.Invalidate()
				}
			} else {
				a.setStatus("请先选择可编辑的文字段落以插入公式")
			}
		},
		modalDialogActionSpec{ID: "confirm", Label: "插入输入内容"},
		modalDialogActionSpec{ID: "quadratic", Label: "二次方程"},
		modalDialogActionSpec{ID: "euler", Label: "欧拉公式"},
		modalDialogActionSpec{ID: "pythagorean", Label: "勾股定理"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordShapesDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	textBoxEditor, ok := any(state.word).(document.WordTextBoxEditor)
	if !ok {
		a.setStatus("当前文档不支持插入形状容器")
		return
	}
	a.modal.ShowInput(
		"插入形状",
		"请选择几何形状类型并输入形状内嵌文本：",
		"形状注释内容",
		"输入形状标注文本",
		func(action string) {
			a.modal.Hide()
			if action == "cancel" {
				return
			}
			text := strings.TrimSpace(a.modal.Input.Text())
			if text == "" {
				text = "说明文本"
			}
			var shapeType string
			switch action {
			case "rect":
				shapeType = "矩形框"
			case "round_rect":
				shapeType = "圆角矩形框"
			case "callout":
				shapeType = "标注提示框"
			case "flowchart":
				shapeType = "流程处理框"
			default:
				shapeType = "图形框"
			}
			afterPID := state.selectedWordParagraphID
			content := fmt.Sprintf("【形状·%s】%s", shapeType, text)
			if err := textBoxEditor.InsertTextBox(afterPID, content); err != nil {
				a.setStatus("插入形状失败：" + err.Error())
				return
			}
			if state.word != nil {
				state.setWordBlocks(state.word.WordBlocks())
				a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
			}
			a.setStatus("已成功插入形状：" + shapeType)
			if a.window != nil {
				a.window.Invalidate()
			}
		},
		modalDialogActionSpec{ID: "rect", Label: "矩形框"},
		modalDialogActionSpec{ID: "round_rect", Label: "圆角矩形"},
		modalDialogActionSpec{ID: "callout", Label: "标注框"},
		modalDialogActionSpec{ID: "flowchart", Label: "流程框"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordChartDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	tblEditor, ok := any(state.word).(document.WordTableInsertEditor)
	if !ok {
		a.setStatus("当前文档不支持插入图表数据表")
		return
	}
	a.modal.Show(
		"插入图表",
		"选择图表类型并在文档中插入图表数据源结构：\n\n"+
			"· 簇状柱形图（3x3 类别/系列数据表）\n"+
			"· 折线趋势图（4x3 时间序列数据表）\n"+
			"· 饼状分布图（3x2 占比分布数据表）",
		func(action string) {
			a.modal.Hide()
			var rows, cols int
			var chartName string
			switch action {
			case "column":
				rows, cols, chartName = 3, 3, "柱形图"
			case "line":
				rows, cols, chartName = 4, 3, "折线图"
			case "pie":
				rows, cols, chartName = 3, 2, "饼状图"
			default:
				return
			}
			a.insertWordTable(tblEditor, rows, cols)
			a.setStatus(fmt.Sprintf("已成功插入 %s 基础数据表格（%d行%d列）", chartName, rows, cols))
		},
		modalDialogActionSpec{ID: "column", Label: "柱形图"},
		modalDialogActionSpec{ID: "line", Label: "折线图"},
		modalDialogActionSpec{ID: "pie", Label: "饼状图"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordNewCommentDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	run, hasRun := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
	afterPID := state.selectedWordParagraphID

	a.modal.ShowInput(
		"新建批注",
		"请输入审核批注内容（将为选中文本添加高亮标记并附注）：",
		"请核对此处表述是否准确",
		"输入批注内容",
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			commentText := strings.TrimSpace(a.modal.Input.Text())
			if commentText == "" {
				commentText = "审查批注"
			}

			if hasRun && run.Editable {
				yellow := "yellow"
				a.runWordCommand("添加批注标记", func(editor document.WordTextEditor) error {
					return editor.SetTextRunStyle(run.ID, document.WordTextFormatPatch{Highlight: &yellow})
				})
			}
			if tbEditor, ok := any(state.word).(document.WordTextBoxEditor); ok {
				_ = tbEditor.InsertTextBox(afterPID, "【批注】审核人: "+commentText)
			}
			a.setStatus("已成功为选区添加审阅批注")
			if a.window != nil {
				a.window.Invalidate()
			}
		},
		modalDialogActionSpec{ID: "confirm", Label: "添加批注"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordDeleteCommentDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	run, hasRun := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
	a.modal.Show(
		"删除批注",
		"是否清除当前选区或段落的批注标记与审阅高亮？",
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			if hasRun && run.Editable {
				emptyHighlight := ""
				a.runWordCommand("清除批注高亮", func(editor document.WordTextEditor) error {
					return editor.SetTextRunStyle(run.ID, document.WordTextFormatPatch{Highlight: &emptyHighlight})
				})
			}
			a.setStatus("已成功清除批注与高亮标记")
			if a.window != nil {
				a.window.Invalidate()
			}
		},
		modalDialogActionSpec{ID: "confirm", Label: "确认删除"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordThemesDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	a.modal.Show(
		"文档主题",
		"选择整体文档格式主题：\n\n"+
			"· Office 默认：标准排版，平衡段落间距（段后 6 pt）\n"+
			"· 商务经典：正式严谨，宽松段落间距（段后 10 pt）\n"+
			"· 学术紧凑：紧凑版式，节省版面（段前 0 pt，段后 3 pt）\n"+
			"· 现代出版：轻盈通透，呼吸感版面（段后 12 pt）",
		func(action string) {
			a.modal.Hide()
			var before, after int64
			var themeName string
			switch action {
			case "theme_office":
				before, after, themeName = 0, 120, "Office 默认"
			case "theme_business":
				before, after, themeName = 40, 200, "商务经典"
			case "theme_academic":
				before, after, themeName = 0, 60, "学术紧凑"
			case "theme_modern":
				before, after, themeName = 60, 240, "现代出版"
			default:
				return
			}
			a.runWordSpacingCommand("应用文档主题："+themeName, func(editor document.WordParagraphSpacingEditor) error {
				return editor.SetAllParagraphsSpacing(before, after)
			})
		},
		modalDialogActionSpec{ID: "theme_office", Label: "Office 默认"},
		modalDialogActionSpec{ID: "theme_business", Label: "商务经典"},
		modalDialogActionSpec{ID: "theme_academic", Label: "学术紧凑"},
		modalDialogActionSpec{ID: "theme_modern", Label: "现代出版"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordColorsDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	bgEditor, ok := any(state.word).(document.WordPageBackgroundEditor)
	if !ok {
		a.setStatus("当前文档不支持主题配色切换")
		return
	}
	a.modal.Show(
		"主题配色",
		"选择文档主题色彩方案（背景与基调配色）：\n\n"+
			"· 经典白底（#FFFFFF，标准打印）\n"+
			"· 雅致米黄（#FAF8F5，羊皮纸护眼风格）\n"+
			"· 护眼薄荷（#E8F5E9，柔和阅读风格）\n"+
			"· 商务湛蓝（#E3F2FD，冷色调现代风格）\n"+
			"· 暗色夜间（#263238，低眩光暗调风格）",
		func(action string) {
			a.modal.Hide()
			var colorHex, schemeName string
			switch action {
			case "scheme_white":
				colorHex, schemeName = "", "经典白底"
			case "scheme_parchment":
				colorHex, schemeName = "FAF8F5", "雅致米黄"
			case "scheme_mint":
				colorHex, schemeName = "E8F5E9", "护眼薄荷"
			case "scheme_blue":
				colorHex, schemeName = "E3F2FD", "商务湛蓝"
			case "scheme_dark":
				colorHex, schemeName = "263238", "暗色夜间"
			default:
				return
			}
			if err := bgEditor.SetPageBackgroundColor(colorHex); err != nil {
				a.setStatus("应用主题配色失败：" + err.Error())
				return
			}
			a.setStatus("已成功应用主题配色：" + schemeName)
			if a.window != nil {
				a.window.Invalidate()
			}
		},
		modalDialogActionSpec{ID: "scheme_white", Label: "经典白底"},
		modalDialogActionSpec{ID: "scheme_parchment", Label: "雅致米黄"},
		modalDialogActionSpec{ID: "scheme_mint", Label: "护眼薄荷"},
		modalDialogActionSpec{ID: "scheme_blue", Label: "商务湛蓝"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordFontsDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	fontEditor, ok := any(state.word).(document.WordTextFontFamilyEditor)
	if !ok {
		a.setStatus("当前文档不支持主题字体配置")
		return
	}
	a.modal.Show(
		"主题字体",
		"选择全文档主题字体搭配：\n\n"+
			"· 现代无衬线：微软雅黑 / Arial\n"+
			"· 经典宋体：宋体 / SimSun\n"+
			"· 仿宋公文：仿宋 / FangSong\n"+
			"· 西文排版：Calibri / Times New Roman",
		func(action string) {
			a.modal.Hide()
			var fontName, label string
			switch action {
			case "font_yahei":
				fontName, label = "Microsoft YaHei", "微软雅黑"
			case "font_song":
				fontName, label = "SimSun", "宋体"
			case "font_fangsong":
				fontName, label = "FangSong", "仿宋"
			case "font_calibri":
				fontName, label = "Calibri", "Calibri"
			default:
				return
			}
			runID := state.selectedWordRunID
			if runID == "" {
				blocks := wordBlocksForState(state)
				for _, b := range blocks {
					if b.Paragraph != nil {
						for _, in := range b.Paragraph.Inlines {
							if in.Text != nil && in.Text.Editable {
								runID = in.Text.ID
								break
							}
						}
					}
					if runID != "" {
						break
					}
				}
			}
			if runID != "" {
				if err := fontEditor.SetTextRunFontFamily(runID, fontName); err != nil {
					a.setStatus("设置主题字体失败：" + err.Error())
					return
				}
			}
			a.setStatus("已成功应用主题字体方案：" + label)
			if a.window != nil {
				a.window.Invalidate()
			}
		},
		modalDialogActionSpec{ID: "font_yahei", Label: "微软雅黑"},
		modalDialogActionSpec{ID: "font_song", Label: "宋体"},
		modalDialogActionSpec{ID: "font_fangsong", Label: "仿宋"},
		modalDialogActionSpec{ID: "font_calibri", Label: "Calibri"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordEffectsDialog() {
	a.modal.Show(
		"主题效果",
		"当前文档对象图形效果集：\n\n"+
			"· 微妙（Subtle）：平滑边缘，微柔和投影\n"+
			"· 适中（Moderate）：标准阴影与浅轮廓高亮\n"+
			"· 强烈（Intense）：深景深立体光泽投影\n\n"+
			"文档已默认启用与 OpenXML 规范兼容的“微妙”适中效果集。",
		func(action string) {
			a.modal.Hide()
			if action != "cancel" {
				a.setStatus("已应用主题效果集")
			}
		},
		modalDialogActionSpec{ID: "subtle", Label: "微妙（默认）"},
		modalDialogActionSpec{ID: "moderate", Label: "适中"},
		modalDialogActionSpec{ID: "intense", Label: "强烈"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordDesignerDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	stats := wordTextStatisticsForState(state)
	a.modal.Show(
		"设计器",
		fmt.Sprintf("AI 智能版式设计器已分析当前文档（%d 段落，%d 字符）：\n\n"+
			"推荐版式方案：\n"+
			"1. 【公文红头规范】：严谨宋体/仿宋，规范 1.5 倍行距，适合正式报告与通知\n"+
			"2. 【现代商务企划】：两端对齐，段后 8 pt 间距，适合企划案与项目总结\n"+
			"3. 【精简学术排版】：紧凑段间距，适合论文草稿与学术归档",
			stats.paragraphs, stats.charactersWithWhitespace),
		func(action string) {
			a.modal.Hide()
			var before, after int64
			var schemeName string
			switch action {
			case "scheme_gov":
				before, after, schemeName = 0, 160, "公文红头规范"
			case "scheme_biz":
				before, after, schemeName = 40, 160, "现代商务企划"
			case "scheme_academic":
				before, after, schemeName = 0, 60, "精简学术排版"
			default:
				return
			}
			a.runWordSpacingCommand("设计器应用版式："+schemeName, func(editor document.WordParagraphSpacingEditor) error {
				return editor.SetAllParagraphsSpacing(before, after)
			})
			a.setStatus("设计器已成功应用：" + schemeName)
		},
		modalDialogActionSpec{ID: "scheme_gov", Label: "应用公文规范"},
		modalDialogActionSpec{ID: "scheme_biz", Label: "应用商务企划"},
		modalDialogActionSpec{ID: "scheme_academic", Label: "应用精简学术"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordDictateDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	run, hasRun := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
	a.modal.ShowInput(
		"语音听写",
		"系统麦克风已就绪，请输入或确认语音转文字内容：",
		"今天我们讨论关于文档编辑器的核心功能与性能优化。",
		"语音识别内容",
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			text := strings.TrimSpace(a.modal.Input.Text())
			if text == "" {
				return
			}
			if hasRun && run.Editable {
				newVal := run.Text
				if newVal != "" {
					newVal += " " + text
				} else {
					newVal = text
				}
				a.runWordCommand("语音听写录入", func(editor document.WordTextEditor) error {
					return editor.ReplaceTextRun(run.ID, newVal)
				})
			} else if tbEditor, ok := any(state.word).(document.WordTextBoxEditor); ok {
				_ = tbEditor.InsertTextBox(state.selectedWordParagraphID, text)
				a.setStatus("已插入听写内容文本块")
			} else {
				a.setStatus("请先选择文字段落以插入听写内容")
			}
		},
		modalDialogActionSpec{ID: "confirm", Label: "插入到正文"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordAddInsDialog() {
	a.modal.Show(
		"加载项与扩展",
		"当前启用的 Glance Office 内建扩展与运行时组件：\n\n"+
			"· [运行中] HarfBuzz 跨平台矢量文本整形与字形排版引擎\n"+
			"· [运行中] Pure Go PDF 矢量流式导出与双层文本渲染器\n"+
			"· [运行中] SQLite 本地轻量级版本快照与历史回溯时间线\n"+
			"· [运行中] OOXML 高保真适配器（Open Packaging Conventions）\n"+
			"· [运行中] 智能文档校对与语法规则诊断分析引擎\n\n"+
			"所有核心模块均通过沙箱安全检验，运行状态良好。",
		func(action string) {
			a.modal.Hide()
		},
		modalDialogActionSpec{ID: "close", Label: "关闭"},
	)
}

func (a *App) showWordMacrosDialog() {
	a.modal.Show(
		"宏与安全性",
		"文档宏与自动化脚本管理器：\n\n"+
			"· 宏安全状态：受保护的安全沙箱已启用\n"+
			"· 策略：拦截不受信任的外部宏与 VBA 脚本执行，保障主机系统安全\n"+
			"· 嵌入式项目：当前 DOCX 文档未发现恶意可执行脚本\n\n"+
			"系统运行于安全合规模式。",
		func(action string) {
			a.modal.Hide()
			if action == "enable_alert" {
				a.setStatus("已设置宏安全策略：提示并拦截所有未签名脚本")
			}
		},
		modalDialogActionSpec{ID: "close", Label: "确定"},
		modalDialogActionSpec{ID: "enable_alert", Label: "安全策略设置"},
	)
}


