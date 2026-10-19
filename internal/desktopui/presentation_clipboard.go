package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/i18n"
	"io"
	"strings"
	"unicode/utf8"

	"gioui.org/io/clipboard"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"glance/internal/application"
	"glance/internal/document"
)

const presentationClipboardMaxRunes = 32767

func presentationTextElement(v *presentationViewState) (uint32, document.PresentationElement, bool) {
	if v == nil || v.pictureCrop.active {
		return 0, document.PresentationElement{}, false
	}
	slide := v.currentSlide()
	if slide == nil {
		return 0, document.PresentationElement{}, false
	}
	_, element, ok := presentationSelectedStyleElement(v)
	if !ok || !element.CanEditTextStyle || (element.Kind != document.ElementKindText && element.Kind != document.ElementKindShape) {
		return 0, document.PresentationElement{}, false
	}
	return slide.ID, element, true
}

func (a *App) handlePresentationClipboardEvents(gtx layout.Context, v *presentationViewState) {
	if v == nil {
		return
	}
	if v.clipboardWritePending {
		gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(v.clipboardWriteText))})
		v.clipboardWritePending = false
		v.clipboardWriteText = ""
	}
	for {
		ev, ok := gtx.Event(transfer.TargetFilter{Target: &v.clipboardTarget, Type: "application/text"})
		if !ok {
			break
		}
		if _, cancelled := ev.(transfer.CancelEvent); cancelled {
			v.clipboardReadPending = false
			v.clipboardDocumentID = ""
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.paste_canceled_296266"}, feedbackInfo, notificationSuppressed)
			continue
		}
		data, ok := ev.(transfer.DataEvent)
		if !ok {
			continue
		}
		reader := data.Open()
		value, err := io.ReadAll(io.LimitReader(reader, 4*presentationClipboardMaxRunes+1))
		_ = reader.Close()
		v.clipboardReadPending = false
		clipboardDocumentID := v.clipboardDocumentID
		clipboardDocumentVersion := v.clipboardDocumentVersion
		pasteSlideID, pasteElementID := v.clipboardPasteSlideID, v.clipboardPasteElementID
		v.clipboardDocumentID = ""
		if err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "presentation.error.operation_failed"}, err, notificationToast)
			continue
		}
		if clipboardDocumentID == "" || a.documentSessionState == nil || a.documentSessionState.documentID != clipboardDocumentID || a.documentGeneration != clipboardDocumentVersion {
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_document_changed_paste_canceled_df1442"}, feedbackInfo, notificationSuppressed)
			continue
		}
		text := string(value)
		if !utf8.Valid(value) || len(value) > 4*presentationClipboardMaxRunes || len([]rune(text)) > presentationClipboardMaxRunes {
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.paste_failed_clipboard_text_exceeds_32_767_43b585"}, feedbackError, notificationToast)
			continue
		}
		if strings.ContainsAny(text, "\r\n") {
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.paste_failed_this_text_element_supports_on_cd5ed4"}, feedbackError, notificationToast)
			continue
		}
		if text == "" {
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.there_is_no_text_to_paste_from_the_clipboa_bf1827"}, feedbackInfo, notificationSuppressed)
			continue
		}
		a.startPresentationTextPaste(v, pasteSlideID, pasteElementID, text)
	}
}

type presentationElementTextCopyArguments struct {
	SlideId   uint32 `json:"slide_id"`
	ElementId int64  `json:"element_id"`
}

func (a *App) startPresentationTextCopy(v *presentationViewState) bool {
	state := a.documentSessionState
	if v == nil || state == nil || state.pptxDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	if v.commandPending {
		return true
	}
	if err := a.flushPresentationTextDraft(); err != nil {
		return true
	}
	slideID, element, ok := presentationTextElement(v)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.select_an_editable_text_box_or_a_shape_wit_de0ce6"}, feedbackWarning, notificationSuppressed)
		return true
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.another_presentation_operation_is_in_progr_9641a9"}, feedbackWarning, notificationToast)
		return true
	}
	v.commandPending = true
	generation := a.documentGeneration
	window, shutdown := a.window, a.shutdown
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, "presentation.element.text.copy", "presentation_element", fmt.Sprint(element.ID), "", presentationElementTextCopyArguments{SlideId: slideID, ElementId: element.ID})
		state.finishOperation(token)
		out := presentationCommandResult{state: state, view: v, generation: generation, clipboardCopy: true, err: err}
		if err == nil {
			var data struct {
				SlideID   uint32 `json:"slide_id"`
				ElementID int64  `json:"element_id"`
				Text      string `json:"text"`
			}
			if json.Unmarshal(result.Data, &data) != nil || data.SlideID != slideID || data.ElementID != element.ID {
				out.err = application.ErrInvalidCommandResult
			} else {
				out.clipboardText = data.Text
				out.clipboardElementID = element.ID
			}
		}
		select {
		case v.commandResults <- out:
		case <-shutdown:
		}
		if window != nil {
			window.Invalidate()
		}
	})
	return true
}

func (a *App) startPresentationClipboardPaste(gtx layout.Context, v *presentationViewState) bool {
	state := a.documentSessionState
	if v == nil || state == nil || state.pptxDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	if v.commandPending || v.clipboardReadPending {
		return true
	}
	slideID, element, ok := presentationTextElement(v)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.select_an_editable_text_box_or_a_shape_wit_de0ce6"}, feedbackWarning, notificationSuppressed)
		return true
	}
	if err := a.flushPresentationTextDraft(); err != nil {
		return true
	}
	v.clipboardDocumentID = state.documentID
	v.clipboardDocumentVersion = a.documentGeneration
	v.clipboardPasteSlideID = slideID
	v.clipboardPasteElementID = element.ID
	v.clipboardReadPending = true
	gtx.Execute(clipboard.ReadCmd{Tag: &v.clipboardTarget})
	a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.reading_text_from_the_clipboard_8fb421"}, feedbackInfo, notificationSuppressed)
	return true
}

func (a *App) startPresentationTextPaste(v *presentationViewState, slideID uint32, elementID int64, text string) bool {
	state := a.documentSessionState
	if v == nil || state == nil || state.pptxDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	if v.commandPending {
		return true
	}
	if err := a.flushPresentationTextDraft(); err != nil {
		return true
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.another_presentation_operation_is_in_progr_9641a9"}, feedbackWarning, notificationToast)
		return true
	}
	v.commandPending = true
	generation := a.documentGeneration
	window, shutdown := a.window, a.shutdown
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, "presentation.element.text.set", "presentation_element", fmt.Sprint(elementID), "", presentationElementTextSetArguments{SlideId: slideID, ElementId: elementID, Text: text})
		state.finishOperation(token)
		out := presentationCommandResult{state: state, view: v, generation: generation, clipboardPaste: true, clipboardText: text, clipboardElementID: elementID, err: err}
		if err == nil {
			var data struct {
				SlideID   uint32 `json:"slide_id"`
				ElementID int64  `json:"element_id"`
				Changed   bool   `json:"changed"`
			}
			if json.Unmarshal(result.Data, &data) != nil || data.SlideID != slideID || data.ElementID != elementID {
				out.err = application.ErrInvalidCommandResult
			} else {
				out.clipboardChanged = data.Changed
			}
		}
		select {
		case v.commandResults <- out:
		case <-shutdown:
		}
		if window != nil {
			window.Invalidate()
		}
	})
	return true
}
