package desktopui

import (
	"fmt"
	"glance/internal/document"
	"glance/internal/i18n"
	"strings"
)

type wordEquationArguments struct {
	Target           string `json:"target"`
	RunID            string `json:"run_id,omitempty"`
	AfterParagraphID string `json:"after_paragraph_id,omitempty"`
	Expression       string `json:"expression"`
}

func (a *App) showWordWordArtDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordTextBoxEditor); !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_current_document_does_not_06e28a"}, feedbackWarning, notificationToast)
		return
	}
	a.modal.ShowInputMessage("word.wordart", i18n.Message{ID: "word.wordart.dialog.title"}, i18n.Message{ID: "word.wordart.dialog.body"}, a.text(i18n.Message{ID: "word.wordart.default_text"}), i18n.Message{ID: "word.wordart.dialog.hint"},
		func(action string) {
			a.modal.Hide()
			if action == "cancel" {
				return
			}
			text := strings.TrimSpace(a.modal.Input.Text())
			if text == "" {
				text = a.text(i18n.Message{ID: "word.wordart.default_text"})
			}
			var stylePrefix string
			switch action {
			case "style_gold":
				stylePrefix = a.text(i18n.Message{ID: "word.wordart.content_prefix.gold"})
			case "style_neon":
				stylePrefix = a.text(i18n.Message{ID: "word.wordart.content_prefix.neon"})
			case "style_hollow":
				stylePrefix = a.text(i18n.Message{ID: "word.wordart.content_prefix.hollow"})
			default:
				stylePrefix = a.text(i18n.Message{ID: "word.wordart.content_prefix.default"})
			}
			afterPID := state.selectedWordParagraphID
			content := fmt.Sprintf("%s %s", stylePrefix, text)
			if !a.runWordApplicationCommand("插入艺术字", "word.text_box.insert", wordTextBoxInsertArguments{AfterParagraphId: afterPID, Text: content}) {
				a.setStatusMessage(i18n.Message{ID: "word.status.inline.this_session_cannot_insert_wordart_797df7"}, feedbackWarning, notificationToast)
				return
			}
		},
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "word.wordart.style.default"}},
		modalDialogActionSpec{ID: "style_gold", LabelRef: i18n.Message{ID: "word.wordart.style.gold"}},
		modalDialogActionSpec{ID: "style_neon", LabelRef: i18n.Message{ID: "word.wordart.style.neon"}},
		modalDialogActionSpec{ID: "style_hollow", LabelRef: i18n.Message{ID: "word.wordart.style.hollow"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

func (a *App) showWordEquationDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	a.modal.ShowInputMessage("word.equation", i18n.Message{ID: "word.equation.dialog.title"}, i18n.Message{ID: "word.equation.dialog.body"},
		"x = (-b ± √(b² - 4ac)) / (2a)",
		i18n.Message{ID: "word.equation.dialog.hint"},
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
			var arguments wordEquationArguments
			if hasRun && run.Editable {
				expression := formula
				if run.Text != "" {
					expression = run.Text + " " + formula
				}
				arguments = wordEquationArguments{Target: "run", RunID: run.ID, Expression: expression}
			} else if _, hasTextBox := any(state.word).(document.WordTextBoxEditor); hasTextBox {
				arguments = wordEquationArguments{Target: "textbox", AfterParagraphID: state.selectedWordParagraphID, Expression: a.text(i18n.Message{ID: "word.equation.content_prefix"}) + formula}
			} else {
				a.setStatusMessage(i18n.Message{ID: "word.status.inline.select_an_editable_text_paragraph_5d5852"}, feedbackWarning, notificationToast)
				return
			}
			if !a.runWordApplicationCommand("插入公式文本", "word.equation.insert", arguments) {
				a.setStatusMessage(i18n.Message{ID: "word.status.inline.this_session_cannot_insert_equation_1e8d06"}, feedbackWarning, notificationToast)
			}
		},
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "word.equation.action.insert"}},
		modalDialogActionSpec{ID: "quadratic", LabelRef: i18n.Message{ID: "word.equation.preset.quadratic"}},
		modalDialogActionSpec{ID: "euler", LabelRef: i18n.Message{ID: "word.equation.preset.euler"}},
		modalDialogActionSpec{ID: "pythagorean", LabelRef: i18n.Message{ID: "word.equation.preset.pythagorean"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

type wordTextBoxInsertArguments struct {
	AfterParagraphId string `json:"after_paragraph_id"`
	Text             string `json:"text"`
}

func (a *App) showWordShapesDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if _, ok := any(state.word).(document.WordTextBoxEditor); !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_current_document_does_not_45df2a"}, feedbackWarning, notificationToast)
		return
	}
	a.modal.ShowInputMessage("word.shapes", i18n.Message{ID: "word.shapes.dialog.title"}, i18n.Message{ID: "word.shapes.dialog.body"}, a.text(i18n.Message{ID: "word.shapes.default_text"}), i18n.Message{ID: "word.shapes.dialog.hint"},
		func(action string) {
			a.modal.Hide()
			if action == "cancel" {
				return
			}
			text := strings.TrimSpace(a.modal.Input.Text())
			if text == "" {
				text = a.text(i18n.Message{ID: "word.shapes.default_text"})
			}
			var shapeType string
			switch action {
			case "rect":
				shapeType = a.text(i18n.Message{ID: "word.shapes.type.rectangle"})
			case "round_rect":
				shapeType = a.text(i18n.Message{ID: "word.shapes.type.round_rectangle"})
			case "callout":
				shapeType = a.text(i18n.Message{ID: "word.shapes.type.callout"})
			case "flowchart":
				shapeType = a.text(i18n.Message{ID: "word.shapes.type.flowchart"})
			default:
				shapeType = a.text(i18n.Message{ID: "word.shapes.type.generic"})
			}
			afterPID := state.selectedWordParagraphID
			content := a.text(i18n.Message{ID: "word.shapes.content_prefix", Args: i18n.Args{"Type": shapeType}}) + text
			if !a.runWordApplicationCommand("插入形状", "word.text_box.insert", wordTextBoxInsertArguments{AfterParagraphId: afterPID, Text: content}) {
				a.setStatusMessage(i18n.Message{ID: "word.status.inline.this_session_cannot_insert_a_8e33fd"}, feedbackWarning, notificationToast)
				return
			}
		},
		modalDialogActionSpec{ID: "rect", LabelRef: i18n.Message{ID: "word.shapes.type.rectangle"}},
		modalDialogActionSpec{ID: "round_rect", LabelRef: i18n.Message{ID: "word.shapes.type.round_rectangle"}},
		modalDialogActionSpec{ID: "callout", LabelRef: i18n.Message{ID: "word.shapes.type.callout"}},
		modalDialogActionSpec{ID: "flowchart", LabelRef: i18n.Message{ID: "word.shapes.type.flowchart"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

type wordTableInsertArguments struct {
	AfterParagraphId string `json:"after_paragraph_id"`
	Rows             int    `json:"rows"`
	Columns          int    `json:"columns"`
}

func (a *App) showWordChartDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	_, ok := any(state.word).(document.WordTableInsertEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_current_document_does_not_1488fb"}, feedbackWarning, notificationToast)
		return
	}
	a.modal.ShowMessage("word.chart.insert", i18n.Message{ID: "word.chart.dialog.title"}, i18n.Message{ID: "word.chart.dialog.body"},
		func(action string) {
			a.modal.Hide()
			var rows, cols int
			switch action {
			case "column":
				rows, cols = 3, 3
			case "line":
				rows, cols = 4, 3
			case "pie":
				rows, cols = 3, 2
			default:
				return
			}
			if !a.runWordApplicationCommand("插入图表基础数据表", "word.table.insert", wordTableInsertArguments{AfterParagraphId: state.selectedWordParagraphID, Rows: rows, Columns: cols}) {
				a.setStatusMessage(i18n.Message{ID: "word.status.inline.this_session_cannot_insert_chart_794979"}, feedbackWarning, notificationToast)
				return
			}
			a.setStatusMessage(i18n.Message{ID: "word.insert.chart.status.inserting", Args: i18n.Args{"Rows": rows, "Columns": cols}}, feedbackInfo, notificationSuppressed)
		},
		modalDialogActionSpec{ID: "column", LabelRef: i18n.Message{ID: "word.chart.type.column"}},
		modalDialogActionSpec{ID: "line", LabelRef: i18n.Message{ID: "word.chart.type.line"}},
		modalDialogActionSpec{ID: "pie", LabelRef: i18n.Message{ID: "word.chart.type.pie"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}
