package desktopui

import (
	"encoding/json"
	"fmt"

	"glance/internal/application"
	"glance/internal/document"
)

func (a *App) showPresentationAnimationAddOptions(view *presentationViewState) {
	if view == nil {
		return
	}
	a.modal.Show("添加进入动画", "选择要应用到当前选中元素的进入效果。", func(action string) {
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
			a.setStatus("无法启动动画设置")
		}
	},
		modalDialogActionSpec{ID: "appear", Label: "出现"},
		modalDialogActionSpec{ID: "fade", Label: "淡入"},
		modalDialogActionSpec{ID: "fly_in", Label: "从左侧飞入"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) startPresentationElementAnimation(view *presentationViewState, effect string, durationMS int) bool {
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
	elementID := view.selectedShape
	if elementID <= 0 {
		a.setStatus("请先选择要设置动画的幻灯片元素")
		return true
	}
	targetFound := false
	for _, element := range slide.Elements {
		if element.ID != elementID {
			continue
		}
		if element.Kind != document.ElementKindText && element.Kind != document.ElementKindShape && element.Kind != document.ElementKindPicture {
			a.setStatus("所选元素类型暂不支持动画")
			return true
		}
		targetFound = true
		break
	}
	if !targetFound {
		a.setStatus("所选元素已不在当前幻灯片中，请重新选择")
		return true
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatus("演示文稿正在处理其他操作")
		return true
	}
	view.commandPending = true
	generation := a.documentGeneration
	window, shutdown := a.window, a.shutdown
	arguments := map[string]any{
		"slide_id": slide.ID, "element_id": elementID,
		"effect": effect, "duration_ms": durationMS,
	}
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
	a.modal.Show("动画持续时间", "选择当前幻灯片选中元素最近一条进入动画的时长。", func(action string) {
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
			a.setStatus("无法启动动画时长设置")
		}
	},
		modalDialogActionSpec{ID: "250", Label: "0.25 秒"},
		modalDialogActionSpec{ID: "500", Label: "0.5 秒"},
		modalDialogActionSpec{ID: "1000", Label: "1 秒"},
		modalDialogActionSpec{ID: "2000", Label: "2 秒"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) startPresentationAnimationDuration(view *presentationViewState, durationMS int) bool {
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
	elementID := view.selectedShape
	if elementID <= 0 {
		a.setStatus("请先选择要调整动画时长的元素")
		return true
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatus("演示文稿正在处理其他操作")
		return true
	}
	view.commandPending = true
	generation := a.documentGeneration
	window, shutdown := a.window, a.shutdown
	a.launchManagedBackground(func() {
		out := presentationCommandResult{state: state, view: view, generation: generation, presentationAnimationDuration: true, presentationAnimationDurationMS: durationMS}
		query, err := a.executeRibbonApplicationCommand(token, "presentation.animations.list", "presentation_slide", fmt.Sprint(slide.ID), "", map[string]any{"slide_id": slide.ID})
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
					command, err := a.executeRibbonApplicationCommand(token, "presentation.element.animation.duration.set", "presentation_element", fmt.Sprint(elementID), "", map[string]any{
						"slide_id": slide.ID, "element_id": elementID, "animation_index": animationIndex, "duration_ms": durationMS,
					})
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
