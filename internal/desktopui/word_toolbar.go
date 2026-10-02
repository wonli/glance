package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"strconv"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/io/semantic"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"glance/internal/document"
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
		{slot: -1, id: "font-family", label: "字体", group: "字体", icon: "file-text"},
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
	if state == nil || state.word == nil || state.formatRunning || a.saving || a.wordImagePickerRunning {
		return false
	}
	switch action.id {
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
	case "bold", "italic", "underline", "font-color":
		_, editable := any(state.word).(document.WordTextEditor)
		run, ok := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
		return editable && ok && run.Editable
	case "align-left", "align-center", "align-right", "align-justify":
		_, editable := any(state.word).(document.WordTextEditor)
		_, ok := selectedWordParagraph(wordBlocksForState(state), state.selectedWordParagraphID)
		return editable && ok
	case "save":
		return (state.word.Dirty() || wordHasPendingTextDrafts(state) || state.wordParagraphSplitIntent != nil || state.wordParagraphJoinIntent != nil) && state.path != ""
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
		}
	}
	if paragraph, ok := selectedWordParagraph(blocks, state.selectedWordParagraphID); ok {
		alignment, isAlignment := map[string]document.WordAlignment{"align-left": document.WordAlignLeft, "align-center": document.WordAlignCenter, "align-right": document.WordAlignRight, "align-justify": document.WordAlignJustified}[action.id]
		return isAlignment && alignment == paragraph.Alignment
	}
	return false
}

