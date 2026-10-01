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
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"glance/internal/document"
)

type wordToolbarTab uint8
type wordToolbarAction = ribbonAction

const (
	wordTabHome wordToolbarTab = iota
	wordTabInsert
	wordTabPicture
)

var (
	wordToolbarTabLabels = []string{"主页", "插入", "图片"}
	wordHomeActions      = []ribbonAction{
		{slot: -1, id: "undo", label: "撤销", group: "历史", icon: "undo"},
		{slot: -1, id: "redo", label: "重做", group: "历史", icon: "redo"},
		{slot: -1, id: "cut", label: "剪切", group: "剪贴板", icon: "cut"},
		{slot: -1, id: "copy", label: "复制", group: "剪贴板", icon: "copy"},
		{slot: -1, id: "paste", label: "粘贴", group: "剪贴板", icon: "paste"},
		{slot: -1, id: "bold", label: "加粗", group: "字体", icon: "bold"},
		{slot: -1, id: "italic", label: "斜体", group: "字体", icon: "italic"},
		{slot: -1, id: "underline", label: "下划线", group: "字体", icon: "underline"},
		{slot: -1, id: "font-color", label: "字体颜色", group: "字体", icon: "type"},
		{slot: -1, id: "align-left", label: "左对齐", group: "段落", icon: "align-left"},
		{slot: -1, id: "align-center", label: "居中对齐", group: "段落", icon: "align-center"},
		{slot: -1, id: "align-right", label: "右对齐", group: "段落", icon: "align-right"},
		{slot: -1, id: "align-justify", label: "两端对齐", group: "段落", icon: "align-distributed"},
		{slot: -1, id: "save", label: "保存", group: "文件", icon: "save"},
	}
	wordInsertActions = []ribbonAction{
		{slot: -1, id: "insert-image", label: "插入图片", group: "插图", icon: "file"},
	}
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

func wordToolbarTabs(state *documentSessionState) []wordToolbarTab {
	if state != nil && state.selectedWordImageID != "" {
		return []wordToolbarTab{wordTabHome, wordTabInsert, wordTabPicture}
	}
	return []wordToolbarTab{wordTabHome, wordTabInsert}
}

func wordToolbarActions(tab wordToolbarTab) []ribbonAction {
	switch tab {
	case wordTabHome:
		return wordHomeActions
	case wordTabInsert:
		return wordInsertActions
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

func (a *App) layoutWordToolbar(gtx layout.Context) layout.Dimensions {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return layout.Dimensions{}
	}
	if state.wordToolbarTab == wordTabPicture && state.selectedWordImageID == "" {
		state.wordToolbarTab = wordTabHome
	}
	tabs := wordToolbarTabs(state)
	labels := make([]string, len(tabs))
	for i, tab := range tabs {
		labels[i] = wordToolbarTabLabels[tab]
	}
	height := gtx.Dp(unit.Dp(90))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, contentSurface, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutRibbonCategoryTabs(gtx, a.theme, labels, state.wordToolbarTabs[:len(tabs)], int(state.wordToolbarTab))
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					stripHeight := gtx.Dp(unit.Dp(56))
					gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = stripHeight, stripHeight
					actions := wordToolbarActions(state.wordToolbarTab)
					shift := op.Offset(image.Pt(0, -gtx.Dp(unit.Dp(4)))).Push(gtx.Ops)
					defer shift.Pop()
					return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(16), Right: unit.Dp(22)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return state.wordToolbarActionList.Layout(gtx, len(actions), func(gtx layout.Context, index int) layout.Dimensions {
							action := actions[index]
							enabled := a.wordToolbarActionEnabled(action)
							button := wordToolbarButton(state, action.id)
							background := contentSurface
							if wordToolbarActionActive(state, action) {
								background = toolbarActiveBackground
							}
							icon, _ := tintedToolbarIcon(action.icon, toolbarIconColor(true))
							return layoutToolbarActionItem(gtx, action, index, actions, func(gtx layout.Context) layout.Dimensions {
								return toolbarIconButton(gtx, button, icon, action.label, enabled, false, background)
							})
						})
					})
				}),
			)
		},
	)
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

func (a *App) runWordImageCommand(operation, imageID string, edit func(document.WordImageEditor) error, clearSelection bool) {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning || a.saving {
		return
	}
	editor, ok := any(state.word).(document.WordImageEditor)
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
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: operation, err: err, clearImageSelection: clearSelection && err == nil, imageID: imageID, blocks: blocks})
	}()
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
	return layout.Inset{Top: unit.Dp(86), Left: unit.Dp(368)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
