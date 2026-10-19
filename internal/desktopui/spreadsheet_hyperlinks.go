package desktopui

import (
	"encoding/json"
	"glance/internal/document"
	"glance/internal/format/xlsx"
	"glance/internal/i18n"
	"strings"
)

func (a *App) runSpreadsheetHyperlinkCommand(operation, commandID, cell string, arguments any, edit func(document.SpreadsheetHyperlinkEditor) error) {
	state := a.documentSessionState
	if state == nil || state.book == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	editor, ok := state.book.(document.SpreadsheetHyperlinkEditor)
	if !ok {
		return
	}
	generation, sheet, window := a.documentGeneration, state.sheet, state.virtualGrid
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return
	}
	a.launchManagedBackground(func() {
		commandRouted := a.applicationSessionRegistered(token.documentID)
		changed := true
		var err error
		if commandRouted {
			result, commandErr := a.executeRibbonApplicationCommand(token, commandID, "spreadsheet_cell", cell, sheet, arguments)
			err = commandErr
			if err == nil {
				var outcome struct {
					Changed bool `json:"changed"`
				}
				err = json.Unmarshal(result.Data, &outcome)
				changed = outcome.Changed
			}
		} else {
			unlock := lockSessionAdapterWrite(state)
			err = edit(editor)
			unlock()
		}
		a.completeSessionOperation(state, token, err, err == nil && changed && !commandRouted)
		select {
		case a.formatResults <- spreadsheetFormatResult{state: state, token: token, revision: state.wordRevisionAtomic.Load(), generation: generation, sheet: sheet, window: window, err: err, operation: operation, applied: changed}:
		case <-a.shutdown:
		}
	})
}

type spreadsheetHyperlinkRemoveArguments struct {
	Sheet string `json:"sheet"`
	Cell  string `json:"cell"`
}

type spreadsheetHyperlinkSetArguments struct {
	Sheet   string `json:"sheet"`
	Cell    string `json:"cell"`
	Target  string `json:"target"`
	Tooltip string `json:"tooltip"`
}

func (a *App) showSpreadsheetLinkDialog() {
	state := a.documentSessionState
	if state == nil || state.book == nil {
		return
	}
	editor, ok := state.book.(document.SpreadsheetHyperlinkEditor)
	if !ok {
		return
	}
	col := state.selection.Active.Column + 1
	row := state.selection.Active.Row + 1
	cellName := xlsx.CellName(col, row)
	sheet := state.sheet

	existingLink, hasLink, _ := editor.GetCellHyperlink(sheet, cellName)
	initialTarget := existingLink.Target
	initialTooltip := existingLink.Tooltip

	actions := []modalDialogActionSpec{
		{ID: "confirm", LabelRef: i18n.Message{ID: "common.action.confirm"}},
	}
	if hasLink {
		actions = append(actions, modalDialogActionSpec{ID: "remove", LabelRef: i18n.Message{ID: "spreadsheet.hyperlink.action.remove"}})
	}
	actions = append(actions, modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}})

	a.modal.ShowDualInputMessage("spreadsheet.hyperlink", i18n.Message{ID: "spreadsheet.hyperlink.dialog.title"}, i18n.Message{ID: "spreadsheet.hyperlink.dialog.body", Args: i18n.Args{"Cell": cellName}},
		i18n.Message{ID: "spreadsheet.hyperlink.dialog.target_hint"}, i18n.Message{ID: "spreadsheet.hyperlink.dialog.text_hint"},
		initialTarget, initialTooltip,
		func(action string) {
			switch action {
			case "confirm":
				target := strings.TrimSpace(a.modal.Input.Text())
				tooltip := strings.TrimSpace(a.modal.SecondInput.Text())
				a.modal.Hide()
				if target == "" {
					a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.inline.the_link_address_cannot_be_578a8d"}, feedbackError, notificationToast)
					return
				}
				a.runSpreadsheetHyperlinkCommand("插入链接", "spreadsheet.hyperlink.set", cellName, spreadsheetHyperlinkSetArguments{Sheet: sheet, Cell: cellName, Target: target, Tooltip: tooltip}, func(ed document.SpreadsheetHyperlinkEditor) error {
					return ed.SetCellHyperlink(sheet, cellName, target, tooltip)
				})
				a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.hyperlink.added", Args: i18n.Args{"Cell": cellName, "Target": target}}, feedbackSuccess, notificationToast)
			case "remove":
				a.modal.Hide()
				a.runSpreadsheetHyperlinkCommand("删除链接", "spreadsheet.hyperlink.remove", cellName, spreadsheetHyperlinkRemoveArguments{Sheet: sheet, Cell: cellName}, func(ed document.SpreadsheetHyperlinkEditor) error {
					return ed.RemoveCellHyperlink(sheet, cellName)
				})
				a.setStatusMessage(i18n.Message{ID: "spreadsheet.status.hyperlink.removed", Args: i18n.Args{"Cell": cellName}}, feedbackSuccess, notificationToast)
			case "cancel":
				a.modal.Hide()
			}
		},
		actions...,
	)
}
