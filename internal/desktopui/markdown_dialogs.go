package desktopui

import (
	"glance/internal/format/markdowndoc"
	"glance/internal/i18n"
)

func (a *App) showMarkdownInsertDialog(kind string) {
	v := a.ensureMarkdownView()
	v.dialogEpoch++
	epoch := v.dialogEpoch
	if markdownEditorComposing(&v.editor) {
		a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.finish_chinese_input_composition_first_b4284e"}, feedbackWarning, notificationSuppressed)
		return
	}
	state := a.documentSessionState
	source := v.text()
	start, end := v.editor.Selection()
	if start > end {
		start, end = end, start
	}
	selected := string([]rune(source)[start:end])
	labels, hints, initial := []i18n.Message{{ID: "markdown.insert.link.label"}, {ID: "markdown.insert.link.destination"}}, []i18n.Message{{ID: "markdown.insert.link.label_hint"}, {ID: "markdown.insert.link.destination_hint"}}, []string{selected, ""}
	title := i18n.Message{ID: "markdown.insert.link.title"}
	dialogID := "markdown.insert.link"
	actions := []modalDialogActionSpec{{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}}, {ID: "insert", LabelRef: i18n.Message{ID: "common.action.insert"}}}
	if kind == "image" {
		title, dialogID = i18n.Message{ID: "markdown.insert.image.title"}, "markdown.insert.image"
		labels = []i18n.Message{{ID: "markdown.insert.image.alt_text"}, {ID: "markdown.insert.image.path"}}
		hints[1] = i18n.Message{ID: "markdown.insert.image.path_hint"}
		actions = []modalDialogActionSpec{{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}}, {ID: "browse", LabelRef: i18n.Message{ID: "markdown.insert.image.action.browse"}}, {ID: "insert", LabelRef: i18n.Message{ID: "common.action.insert"}}}
	} else if kind == "math" {
		title, dialogID = i18n.Message{ID: "markdown.insert.math.title"}, "markdown.insert.math"
		labels = []i18n.Message{{ID: "markdown.insert.math.label"}}
		hints = []i18n.Message{{ID: "markdown.insert.math.hint"}}
		initial = []string{selected}
		actions = []modalDialogActionSpec{{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}}, {ID: "inline", LabelRef: i18n.Message{ID: "markdown.insert.math.action.inline"}}, {ID: "block", LabelRef: i18n.Message{ID: "markdown.insert.math.action.block"}}}
	}
	a.modal.ShowFormMessage(dialogID, title, i18n.Message{}, labels, hints, initial, func(id string) {
		if id == "browse" {
			if v.imagePickPending {
				return
			}
			v.imagePickPending = true
			choose := a.wordImageFilePicker
			if choose == nil {
				pickerLabels := a.filePickerLabels()
				choose = func() (string, error) { return chooseOfficeImageFile(pickerLabels) }
			}
			window, shutdown := a.window, a.shutdown
			a.launchManagedBackground(func() {
				path, err := choose()
				result := markdownImagePickResult{path: path, err: err, epoch: epoch}
				select {
				case v.imagePicks <- result:
					if window != nil {
						window.Invalidate()
					}
				case <-shutdown:
				}
			})
			return
		}
		if id == "cancel" {
			a.modal.Hide()
			return
		}
		if a.documentSessionState != state || v.text() != source {
			a.modal.Hide()
			a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.the_document_content_changed_the_insertion_258927"}, feedbackInfo, notificationSuppressed)
			return
		}
		args := markdowndoc.EditArgs{Label: a.modal.FormInputs[0].Text(), Display: id == "block"}
		if kind == "math" {
			if _, err := markdowndoc.RenderMath(args.Label, args.Display); err != nil {
				a.setStatusErrorMessage(i18n.Message{ID: "markdown.error.operation_failed"}, err, notificationToast)
				return
			}
			v.editor.SetCaret(start, end)
			out, err := markdowndoc.ApplyEdit(source, "math", start, end, args)
			if err != nil {
				a.setStatusErrorMessage(i18n.Message{ID: "markdown.error.operation_failed"}, err, notificationToast)
				return
			} // Formula text replaces the retained selection before wrapping.
			_ = out
			v.editor.Insert(args.Label)
			v.editor.SetCaret(start, start+len([]rune(args.Label)))
			args.Label = ""
		} else {
			args.Destination = a.modal.FormInputs[1].Text()
			if args.Destination == "" {
				a.setStatusMessage(i18n.Message{ID: "desktop.status.inline.enter_the_destination_address_ebc866"}, feedbackInfo, notificationSuppressed)
				return
			}
			v.editor.SetCaret(start, end)
		}
		a.modal.Hide()
		if err := a.applyMarkdownEdit(kind, args); err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "markdown.error.operation_failed"}, err, notificationToast)
		}
	}, actions...)
}

func (a *App) pollMarkdownImagePicker() {
	v := a.ensureMarkdownView()
	select {
	case result := <-v.imagePicks:
		v.imagePickPending = false
		if !a.modal.Visible || result.epoch != v.dialogEpoch || a.modal.FormInputCount != 2 {
			return
		}
		if result.err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "markdown.error.operation_failed"}, result.err, notificationToast)
			return
		}
		if result.path != "" {
			a.modal.FormInputs[1].SetText(result.path)
		}
	default:
	}
}
