package desktopui

import (
	"encoding/json"
	"fmt"
	"strconv"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"glance/internal/application"
	"glance/internal/document"
)

type presentationLayoutsListArguments struct {
}

func (a *App) startPresentationLayoutList(view *presentationViewState) bool {
	state := a.documentSessionState
	if view == nil || state == nil || state.pptxDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	if view.commandPending {
		return true
	}
	if err := a.flushPresentationNotesDraft(view); err != nil {
		return true
	}
	if err := a.flushPresentationTextDraft(); err != nil {
		return true
	}
	if view.activeSlide < 0 || view.activeSlide >= len(view.snapshot.Slides) {
		a.setStatus("未找到当前幻灯片")
		return true
	}
	slideID := view.snapshot.Slides[view.activeSlide].ID
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		a.setStatus("演示文稿正在处理其他操作")
		return true
	}
	view.commandPending = true
	generation := a.documentGeneration
	window, shutdown := a.window, a.shutdown
	a.launchManagedBackground(func() {
		out := presentationCommandResult{
			state: state, view: view, generation: generation,
			presentationLayoutList: true, presentationLayoutSlideID: slideID,
		}
		command, err := a.executeRibbonApplicationCommand(token, "presentation.layouts.list", "presentation_document", token.documentID, "", presentationLayoutsListArguments{})
		if err != nil {
			out.err = err
		} else {
			var summary document.PresentationLayoutSummary
			if json.Unmarshal(command.Data, &summary) != nil || command.Revision != token.revision || summary.Revision != token.revision || len(summary.Layouts) > 500 || summary.Total < len(summary.Layouts) || summary.Truncated != (summary.Total > len(summary.Layouts)) {
				out.err = application.ErrInvalidCommandResult
			} else {
				for index, option := range summary.Layouts {
					if option.Index != index || option.Name == "" {
						out.err = application.ErrInvalidCommandResult
						break
					}
				}
				out.presentationLayoutOptions = summary
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

func (a *App) showPresentationLayoutChoices(view *presentationViewState, slideID uint32, summary document.PresentationLayoutSummary) {
	if view == nil || slideID == 0 || summary.Revision == 0 {
		a.setStatus("版式目录无效，请重新打开版式选择")
		return
	}
	view.layoutOptions = append(view.layoutOptions[:0], summary.Layouts...)
	view.layoutSlideID = slideID
	view.layoutRevision = summary.Revision
	view.layoutList.Position.First = 0
	view.layoutList.Position.Offset = 0
	view.layoutOptionButtons = make(map[int]*widget.Clickable, len(summary.Layouts))
	var message string
	if len(summary.Layouts) == 0 {
		message = "当前演示文稿没有可用版式。"
	} else {
		message = fmt.Sprintf("为幻灯片 ID %d 选择版式（读取修订：%d）。", slideID, summary.Revision)
		if summary.Truncated {
			message += fmt.Sprintf("\n版式总数为 %d，当前显示前 %d 项。", summary.Total, len(summary.Layouts))
		}
	}
	a.modal.ShowCustom("幻灯片版式", message, 500,
		func(gtx layout.Context) layout.Dimensions {
			return a.layoutPresentationLayoutChoices(gtx, view)
		},
		func(action string) {
			if action == "close" {
				a.modal.Hide()
				return
			}
			index, err := strconv.Atoi(action)
			if err != nil || index < 0 || index >= len(view.layoutOptions) {
				a.setStatus("版式选项已失效，请重新读取目录")
				a.modal.Hide()
				return
			}
			option := view.layoutOptions[index]
			a.modal.Hide()
			a.startPresentationLayoutSet(view, view.layoutSlideID, option, view.layoutRevision)
		},
		modalDialogActionSpec{ID: "close", Label: "关闭"},
	)
}

func (a *App) layoutPresentationLayoutChoices(gtx layout.Context, view *presentationViewState) layout.Dimensions {
	if view == nil {
		return layout.Dimensions{}
	}
	view.layoutList.Axis = layout.Vertical
	maxHeight := gtx.Dp(unit.Dp(300))
	if gtx.Constraints.Max.Y < maxHeight {
		maxHeight = gtx.Constraints.Max.Y
	}
	gtx.Constraints.Max.Y = maxHeight
	return material.List(a.theme, &view.layoutList).Layout(gtx, len(view.layoutOptions), func(gtx layout.Context, index int) layout.Dimensions {
		option := view.layoutOptions[index]
		button := view.layoutOptionButtons[index]
		if button == nil {
			button = new(widget.Clickable)
			view.layoutOptionButtons[index] = button
		}
		if button.Clicked(gtx) && a.modal.OnAction != nil {
			a.modal.OnAction(strconv.Itoa(option.Index))
		}
		gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(44))
		gtx.Constraints.Max.Y = gtx.Constraints.Min.Y
		return material.Button(a.theme, button, fmt.Sprintf("%d · %s", option.Index+1, option.Name)).Layout(gtx)
	})
}

type presentationSlideLayoutSetArguments struct {
	SlideId     uint32 `json:"slide_id"`
	LayoutIndex int    `json:"layout_index"`
}

func (a *App) startPresentationLayoutSet(view *presentationViewState, slideID uint32, option document.PresentationLayoutOption, expectedRevision uint64) bool {
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
	if token.revision != expectedRevision {
		state.finishOperation(token)
		a.setStatus("文档在选择版式期间发生变化，请重新打开版式目录")
		return true
	}
	view.commandPending = true
	generation := a.documentGeneration
	window, shutdown := a.window, a.shutdown
	a.launchManagedBackground(func() {
		out := presentationCommandResult{
			state: state, view: view, generation: generation,
			presentationLayoutSet: true, presentationLayoutSlideID: slideID,
		}
		command, err := a.executeRibbonApplicationCommand(token, "presentation.slide.layout.set", "presentation_slide", fmt.Sprint(slideID), "", presentationSlideLayoutSetArguments{SlideId: slideID, LayoutIndex: option.Index})
		if err != nil {
			out.err = err
		} else {
			var data struct {
				Changed       bool   `json:"changed"`
				SlideID       uint32 `json:"slide_id"`
				LayoutIndex   int    `json:"layout_index"`
				LayoutName    string `json:"layout_name"`
				NameTruncated bool   `json:"name_truncated"`
			}
			if json.Unmarshal(command.Data, &data) != nil || data.SlideID != slideID || data.LayoutIndex != option.Index || data.LayoutName != option.Name || data.NameTruncated != option.NameTruncated {
				out.err = application.ErrInvalidCommandResult
			} else {
				out.presentationLayoutOption = document.PresentationLayoutOption{Index: data.LayoutIndex, Name: data.LayoutName, NameTruncated: data.NameTruncated}
				out.presentationLayoutChanged = data.Changed
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
