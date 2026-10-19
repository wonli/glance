package desktopui

import (
	"glance/internal/document"
	"glance/internal/i18n"
	"strings"
)

func (a *App) showWordPageColorDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordPageBackgroundEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_current_document_does_not_36eb4b"}, feedbackWarning, notificationToast)
		return
	}
	currentColor, _ := editor.GetPageBackgroundColor()
	body := i18n.Message{ID: "word.page_style.color.dialog.body.empty"}
	if currentColor != "" {
		body = i18n.Message{ID: "word.page_style.color.dialog.body.current", Args: i18n.Args{"Color": currentColor}}
	}

	a.modal.ShowInputMessage("word.page_style.color", i18n.Message{ID: "word.page_style.color.dialog.title"}, body, currentColor, i18n.Message{ID: "word.page_style.color.dialog.hint"}, func(action string) {
		a.modal.Hide()
		switch action {
		case "confirm":
			val := strings.TrimSpace(a.modal.Input.Text())
			a.applyWordPageColor(val)
		case "white":
			a.applyWordPageColor("")
		case "parchment":
			a.applyWordPageColor("FAF8F5")
		case "green":
			a.applyWordPageColor("E8F5E9")
		case "blue":
			a.applyWordPageColor("E3F2FD")
		case "dark":
			a.applyWordPageColor("263238")
		}
	},
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "word.page_style.color.action.custom"}},
		modalDialogActionSpec{ID: "parchment", LabelRef: i18n.Message{ID: "word.page_style.color.action.parchment"}},
		modalDialogActionSpec{ID: "green", LabelRef: i18n.Message{ID: "word.page_style.color.action.green"}},
		modalDialogActionSpec{ID: "white", LabelRef: i18n.Message{ID: "word.page_style.color.action.white"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

func (a *App) applyWordPageColor(hexColor string) {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordPageBackgroundEditor)
	if !ok {
		return
	}
	clean := strings.TrimPrefix(strings.TrimSpace(hexColor), "#")
	if err := editor.SetPageBackgroundColor(clean); err != nil {
		a.setWordCommandStatus(err)
		return
	}
	if clean == "" || strings.EqualFold(clean, "none") {
		state.wordBackgroundColor = nil
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.cleared_the_page_background_color_09aa9a"}, feedbackSuccess, notificationToast)
	} else {
		c, err := parseHexColor(clean)
		if err == nil {
			state.wordBackgroundColor = &c
		}
		a.setStatusMessage(i18n.Message{ID: "word.page_style.status.color_set", Args: i18n.Args{"Color": strings.ToUpper(clean)}}, feedbackSuccess, notificationToast)
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) showWordPageBorderDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	editor, ok := any(state.word).(document.WordPageSetupEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_current_document_does_not_813cec"}, feedbackWarning, notificationToast)
		return
	}
	a.modal.ShowInputMessage("word.page_style.border", i18n.Message{ID: "word.page_style.border.dialog.title"}, i18n.Message{ID: "word.page_style.border.dialog.body"}, "box", i18n.Message{ID: "word.page_style.border.dialog.hint"}, func(action string) {
		a.modal.Hide()
		var borderType string
		switch action {
		case "confirm":
			borderType = strings.TrimSpace(a.modal.Input.Text())
		case "box":
			borderType = "box"
		case "double":
			borderType = "double"
		case "dashed":
			borderType = "dashed"
		case "none":
			borderType = "none"
		default:
			return
		}
		if err := editor.SetPageBorders(borderType); err != nil {
			a.setWordCommandStatus(err)
			return
		}
		if borderType == "none" || borderType == "" {
			a.setStatusMessage(i18n.Message{ID: "word.status.inline.cleared_the_page_border_0ffa07"}, feedbackSuccess, notificationToast)
		} else {
			a.setStatusMessage(i18n.Message{ID: "word.page_style.status.border_set"}, feedbackSuccess, notificationToast)
		}
		if a.window != nil {
			a.window.Invalidate()
		}
	},
		modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "word.page_style.border.action.custom"}},
		modalDialogActionSpec{ID: "box", LabelRef: i18n.Message{ID: "word.page_style.border.action.box"}},
		modalDialogActionSpec{ID: "double", LabelRef: i18n.Message{ID: "word.page_style.border.action.double"}},
		modalDialogActionSpec{ID: "dashed", LabelRef: i18n.Message{ID: "word.page_style.border.action.dashed"}},
		modalDialogActionSpec{ID: "none", LabelRef: i18n.Message{ID: "word.page_style.border.action.none"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}
