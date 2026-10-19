package desktopui

import (
	"encoding/json"
	"fmt"
	"glance/internal/i18n"

	"glance/internal/application"
	"glance/internal/document"
)

type presentationAlignment struct {
	action string
	label  string
	x      float64
	y      float64
}

func (a *App) showPresentationAlignmentOptions(view *presentationViewState) {
	if view == nil || view.selectedShape == 0 {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.select_a_slide_object_that_can_be_arranged_79adea"}, feedbackWarning, notificationSuppressed)
		return
	}
	slide := view.currentSlide()
	if slide == nil {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_current_slide_was_not_found_2b6179"}, feedbackWarning, notificationToast)
		return
	}
	var selected *document.PresentationElement
	for index := range slide.Elements {
		if slide.Elements[index].ID == view.selectedShape {
			selected = &slide.Elements[index]
			break
		}
	}
	if selected == nil {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_current_selection_is_no_longer_valid_s_f43588"}, feedbackWarning, notificationToast)
		return
	}
	options := []presentationAlignment{
		{action: "left", label: "左对齐", x: 0, y: selected.Bounds.Y},
		{action: "center", label: "水平居中", x: (slide.WidthPT - selected.Bounds.W) / 2, y: selected.Bounds.Y},
		{action: "right", label: "右对齐", x: slide.WidthPT - selected.Bounds.W, y: selected.Bounds.Y},
		{action: "top", label: "顶端对齐", x: selected.Bounds.X, y: 0},
		{action: "middle", label: "垂直居中", x: selected.Bounds.X, y: (slide.HeightPT - selected.Bounds.H) / 2},
		{action: "bottom", label: "底端对齐", x: selected.Bounds.X, y: slide.HeightPT - selected.Bounds.H},
	}
	a.modal.ShowMessage("presentation.alignment", i18n.Message{ID: "presentation.alignment.title"}, i18n.Message{ID: "presentation.alignment.body"}, func(action string) {
		a.modal.Hide()
		for _, option := range options {
			if option.action == action {
				a.startPresentationElementAlignment(view, slide, *selected, option)
				return
			}
		}
	},
		modalDialogActionSpec{ID: "left", LabelRef: i18n.Message{ID: "presentation.alignment.left_label"}},
		modalDialogActionSpec{ID: "center", LabelRef: i18n.Message{ID: "presentation.alignment.center_label"}},
		modalDialogActionSpec{ID: "right", LabelRef: i18n.Message{ID: "presentation.alignment.right_label"}},
		modalDialogActionSpec{ID: "top", LabelRef: i18n.Message{ID: "presentation.alignment.top_label"}},
		modalDialogActionSpec{ID: "middle", LabelRef: i18n.Message{ID: "presentation.alignment.middle_label"}},
		modalDialogActionSpec{ID: "bottom", LabelRef: i18n.Message{ID: "presentation.alignment.bottom_label"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

func (a *App) startPresentationElementAlignment(view *presentationViewState, slide *document.PresentationSlide, element document.PresentationElement, alignment presentationAlignment) bool {
	state := a.documentSessionState
	if view == nil || slide == nil || state == nil || state.pptxDoc == nil || !a.applicationSessionRegistered(state.documentID) {
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
	bounds := element.Bounds
	bounds.X, bounds.Y = alignment.x, alignment.y
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, "presentation.element.transform.set", "presentation_element", fmt.Sprint(element.ID), "", presentationElementTransformSetArguments{SlideId: slide.ID, ElementId: element.ID, X: bounds.X, Y: bounds.Y, Width: bounds.W, Height: bounds.H, RotationDeg: element.RotationDeg})
		state.finishOperation(token)
		out := presentationCommandResult{state: state, view: view, generation: generation, elementAlignment: true, alignmentAction: alignment.action, err: err}
		if err == nil {
			var data struct {
				SlideID   uint32 `json:"slide_id"`
				ElementID int64  `json:"element_id"`
				Changed   bool   `json:"changed"`
			}
			if json.Unmarshal(result.Data, &data) != nil || data.SlideID != slide.ID || data.ElementID != element.ID {
				out.err = application.ErrInvalidCommandResult
			} else {
				out.alignmentChanged = data.Changed
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
