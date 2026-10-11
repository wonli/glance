package desktopui

import (
	"image"
	"image/color"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"glance/internal/document"
)

type wordSymbolChoice struct {
	value string
	name  string
}

type wordSymbolGroup struct {
	label string
	start int
}

var (
	wordSymbolChoices = [...]wordSymbolChoice{
		{value: "©", name: "版权符号"}, {value: "®", name: "注册商标"}, {value: "™", name: "商标"},
		{value: "§", name: "章节符号"}, {value: "¶", name: "段落符号"}, {value: "•", name: "项目符号"},
		{value: "€", name: "欧元"}, {value: "£", name: "英镑"}, {value: "¥", name: "日元或人民币"},
		{value: "$", name: "美元"}, {value: "¢", name: "美分"}, {value: "₹", name: "印度卢比"},
		{value: "°", name: "度"}, {value: "±", name: "正负号"}, {value: "×", name: "乘号"},
		{value: "÷", name: "除号"}, {value: "≤", name: "小于等于"}, {value: "≥", name: "大于等于"},
		{value: "←", name: "向左箭头"}, {value: "→", name: "向右箭头"}, {value: "↑", name: "向上箭头"},
		{value: "↓", name: "向下箭头"}, {value: "↔", name: "左右箭头"}, {value: "✓", name: "勾号"},
	}
	wordSymbolGroups = [...]wordSymbolGroup{
		{label: "常用符号", start: 0},
		{label: "货币", start: 6},
		{label: "数学", start: 12},
		{label: "箭头与标记", start: 18},
	}
)

func (a *App) openWordSymbolPicker() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	blocks := wordBlocksForState(state)
	run, ok := selectedWordRun(blocks, state.selectedWordRunID)
	if !ok || !run.Editable || !wordBodyRunEditable(blocks, run.ID) {
		a.setStatus("请先将光标放在可编辑的正文文字中")
		return
	}
	if _, ok := any(state.word).(document.WordTextRangeEditor); !ok {
		a.setStatus("当前 DOCX 适配器不支持按文字范围插入符号")
		return
	}
	paragraphID := state.selectedWordParagraphID
	if _, found := selectedWordParagraph(blocks, paragraphID); !found || !wordBlockContainsParagraphForRun(blocks, paragraphID, run.ID) {
		paragraphID = wordParagraphIDForRun(blocks, run.ID)
	}
	if paragraphID == "" {
		a.setStatus("无法定位插入符号的正文段落")
		return
	}
	editor := wordRunEditor(state, run)
	start, end := editor.Selection()
	length := len([]rune(editor.Text()))
	if start < 0 || end < start || end > length {
		start, end = max(0, min(start, length)), max(0, min(end, length))
		if end < start {
			start, end = end, start
		}
	}
	a.wordSymbolRunID = run.ID
	a.wordSymbolParagraphID = paragraphID
	a.wordSymbolStart = start
	a.wordSymbolEnd = end
	a.wordSymbolRevision = state.wordRevisionAtomic.Load()
	a.wordSymbolDraftEpoch = state.wordDraftEpoch.Load()
	a.wordSymbolPickerVisible = true
}

func wordBlockContainsParagraphForRun(blocks []document.WordBlock, paragraphID, runID string) bool {
	for _, paragraph := range allWordParagraphs(blocks) {
		if paragraph.ID != paragraphID {
			continue
		}
		for _, inline := range paragraph.Inlines {
			if inline.Text != nil && inline.Text.ID == runID {
				return true
			}
		}
	}
	return false
}

func wordParagraphIDForRun(blocks []document.WordBlock, runID string) string {
	for _, paragraph := range allWordParagraphs(blocks) {
		for _, inline := range paragraph.Inlines {
			if inline.Text != nil && inline.Text.ID == runID {
				return paragraph.ID
			}
		}
	}
	return ""
}

func (a *App) closeWordSymbolPicker() {
	a.wordSymbolPickerVisible = false
	a.wordSymbolRunID = ""
	a.wordSymbolParagraphID = ""
	a.wordSymbolStart = 0
	a.wordSymbolEnd = 0
	a.wordSymbolRevision = 0
	a.wordSymbolDraftEpoch = 0
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) chooseWordSymbol(index int) {
	if index < 0 || index >= len(wordSymbolChoices) || !a.wordSymbolPickerVisible {
		return
	}
	choice := wordSymbolChoices[index]
	runID, paragraphID := a.wordSymbolRunID, a.wordSymbolParagraphID
	start, end, revision := a.wordSymbolStart, a.wordSymbolEnd, a.wordSymbolRevision
	draftEpoch := a.wordSymbolDraftEpoch
	a.closeWordSymbolPicker()
	if !a.runWordSymbolInsertion(runID, paragraphID, start, end, revision, draftEpoch, choice) {
		if a.status == "" {
			a.setStatus("无法插入符号，请重试")
		}
	}
}

