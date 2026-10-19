package desktopui

import (
	"glance/internal/document"
	"glance/internal/i18n"
	"strconv"
	"strings"
)

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
	label := a.text(i18n.Message{ID: "word.page_setup.orientation.portrait"})
	if currentOrient == "landscape" {
		label = a.text(i18n.Message{ID: "word.page_setup.orientation.landscape"})
	}
	a.modal.ShowMessage("word.page_setup.orientation", i18n.Message{ID: "word.page_setup.orientation.dialog.title"}, i18n.Message{ID: "word.page_setup.orientation.dialog.body", Args: i18n.Args{"Orientation": label}}, func(action string) {
		a.modal.Hide()
		if action == "portrait" || action == "landscape" {
			a.runWordPageSetupCommand("修改页面方向", func(editor document.WordPageSetupEditor) error {
				return editor.SetPageOrientation(action)
			})
		}
	},
		modalDialogActionSpec{ID: "portrait", LabelRef: i18n.Message{ID: "word.page_setup.orientation.portrait"}},
		modalDialogActionSpec{ID: "landscape", LabelRef: i18n.Message{ID: "word.page_setup.orientation.landscape"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
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
	labels := []i18n.Message{{ID: "word.page_setup.margin.top"}, {ID: "word.page_setup.margin.right"}, {ID: "word.page_setup.margin.bottom"}, {ID: "word.page_setup.margin.left"}}
	hints := []i18n.Message{{ID: "word.page_setup.margin.default"}, {ID: "word.page_setup.margin.default"}, {ID: "word.page_setup.margin.default"}, {ID: "word.page_setup.margin.default"}}
	initial := []string{
		strconv.FormatFloat(topPT, 'f', -1, 64),
		strconv.FormatFloat(rightPT, 'f', -1, 64),
		strconv.FormatFloat(bottomPT, 'f', -1, 64),
		strconv.FormatFloat(leftPT, 'f', -1, 64),
	}
	a.modal.ShowFormMessage("word.page_setup.margins", i18n.Message{ID: "word.page_setup.margins.dialog.title"}, i18n.Message{ID: "word.page_setup.margins.dialog.body"},
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
					a.setStatusMessage(i18n.Message{ID: "word.status.inline.margins_must_be_values_from_211781"}, feedbackError, notificationToast)
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
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "common.action.apply"}},
		modalDialogActionSpec{ID: "normal", LabelRef: i18n.Message{ID: "word.page_setup.margins.preset.normal"}},
		modalDialogActionSpec{ID: "narrow", LabelRef: i18n.Message{ID: "word.page_setup.margins.preset.narrow"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
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
	labels := []i18n.Message{{ID: "word.page_setup.paper_size.width"}, {ID: "word.page_setup.paper_size.height"}}
	hints := []i18n.Message{{ID: "word.page_setup.paper_size.width_hint"}, {ID: "word.page_setup.paper_size.height_hint"}}
	initial := []string{
		strconv.FormatFloat(wPT, 'f', 1, 64),
		strconv.FormatFloat(hPT, 'f', 1, 64),
	}
	a.modal.ShowFormMessage("word.page_setup.paper_size", i18n.Message{ID: "word.page_setup.paper_size.dialog.title"}, i18n.Message{ID: "word.page_setup.paper_size.dialog.body"},
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
					a.setStatusMessage(i18n.Message{ID: "word.status.inline.paper_width_and_height_must_1e89e5"}, feedbackError, notificationToast)
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
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "common.action.apply"}},
		modalDialogActionSpec{ID: "a4", LabelRef: i18n.Message{ID: "word.page_setup.paper_size.a4"}},
		modalDialogActionSpec{ID: "letter", LabelRef: i18n.Message{ID: "word.page_setup.paper_size.letter"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
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
	a.modal.ShowMessage("word.page_setup.columns", i18n.Message{ID: "word.page_setup.columns.dialog.title"}, i18n.Message{ID: "word.page_setup.columns.dialog.body"}, func(action string) {
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
		a.runWordPageSetupCommand(a.text(i18n.Message{ID: "word.page_setup.columns.operation", Args: i18n.Args{"Columns": cols}}), func(editor document.WordPageSetupEditor) error {
			return editor.SetPageColumns(cols)
		})
	},
		modalDialogActionSpec{ID: "col1", LabelRef: i18n.Message{ID: "word.page_setup.columns.option.one"}},
		modalDialogActionSpec{ID: "col2", LabelRef: i18n.Message{ID: "word.page_setup.columns.option.two"}},
		modalDialogActionSpec{ID: "col3", LabelRef: i18n.Message{ID: "word.page_setup.columns.option.three"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
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
	a.modal.ShowMessage("word.page_setup.line_numbers", i18n.Message{ID: "word.page_setup.line_numbers.dialog.title"}, i18n.Message{ID: "word.page_setup.line_numbers.dialog.body"}, func(action string) {
		a.modal.Hide()
		var mode string
		var modeLabelID string
		switch action {
		case "continuous":
			mode = "continuous"
			modeLabelID = "word.page_setup.line_numbers.mode.continuous"
		case "newPage":
			mode = "newPage"
			modeLabelID = "word.page_setup.line_numbers.mode.new_page"
		case "none":
			mode = "none"
			modeLabelID = "word.page_setup.line_numbers.mode.none"
		default:
			return
		}
		a.runWordPageSetupCommand(a.text(i18n.Message{ID: "word.page_setup.line_numbers.operation", Args: i18n.Args{"Mode": a.text(i18n.Message{ID: modeLabelID})}}), func(editor document.WordPageSetupEditor) error {
			return editor.SetPageLineNumbers(mode)
		})
	},
		modalDialogActionSpec{ID: "continuous", LabelRef: i18n.Message{ID: "word.page_setup.line_numbers.mode.continuous"}},
		modalDialogActionSpec{ID: "newPage", LabelRef: i18n.Message{ID: "word.page_setup.line_numbers.mode.new_page"}},
		modalDialogActionSpec{ID: "none", LabelRef: i18n.Message{ID: "word.page_setup.line_numbers.mode.none"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
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
	a.modal.ShowMessage("word.page_setup.breaks", i18n.Message{ID: "word.page_setup.breaks.dialog.title"}, i18n.Message{ID: "word.page_setup.breaks.dialog.body"}, func(action string) {
		a.modal.Hide()
		switch action {
		case "page":
			a.queueWordBreak(document.WordBreakPage)
		case "column":
			a.queueWordBreak(document.WordBreakColumn)
		}
	},
		modalDialogActionSpec{ID: "page", LabelRef: i18n.Message{ID: "word.page_setup.breaks.page"}},
		modalDialogActionSpec{ID: "column", LabelRef: i18n.Message{ID: "word.page_setup.breaks.column"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
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
	a.launchManagedBackground(func() {
		blocks, err := runWordMutation(state, func() error { return edit(editor) })
		if err == nil {
			state.invalidateWordLayoutAfterBlocksChange()
		}
		a.publishWordCommand(wordCommandResult{state: state, generation: generation, operation: operation, err: err, blocks: blocks})
	})
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
	a.modal.ShowMessage("word.word_count", i18n.Message{ID: "word.word_count.dialog.title"}, i18n.Message{ID: "word.word_count.dialog.body", Args: i18n.Args{"WithoutWhitespace": statistics.charactersWithoutWhitespace, "WithWhitespace": statistics.charactersWithWhitespace, "Paragraphs": statistics.paragraphs, "Tables": statistics.tables}}, func(action string) {
		if action == "close" {
			a.modal.Hide()
		}
	}, modalDialogActionSpec{ID: "close", LabelRef: i18n.Message{ID: "common.action.close"}})
}
