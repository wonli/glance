package desktopui

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/format/jsondoc"
)

const (
	jsonToolbarHeightDP = 54
	jsonToolbarIconSize = unit.Dp(18)
)

var jsonToolbarActions = []ribbonAction{
	{id: "save", label: "保存", group: "文件", icon: "save"},
	{id: "save-as", label: "另存为", group: "文件", icon: "save-as"},
	{id: "format", label: "格式化", group: "格式", icon: "braces"},
	{id: "minify", label: "压缩", group: "格式", icon: "minimize-2"},
	{id: "sort-keys", label: "排序键", group: "工具", icon: "arrow-up-down"},
	{id: "validate", label: "验证", group: "工具", icon: "circle-check"},
}

func (a *App) layoutJSON(gtx layout.Context) layout.Dimensions {
	a.handleJSONToolbarEvents(gtx)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(a.layoutJSONToolbar),
		layout.Flexed(1, a.layoutJSONEditor),
	)
}

func (a *App) handleJSONToolbarEvents(gtx layout.Context) {
	if a == nil || a.jsonDoc == nil || a.modal.Visible {
		return
	}
	state := a.documentSessionState
	if state == nil {
		return
	}

	saveBtn := jsonToolbarButton(state, a, "save")
	if saveBtn != nil && saveBtn.Clicked(gtx) && !a.isSaving() && a.isDirty() {
		if a.path == "" {
			a.startSaveAsPicker("json")
		} else {
			a.startSave()
		}
	}

	saveAsBtn := jsonToolbarButton(state, a, "save-as")
	if saveAsBtn != nil && saveAsBtn.Clicked(gtx) && !a.isSaving() {
		a.startSaveAsPicker("json")
	}

	formatBtn := jsonToolbarButton(state, a, "format")
	if formatBtn != nil && formatBtn.Clicked(gtx) && !a.isSaving() {
		a.formatJSONDocument()
	}

	minifyBtn := jsonToolbarButton(state, a, "minify")
	if minifyBtn != nil && minifyBtn.Clicked(gtx) && !a.isSaving() {
		a.minifyJSONDocument()
	}

	sortBtn := jsonToolbarButton(state, a, "sort-keys")
	if sortBtn != nil && sortBtn.Clicked(gtx) && !a.isSaving() {
		a.sortJSONKeysDocument()
	}

	validateBtn := jsonToolbarButton(state, a, "validate")
	if validateBtn != nil && validateBtn.Clicked(gtx) && !a.isSaving() {
		a.validateJSONDocument()
	}
}

func (a *App) layoutJSONToolbar(gtx layout.Context) layout.Dimensions {
	state := a.documentSessionState
	if state == nil || a.jsonDoc == nil {
		return layout.Dimensions{}
	}
	state.jsonToolbarActionList.List.Axis = layout.Horizontal
	height := gtx.Dp(unit.Dp(jsonToolbarHeightDP))
	gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
	actions := jsonToolbarActions

	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, contentSurface, clip.Rect{Max: gtx.Constraints.Max}.Op())
			paint.FillShape(gtx.Ops, ribbonDividerColor, clip.Rect{Min: image.Pt(0, height-gtx.Dp(unit.Dp(1))), Max: image.Pt(gtx.Constraints.Max.X, height)}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(9), Bottom: unit.Dp(9), Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return state.jsonToolbarActionList.Layout(gtx, len(actions), func(gtx layout.Context, index int) layout.Dimensions {
					action := actions[index]
					enabled := a.jsonToolbarActionEnabled(action)
					button := jsonToolbarButton(state, a, action.id)
					icon, _ := jsonToolbarActionIcon(action, enabled)
					background := contentSurface
					return layoutToolbarActionItem(gtx, action, index, actions, func(gtx layout.Context) layout.Dimensions {
						return toolbarIconButton(gtx, button, icon, jsonToolbarIconSize, action.label, enabled, false, background)
					})
				})
			})
		},
	)
}

func (a *App) layoutJSONSaveToolbar(gtx layout.Context) layout.Dimensions {
	return a.layoutJSONToolbar(gtx)
}

