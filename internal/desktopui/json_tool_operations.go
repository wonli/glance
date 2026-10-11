package desktopui

import (
	"encoding/json"
	"fmt"
	"strings"

	"glance/internal/format/jsondoc"
)

type jsonToolResult struct {
	id, source, text   string
	data               json.RawMessage
	err                error
	applicationCommand bool
	operationToken     sessionOperationToken
	generation         uint64
}

// Transform an immutable snapshot off the window loop. A newer source draft
// cancels installation, without reserving a session mutation while parsing.
func (a *App) startJSONTool(id string) {
	v := a.ensureJSONView()
	if v.toolPending {
		return
	}
	source := a.jsonDraftText()
	v.toolPending = true
	if a.startJSONApplicationTool(v, id, source) {
		return
	}
	results, window, done := v.toolResults, a.window, v.done
	a.launchManagedBackground(func() {
		defer releaseClosedJSONWorkerMemory(done, len(source) > jsonSourceEditLimit)
		result := jsonToolResult{id: id, source: source}
		switch id {
		case "format":
			result.text, result.err = jsondoc.Format(source)
		case "minify":
			result.text, result.err = jsondoc.Minify(source)
		case "sort-keys":
			result.text, result.err = jsondoc.SortKeys(source)
		case "validate":
			result.err = jsondoc.Validate(source)
		}
		select {
		case <-done:
			return
		default:
		}
		select {
		case results <- result:
		case <-done:
			return
		}
		if window != nil {
			window.Invalidate()
		}
	})
}

func (a *App) startJSONApplicationTool(v *jsonWorkspaceView, id, source string) bool {
	state := a.documentSessionState
	if state == nil || state.jsonDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	capabilityID := "json.text.transform"
	arguments := map[string]any{}
	kind := sessionOperationMutation
	switch id {
	case "format":
		arguments["operation"] = "format"
	case "minify":
		arguments["operation"] = "minify"
	case "sort-keys":
		arguments["operation"] = "sort_keys"
	case "validate":
		capabilityID = "json.document.validate"
		kind = sessionOperationRead
	case "search":
		query := strings.TrimSpace(v.search.Text())
		if query == "" {
			return false
		}
		capabilityID = "json.document.search"
		arguments["query"], arguments["limit"] = query, 100
		kind = sessionOperationRead
	case "copy":
		capabilityID = "json.document.read_text"
		kind = sessionOperationRead
	default:
		return false
	}
	token, ok := state.beginOperation(kind)
	if !ok {
		v.toolPending = false
		return true
	}
	generation := a.documentGeneration
	results, window := v.toolResults, a.window
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, capabilityID, "json_document", token.documentID, "", arguments)
		state.finishOperation(token)
		toolResult := jsonToolResult{
			id: id, source: source, applicationCommand: true,
			operationToken: token, generation: generation, err: err,
		}
		if err == nil {
			toolResult.data = append(json.RawMessage(nil), result.Data...)
		}
		select {
		case results <- toolResult:
		case <-v.done:
		}
		if window != nil {
			window.Invalidate()
		}
	})
	return true
}

func (a *App) startJSONViewApplicationTool(v *jsonWorkspaceView, operation string, level int) bool {
	state := a.documentSessionState
	if v == nil || state == nil || state.jsonDoc == nil || !a.applicationSessionRegistered(state.documentID) {
		return false
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		v.toolMessage = "JSON 文档正在处理其他操作"
		return true
	}
	arguments := map[string]any{"operation": operation}
	if operation == "expand_level" {
		arguments["level"] = level
	}
	v.toolPending = true
	generation := a.documentGeneration
	results, window := v.toolResults, a.window
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommand(token, "json.view.set", "attached_window", fmt.Sprint(a.windowID), "", arguments)
		state.finishOperation(token)
		toolResult := jsonToolResult{
			id: "view", source: state.jsonDoc.Text(), applicationCommand: true,
			operationToken: token, generation: generation, err: err,
		}
		if err == nil {
			toolResult.data = append(json.RawMessage(nil), result.Data...)
		}
		select {
		case results <- toolResult:
		case <-v.done:
		}
		if window != nil {
			window.Invalidate()
		}
	})
	return true
}

func (a *App) applyJSONToolResult(result jsonToolResult) {
	v := a.ensureJSONView()
	v.toolPending = false
	if result.applicationCommand {
		a.applyJSONApplicationToolResult(v, result)
		return
	}
	if a.jsonDraftText() != result.source || a.jsonDoc.Text() != result.source {
		return
	}
	label := map[string]string{"format": "格式化", "minify": "压缩", "sort-keys": "排序键", "validate": "验证"}[result.id]
	if result.err != nil {
		a.setStatus("JSON " + label + "失败：" + result.err.Error())
		v.toolMessage = a.status
		return
	}
	if result.id == "validate" {
		a.setStatus("JSON 格式正确")
		v.toolMessage = a.status
		return
	}
	if a.applyJSONTextMutationIfSource(result.text, "已"+label+" JSON", result.source) {
		v.toolMessage = "已" + label + " JSON"
	} else {
		v.toolMessage = a.status
	}
}

