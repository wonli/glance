package desktopui

import (
	"glance/internal/document"
	"glance/internal/i18n"
	"strconv"
	"strings"
)

func (a *App) showWordInsertTableDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordTableInsertEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_current_document_does_not_53740a"}, feedbackWarning, notificationToast)
		return
	}
	a.modal.ShowInputMessage("word.table.insert", i18n.Message{ID: "word.table.insert.dialog.title"}, i18n.Message{ID: "word.table.insert.dialog.body"}, "3x3", i18n.Message{ID: "word.table.insert.dialog.hint"}, func(action string) {
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
				a.setStatusMessage(i18n.Message{ID: "word.status.inline.invalid_table_size_enter_a_58c295"}, feedbackError, notificationToast)
				return
			}
			r, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
			c, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err1 != nil || err2 != nil || r < 1 || c < 1 || r > 100 || c > 50 {
				a.setStatusMessage(i18n.Message{ID: "word.status.inline.rows_must_be_1_100_fb5aa8"}, feedbackInfo, notificationSuppressed)
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
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "word.table.insert.action.custom"}},
		modalDialogActionSpec{ID: "3x3", LabelRef: i18n.Message{ID: "word.table.insert.action.preset", Args: i18n.Args{"Rows": 3, "Columns": 3}}},
		modalDialogActionSpec{ID: "4x4", LabelRef: i18n.Message{ID: "word.table.insert.action.preset", Args: i18n.Args{"Rows": 4, "Columns": 4}}},
		modalDialogActionSpec{ID: "5x3", LabelRef: i18n.Message{ID: "word.table.insert.action.preset", Args: i18n.Args{"Rows": 5, "Columns": 3}}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
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
		a.setWordCommandStatus(err)
		return
	}
	if state.word != nil {
		state.setWordBlocks(state.word.WordBlocks())
		a.ensureDesktopRuntime().publishSessionRevision(state.documentID, state.wordRevisionAtomic.Load())
	}
	a.setStatusMessage(i18n.Message{ID: "word.table.status.inserted", Args: i18n.Args{"Rows": rows, "Columns": cols}}, feedbackSuccess, notificationToast)
	_ = tableID
	if a.window != nil {
		a.window.Invalidate()
	}
}
