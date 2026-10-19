package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/i18n"

	"glance/internal/application"
)

type presentationTransitionArguments struct {
	SlideID        uint32 `json:"slide_id"`
	TransitionType string `json:"transition_type,omitempty"`
	Direction      string `json:"direction,omitempty"`
	Speed          string `json:"speed,omitempty"`
	ApplyToAll     bool   `json:"apply_to_all,omitempty"`
}

func (a *App) runPresentationTransitionAction(view *presentationViewState, actionID string) bool {
	if view == nil {
		return false
	}
	slide := view.currentSlide()
	if slide == nil {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_current_slide_was_not_found_2b6179"}, feedbackWarning, notificationToast)
		return true
	}
	switch actionID {
	case "trans_fade":
		a.startPresentationTransition(view, presentationTransitionArguments{TransitionType: "fade"})
	case "trans_push":
		a.startPresentationTransition(view, presentationTransitionArguments{TransitionType: "push", Direction: "l"})
	case "trans_options":
		a.modal.ShowMessage("presentation.transition.direction", i18n.Message{ID: "presentation.transition.direction.title"}, i18n.Message{ID: "presentation.transition.direction.body"}, func(action string) {
			a.modal.Hide()
			direction := map[string]string{"left": "l", "right": "r", "up": "u", "down": "d"}[action]
			if direction != "" {
				a.startPresentationTransition(view, presentationTransitionArguments{Direction: direction})
			}
		},
			modalDialogActionSpec{ID: "left", LabelRef: i18n.Message{ID: "presentation.transition.direction.left"}},
			modalDialogActionSpec{ID: "right", LabelRef: i18n.Message{ID: "presentation.transition.direction.right"}},
			modalDialogActionSpec{ID: "up", LabelRef: i18n.Message{ID: "presentation.transition.direction.up"}},
			modalDialogActionSpec{ID: "down", LabelRef: i18n.Message{ID: "presentation.transition.direction.down"}},
			modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
		)
	case "trans_duration":
		a.modal.ShowMessage("presentation.transition.speed", i18n.Message{ID: "presentation.transition.speed.title"}, i18n.Message{ID: "presentation.transition.speed.body"}, func(action string) {
			a.modal.Hide()
			if action == "slow" || action == "med" || action == "fast" {
				a.startPresentationTransition(view, presentationTransitionArguments{Speed: action})
			}
		},
			modalDialogActionSpec{ID: "slow", LabelRef: i18n.Message{ID: "presentation.transition.speed.slow"}},
			modalDialogActionSpec{ID: "med", LabelRef: i18n.Message{ID: "presentation.transition.speed.medium"}},
			modalDialogActionSpec{ID: "fast", LabelRef: i18n.Message{ID: "presentation.transition.speed.fast"}},
			modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
		)
	case "trans_apply_all":
		a.startPresentationTransition(view, presentationTransitionArguments{ApplyToAll: true})
	default:
		return false
	}
	return true
}

func (a *App) startPresentationTransition(view *presentationViewState, arguments presentationTransitionArguments) bool {
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
	arguments.SlideID = slide.ID
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.another_presentation_operation_is_in_progr_9641a9"}, feedbackWarning, notificationToast)
		return true
	}
	view.commandPending = true
	generation := a.documentGeneration
	window, shutdown := a.window, a.shutdown
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, "presentation.transition.set", "presentation_slide", fmt.Sprint(slide.ID), "", arguments)
		state.finishOperation(token)
		out := presentationCommandResult{state: state, view: view, generation: generation, transition: true, err: err}
		if err == nil {
			var data struct {
				Changed          bool     `json:"changed"`
				AffectedSlideIDs []uint32 `json:"affected_slide_ids"`
			}
			if json.Unmarshal(result.Data, &data) != nil {
				out.err = application.ErrInvalidCommandResult
			} else {
				out.transitionChanged = data.Changed
				out.transitionAffected = len(data.AffectedSlideIDs)
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
