package desktopui

import (
	"encoding/json"
	"fmt"

	"glance/internal/application"
)

func (a *App) showPresentationBackgroundOptions(view *presentationViewState) {
	if view == nil || view.currentSlide() == nil {
		a.setStatus("未找到当前幻灯片")
		return
	}
	a.modal.Show("幻灯片背景", "为当前幻灯片设置纯色背景。", func(action string) {
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
		modalDialogActionSpec{ID: "white", Label: "白色"},
		modalDialogActionSpec{ID: "light-gray", Label: "浅灰"},
		modalDialogActionSpec{ID: "light-blue", Label: "浅蓝"},
		modalDialogActionSpec{ID: "navy", Label: "深蓝"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) startPresentationBackgroundSet(view *presentationViewState, colorHex string) bool {
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
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatus("演示文稿正在处理其他操作")
		return true
	}
	view.commandPending = true
	generation := a.documentGeneration
	window, shutdown := a.window, a.shutdown
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, "presentation.slide.background.set", "presentation_slide", fmt.Sprint(slide.ID), "", map[string]any{
			"slide_id": slide.ID, "color_hex": colorHex,
		})
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
