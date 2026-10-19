package desktopui

import (
	"encoding/json"
	"glance/internal/i18n"
	"math"

	"glance/internal/application"
)

func (a *App) showPresentationSlideSizeOptions(view *presentationViewState) {
	if view == nil || view.snapshot.SlideCount == 0 {
		a.setStatusMessage(i18n.Message{ID: "presentation.status.inline.the_presentation_was_not_found_ac507e"}, feedbackWarning, notificationToast)
		return
	}
	a.modal.ShowMessage("presentation.slide_size", i18n.Message{ID: "presentation.slide_size.title"}, i18n.Message{ID: "presentation.slide_size.body"}, func(action string) {
		a.modal.Hide()
		sizes := map[string][2]float64{
			"widescreen": {960, 540},
			"standard":   {720, 540},
			"a4":         {841.89, 595.28},
		}[action]
		if sizes[0] <= 0 || sizes[1] <= 0 {
			return
		}
		a.startPresentationSlideSizeSet(view, sizes[0], sizes[1])
	},
		modalDialogActionSpec{ID: "widescreen", LabelRef: i18n.Message{ID: "presentation.slide_size.widescreen"}},
		modalDialogActionSpec{ID: "standard", LabelRef: i18n.Message{ID: "presentation.slide_size.standard"}},
		modalDialogActionSpec{ID: "a4", LabelRef: i18n.Message{ID: "presentation.slide_size.a4"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

type presentationSlideSizeSetArguments struct {
	WidthPt  float64 `json:"width_pt"`
	HeightPt float64 `json:"height_pt"`
}

func (a *App) startPresentationSlideSizeSet(view *presentationViewState, widthPT, heightPT float64) bool {
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
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, "presentation.slide.size.set", "presentation_document", token.documentID, "", presentationSlideSizeSetArguments{WidthPt: widthPT, HeightPt: heightPT})
		state.finishOperation(token)
		out := presentationCommandResult{state: state, view: view, generation: generation, slideSize: true, err: err}
		if err == nil {
			var data struct {
				Changed  bool    `json:"changed"`
				WidthPT  float64 `json:"width_pt"`
				HeightPT float64 `json:"height_pt"`
			}
			if json.Unmarshal(result.Data, &data) != nil || math.Abs(data.WidthPT-widthPT) > 0.5/12700+1e-9 || math.Abs(data.HeightPT-heightPT) > 0.5/12700+1e-9 {
				out.err = application.ErrInvalidCommandResult
			} else {
				out.slideSizeChanged = data.Changed
				out.slideWidthPT = data.WidthPT
				out.slideHeightPT = data.HeightPT
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
