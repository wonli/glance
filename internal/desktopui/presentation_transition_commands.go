package desktopui

import (
	"encoding/json"
	"fmt"

	"glance/internal/application"
)

func (a *App) runPresentationTransitionAction(view *presentationViewState, actionID string) bool {
	if view == nil {
		return false
	}
	slide := view.currentSlide()
	if slide == nil {
		a.setStatus("未找到当前幻灯片")
		return true
	}
	switch actionID {
	case "trans_fade":
		a.startPresentationTransition(view, map[string]any{"transition_type": "fade"})
	case "trans_push":
		a.startPresentationTransition(view, map[string]any{"transition_type": "push", "direction": "l"})
	case "trans_options":
		a.modal.Show("推进方向", "选择方向；当前页需要已设置推进、擦除或覆盖效果。", func(action string) {
			a.modal.Hide()
			direction := map[string]string{"left": "l", "right": "r", "up": "u", "down": "d"}[action]
			if direction != "" {
				a.startPresentationTransition(view, map[string]any{"direction": direction})
			}
		},
			modalDialogActionSpec{ID: "left", Label: "向左"},
			modalDialogActionSpec{ID: "right", Label: "向右"},
			modalDialogActionSpec{ID: "up", Label: "向上"},
			modalDialogActionSpec{ID: "down", Label: "向下"},
			modalDialogActionSpec{ID: "cancel", Label: "取消"},
		)
	case "trans_duration":
		a.modal.Show("切换速度", "为当前页已设置的切换效果选择速度。", func(action string) {
			a.modal.Hide()
			if action == "slow" || action == "med" || action == "fast" {
				a.startPresentationTransition(view, map[string]any{"speed": action})
			}
		},
			modalDialogActionSpec{ID: "slow", Label: "慢"},
			modalDialogActionSpec{ID: "med", Label: "中"},
			modalDialogActionSpec{ID: "fast", Label: "快"},
			modalDialogActionSpec{ID: "cancel", Label: "取消"},
		)
	case "trans_apply_all":
		a.startPresentationTransition(view, map[string]any{"apply_to_all": true})
	default:
		return false
	}
	return true
}

func (a *App) startPresentationTransition(view *presentationViewState, arguments map[string]any) bool {
	state := a.documentSessionState
	if view == nil || state == nil || state.pptxDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		a.setStatus("演示文稿能力尚未连接")
		return true
	}
	if view.commandPending {
		a.setStatus("演示文稿正在处理其他操作")
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
		a.setStatus("未找到当前幻灯片")
		return true
	}
	arguments["slide_id"] = slide.ID
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatus("演示文稿正在处理其他操作")
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
