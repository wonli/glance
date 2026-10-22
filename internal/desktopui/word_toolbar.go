package desktopui

import (
	"image/color"
	"strings"

	"gioui.org/widget"

	"glance/internal/document"
	"glance/internal/i18n"
	"glance/internal/wordlayout"
)

type wordToolbarTab uint8
type wordToolbarAction = ribbonAction

const (
	wordTabFile wordToolbarTab = iota
	wordTabHome
	wordTabInsert
	wordTabDesign
	wordTabLayout
	wordTabReferences
	wordTabReview
	wordTabView
	wordTabPicture
)

const (
	wordDefaultFontSizePt      = wordlayout.DefaultFontSizePT
	wordMinFontSizePt          = 1.0
	wordMaxFontSizePt          = 1638.0
	wordFontSizeStepPt         = 1.0
	wordParagraphIndentStepEMU = int64(457200)
	wordMaxParagraphIndentEMU  = int64(1<<31-1) * 635
)

var (
	wordToolbarTabLabels = []string{"文件", "开始", "插入", "设计", "布局", "引用", "审阅", "视图", "图片"}
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
	wordReferenceActions = wordActions(
		"references-toc|目录|目录|list", "references-update-toc|更新目录|目录|refresh-cw",
		"references-footnotes|插入脚注|脚注|list-ordered", "references-endnote|插入尾注|脚注|file-text",
		"references-citations|插入引文|引文与书目|bookmark", "references-bibliography|书目|引文与书目|book-open",
		"references-caption|插入题注|题注|file-text", "references-cross-reference|交叉引用|题注|link",
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
		"view-new-window|新建窗口|窗口|app-window", "view-side-by-side|并排查看|窗口|columns-2",
		"view-macros|宏|宏|code-xml",
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
	tabs := []wordToolbarTab{wordTabFile, wordTabHome, wordTabInsert, wordTabReferences, wordTabDesign, wordTabLayout, wordTabReview, wordTabView}
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
	case wordTabReferences:
		return wordReferenceActions
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

func wordToolbarTabMessage(tab wordToolbarTab) i18n.Message {
	return i18n.Message{ID: []string{
		"word.ribbon.tab.file", "word.ribbon.tab.home", "word.ribbon.tab.insert",
		"word.ribbon.tab.design", "word.ribbon.tab.layout", "word.ribbon.tab.references",
		"word.ribbon.tab.review", "word.ribbon.tab.view", "word.ribbon.tab.picture",
	}[tab]}
}

func (a *App) localizedWordAction(action ribbonAction) ribbonAction {
	if action.id == "" {
		return action
	}
	label := a.text(i18n.Message{ID: "word.ribbon.action." + strings.ReplaceAll(action.id, "-", "_")})
	if !strings.HasPrefix(label, "Missing translation:") && !strings.HasPrefix(label, "缺少翻译：") {
		action.label = label
	}
	return action
}

func (a *App) localizedWordActions(actions []ribbonAction) []ribbonAction {
	localized := append([]ribbonAction(nil), actions...)
	for index := range localized {
		localized[index] = a.localizedWordAction(localized[index])
	}
	return localized
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
		return a.nativePrinter != nil && a.nativePrinter.Available()
	case "autosave":
		return a.applicationSessionRegistered(state.documentID)
	case "doc-info":
		_, supported := any(state.word).(document.WordDocumentPropertiesEditor)
		return supported
	case "view-reading", "view-print-layout", "view-web-layout", "view-navigation", "view-gridlines", "view-single-page", "view-multiple-pages":
		return true
	case "view-ruler", "view-zoom":
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
		if state.wordTableSelection != nil && (action.id == "copy" || action.id == "cut") {
			_, supported := any(state.word).(document.WordTableSelectionEditor)
			return supported
		}
		run, ok := selectedWordFormattingRun(state, state.selectedWordRunID)
		return ok && (action.id == "copy" || run.Editable)
	case "style-body", "style-heading", "style-heading-2":
		_, editable := any(state.word).(document.WordParagraphStyleEditor)
		return editable
	case "bold", "italic", "underline":
		if start, end, selected := wordTextFormattingSelection(state); selected {
			_, formatter := any(state.word).(document.WordTextSelectionFormatter)
			run, ok := firstSelectedWordTextRun(state, start, end)
			return formatter && ok && run.Editable
		}
		_, editable := any(state.word).(document.WordTextEditor)
		run, ok := selectedWordFormattingRun(state, state.selectedWordRunID)
		return editable && ok && run.Editable
	case "font-color", "text-highlight":
		_, editable := any(state.word).(document.WordTextEditor)
		run, ok := selectedWordFormattingRun(state, state.selectedWordRunID)
		if !ok && wordPageTextSelectionActive(state.wordPageTextSelection) {
			start, _, hasSel := wordPageOrderedSelection(state.wordPageTextSelection)
			if hasSel {
				run, ok = selectedWordFormattingRun(state, start.runID)
			}
		}
		return editable && ok && run.Editable
	case "font-family":
		_, editable := any(state.word).(document.WordTextFontFamilyEditor)
		run, ok := selectedWordFormattingRun(state, state.selectedWordRunID)
		if !ok && wordPageTextSelectionActive(state.wordPageTextSelection) {
			start, _, hasSel := wordPageOrderedSelection(state.wordPageTextSelection)
			if hasSel {
				run, ok = selectedWordFormattingRun(state, start.runID)
			}
		}
		return editable && ok && run.Editable
	case "font-size", "font-size-decrease", "font-size-increase":
		_, editable := any(state.word).(document.WordTextFontSizeEditor)
		run, ok := selectedWordFormattingRun(state, state.selectedWordRunID)
		if !ok && wordPageTextSelectionActive(state.wordPageTextSelection) {
			start, _, hasSel := wordPageOrderedSelection(state.wordPageTextSelection)
			if hasSel {
				run, ok = selectedWordFormattingRun(state, start.runID)
			}
		}
		if !editable || !ok || !run.Editable {
			return false
		}
		if action.id == "font-size" {
			return true
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
		run, ok := selectedWordFormattingRun(state, state.selectedWordRunID)
		return editable && ok && run.Editable && wordBodyRunEditable(wordBlocksForState(state), run.ID)
	case "page-break", "layout-breaks", "blank-page":
		_, editor := any(state.word).(document.WordPageBreakEditor)
		run, ok := selectedWordFormattingRun(state, state.selectedWordRunID)
		return editor && ok && run.Editable && wordBodyRunEditable(wordBlocksForState(state), run.ID)
	case "format-painter":
		if state.wordFormatPainterActive {
			return true
		}
		_, painter := any(state.word).(document.WordFormatPainter)
		run, ok := selectedWordFormattingRun(state, state.selectedWordRunID)
		return painter && ok && run.Editable
	case "review-word-count":
		_, content := any(state.word).(document.WordContentReader)
		return content
	case "review-restrict":
		_, ok := any(state.word).(document.WordDocumentProtectionEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case "save":
		if sessionPath(state) == "" {
			return true
		}
		return sessionDirty(state) || wordHasPendingTextDrafts(state) || state.wordParagraphSplitIntent != nil || state.wordParagraphJoinIntent != nil
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
	case "review-editor", "editor", "review-translate", "view-macros":
		return a.applicationSessionRegistered(state.documentID)
	case "review-compare":
		return state.word != nil && !state.formatRunning.Load() && !a.isSaving()
	case "layout-position":
		image := findWordImage(wordBlocksForState(state), state.selectedWordImageID)
		_, editable := any(state.word).(document.WordImageEditor)
		return editable && image != nil && image.Editable
	case "layout-align":
		return state.word != nil && !state.formatRunning.Load() && !a.isSaving()
	case "layout-wrap":
		image := findWordImage(wordBlocksForState(state), state.selectedWordImageID)
		return image != nil && image.WrapEditable && image.WrapMode != "inline" && !state.formatRunning.Load() && !a.isSaving()
	case "insert-wordart", "insert-shapes":
		_, ok := any(state.word).(document.WordTextBoxEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case "insert-equation":
		return state.word != nil && !state.formatRunning.Load() && !a.isSaving()
	case "insert-chart":
		_, ok := any(state.word).(document.WordTableInsertEditor)
		return ok && !state.formatRunning.Load() && !a.isSaving()
	case "review-new-comment":
		_, editor := any(state.word).(document.WordCommentEditor)
		run, selected := selectedWordFormattingRun(state, state.selectedWordRunID)
		return editor && selected && run.Editable && wordBodyRunEditable(wordBlocksForState(state), run.ID) && a.applicationSessionRegistered(state.documentID)
	case "review-delete-comment":
		_, reader := any(state.word).(document.WordCommentReader)
		return reader && a.applicationSessionRegistered(state.documentID)
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
		editor, ok := any(state.word).(document.WordThemeEffectsEditor)
		return ok && editor.CanSetWordThemeEffects() && !state.formatRunning.Load() && !a.isSaving()
	case "designer", "dictate", "add-ins":
		return state.word != nil && !state.formatRunning.Load() && !a.isSaving()
	case "references-toc", "references-footnotes", "references-citations", "references-caption":
		return state.word != nil && !state.formatRunning.Load() && !a.isSaving()
	default:
		return false
	}
}

func (a *App) wordToolbarActionButtonEnabled(action ribbonAction) bool {
	if action.id != "save" && action.id != "print" && action.id != "doc-info" {
		return a.wordToolbarActionEnabled(action)
	}
	state := a.documentSessionState
	return state != nil && state.word != nil && !state.formatRunning.Load() && !a.isSaving() && !a.fileCommandRunning && !a.wordImagePickerRunning
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
	if action.id == "style-body" || action.id == "style-heading" || action.id == "style-heading-2" {
		return wordParagraphStyleActionActive(state, action.id)
	}
	if action.id == "bold" || action.id == "italic" || action.id == "underline" || action.id == "font-color" || action.id == "text-highlight" {
		var (
			run document.WordTextRun
			ok  bool
		)
		if start, end, selected := wordTextFormattingSelection(state); selected {
			run, ok = firstSelectedWordTextRun(state, start, end)
		} else {
			run, ok = selectedWordFormattingRun(state, state.selectedWordRunID)
		}
		if ok {
			run, _ = wordRunWithResolvedStyle(state, run)
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
		return false
	}
	blocks := wordBlocksForState(state)
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
	action = a.localizedWordAction(action)
	if action.id == "version-history" && canonicalSessionPath(sessionPath(a.documentSessionState)) == "" {
		return a.text(i18n.Message{ID: "word.ribbon.action.unavailable_save_first", Args: i18n.Args{"Action": action.label}})
	}
	if action.id == "view-new-window" {
		if _, reason := wordNewWindowAvailability(a.documentSessionState, a.desktopRuntime); reason.ID != "" {
			return a.text(i18n.Message{ID: "word.new_window.unavailable", Args: i18n.Args{"Action": action.label, "Reason": a.text(reason)}})
		}
	}
	return action.label
}