func (a *App) applyJSONApplicationToolResult(v *jsonWorkspaceView, result jsonToolResult) {
	state := result.operationTokenState(a)
	if state != nil {
		state.finishOperation(result.operationToken)
	}
	if result.err != nil {
		label := map[string]string{"format": "格式化", "minify": "压缩", "sort-keys": "排序键", "validate": "验证", "search": "搜索", "copy": "复制", "view": "视图"}[result.id]
		a.setStatus("JSON " + label + "失败：" + result.err.Error())
		v.toolMessage = a.status
		return
	}
	if state == nil || state.jsonDoc == nil {
		return
	}
	if result.id == "view" {
		var view struct {
			Changed bool `json:"changed"`
			Level   int  `json:"level"`
		}
		if err := json.Unmarshal(result.data, &view); err != nil {
			a.setStatus("JSON 视图结果无法读取：" + err.Error())
		} else if view.Changed {
			switch view.Level {
			case -1:
				a.setStatus("JSON 树已全部展开")
			case 0:
				a.setStatus("JSON 树已全部折叠")
			default:
				a.setStatus(fmt.Sprintf("JSON 树已展开到第 %d 层", view.Level))
			}
		} else {
			a.setStatus("JSON 树视图未变化")
		}
		v.toolMessage = a.status
		return
	}
	if result.id == "validate" {
		var validation struct {
			Valid      bool   `json:"valid"`
			Diagnostic string `json:"diagnostic"`
		}
		if err := json.Unmarshal(result.data, &validation); err != nil {
			a.setStatus("JSON 验证结果无法读取：" + err.Error())
		} else if validation.Valid {
			a.setStatus("JSON 格式正确")
		} else {
			a.setStatus("JSON 验证失败：" + validation.Diagnostic)
		}
		v.toolMessage = a.status
		return
	}
	if result.id == "search" {
		if state.jsonDoc.Text() != result.source || a.jsonDraftText() != result.source {
			return
		}
		var search struct {
			Matches []struct {
				Pointer string `json:"pointer"`
			} `json:"matches"`
			Truncated bool `json:"truncated"`
		}
		if err := json.Unmarshal(result.data, &search); err != nil {
			a.setStatus("JSON 搜索结果无法读取：" + err.Error())
		} else if search.Truncated {
			a.setStatus(fmt.Sprintf("JSON 搜索已返回 %d 项，结果已截取", len(search.Matches)))
		} else {
			a.setStatus(fmt.Sprintf("JSON 搜索找到 %d 项", len(search.Matches)))
		}
		v.toolMessage = a.status
		return
	}
	if result.id == "copy" {
		if state.jsonDoc.Text() != result.source || a.jsonDraftText() != result.source {
			a.setStatus("JSON 文档在复制期间已变化，请重试")
			v.toolMessage = a.status
			return
		}
		var copyResult struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(result.data, &copyResult); err != nil {
			a.setStatus("JSON 原文读取结果无法解析：" + err.Error())
		} else {
			v.clipboardWriteText = copyResult.Text
			v.clipboardWritePending = true
			a.setStatus("已复制 JSON")
		}
		v.toolMessage = a.status
		return
	}
	var output struct {
		Changed bool `json:"changed"`
	}
	if err := json.Unmarshal(result.data, &output); err != nil {
		a.setStatus("JSON 操作结果无法读取：" + err.Error())
		v.toolMessage = a.status
		return
	}
	text := state.jsonDoc.Text()
	if text != result.source && a.jsonDraftText() != result.source {
		// Keep a newer editor draft visible; the adapter mutation remains
		// committed and can be reconciled after the draft is resolved.
		v.sourceSynced = text
		return
	}
	if len(text) <= jsonSourceEditLimit {
		a.jsonEditor.SetText(text)
		v.sourceLoaded = true
	} else if v.sourceLoaded {
		a.jsonEditor.SetText("")
		v.sourceLoaded = false
	}
	v.sourceSynced = text
	v.requested = text
	label := map[string]string{"format": "格式化", "minify": "压缩", "sort-keys": "排序键"}[result.id]
	if output.Changed {
		a.persistActiveSession()
		a.setStatus("已" + label + " JSON")
	} else {
		a.setStatus("JSON 无需" + label)
	}
	v.toolMessage = a.status
}

func (result jsonToolResult) operationTokenState(a *App) *documentSessionState {
	if result.generation != a.documentGeneration || a.documentSessionState == nil || a.documentSessionState.documentID != result.operationToken.documentID {
		return nil
	}
	return a.documentSessionState
}

func (a *App) pollJSONToolResults() {
	v := a.ensureJSONView()
	select {
	case result := <-v.toolResults:
		a.applyJSONToolResult(result)
	default:
	}
}
