package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/i18n"

	"glance/internal/application"
	"glance/internal/document"
)

func (a *App) showPresentationAnimationAddOptions(view *presentationViewState) {
	if view == nil {
		return
	}
	a.modal.ShowMessage("presentation.animation.add", i18n.Message{ID: "presentation.animation.add.title"}, i18n.Message{ID: "presentation.animation.add.body"}, func(action string) {
		a.modal.Hide()
		effect := ""
		switch action {
		case "appear":
			effect = "appear"
		case "fade":
			effect = "fade"
		case "fly_in":
			effect = "fly_in"
		default:
			return
		}
		if !a.startPresentationElementAnimation(view, effect, 500) {
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_start_the_animation_operation_04cc99"}, feedbackWarning, notificationToast)
		}
	},
		modalDialogActionSpec{ID: "appear", LabelRef: i18n.Message{ID: "presentation.animation.effect.appear"}},
		modalDialogActionSpec{ID: "fade", LabelRef: i18n.Message{ID: "presentation.animation.effect.fade_in"}},
		modalDialogActionSpec{ID: "fly_in", LabelRef: i18n.Message{ID: "presentation.animation.effect.fly_in"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

type startPresentationElementAnimationPayload struct {
	SlideID    uint32 `json:"slide_id"`
	ElementID  int64  `json:"element_id"`
	Effect     string `json:"effect"`
	DurationMS int    `json:"duration_ms"`
}

func (a *App) startPresentationElementAnimation(view *presentationViewState, effect string, durationMS int) bool {
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
	elementID := view.selectedShape
	if elementID <= 0 {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.select_a_slide_element_to_animate_first_2c75c1"}, feedbackWarning, notificationSuppressed)
		return true
	}
	targetFound := false
	for _, element := range slide.Elements {
		if element.ID != elementID {
			continue
		}
		if element.Kind != document.ElementKindText && element.Kind != document.ElementKindShape && element.Kind != document.ElementKindPicture {
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.animations_are_not_supported_for_the_selec_739e47"}, feedbackWarning, notificationToast)
			return true
		}
		targetFound = true
		break
	}
	if !targetFound {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_selected_element_is_no_longer_on_the_c_2aeb64"}, feedbackInfo, notificationToast)
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
	arguments := startPresentationElementAnimationPayload{SlideID: slide.ID, ElementID: elementID, Effect: effect, DurationMS: durationMS}
	a.launchManagedBackground(func() {
		command, err := a.executeRibbonApplicationCommand(token, "presentation.element.animation.add", "presentation_element", fmt.Sprint(elementID), "", arguments)
		state.finishOperation(token)
		out := presentationCommandResult{state: state, view: view, generation: generation, presentationAnimation: true, presentationAnimationEffect: effect, err: err}
		if err == nil {
			var data struct {
				Changed   bool   `json:"changed"`
				SlideID   uint32 `json:"slide_id"`
				ElementID int64  `json:"element_id"`
				Effect    string `json:"effect"`
			}
			if json.Unmarshal(command.Data, &data) != nil || data.SlideID != slide.ID || data.ElementID != elementID || data.Effect != effect {
				out.err = application.ErrInvalidCommandResult
			} else {
				out.presentationAnimationChanged = data.Changed
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

func (a *App) showPresentationAnimationDurationOptions(view *presentationViewState) {
	if view == nil {
		return
	}
	a.modal.ShowMessage("presentation.animation.duration", i18n.Message{ID: "presentation.animation.duration.title"}, i18n.Message{ID: "presentation.animation.duration.body"}, func(action string) {
		a.modal.Hide()
		durationMS := 0
		switch action {
		case "250":
			durationMS = 250
		case "500":
			durationMS = 500
		case "1000":
			durationMS = 1000
		case "2000":
			durationMS = 2000
		default:
			return
		}
		if !a.startPresentationAnimationDuration(view, durationMS) {
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.could_not_start_the_animation_duration_ope_583d40"}, feedbackWarning, notificationToast)
		}
	},
		modalDialogActionSpec{ID: "250", LabelRef: i18n.Message{ID: "presentation.animation.duration.250ms"}},
		modalDialogActionSpec{ID: "500", LabelRef: i18n.Message{ID: "presentation.animation.duration.500ms"}},
		modalDialogActionSpec{ID: "1000", LabelRef: i18n.Message{ID: "presentation.animation.duration.1000ms"}},
		modalDialogActionSpec{ID: "2000", LabelRef: i18n.Message{ID: "presentation.animation.duration.2000ms"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

type presentationanimationSlideIdElementIdArguments struct {
	SlideId        uint32 `json:"slide_id"`
	ElementId      int64  `json:"element_id"`
	AnimationIndex int    `json:"animation_index"`
	DurationMs     int    `json:"duration_ms"`
}

type presentationAnimationsListArguments struct {
	SlideId uint32 `json:"slide_id"`
}

func (a *App) startPresentationAnimationDuration(view *presentationViewState, durationMS int) bool {
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
	elementID := view.selectedShape
	if elementID <= 0 {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.select_an_element_whose_animation_duration_2e54cb"}, feedbackWarning, notificationSuppressed)
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
		out := presentationCommandResult{state: state, view: view, generation: generation, presentationAnimationDuration: true, presentationAnimationDurationMS: durationMS}
		query, err := a.executeRibbonApplicationCommand(token, "presentation.animations.list", "presentation_slide", fmt.Sprint(slide.ID), "", presentationAnimationsListArguments{SlideId: slide.ID})
		if err != nil {
			out.err = err
		} else {
			var inventory struct {
				Animations []struct {
					Index           int    `json:"index"`
					Kind            string `json:"kind"`
					TargetElementID uint32 `json:"target_element_id"`
				} `json:"animations"`
			}
			if json.Unmarshal(query.Data, &inventory) != nil {
				out.err = application.ErrInvalidCommandResult
			} else {
				animationIndex := 0
				for index := len(inventory.Animations) - 1; index >= 0; index-- {
					animation := inventory.Animations[index]
					if int64(animation.TargetElementID) == elementID && (animation.Kind == "animEffect" || animation.Kind == "animMotion") {
						animationIndex = animation.Index
						break
					}
				}
				if animationIndex == 0 {
					out.err = fmt.Errorf("当前元素没有可调整时长的进入动画")
				} else {
					command, err := a.executeRibbonApplicationCommand(token, "presentation.element.animation.duration.set", "presentation_element", fmt.Sprint(elementID), "", presentationanimationSlideIdElementIdArguments{SlideId: slide.ID, ElementId: elementID, AnimationIndex: animationIndex, DurationMs: durationMS})
					if err != nil {
						out.err = err
					} else {
						var data struct {
							Changed        bool   `json:"changed"`
							SlideID        uint32 `json:"slide_id"`
							ElementID      int64  `json:"element_id"`
							AnimationIndex int    `json:"animation_index"`
							DurationMS     int    `json:"duration_ms"`
						}
						if json.Unmarshal(command.Data, &data) != nil || data.SlideID != slide.ID || data.ElementID != elementID || data.AnimationIndex != animationIndex || data.DurationMS != durationMS {
							out.err = application.ErrInvalidCommandResult
						} else {
							out.presentationAnimationDurationChanged = data.Changed
						}
					}
				}
			}
		}
		state.finishOperation(token)
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
