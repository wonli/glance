package desktopui

import (
	"encoding/json"
	"fmt"

	"glance/internal/application"
)

func (a *App) showPresentationThemeOptions(view *presentationViewState) {
	options := map[string]string{
		"corporate": "glance-corporate",
		"forest":    "glance-forest",
		"warm":      "glance-warm",
	}
	a.modal.Show("演示文稿主题", "将更新文档内所有主题的颜色和字体方案，保留母版、版式、主题效果、背景素材和对象的直接格式。", func(action string) {
		a.modal.Hide()
		if themeID := options[action]; themeID != "" {
			a.startPresentationThemeMutation(view, "theme", themeID)
		}
	},
		modalDialogActionSpec{ID: "corporate", Label: "企业蓝"},
		modalDialogActionSpec{ID: "forest", Label: "林地绿"},
		modalDialogActionSpec{ID: "warm", Label: "暖阳橙"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showPresentationPaletteOptions(view *presentationViewState) {
	options := map[string]string{
		"office": "office",
		"ocean":  "ocean",
		"forest": "forest",
		"warm":   "warm",
	}
	a.modal.Show("演示文稿配色", "只更新文档内所有主题的颜色方案；字体、效果、背景、版式和对象的直接格式保持不变。", func(action string) {
		a.modal.Hide()
		if paletteID := options[action]; paletteID != "" {
			a.startPresentationThemeMutation(view, "palette", paletteID)
		}
	},
		modalDialogActionSpec{ID: "office", Label: "Office"},
		modalDialogActionSpec{ID: "ocean", Label: "海湾蓝"},
		modalDialogActionSpec{ID: "forest", Label: "林地绿"},
		modalDialogActionSpec{ID: "warm", Label: "暖阳橙"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
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
		a.setStatus("演示文稿能力尚未连接")
		return true
	}
	if view.commandPending {
		a.setStatus("演示文稿正在处理其他操作")
		return true
	}
	selected := presentationSelectedIDs(view)
	if len(selected) < 2 {
		a.setStatus("按住 Shift 或 Command/Ctrl 选择至少两个对象后再组合")
		return true
	}
	if len(selected) > 100 {
		a.setStatus("一次最多组合 100 个对象")
		return true
	}
	slide := view.currentSlide()
	if slide == nil {
		a.setStatus("未找到当前幻灯片")
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
			a.setStatus("选中的对象 ID 无效")
			return true
		}
		elementIDs[index] = uint32(id)
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
