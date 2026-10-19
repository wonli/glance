package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/i18n"

	"glance/internal/application"
)

func (a *App) showPresentationThemeOptions(view *presentationViewState) {
	options := map[string]string{
		"corporate": "glance-corporate",
		"forest":    "glance-forest",
		"warm":      "glance-warm",
	}
	a.modal.ShowMessage("presentation.theme", i18n.Message{ID: "presentation.theme.title"}, i18n.Message{ID: "presentation.theme.body"}, func(action string) {
		a.modal.Hide()
		if themeID := options[action]; themeID != "" {
			a.startPresentationThemeMutation(view, "theme", themeID)
		}
	},
		modalDialogActionSpec{ID: "corporate", LabelRef: i18n.Message{ID: "presentation.theme.option.corporate"}},
		modalDialogActionSpec{ID: "forest", LabelRef: i18n.Message{ID: "presentation.theme.option.forest"}},
		modalDialogActionSpec{ID: "warm", LabelRef: i18n.Message{ID: "presentation.theme.option.warm"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

func (a *App) showPresentationPaletteOptions(view *presentationViewState) {
	options := map[string]string{
		"office": "office",
		"ocean":  "ocean",
		"forest": "forest",
		"warm":   "warm",
	}
	a.modal.ShowMessage("presentation.palette", i18n.Message{ID: "presentation.palette.title"}, i18n.Message{ID: "presentation.palette.body"}, func(action string) {
		a.modal.Hide()
		if paletteID := options[action]; paletteID != "" {
			a.startPresentationThemeMutation(view, "palette", paletteID)
		}
	},
		modalDialogActionSpec{ID: "office", Label: "Office"},
		modalDialogActionSpec{ID: "ocean", LabelRef: i18n.Message{ID: "presentation.palette.option.ocean"}},
		modalDialogActionSpec{ID: "forest", LabelRef: i18n.Message{ID: "presentation.theme.option.forest"}},
		modalDialogActionSpec{ID: "warm", LabelRef: i18n.Message{ID: "presentation.theme.option.warm"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

type startPresentationThemeMutationPayload struct {
	ThemeID string `json:"theme_id"`
}

type presentationthemePaletteIdArguments struct {
	PaletteID string `json:"palette_id"`
}

func (a *App) startPresentationThemeMutation(view *presentationViewState, operation, value string) bool {
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
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.another_presentation_operation_is_in_progr_9641a9"}, feedbackWarning, notificationToast)
		return true
	}
	view.commandPending = true
	generation := a.documentGeneration
	window, shutdown := a.window, a.shutdown
	commandID := "presentation.theme.set"
	var arguments any
	if operation == "palette" {
		commandID = "presentation.theme.palette.set"
		arguments = presentationthemePaletteIdArguments{PaletteID: value}
	} else {
		arguments = startPresentationThemeMutationPayload{ThemeID: value}
	}
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, commandID, "presentation_document", token.documentID, "", arguments)
		state.finishOperation(token)
		out := presentationCommandResult{
			state: state, view: view, generation: generation, presentationTheme: true,
			presentationThemeOperation: operation, presentationThemeID: value, err: err,
		}
		if err == nil {
			var data struct {
				Changed bool   `json:"changed"`
				ThemeID string `json:"theme_id"`
				Palette string `json:"palette_id"`
			}
			if json.Unmarshal(result.Data, &data) != nil {
				out.err = application.ErrInvalidCommandResult
			} else {
				out.presentationThemeChanged = data.Changed
				returned := data.ThemeID
				if operation == "palette" {
					returned = data.Palette
				}
				if returned != value {
					out.err = application.ErrInvalidCommandResult
				}
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

type presentationElementGroupSetArguments struct {
	SlideId    uint32   `json:"slide_id"`
	ElementIds []uint32 `json:"element_ids"`
}

func (a *App) startPresentationGroup(view *presentationViewState) bool {
	state := a.documentSessionState
	if view == nil || state == nil || state.pptxDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.presentation_capabilities_are_not_connecte_811cbe"}, feedbackInfo, notificationToast)
		return true
	}
	if view.commandPending {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.another_presentation_operation_is_in_progr_9641a9"}, feedbackWarning, notificationToast)
		return true
	}
	selected := presentationSelectedIDs(view)
	if len(selected) < 2 {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.hold_shift_or_command_ctrl_and_select_at_l_64a1a2"}, feedbackInfo, notificationToast)
		return true
	}
	if len(selected) > 100 {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.you_can_group_up_to_100_objects_at_a_time_4dda04"}, feedbackInfo, notificationToast)
		return true
	}
	slide := view.currentSlide()
	if slide == nil {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_current_slide_was_not_found_2b6179"}, feedbackWarning, notificationToast)
		return true
	}
	if err := a.flushPresentationNotesDraft(view); err != nil {
		return true
	}
	if err := a.flushPresentationTextDraft(); err != nil {
		return true
	}
	elementIDs := make([]uint32, len(selected))
	for index, id := range selected {
		if id <= 0 || uint64(id) > uint64(^uint32(0)) {
			a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_selected_object_id_is_invalid_5eeb22"}, feedbackError, notificationToast)
			return true
		}
		elementIDs[index] = uint32(id)
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
		result, err := a.executeRibbonApplicationCommand(token, "presentation.element.group.set", "presentation_slide", fmt.Sprint(slide.ID), "", presentationElementGroupSetArguments{SlideId: slide.ID, ElementIds: elementIDs})
		state.finishOperation(token)
		out := presentationCommandResult{
			state: state, view: view, generation: generation, presentationGroup: true,
			presentationGroupElementIDs: append([]uint32(nil), elementIDs...), err: err,
		}
		if err == nil {
			var data struct {
				Changed    bool     `json:"changed"`
				SlideID    uint32   `json:"slide_id"`
				GroupID    uint32   `json:"group_id"`
				ElementIDs []uint32 `json:"element_ids"`
			}
			if json.Unmarshal(result.Data, &data) != nil || data.SlideID != slide.ID || data.GroupID == 0 || len(data.ElementIDs) != len(elementIDs) {
				out.err = application.ErrInvalidCommandResult
			} else {
				for index := range elementIDs {
					if data.ElementIDs[index] != elementIDs[index] {
						out.err = application.ErrInvalidCommandResult
						break
					}
				}
				out.presentationGroupID = data.GroupID
				out.presentationGroupChanged = data.Changed
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
