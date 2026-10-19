package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/i18n"

	"glance/internal/application"
)

func (a *App) showPresentationBackgroundOptions(view *presentationViewState) {
	if view == nil || view.currentSlide() == nil {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_current_slide_was_not_found_2b6179"}, feedbackWarning, notificationToast)
		return
	}
	a.modal.ShowMessage("presentation.background", i18n.Message{ID: "presentation.background.title"}, i18n.Message{ID: "presentation.background.body"}, func(action string) {
		a.modal.Hide()
		colorHex := map[string]string{
			"white":      "#FFFFFF",
			"light-gray": "#F2F2F2",
			"light-blue": "#EAF2F8",
			"navy":       "#17365D",
		}[action]
		if colorHex == "" {
			return
		}
		a.startPresentationBackgroundSet(view, colorHex)
	},
		modalDialogActionSpec{ID: "white", LabelRef: i18n.Message{ID: "presentation.background.white"}},
		modalDialogActionSpec{ID: "light-gray", LabelRef: i18n.Message{ID: "presentation.background.light_gray"}},
		modalDialogActionSpec{ID: "light-blue", LabelRef: i18n.Message{ID: "presentation.background.light_blue"}},
		modalDialogActionSpec{ID: "navy", LabelRef: i18n.Message{ID: "presentation.background.navy"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

type presentationSlideBackgroundSetArguments struct {
	SlideId  uint32 `json:"slide_id"`
	ColorHex string `json:"color_hex"`
}

func (a *App) startPresentationBackgroundSet(view *presentationViewState, colorHex string) bool {
	state := a.documentSessionState
	if view == nil || state == nil || state.pptxDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.presentation_capabilities_are_not_connecte_811cbe"}, feedbackInfo, notificationToast)
		return true
	}
	if view.commandPending {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.another_presentation_operation_is_in_progr_9641a9"}, feedbackWarning, notificationToast)
		return true
	}
	if err := a.flushPresentationNotesDraft(view); err != nil {
		return true
	}
	if err := a.flushPresentationTextDraft(); err != nil {
		return true
	}
	slide := view.currentSlide()
	if slide == nil {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_current_slide_was_not_found_2b6179"}, feedbackWarning, notificationToast)
		return true
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.another_presentation_operation_is_in_progr_9641a9"}, feedbackWarning, notificationToast)
		return true
	}
	view.commandPending = true
	generation := a.documentGeneration
	window, shutdown := a.window, a.shutdown
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, "presentation.slide.background.set", "presentation_slide", fmt.Sprint(slide.ID), "", presentationSlideBackgroundSetArguments{SlideId: slide.ID, ColorHex: colorHex})
		state.finishOperation(token)
		out := presentationCommandResult{state: state, view: view, generation: generation, background: true, err: err}
		if err == nil {
			var data struct {
				Changed  bool   `json:"changed"`
				SlideID  uint32 `json:"slide_id"`
				ColorHex string `json:"color_hex"`
			}
			if json.Unmarshal(result.Data, &data) != nil || data.SlideID != slide.ID || data.ColorHex != colorHex {
				out.err = application.ErrInvalidCommandResult
			} else {
				out.backgroundChanged = data.Changed
			}
		}
		select {
		case view.commandResults <- out:
		case <-shutdown:
		}
		if window != nil {
			window.Invalidate()
		}
	})
	return true
}