func (a *App) jsonToolbarActionEnabled(action ribbonAction) bool {
	if a == nil || a.jsonDoc == nil || a.isSaving() {
		return false
	}
	switch action.id {
	case "save":
		return a.isDirty()
	case "save-as", "format", "minify", "sort-keys", "validate":
		return true
	default:
		return false
	}
}

func jsonToolbarButton(state *documentSessionState, a *App, id string) *widget.Clickable {
	if id == "save" && a != nil {
		return &a.actionSave
	}
	if id == "save-as" && a != nil {
		return &a.actionSaveAs
	}
	if state == nil {
		return nil
	}
	if state.jsonToolbarActionButtons == nil {
		state.jsonToolbarActionButtons = make(map[string]*widget.Clickable)
	}
	button := state.jsonToolbarActionButtons[id]
	if button == nil {
		button = new(widget.Clickable)
		state.jsonToolbarActionButtons[id] = button
	}
	return button
}

func (a *App) formatJSONDocument() {
	if a == nil || a.documentSessionState == nil || a.jsonDoc == nil {
		return
	}
	text := a.jsonEditor.Text()
	formatted, err := jsondoc.Format(text)
	if err != nil {
		a.status = "JSON 格式化失败：" + err.Error()
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	a.applyJSONTextMutation(formatted, "已格式化 JSON")
}

func (a *App) minifyJSONDocument() {
	if a == nil || a.documentSessionState == nil || a.jsonDoc == nil {
		return
	}
	text := a.jsonEditor.Text()
	minified, err := jsondoc.Minify(text)
	if err != nil {
		a.status = "JSON 压缩失败：" + err.Error()
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	a.applyJSONTextMutation(minified, "已压缩 JSON")
}

func (a *App) sortJSONKeysDocument() {
	if a == nil || a.documentSessionState == nil || a.jsonDoc == nil {
		return
	}
	text := a.jsonEditor.Text()
	sorted, err := jsondoc.SortKeys(text)
	if err != nil {
		a.status = "JSON 排序键失败：" + err.Error()
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	a.applyJSONTextMutation(sorted, "已对 JSON 键排序")
}

func (a *App) validateJSONDocument() {
	if a == nil || a.documentSessionState == nil || a.jsonDoc == nil {
		return
	}
	text := a.jsonEditor.Text()
	if err := jsondoc.Validate(text); err != nil {
		a.status = "JSON 验证失败：" + err.Error()
	} else {
		a.status = "JSON 格式正确"
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) applyJSONTextMutation(newText, successStatus string) {
	token, ok := a.documentSessionState.beginOperation(sessionOperationMutation)
	if !ok {
		a.status = "JSON 文档正在处理其他操作"
		if a.window != nil {
			a.window.Invalidate()
		}
		return
	}
	unlock := lockSessionAdapterWrite(a.documentSessionState)
	err := a.jsonDoc.SetText(newText)
	unlock()
	a.completeSessionOperation(a.documentSessionState, token, err, err == nil)
	if err != nil {
		a.status = err.Error()
	} else {
		a.jsonEditor.SetText(newText)
		a.persistActiveSession()
		a.status = successStatus
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) layoutJSONEditor(gtx layout.Context) layout.Dimensions {
	unlock := lockSessionAdapterRead(a.documentSessionState)
	text := a.jsonDoc.Text()
	unlock()
	if a.jsonEditor.Text() != text {
		a.jsonEditor.SetText(text)
	}
	return layout.Inset{Top: unit.Dp(8), Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		style := material.Editor(a.theme, &a.jsonEditor, "编辑 JSON 文本")
		dims := style.Layout(gtx)
		updated := a.jsonEditor.Text()
		if updated != text {
			token, ok := a.documentSessionState.beginOperation(sessionOperationMutation)
			if !ok {
				a.status = "JSON 文档正在处理其他操作"
				return dims
			}
			unlock := lockSessionAdapterWrite(a.documentSessionState)
			err := a.jsonDoc.SetText(updated)
			unlock()
			a.completeSessionOperation(a.documentSessionState, token, err, err == nil)
			if err != nil {
				a.status = err.Error()
			} else {
				a.persistActiveSession()
			}
		}
		return dims
	})
}
