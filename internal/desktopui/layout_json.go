package desktopui

import (
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/format/jsondoc"
)

const (
	jsonToolbarHeightDP = 68
	jsonToolbarIconSize = toolbarIconSize
)

var jsonToolbarActions = []ribbonAction{
	{id: "save", label: "保存", group: "文件", icon: "save"},
	{id: "save-as", label: "另存为", group: "文件", icon: "save-as"},
	{id: "import", label: "导入", group: "文件", icon: "folder-open"},
	{id: "expand-all", label: "全部展开", group: "展开", icon: "unfold-vertical"},
	{id: "collapse-all", label: "全部折叠", group: "展开", icon: "fold-vertical"},
	{id: "expand-level", label: "展开层级", group: "展开", icon: "list-tree"},
	{id: "format", label: "格式化", group: "格式", icon: "braces"},
	{id: "minify", label: "压缩", group: "格式", icon: "minimize-2"},
	{id: "search", label: "搜索", group: "格式", icon: "search"},
	{id: "copy", label: "复制", group: "格式", icon: "copy"},
	{id: "sort-keys", label: "排序", group: "工具", icon: "arrow-up-down"},
	{id: "validate", label: "校验", group: "工具", icon: "circle-check"},
}

func (a *App) layoutJSON(gtx layout.Context) layout.Dimensions {
	a.handleJSONWorkspaceEvents(gtx)
	a.handleJSONToolbarEvents(gtx)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(a.layoutJSONToolbar),
		layout.Flexed(1, a.layoutJSONWorkspace),
		layout.Rigid(a.layoutJSONStatus),
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
		a.startJSONTool("format")
	}

	minifyBtn := jsonToolbarButton(state, a, "minify")
	if minifyBtn != nil && minifyBtn.Clicked(gtx) && !a.isSaving() {
		a.startJSONTool("minify")
	}

	sortBtn := jsonToolbarButton(state, a, "sort-keys")
	if sortBtn != nil && sortBtn.Clicked(gtx) && !a.isSaving() {
		a.startJSONTool("sort-keys")
	}

	validateBtn := jsonToolbarButton(state, a, "validate")
	if validateBtn != nil && validateBtn.Clicked(gtx) && !a.isSaving() {
		a.startJSONTool("validate")
	}
	for _, id := range []string{"expand-all", "collapse-all", "expand-level", "search", "copy", "level-1", "level-2", "level-3", "mode-tree", "mode-source", "mode-compare", "paste", "import", "copy-value", "copy-key", "copy-object", "copy-dot", "copy-path", "copy-pointer", "row-copy-value", "row-copy-path", "level-all", "apply-value", "cancel-value"} {
		if jsonToolbarButton(state, a, id).Clicked(gtx) {
			a.handleJSONWorkspaceAction(gtx, id)
		}
	}

}

func (a *App) layoutJSONSaveToolbar(gtx layout.Context) layout.Dimensions {
	return a.layoutJSONToolbar(gtx)
}

func (a *App) jsonToolbarActionEnabled(action ribbonAction) bool {
	if a == nil || a.jsonDoc == nil || a.isSaving() {
		return false
	}
	if a.jsonView != nil && a.jsonView.toolPending {
		return false
	}
	switch action.id {
	case "save":
		return a.isDirty()
	case "import":
		return !a.opening
	case "save-as", "format", "minify", "sort-keys", "validate", "expand-all", "collapse-all", "expand-level", "search", "copy":
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
	text := a.jsonDraftText()
	formatted, err := jsondoc.Format(text)
	if err != nil {
		a.setStatus("JSON 格式化失败：" + err.Error())
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
	text := a.jsonDraftText()
	minified, err := jsondoc.Minify(text)
	if err != nil {
		a.setStatus("JSON 压缩失败：" + err.Error())
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
	text := a.jsonDraftText()
	sorted, err := jsondoc.SortKeys(text)
	if err != nil {
		a.setStatus("JSON 排序键失败：" + err.Error())
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
	text := a.jsonDraftText()
	if err := jsondoc.Validate(text); err != nil {
		a.setStatus("JSON 验证失败：" + err.Error())
	} else {
		a.setStatus("JSON 格式正确")
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) applyJSONTextMutation(newText, successStatus string) {
	a.applyJSONTextMutationChecked(newText, successStatus, nil)
}

func (a *App) applyJSONTextMutationIfSource(newText, successStatus, expectedSource string) bool {
	return a.applyJSONTextMutationChecked(newText, successStatus, &expectedSource)
}

func (a *App) applyJSONTextMutationChecked(newText, successStatus string, expectedSource *string) bool {
	token, ok := a.documentSessionState.beginOperation(sessionOperationMutation)
	if !ok {
		a.setStatus("JSON 文档正在处理其他操作")
		if a.window != nil {
			a.window.Invalidate()
		}
		return false
	}
	unlock := lockSessionAdapterWrite(a.documentSessionState)
	current := a.jsonDoc.Text()
	if expectedSource != nil && current != *expectedSource {
		unlock()
		a.completeSessionOperation(a.documentSessionState, token, nil, false)
		a.setStatus("JSON 内容已经改变，已取消操作")
		return false
	}
	if current == newText {
		unlock()
		a.completeSessionOperation(a.documentSessionState, token, nil, false)
		return true
	}
	err := a.jsonDoc.SetText(newText)
	unlock()
	a.completeSessionOperation(a.documentSessionState, token, err, err == nil)
	if err != nil {
		a.setStatus(err.Error())
	} else {
		v := a.ensureJSONView()
		if len(newText) <= jsonSourceEditLimit {
			a.jsonEditor.SetText(newText)
			v.sourceLoaded = true
		} else if v.sourceLoaded {
			a.jsonEditor.SetText("")
			v.sourceLoaded = false
		}
		v.sourceSynced = newText
		a.persistActiveSession()
		a.setStatus(successStatus)
	}
	if a.window != nil {
		a.window.Invalidate()
	}
	return err == nil
}

func (a *App) layoutJSONEditor(gtx layout.Context) layout.Dimensions {
	v := a.ensureJSONView()
	if !v.sourceLoaded {
		return a.layoutJSONSourcePreview(gtx, v.parsed, v.sourceSegments, &v.sourceList)
	}
	return layout.Inset{Top: unit.Dp(16), Left: unit.Dp(20), Right: unit.Dp(20), Bottom: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		style := material.Editor(a.theme, &a.jsonEditor, "编辑 JSON 文本")
		style.Font.Typeface = jsonMonoFace
		style.TextSize = unit.Sp(13)
		return style.Layout(gtx)
	})
}
