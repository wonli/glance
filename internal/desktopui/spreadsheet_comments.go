package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/document"
	"glance/internal/format/xlsx"
	"glance/internal/i18n"
	"strings"
)

type spreadsheetCommentAddArguments struct {
	Sheet  string `json:"sheet"`
	Cell   string `json:"cell"`
	Author string `json:"author"`
	Text   string `json:"text"`
}

func (a *App) showSpreadsheetNewCommentDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	_, ok := state.book.(document.SpreadsheetCommentEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.this_workbook_does_not_support_3f4364"}, feedbackWarning, notificationToast)
		return
	}
	activeCol := state.selection.Active.Column + 1
	activeRow := state.selection.Active.Row + 1
	cellName := xlsx.CellName(activeCol, activeRow)
	if cellName == "" {
		cellName = fmt.Sprintf("%s%d", columnName(activeCol), activeRow)
	}

	a.modal.ShowInputMessage("spreadsheet.comment.add", i18n.Message{ID: "spreadsheet.comment.dialog.title"}, i18n.Message{ID: "spreadsheet.comment.dialog.body", Args: i18n.Args{"Cell": cellName}}, "", i18n.Message{ID: "spreadsheet.comment.dialog.hint"}, func(action string) {
		text := strings.TrimSpace(a.modal.Input.Text())
		a.modal.Hide()
		if action != "confirm" || text == "" {
			return
		}
		sheet := state.sheet
		comment := document.SpreadsheetComment{
			Author: "用户",
			Cell:   cellName,
			Text:   text,
		}
		a.runSpreadsheetCommentCommand("新建批注", "spreadsheet.comment.add", spreadsheetCommentAddArguments{Sheet: sheet, Cell: cellName, Author: comment.Author, Text: comment.Text}, func(ed document.SpreadsheetCommentEditor) error {
			return ed.AddComment(sheet, comment)
		})
	}, modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "common.action.confirm"}}, modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}})
}

type spreadsheetCommentDeleteArguments struct {
	Sheet string `json:"sheet"`
	Cell  string `json:"cell"`
}

func (a *App) deleteSpreadsheetComment() {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() {
		return
	}
	_, ok := state.book.(document.SpreadsheetCommentEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.this_workbook_does_not_support_3f4364"}, feedbackWarning, notificationToast)
		return
	}
	activeCol := state.selection.Active.Column + 1
	activeRow := state.selection.Active.Row + 1
	cellName := xlsx.CellName(activeCol, activeRow)
	if cellName == "" {
		cellName = fmt.Sprintf("%s%d", columnName(activeCol), activeRow)
	}

	sheet := state.sheet
	a.runSpreadsheetCommentCommand("删除批注", "spreadsheet.comment.delete", spreadsheetCommentDeleteArguments{Sheet: sheet, Cell: cellName}, func(ed document.SpreadsheetCommentEditor) error {
		return ed.DeleteComment(sheet, cellName)
	})
}

func (a *App) runSpreadsheetCommentCommand(operation, commandID string, arguments any, edit func(document.SpreadsheetCommentEditor) error) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	if _, ok := state.book.(document.SpreadsheetCommentEditor); !ok {
		return
	}
	sheet, generation, window := state.sheet, a.documentGeneration, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	a.launchManagedBackground(func() {
		commandRouted := a.applicationSessionRegistered(token.documentID)
		changed := true
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, commandID, "spreadsheet_cell", "selection", sheet, arguments)
			err = commandErr
			if err == nil {
				var outcome struct {
					Changed bool `json:"changed"`
				}
				err = json.Unmarshal(result.Data, &outcome)
				changed = outcome.Changed
			}
		} else {
			editor := state.book.(document.SpreadsheetCommentEditor)
			unlock := lockSessionAdapterWrite(state)
			err = edit(editor)
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		if err == nil {
			a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.operation_completed", Args: i18n.Args{"Operation": a.localizedSpreadsheetToolbarText(operation)}}, feedbackSuccess, notificationToast)
		}
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation, applied: changed}:
		case <-a.shutdown:
		}
	})
}
