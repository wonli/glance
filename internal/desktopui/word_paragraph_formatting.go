package desktopui

import (
	"glance/internal/document"
	"glance/internal/i18n"
	"math"
	"strconv"
	"strings"
)

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
	title, body, currentPT, operation := i18n.Message{ID: "word.paragraph_format.spacing_before.title"}, i18n.Message{ID: "word.paragraph_format.spacing_before.body"}, paragraph.Layout.BeforePT, i18n.Message{ID: "word.paragraph_format.spacing_before.operation"}
	if actionID == "layout-after" {
		title, body, currentPT, operation = i18n.Message{ID: "word.paragraph_format.spacing_after.title"}, i18n.Message{ID: "word.paragraph_format.spacing_after.body"}, paragraph.Layout.AfterPT, i18n.Message{ID: "word.paragraph_format.spacing_after.operation"}
	}
	initialVal := strconv.FormatFloat(currentPT, 'f', -1, 64)
	a.modal.ShowInputMessage("word.paragraph_format.spacing", title, body, initialVal, i18n.Message{ID: "word.paragraph_format.spacing.hint"}, func(action string) {
		draft := strings.TrimSpace(a.modal.Input.Text())
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		pt, err := strconv.ParseFloat(draft, 64)
		if err != nil || math.IsNaN(pt) || math.IsInf(pt, 0) || pt < 0 || pt > 1584 {
			a.setStatusMessage(i18n.Message{ID: "word.paragraph_format.error.range"}, feedbackError, notificationToast)
			if a.window != nil {
				a.window.Invalidate()
			}
			return
		}
		twips := int64(math.Round(pt * 20.0))
		a.runWordSpacingCommand(a.text(operation), func(editor document.WordParagraphSpacingEditor) error {
			if actionID == "layout-before" {
				return editor.SetParagraphSpacing(paragraphID, &twips, nil)
			}
			return editor.SetParagraphSpacing(paragraphID, nil, &twips)
		})
	}, modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "common.action.apply"}}, modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}})
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
	a.launchManagedBackground(func() {
		blocks, err := runWordMutation(state, func() error { return edit(editor) })
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: operation, err: err, blocks: blocks})
	})
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
	labels := []i18n.Message{{ID: "word.paragraph_format.indent.left"}, {ID: "word.paragraph_format.indent.right"}}
	hints := []i18n.Message{{ID: "word.paragraph_format.indent.hint"}, {ID: "word.paragraph_format.indent.hint"}}
	initial := []string{
		strconv.FormatFloat(leftPT, 'f', -1, 64),
		strconv.FormatFloat(rightPT, 'f', -1, 64),
	}
	a.modal.ShowFormMessage("word.paragraph_format.indent", i18n.Message{ID: "word.paragraph_format.indent.title"}, i18n.Message{ID: "word.paragraph_format.indent.body"},
		labels, hints, initial,
		func(action string) {
			switch action {
			case "none":
				a.modal.Hide()
				a.runWordCommand(a.text(i18n.Message{ID: "word.paragraph_format.indent.operation.clear"}), func(editor document.WordTextEditor) error {
					return editor.SetParagraphIndents(paragraphID, 0, 0)
				})
			case "confirm":
				left, err1 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[0].Text()), 64)
				right, err2 := strconv.ParseFloat(strings.TrimSpace(a.modal.FormInputs[1].Text()), 64)
				if err1 != nil || err2 != nil || left < 0 || right < 0 || left > 1000 || right > 1000 {
					a.setStatusMessage(i18n.Message{ID: "word.status.inline.indent_must_be_a_value_999ca8"}, feedbackError, notificationToast)
					if a.window != nil {
						a.window.Invalidate()
					}
					return
				}
				a.modal.Hide()
				leftEMU := int64(math.Round(left * 12700.0))
				rightEMU := int64(math.Round(right * 12700.0))
				a.runWordCommand(a.text(i18n.Message{ID: "word.paragraph_format.indent.operation.set"}), func(editor document.WordTextEditor) error {
					return editor.SetParagraphIndents(paragraphID, leftEMU, rightEMU)
				})
			case "cancel":
				a.modal.Hide()
			}
		},
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "common.action.apply"}},
		modalDialogActionSpec{ID: "none", LabelRef: i18n.Message{ID: "word.paragraph_format.indent.none"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}
