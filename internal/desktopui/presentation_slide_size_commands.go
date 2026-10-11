package desktopui

import (
	"encoding/json"
	"math"

	"glance/internal/application"
)

func (a *App) showPresentationSlideSizeOptions(view *presentationViewState) {
	if view == nil || view.snapshot.SlideCount == 0 {
		a.setStatus("未找到演示文稿")
		return
	}
	a.modal.Show("幻灯片大小", "调整整个演示文稿的画布。现有对象保持原 point 坐标，不会自动缩放或移动。", func(action string) {
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
		modalDialogActionSpec{ID: "widescreen", Label: "宽屏 16:9"},
		modalDialogActionSpec{ID: "standard", Label: "标准 4:3"},
		modalDialogActionSpec{ID: "a4", Label: "A4 横向"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) startPresentationSlideSizeSet(view *presentationViewState, widthPT, heightPT float64) bool {
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
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatus("演示文稿正在处理其他操作")
		return true
	}
	view.commandPending = true
	generation := a.documentGeneration
	window, shutdown := a.window, a.shutdown
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, "presentation.slide.size.set", "presentation_document", token.documentID, "", map[string]any{
			"width_pt": widthPT, "height_pt": heightPT,
		})
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