func (a *App) handleWordToolbarEvents(gtx layout.Context) {
	state := a.documentSessionState
	if state == nil || state.word == nil || a.modal.Visible {
		return
	}
	tabs := wordToolbarTabs(state)
	for index, tab := range tabs {
		if state.wordToolbarTabs[index].Clicked(gtx) {
			state.wordToolbarTab = tab
			state.wordToolbarActionList.List.Position = layout.Position{}
			state.wordToolbarColorPicker = false
			break
		}
	}
	for _, action := range wordToolbarActions(state.wordToolbarTab) {
		button := state.wordToolbarActionButtons[action.id]
		if button == nil || !button.Clicked(gtx) || !a.wordToolbarActionEnabled(action) {
			continue
		}
		if wordHasPendingTextDrafts(state) && action.id == "paste" {
			state.wordClipboardRunID = state.selectedWordRunID
			state.wordForceDraftFlush = true
			state.wordFlushRequested = true
			state.wordFlushAllDrafts = true
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
			state.wordForceDraftFlush = true
			state.wordFlushRequested = true
			state.wordFlushAllDrafts = true
			state.wordDeferredToolbarAction = action.id
			continue
		}
		if wordHasPendingTextDrafts(state) && action.id != "copy" {
			state.wordForceDraftFlush = true
			state.wordFlushRequested = true
			state.wordFlushAllDrafts = true
			if action.id == "save" {
				state.wordSaveAfterDrafts = true
			} else {
				state.wordDeferredToolbarAction = action.id
			}
			continue
		}
		switch action.id {
		case "undo":
			a.applyWordHistory(true)
		case "redo":
			a.applyWordHistory(false)
		case "cut", "copy":
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
		case "font-color":
			state.wordToolbarColorPicker = !state.wordToolbarColorPicker
		case "align-left", "align-center", "align-right", "align-justify":
			a.setWordAlignment(action.id)
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
	if state.wordToolbarColorPicker {
		for index := range state.wordToolbarColorButtons {
			if state.wordToolbarColorButtons[index].Clicked(gtx) {
				colorValue := wordFontColors[index].value
				runID := state.selectedWordRunID
				state.wordToolbarColorPicker = false
				a.setWordFontColor(runID, colorValue)
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
	case "font-color":
		state.wordToolbarColorPicker = !state.wordToolbarColorPicker
	case "align-left", "align-center", "align-right", "align-justify":
		a.setWordAlignment(actionID)
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
			a.status = "读取剪贴板失败：" + err.Error()
			continue
		}
		runID := state.wordClipboardRunID
		state.wordClipboardRunID = ""
		if state.formatRunning || wordHasPendingTextDrafts(state) {
			state.wordPendingPasteRunID = runID
			state.wordPendingPasteText = string(value)
			if wordHasPendingTextDrafts(state) {
				state.wordForceDraftFlush = true
				state.wordFlushRequested = true
				state.wordFlushAllDrafts = true
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

func (a *App) runWordCommand(operation string, edit func(document.WordTextEditor) error) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning || a.saving {
		return
	}
	editor, ok := any(state.word).(document.WordTextEditor)
	if !ok {
		return
	}
	state.formatRunning = true
	generation := a.documentGeneration
	go func() {
		err := edit(editor)
		var blocks []document.WordBlock
		if err == nil {
			blocks = cloneWordBlockSnapshot(state.word.WordBlocks())
		}
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: operation, err: err, blocks: blocks})
	}()
}

func (a *App) runWordImageCommand(operation, imageID string, edit func(document.WordImageEditor) error, clearSelection bool) bool {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning || a.saving {
		return false
	}
	editor, ok := any(state.word).(document.WordImageEditor)
	if !ok {
		return false
	}
	state.formatRunning = true
	generation := a.documentGeneration
	go func() {
		err := edit(editor)
		var blocks []document.WordBlock
		if err == nil {
			blocks = cloneWordBlockSnapshot(state.word.WordBlocks())
		}
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
	state               *documentSessionState
	blocks              []document.WordBlock
	selectInsertedImage bool
	generation          uint64
	operation           string
	err                 error
	clearImageSelection bool
	imageID             string
	undo                bool
	history             bool
	applied             bool
	textDraft           bool
	paragraphSplit      bool
	paragraphJoin       bool
	runID               string
	runText             string
	splitSourceRunID    string
	splitPrefix         string
	splitParagraphID    string
	splitRunID          string
	splitRunText        string
	joinParagraphID     string
	joinRunID           string
	joinCaret           int
	insertedParagraphID string
	bodyParagraphInsert bool
}

func (a *App) publishWordCommand(result wordCommandResult) {
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
	if state == nil || state.word == nil || state.formatRunning || a.saving {
		return
	}
	history, ok := any(state.word).(document.WordHistory)
	if !ok {
		return
	}
	state.formatRunning = true
	generation := a.documentGeneration
	go func() {
		var applied bool
		var err error
		if undo {
			applied, err = history.Undo()
		} else {
			applied, err = history.Redo()
		}
		var blocks []document.WordBlock
		if err == nil && applied {
			blocks = cloneWordBlockSnapshot(state.word.WordBlocks())
		}
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
			a.status = "图片宽度必须大于 0 且不超过 100 英寸"
			return
		}
		widthEMU := int64(math.Round(widthInches * 914400))
		a.runWordImageCommand("调整图片", imageID, func(editor document.WordImageEditor) error { return editor.ResizeInlineImage(imageID, widthEMU) }, false)
	}, modalDialogActionSpec{ID: "confirm", Label: "应用"}, modalDialogActionSpec{ID: "cancel", Label: "取消"})
}

func (a *App) layoutWordToolbarColorPicker(gtx layout.Context) layout.Dimensions {
	state := a.documentSessionState
	if state == nil || !state.wordToolbarColorPicker {
		return layout.Dimensions{}
	}
	return layout.Inset{Top: unit.Dp(wordRibbonHeightDP + 8), Left: unit.Dp(440)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		width := gtx.Dp(unit.Dp(8*28 + 7*6 + 16))
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
					children := make([]layout.FlexChild, 0, len(wordFontColors))
					for index, item := range wordFontColors {
						index, item := index, item
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							button := &state.wordToolbarColorButtons[index]
							return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								size := gtx.Dp(unit.Dp(28))
								bounds := image.Rect(0, 0, size, size)
								paint.FillShape(gtx.Ops, item.color, clip.UniformRRect(bounds, gtx.Dp(unit.Dp(5))).Op(gtx.Ops))
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
