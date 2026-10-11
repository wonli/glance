package desktopui

import (
	"glance/internal/document"
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