func (a *App) runWordSymbolInsertion(runID, paragraphID string, start, end int, revision, draftEpoch uint64, choice wordSymbolChoice) bool {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		a.setStatus("文档正在处理其他操作，请稍后重试插入符号")
		return false
	}
	editor, ok := any(state.word).(document.WordTextRangeEditor)
	if !ok {
		a.setStatus("当前 DOCX 适配器不支持按文字范围插入符号")
		return false
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatus("文档正在处理其他操作，请稍后重试插入符号")
		return false
	}
	if token.revision != revision || state.wordDraftEpoch.Load() != draftEpoch || wordHasPendingTextDrafts(state) {
		state.finishOperation(token)
		a.setStatus("文档内容或编辑草稿已变化，请提交后重新打开符号面板")
		return false
	}
	generation := a.documentGeneration
	a.launchManagedBackground(func() {
		blocks, err := runWordMutation(state, func() error {
			return editor.ReplaceTextRange(runID, start, end, choice.value)
		})
		a.publishWordCommand(wordCommandResult{
			state: state, generation: generation, operation: "插入符号", err: err, blocks: blocks,
			wordSymbolInsertion: true, replaceRunID: runID, replaceParagraphID: paragraphID, replaceStart: start,
			insertedSymbol: choice.value,
		})
	})
	return true
}

func (a *App) completeWordSymbolInsertion(result wordCommandResult) {
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
	run, ok := selectedWordRun(wordBlocksForState(state), result.replaceRunID)
	if !ok || !run.Editable {
		a.setStatus("符号已插入，但无法定位编辑位置")
		return
	}
	state.setCommittedWordTextAndClearDraft(run.ID, run.Text)
	textEditor := wordRunEditor(state, run)
	textEditor.SetText(run.Text)
	caret := result.replaceStart + len([]rune(result.insertedSymbol))
	caret = max(0, min(caret, len([]rune(run.Text))))
	textEditor.SetCaret(caret, caret)
	state.selectedWordRunID = run.ID
	state.selectedWordParagraphID = result.replaceParagraphID
	state.selectedWordImageID = ""
	state.selectedBlock = -1
	state.selectedBlockEditable = true
	state.wordPendingFocusRunID = run.ID
	state.wordPendingCaretRunID = ""
	state.wordPendingCaretByX = false
	a.setStatus("已插入符号 " + result.insertedSymbol)
	a.persistActiveSession()
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) layoutWordSymbolPicker(gtx layout.Context) layout.Dimensions {
	if !a.wordSymbolPickerVisible {
		return layout.Dimensions{}
	}
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return a.wordSymbolPickerScrim.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if a.wordSymbolPickerScrim.Clicked(gtx) {
					a.closeWordSymbolPicker()
				}
				size := gtx.Constraints.Max
				paint.FillShape(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 96}, clip.Rect{Max: size}.Op())
				semantic.DescriptionOp("关闭符号面板").Add(gtx.Ops)
				return layout.Dimensions{Size: size}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			width := gtx.Dp(unit.Dp(456))
			if width > gtx.Constraints.Max.X {
				width = gtx.Constraints.Max.X
			}
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			child := op.Record(gtx.Ops)
			dims := layout.Inset{Top: unit.Dp(18), Bottom: unit.Dp(18), Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				children := []layout.FlexChild{
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, material.H6(a.theme, "插入符号").Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if a.wordSymbolPickerClose.Clicked(gtx) {
									a.closeWordSymbolPicker()
								}
								button := material.Button(a.theme, &a.wordSymbolPickerClose, "关闭")
								button.TextSize = a.theme.TextSize * 12 / 16
								return button.Layout(gtx)
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						label := material.Body2(a.theme, "选择一个字符，将其插入到当前光标或选区。")
						label.Color = mutedText
						return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(10)}.Layout(gtx, label.Layout)
					}),
				}
				for _, group := range wordSymbolGroups {
					group := group
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						label := material.Caption(a.theme, group.label)
						label.Color = mutedText
						return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(2)}.Layout(gtx, label.Layout)
					}))
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						buttons := make([]layout.FlexChild, 0, 6)
						for column := 0; column < 6; column++ {
							index := group.start + column
							choice := wordSymbolChoices[group.start+column]
							buttons = append(buttons, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								if a.wordSymbolButtons[index].Clicked(gtx) {
									a.chooseWordSymbol(index)
								}
								semantic.DescriptionOp(choice.name + " " + choice.value).Add(gtx.Ops)
								button := material.Button(a.theme, &a.wordSymbolButtons[index], choice.value)
								button.TextSize = a.theme.TextSize * 20 / 16
								button.Background = contentSurface
								button.Color = color.NRGBA{R: 39, G: 73, B: 112, A: 255}
								button.CornerRadius = 6
								button.Inset = layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4)}
								return layout.Inset{Left: unit.Dp(2), Right: unit.Dp(2), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, button.Layout)
							}))
						}
						return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, buttons...)
					}))
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			})
			call := child.Stop()
			radius := gtx.Dp(unit.Dp(10))
			paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.RRect{Rect: image.Rect(0, 0, dims.Size.X, dims.Size.Y), NW: radius, NE: radius, SW: radius, SE: radius}.Op(gtx.Ops))
			call.Add(gtx.Ops)
			return dims
		}),
	)
}
