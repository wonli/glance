package desktopui

import (
	"glance/internal/i18n"
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
	value  string
	nameID string
}

type wordSymbolGroup struct {
	labelID string
	start   int
}

var (
	wordSymbolChoices = [...]wordSymbolChoice{
		{value: "©", nameID: "word.symbol.name.copyright"}, {value: "®", nameID: "word.symbol.name.registered"}, {value: "™", nameID: "word.symbol.name.trademark"},
		{value: "§", nameID: "word.symbol.name.section"}, {value: "¶", nameID: "word.symbol.name.paragraph"}, {value: "•", nameID: "word.symbol.name.bullet"},
		{value: "€", nameID: "word.symbol.name.euro"}, {value: "£", nameID: "word.symbol.name.pound"}, {value: "¥", nameID: "word.symbol.name.yen_yuan"},
		{value: "$", nameID: "word.symbol.name.dollar"}, {value: "¢", nameID: "word.symbol.name.cent"}, {value: "₹", nameID: "word.symbol.name.rupee"},
		{value: "°", nameID: "word.symbol.name.degree"}, {value: "±", nameID: "word.symbol.name.plus_minus"}, {value: "×", nameID: "word.symbol.name.multiply"},
		{value: "÷", nameID: "word.symbol.name.divide"}, {value: "≤", nameID: "word.symbol.name.less_or_equal"}, {value: "≥", nameID: "word.symbol.name.greater_or_equal"},
		{value: "←", nameID: "word.symbol.name.arrow_left"}, {value: "→", nameID: "word.symbol.name.arrow_right"}, {value: "↑", nameID: "word.symbol.name.arrow_up"},
		{value: "↓", nameID: "word.symbol.name.arrow_down"}, {value: "↔", nameID: "word.symbol.name.arrow_both"}, {value: "✓", nameID: "word.symbol.name.check"},
	}
	wordSymbolGroups = [...]wordSymbolGroup{
		{labelID: "word.symbol.group.common", start: 0},
		{labelID: "word.symbol.group.currency", start: 6},
		{labelID: "word.symbol.group.math", start: 12},
		{labelID: "word.symbol.group.arrows_marks", start: 18},
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
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.place_the_caret_in_editable_ce48f1"}, feedbackWarning, notificationToast)
		return
	}
	if _, ok := any(state.word).(document.WordTextRangeEditor); !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.inserting_a_symbol_in_a_19ae33"}, feedbackWarning, notificationToast)
		return
	}
	paragraphID := state.selectedWordParagraphID
	if _, found := selectedWordParagraph(blocks, paragraphID); !found || !wordBlockContainsParagraphForRun(blocks, paragraphID, run.ID) {
		paragraphID = wordParagraphIDForRun(blocks, run.ID)
	}
	if paragraphID == "" {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.could_not_find_the_body_0bec7c"}, feedbackWarning, notificationToast)
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
		if a.statusText() == "" {
			a.setStatusMessage(i18n.Message{ID: "word.status.inline.could_not_insert_the_symbol_f4c607"}, feedbackWarning, notificationToast)
		}
	}
}

func (a *App) runWordSymbolInsertion(runID, paragraphID string, start, end int, revision, draftEpoch uint64, choice wordSymbolChoice) bool {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.another_operation_is_in_progress_1ee248"}, feedbackInfo, notificationToast)
		return false
	}
	editor, ok := any(state.word).(document.WordTextRangeEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.inserting_a_symbol_in_a_19ae33"}, feedbackWarning, notificationToast)
		return false
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.another_operation_is_in_progress_1ee248"}, feedbackInfo, notificationToast)
		return false
	}
	if token.revision != revision || state.wordDraftEpoch.Load() != draftEpoch || wordHasPendingTextDrafts(state) {
		state.finishOperation(token)
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_document_or_edit_draft_709ce3"}, feedbackWarning, notificationToast)
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
		a.setWordCommandStatus(result.err)
		return
	}
	if result.blocks != nil {
		state.setWordBlocks(result.blocks)
		a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
	}
	run, ok := selectedWordRun(wordBlocksForState(state), result.replaceRunID)
	if !ok || !run.Editable {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_symbol_was_inserted_but_519405"}, feedbackWarning, notificationToast)
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
	a.setStatusMessage(i18n.Message{ID: "word.symbol.status.inserted", Args: i18n.Args{"Symbol": result.insertedSymbol}}, feedbackSuccess, notificationToast)
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
				semantic.DescriptionOp(a.textID("word.symbol.picker.close_description")).Add(gtx.Ops)
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
							layout.Flexed(1, material.H6(a.theme, a.textID("word.symbol.picker.title")).Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if a.wordSymbolPickerClose.Clicked(gtx) {
									a.closeWordSymbolPicker()
								}
								button := material.Button(a.theme, &a.wordSymbolPickerClose, a.textID("common.action.close"))
								button.TextSize = a.theme.TextSize * 12 / 16
								return button.Layout(gtx)
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						label := material.Body2(a.theme, a.textID("word.symbol.picker.description"))
						label.Color = mutedText
						return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(10)}.Layout(gtx, label.Layout)
					}),
				}
				for _, group := range wordSymbolGroups {
					group := group
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						label := material.Caption(a.theme, a.textID(group.labelID))
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
								semantic.DescriptionOp(a.textID(choice.nameID) + " " + choice.value).Add(gtx.Ops)
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
