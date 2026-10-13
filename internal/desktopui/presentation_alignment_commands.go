package desktopui

import (
	"encoding/json"
	"fmt"

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
		a.setStatus("请先选择一个可排列的幻灯片对象")
		return
	}
	slide := view.currentSlide()
	if slide == nil {
		a.setStatus("未找到当前幻灯片")
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
		a.setStatus("当前选择已失效，请重新选择对象")
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
	a.modal.Show("排列对齐", "相对于当前幻灯片画布对齐所选对象。", func(action string) {
		a.modal.Hide()
		for _, option := range options {
			if option.action == action {
				a.startPresentationElementAlignment(view, slide, *selected, option)
				return
			}
		}
	},
		modalDialogActionSpec{ID: "left", Label: "左对齐"},
		modalDialogActionSpec{ID: "center", Label: "水平居中"},
		modalDialogActionSpec{ID: "right", Label: "右对齐"},
		modalDialogActionSpec{ID: "top", Label: "顶端对齐"},
		modalDialogActionSpec{ID: "middle", Label: "垂直居中"},
		modalDialogActionSpec{ID: "bottom", Label: "底端对齐"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) startPresentationElementAlignment(view *presentationViewState, slide *document.PresentationSlide, element document.PresentationElement, alignment presentationAlignment) bool {
	state := a.documentSessionState
	if view == nil || slide == nil || state == nil || state.pptxDoc == nil || !a.applicationSessionRegistered(state.documentID) {
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
	bounds := element.Bounds
	bounds.X, bounds.Y = alignment.x, alignment.y
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, "presentation.element.transform.set", "presentation_element", fmt.Sprint(element.ID), "", presentationElementTransformSetArguments{SlideId: slide.ID, ElementId: element.ID, X: bounds.X, Y: bounds.Y, Width: bounds.W, Height: bounds.H, RotationDeg: element.RotationDeg})
		state.finishOperation(token)
		out := presentationCommandResult{state: state, view: view, generation: generation, elementAlignment: true, alignmentAction: alignment.label, err: err}
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
